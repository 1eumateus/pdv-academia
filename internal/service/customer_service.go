// Caminho: pdv/internal/service/customer_service.go

package service

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

const (
	maxPhoneLen = 30
	maxFieldLen = 200
	maxNotesLen = 1000
)

type CustomerService struct {
	Base
	customers repository.CustomerRepo
	sales     repository.SaleRepo
	credits   repository.CreditRepo
}

func NewCustomerService(b Base) *CustomerService { return &CustomerService{Base: b} }

// optText limpa um campo opcional e limita o tamanho.
func optText(v *string, label string, max int) (*string, error) {
	v = trimPtr(v)
	if v != nil && len([]rune(*v)) > max {
		return nil, invalid("O campo %s deve ter no máximo %d caracteres.", label, max)
	}
	return v, nil
}

// validateCustomer valida e normaliza o cadastro. Só o nome é obrigatório.
func validateCustomer(in models.CustomerInput) (repository.CustomerFields, error) {
	var (
		f   = repository.CustomerFields{LimitCents: in.CreditLimitCents, Active: in.Active}
		err error
	)
	if f.Name, err = cleanName(in.Name, "nome do cliente"); err != nil {
		return f, err
	}
	if in.CreditLimitCents < 0 {
		return f, invalid("O limite de crédito não pode ser negativo.")
	}
	if in.CreditLimitCents > MaxPriceCents {
		return f, invalid("O limite de crédito excede o valor suportado.")
	}
	texts := []struct {
		dst   **string
		src   *string
		label string
		max   int
	}{
		{&f.Phone, in.Phone, "telefone", maxPhoneLen},
		{&f.Street, in.Street, "rua", maxFieldLen},
		{&f.Number, in.Number, "número", 20},
		{&f.Complement, in.Complement, "complemento", maxFieldLen},
		{&f.District, in.District, "bairro", maxFieldLen},
		{&f.City, in.City, "cidade", maxFieldLen},
		{&f.Reference, in.Reference, "ponto de referência", maxFieldLen},
		{&f.Notes, in.Notes, "observações", maxNotesLen},
	}
	for _, t := range texts {
		if *t.dst, err = optText(t.src, t.label, t.max); err != nil {
			return f, err
		}
	}

	if d := trimPtr(in.Document); d != nil {
		doc := NormalizeDocument(*d)
		switch {
		case len(doc) == 11 && ValidCPF(doc), len(doc) == 14 && ValidCNPJ(doc):
			f.Document = &doc
		case len(doc) == 11:
			return f, invalid("CPF inválido. Confira os números.")
		case len(doc) == 14:
			return f, invalid("CNPJ inválido. Confira os caracteres.")
		default:
			return f, invalid("Documento inválido: informe um CPF (11 dígitos) ou CNPJ (14 caracteres).")
		}
	}
	if e := trimPtr(in.Email); e != nil {
		at := strings.LastIndex(*e, "@")
		if len(*e) > 254 || strings.ContainsAny(*e, " ,;") || at < 1 || !strings.Contains((*e)[at:], ".") ||
			strings.HasSuffix(*e, ".") {
			return f, invalid("E-mail inválido.")
		}
		lower := strings.ToLower(*e)
		f.Email = &lower
	}
	if z := trimPtr(in.ZipCode); z != nil {
		zip := onlyDigits(*z)
		if len(zip) != 8 || len(*z) > 10 {
			return f, invalid("CEP inválido: informe 8 dígitos.")
		}
		f.ZipCode = &zip
	}
	if st := trimPtr(in.State); st != nil {
		uf := strings.ToUpper(*st)
		if !brazilianStates[uf] {
			return f, invalid("UF inválida. Use a sigla do estado, por exemplo SP.")
		}
		f.State = &uf
	}
	return f, nil
}

const duplicateDocumentMsg = "Já existe um cliente cadastrado com este CPF/CNPJ."

// cleanBirthDate: AAAA-MM-DD entre 1900 e hoje. nil mantém, "" apaga.
func cleanBirthDate(in *string, now time.Time) error {
	if in == nil || strings.TrimSpace(*in) == "" {
		return nil
	}
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(*in))
	if err != nil || d.Year() < 1900 || d.After(now) {
		return invalid("Data de nascimento inválida.")
	}
	*in = d.Format(time.DateOnly)
	return nil
}

func (s *CustomerService) Create(ctx context.Context, in models.CustomerInput) (models.Customer, error) {
	f, err := validateCustomer(in)
	if err != nil {
		return models.Customer{}, err
	}
	if err := cleanBirthDate(in.BirthDate, s.Now()); err != nil {
		return models.Customer{}, err
	}
	var out models.Customer
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		// Limite de fiado é decisão do gerente; cliente sem limite é livre.
		var authBy *int64
		if f.LimitCents > 0 {
			if authBy, err = requirePerm(ctx, s.Now(), PermCreditLimit); err != nil {
				return err
			}
		}
		id, err := s.customers.Insert(ctx, tx, f, s.Now())
		if err != nil {
			return fromRepo(err, nil, duplicateDocumentMsg)
		}
		if b := trimPtr(in.BirthDate); b != nil {
			if err := s.customers.SetBirthDate(ctx, tx, id, b); err != nil {
				return err
			}
		}
		if f.LimitCents > 0 {
			if err := audit(ctx, tx, s.Now(), authBy, "limite_fiado", "cliente", &id, nil,
				map[string]any{"cliente": f.Name, "limite": f.LimitCents}); err != nil {
				return err
			}
		}
		out, err = s.customers.GetByID(ctx, tx, id)
		return err
	})
	return out, err
}

// Update altera dados cadastrais. Reduzir o limite abaixo da dívida atual é
// permitido: apenas bloqueia novas vendas a prazo. Desativar um cliente com
// dívida também é permitido: ele continua podendo pagar.
func (s *CustomerService) Update(ctx context.Context, in models.CustomerInput) (models.Customer, error) {
	f, err := validateCustomer(in)
	if err != nil {
		return models.Customer{}, err
	}
	if err := cleanBirthDate(in.BirthDate, s.Now()); err != nil {
		return models.Customer{}, err
	}
	var out models.Customer
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		before, err := s.customers.GetByID(ctx, tx, in.ID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", in.ID), "")
		}
		if before.CreditLimitCents != f.LimitCents {
			authBy, err := requirePerm(ctx, s.Now(), PermCreditLimit)
			if err != nil {
				return err
			}
			if err := audit(ctx, tx, s.Now(), authBy, "limite_fiado", "cliente", &in.ID,
				map[string]any{"limite": before.CreditLimitCents}, map[string]any{"limite": f.LimitCents}); err != nil {
				return err
			}
		}
		if err := s.customers.Update(ctx, tx, in.ID, f, s.Now()); err != nil {
			return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", in.ID), duplicateDocumentMsg)
		}
		if in.BirthDate != nil && strings.TrimSpace(*in.BirthDate) != str(before.BirthDate) {
			if err := s.customers.SetBirthDate(ctx, tx, in.ID, trimPtr(in.BirthDate)); err != nil {
				return err
			}
		}
		out, err = s.customers.GetByID(ctx, tx, in.ID)
		return err
	})
	return out, err
}

// SetActive ativa ou inativa o cliente (inativo não compra a prazo e a catraca nega).
func (s *CustomerService) SetActive(ctx context.Context, id int64, active bool) (models.Customer, error) {
	var out models.Customer
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if err := s.customers.SetActive(ctx, tx, id, active, now); err != nil {
			return err
		}
		c, err := s.customers.GetByID(ctx, tx, id)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", id), "")
		}
		out = c
		action := "cliente_inativado"
		if active {
			action = "cliente_ativado"
		}
		return audit(ctx, tx, now, nil, action, "cliente", &id, nil, nil)
	})
	return out, err
}

func (s *CustomerService) Get(ctx context.Context, id int64) (models.Customer, error) {
	c, err := s.customers.GetByID(ctx, s.DB, id)
	return c, fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", id), "")
}

func (s *CustomerService) List(ctx context.Context, term string) ([]models.Customer, error) {
	return s.customers.List(ctx, s.DB, trimOrEmpty(term))
}

// History devolve vendas a prazo e recebimentos do cliente.
func (s *CustomerService) History(ctx context.Context, id int64) (models.CustomerHistory, error) {
	var h models.CustomerHistory
	c, err := s.Get(ctx, id)
	if err != nil {
		return h, err
	}
	h.Customer = c
	if h.Sales, err = s.sales.ListCreditByCustomer(ctx, s.DB, id); err != nil {
		return h, err
	}
	if h.Payments, err = s.credits.ListByCustomer(ctx, s.DB, id); err != nil {
		return h, err
	}
	h.Notes, err = currentNotes(ctx, s.DB, c, s.Now())
	return h, err
}

// currentNotes lê as notas do fiado do cliente com o que já foi pago de cada uma.
func currentNotes(ctx context.Context, q repository.DBTX, c models.Customer, now time.Time) ([]models.FiadoNote, error) {
	parts, err := (repository.AnalyticsRepo{}).FiadoNotes(ctx, q, c.ID)
	if err != nil {
		return nil, err
	}
	direct, err := (repository.CreditRepo{}).PaidBySale(ctx, q, c.ID)
	if err != nil {
		return nil, err
	}
	return fiadoNotes(parts, direct, c.CreditBalanceCents, now), nil
}

// fiadoNotes mostra cada compra no fiado com o que já foi pago dela. O que foi
// pago direto numa nota (recebimento com a nota escolhida, devolução abatida no
// fiado) fica nela; o restante do que o cliente pagou quita as mais antigas
// primeiro. A soma do que está em aberto é sempre a dívida atual.
func fiadoNotes(parts []models.FiadoNote, direct map[int64]int64, balance int64, now time.Time) []models.FiadoNote {
	var total, directSum int64
	for i := range parts {
		total += parts[i].AmountCents
		parts[i].PaidCents = min(direct[parts[i].SaleID], parts[i].AmountCents)
		directSum += parts[i].PaidCents
	}
	out := []models.FiadoNote{}
	pool := total - balance - directSum // pago sem nota escolhida
	if pool < 0 {
		// Dívida maior que as compras registradas (dados antigos): entra como nota sem venda.
		out = append(out, models.FiadoNote{Description: "Saldo anterior (sem venda registrada)", AmountCents: -pool, OpenCents: -pool, Status: "aberta"})
		pool = 0
	}
	for _, n := range parts { // da mais antiga para a mais nova
		take := min(pool, n.AmountCents-n.PaidCents)
		n.PaidCents += take
		pool -= take
		n.OpenCents = n.AmountCents - n.PaidCents
		switch {
		case n.OpenCents == 0:
			n.Status = "paga"
		case n.PaidCents > 0:
			n.Status = "parcial"
		default:
			n.Status = "aberta"
		}
		if n.OpenCents > 0 {
			n.DaysOpen = int64(now.Sub(n.CreatedAt) / (24 * time.Hour))
		}
		out = append(out, n)
	}
	return out
}

func trimOrEmpty(s string) string {
	if p := trimPtr(&s); p != nil {
		return *p
	}
	return ""
}
