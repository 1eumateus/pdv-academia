// Caminho: pdv/internal/service/customer_test.go

package service

import (
	"errors"
	"testing"

	"pdv/internal/models"
)

func TestDocumentValidation(t *testing.T) {
	valid := []string{"52998224725", "11222333000181", "12ABC34501DE35"}
	for _, d := range valid {
		if !(ValidCPF(d) || ValidCNPJ(d)) {
			t.Errorf("%s deveria ser válido", d)
		}
	}
	invalidDocs := []string{"52998224724", "11111111111", "11222333000182", "12ABC34501DE36", "12ABC34501DEAB", "00000000000000"}
	for _, d := range invalidDocs {
		if ValidCPF(d) || ValidCNPJ(d) {
			t.Errorf("%s deveria ser inválido", d)
		}
	}
	if NormalizeDocument("12.abc.345/01de-35") != "12ABC34501DE35" {
		t.Fatal("normalização incorreta")
	}
}

func TestCustomerOptionalProfile(t *testing.T) {
	e := newEnv(t)
	// Só o nome é obrigatório.
	min, err := e.customers.Create(e.ctx, models.CustomerInput{Name: "Ana", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if min.Document != nil || min.Street != nil || min.Email != nil {
		t.Fatalf("campos vazios deveriam ser nulos: %+v", min)
	}

	c, err := e.customers.Create(e.ctx, models.CustomerInput{
		Name: "Bruno", Active: true,
		Document: ptr("529.982.247-25"), Email: ptr(" Bruno@Email.com "), ZipCode: ptr("01310-100"),
		Street: ptr("Av. Paulista"), Number: ptr("1000"), Complement: ptr("ap 12"), District: ptr("Bela Vista"),
		City: ptr("São Paulo"), State: ptr("sp"), Reference: ptr("Perto do metrô"), Notes: ptr("Entregar após 18h"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if *c.Document != "52998224725" || *c.Email != "bruno@email.com" || *c.ZipCode != "01310100" || *c.State != "SP" {
		t.Fatalf("normalização incorreta: %+v", c)
	}
	if *c.Reference != "Perto do metrô" || *c.Notes != "Entregar após 18h" {
		t.Fatal("campos de entrega não gravados")
	}

	// Busca por documento com ou sem pontuação.
	// Busca por documento (com ou sem pontuação), e-mail, rua, bairro e cidade.
	for _, term := range []string{"529.982", "52998224725", "bruno@", "paulista", "Bela Vista", "são paulo"} {
		list, err := e.customers.List(e.ctx, term)
		if err != nil || len(list) != 1 || list[0].ID != c.ID {
			t.Fatalf("busca por %q: %v %d", term, err, len(list))
		}
	}

	// Documento duplicado é recusado; sem documento, não há conflito.
	_, err = e.customers.Create(e.ctx, models.CustomerInput{Name: "Outro", Document: ptr("52998224725"), Active: true})
	wantCode(t, err, CodeDuplicate)
	if _, err := e.customers.Create(e.ctx, models.CustomerInput{Name: "Sem doc", Active: true}); err != nil {
		t.Fatal(err)
	}

	// Edição: limpar o endereço volta a nulo.
	in := models.CustomerInput{ID: c.ID, Name: "Bruno", Active: true, Document: c.Document, Street: ptr("  ")}
	upd, err := e.customers.Update(e.ctx, in)
	if err != nil || upd.Street != nil || upd.City != nil || *upd.Document != "52998224725" {
		t.Fatalf("edição: %v %+v", err, upd)
	}
}

func TestCustomerProfileValidation(t *testing.T) {
	e := newEnv(t)
	cases := []models.CustomerInput{
		{Name: "X", Document: ptr("123.456.789-00")},
		{Name: "X", Document: ptr("11.222.333/0001-82")},
		{Name: "X", Document: ptr("12345")},
		{Name: "X", Email: ptr("sem-arroba.com")},
		{Name: "X", Email: ptr("a@b")},
		{Name: "X", ZipCode: ptr("1234")},
		{Name: "X", State: ptr("XX")},
	}
	for _, c := range cases {
		if _, err := e.customers.Create(e.ctx, c); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: esperado erro de validação, veio %v", c, err)
		}
	}
	if e.count("customers") != 0 {
		t.Fatal("cadastro inválido foi gravado")
	}
}

func TestSaleDetailsIncludeCustomerForDelivery(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Gás", 11000, 5)
	c, _ := e.customers.Create(e.ctx, models.CustomerInput{Name: "Carla", Active: true, Phone: ptr("11 99999-0000"),
		Street: ptr("Rua A"), Number: ptr("10"), Reference: ptr("Portão azul")})
	d, err := e.sell("pix", &c.ID, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if d.Customer == nil || *d.Customer.Street != "Rua A" || *d.Customer.Reference != "Portão azul" {
		t.Fatalf("detalhes sem dados de entrega: %+v", d.Customer)
	}
	noCust, _ := e.sell("dinheiro", nil, item(p.ID, 1))
	if noCust.Customer != nil {
		t.Fatal("venda sem cliente não deveria trazer cliente")
	}
}

func TestCustomerBirthDate(t *testing.T) {
	e := newEnv(t)
	bad := "2999-01-01"
	if _, err := e.customers.Create(e.ctx, models.CustomerInput{Name: "Futuro", Active: true, BirthDate: &bad}); codeOf(err) != CodeInvalid {
		t.Fatalf("nascimento no futuro: esperava OPERACAO_INVALIDA, veio %v", err)
	}
	birth := "1995-10-27"
	c, err := e.customers.Create(e.ctx, models.CustomerInput{Name: "Ana", Active: true, BirthDate: &birth})
	if err != nil || c.BirthDate == nil || *c.BirthDate != birth {
		t.Fatalf("nascimento no cadastro: %v %+v", err, c.BirthDate)
	}
	// Edição sem a data (ex.: importação) mantém; "" apaga.
	in := models.CustomerInput{ID: c.ID, Name: "Ana Souza", Active: true}
	if c, _ = e.customers.Update(e.ctx, in); c.BirthDate == nil {
		t.Fatal("nil deveria manter a data de nascimento")
	}
	empty := ""
	in.BirthDate = &empty
	if c, _ = e.customers.Update(e.ctx, in); c.BirthDate != nil {
		t.Fatal("vazio deveria apagar a data de nascimento")
	}
	st, err := NewSettingsService(NewBase(e.db)).Get(e.ctx)
	if err != nil || st.DueDays != 3 || st.MissingDays != 15 || st.MsgDue == "" || st.MsgBirthday == "" {
		t.Fatalf("padrões das mensagens: %v %+v", err, st)
	}
}
