// Caminho: pdv/internal/service/receipt_test.go

package service

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"pdv/internal/models"
)

func receiptText(r models.Receipt) string {
	var b strings.Builder
	for _, l := range r.Lines {
		if l.Kind == models.LineItem {
			b.WriteString(l.Text + " " + l.Qty + " " + l.Price + " " + l.Right + "\n")
			continue
		}
		b.WriteString(l.Text + " " + l.Right + "\n")
	}
	return b.String()
}

func storeSettings() models.Settings {
	return models.Settings{
		StoreName: "Loja Teste", LegalName: "Loja Teste LTDA", Document: "11.222.333/0001-81", ZipCode: "01001-000",
		Street: "Praça da Sé", Number: "1", City: "São Paulo", State: "sp", Phone: "(11) 3333-4444",
		ReceiptFooter: "Obrigado pela preferência!", PaperWidth: "80", QuoteValidityDays: 7,
	}
}

func TestSettingsSaveNormalizesAndValidates(t *testing.T) {
	e := newEnv(t)
	svc := NewSettingsService(NewBase(e.db))

	got, err := svc.Save(e.ctx, storeSettings())
	if err != nil {
		t.Fatal(err)
	}
	if got.Document != "11222333000181" || got.ZipCode != "01001000" || got.State != "SP" || got.LegalName != "Loja Teste LTDA" {
		t.Fatalf("normalização errada: %+v", got)
	}

	cases := map[string]func(*models.Settings){
		"cnpj":   func(s *models.Settings) { s.Document = "11.222.333/0001-82" },
		"papel":  func(s *models.Settings) { s.PaperWidth = "110" },
		"logo":   func(s *models.Settings) { s.Logo = "data:image/gif;base64,AAAA" },
		"dias":   func(s *models.Settings) { s.QuoteValidityDays = 0 },
		"uf":     func(s *models.Settings) { s.State = "XX" },
		"rodape": func(s *models.Settings) { s.ReceiptFooter = strings.Repeat("a", maxFooterLen+1) },
	}
	for name, mutate := range cases {
		in := storeSettings()
		mutate(&in)
		if _, err := svc.Save(e.ctx, in); !errors.Is(err, &Error{Code: CodeInvalid}) {
			t.Errorf("%s: esperava erro de validação, veio %v", name, err)
		}
	}
	// As tentativas inválidas não alteram o que foi salvo.
	if again, _ := svc.Get(e.ctx); again.Document != "11222333000181" || again.PaperWidth != "80" {
		t.Fatalf("configuração alterada por tentativa inválida: %+v", again)
	}
}

func TestSettingsDefaults(t *testing.T) {
	e := newEnv(t)
	s, err := NewSettingsService(NewBase(e.db)).Get(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.StoreName != defaultStoreName || s.PaperWidth != "80" || !s.CutPaper || s.QuoteValidityDays != defaultQuoteDays {
		t.Fatalf("padrões errados: %+v", s)
	}
}

func TestReceiptFiadoSaleHasStoreCustomerAndBalance(t *testing.T) {
	e := newEnv(t).withCash()
	b := NewBase(e.db)
	if _, err := NewSettingsService(b).Save(e.ctx, storeSettings()); err != nil {
		t.Fatal(err)
	}
	street, city := "Rua das Flores", "Campinas"
	c, err := e.customers.Create(e.ctx, models.CustomerInput{Name: "Maria", Street: &street, City: &city,
		CreditLimitCents: 100000, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	p := e.product("Caderno", 1500, 10)
	sale, err := e.sell(models.PaymentCredit, &c.ID, item(p.ID, 2))
	if err != nil {
		t.Fatal(err)
	}

	r, err := NewReceiptService(b).Build(e.ctx, models.ReceiptSale, sale.Sale.ID)
	if err != nil {
		t.Fatal(err)
	}
	txt := receiptText(r)
	for _, want := range []string{"Loja Teste", "CNPJ 11.222.333/0001-81", "Praça da Sé, 1", "CEP 01001-000",
		"Produto Qtd Preço Total", "Caderno 2 15,00 30,00", "R$ 30,00", "A prazo", "Cliente: Maria", "Rua das Flores", "Campinas",
		"Saldo devedor atual R$ 30,00", "Assinatura do cliente", "Obrigado pela preferência!", "DOCUMENTO NÃO FISCAL"} {
		if !strings.Contains(txt, want) {
			t.Errorf("comprovante sem %q:\n%s", want, txt)
		}
	}
}

func TestReceiptOtherKinds(t *testing.T) {
	e := newEnv(t).withCash()
	b := NewBase(e.db)
	rs := NewReceiptService(b)
	c := e.customer("João", 100000)
	p := e.product("Caneta", 500, 10)
	if _, err := e.sell(models.PaymentCredit, &c.ID, item(p.ID, 4)); err != nil {
		t.Fatal(err)
	}
	pay, err := e.credit.PayCustomerCredit(e.ctx, models.PayCreditInput{CustomerID: c.ID, AmountCents: 500,
		PaymentMethod: models.PaymentCash, RequestID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	mov, err := e.cash.RegisterMovement(e.ctx, models.CashWithdrawal, 2000, "Pagamento do gás")
	if err != nil {
		t.Fatal(err)
	}
	cur, _ := e.cash.Current(e.ctx)

	checks := []struct {
		kind  string
		id    int64
		wants []string
	}{
		{models.ReceiptCreditPayment, pay.ID, []string{"RECIBO DE PAGAMENTO", "Valor R$ 5,00", "Saldo devedor atual R$ 15,00"}},
		{models.ReceiptCashMovement, mov.ID, []string{"SANGRIA DE CAIXA", "R$ 20,00", "Pagamento do gás"}},
		{models.ReceiptCashSession, cur.Session.ID, []string{"RESUMO PARCIAL", "Saldo inicial R$ 100,00", "Esperado em dinheiro R$ 85,00"}},
		{models.ReceiptTest, 0, []string{"TESTE DE IMPRESSÃO"}},
	}
	for _, c := range checks {
		r, err := rs.Build(e.ctx, c.kind, c.id)
		if err != nil {
			t.Fatalf("%s: %v", c.kind, err)
		}
		txt := receiptText(r)
		for _, w := range c.wants {
			if !strings.Contains(txt, w) {
				t.Errorf("%s sem %q:\n%s", c.kind, w, txt)
			}
		}
	}
	if _, err := rs.Build(e.ctx, "nada", 1); !errors.Is(err, &Error{Code: CodeInvalid}) {
		t.Fatalf("tipo desconhecido deveria falhar: %v", err)
	}
}

func TestPrintSendsEscPosAndFailureKeepsSale(t *testing.T) {
	e := newEnv(t).withCash()
	b := NewBase(e.db)
	rs := NewReceiptService(b)
	p := e.product("Bala", 100, 10)
	sale, err := e.sell(models.PaymentCash, nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}

	// Sem impressora configurada: erro claro, nada enviado.
	rs.Send = func(string, []byte) error { t.Fatal("não deveria enviar"); return nil }
	if err := rs.Print(e.ctx, models.ReceiptSale, sale.Sale.ID); !errors.Is(err, &Error{Code: CodeInvalid}) {
		t.Fatalf("esperava erro de impressora não configurada: %v", err)
	}

	st := storeSettings()
	st.PrinterName, st.CutPaper, st.OpenDrawer = "Termica", true, true
	if _, err := NewSettingsService(b).Save(e.ctx, st); err != nil {
		t.Fatal(err)
	}
	var sent []byte
	rs.Send = func(name string, data []byte) error {
		if name != "Termica" {
			t.Fatalf("impressora errada: %s", name)
		}
		sent = data
		return nil
	}
	if err := rs.Print(e.ctx, models.ReceiptSale, sale.Sale.ID); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(sent, []byte("\x1b@")) || !bytes.Contains(sent, []byte("DOCUMENTO NAO FISCAL")) ||
		!bytes.Contains(sent, []byte("\x1bp")) || !bytes.Contains(sent, []byte("\x1dV")) {
		t.Fatalf("ESC/POS incompleto: %q", sent)
	}

	rs.Send = func(string, []byte) error { return errors.New("sem papel") }
	err = rs.Print(e.ctx, models.ReceiptSale, sale.Sale.ID)
	if err == nil || !strings.Contains(err.Error(), "sem papel") || !strings.Contains(err.Error(), "já foi registrada") {
		t.Fatalf("mensagem de falha ruim: %v", err)
	}
	if e.count("sales") != 1 {
		t.Fatal("falha de impressão não pode afetar a venda")
	}
}

func TestPixSettingsAndFiadoReceipt(t *testing.T) {
	e := newEnv(t).withCash()
	b := NewBase(e.db)
	ss := NewSettingsService(b)
	st := storeSettings()
	st.PixKey = "Loja@Exemplo.com.br"
	if _, err := ss.Save(e.ctx, st); err != nil {
		t.Fatal(err)
	}
	got, _ := ss.Get(e.ctx)
	if got.PixKey != "loja@exemplo.com.br" || got.PixName != "Loja Teste" || got.PixCity != "Sao Paulo" {
		t.Fatalf("Pix normalizado: %q %q %q", got.PixKey, got.PixName, got.PixCity)
	}
	st.PixKey = "123.456.789-00"
	if _, err := ss.Save(e.ctx, st); codeOf(err) != CodeInvalid {
		t.Fatalf("CPF inválido como chave: %v", err)
	}
	code, err := ss.PixQR(e.ctx, 1050, "VENDA1")
	if err != nil || !strings.Contains(code.Payload, "540510.50") || !strings.HasPrefix(code.Image, "data:image/png") {
		t.Fatalf("QR Pix: %+v %v", code.Payload, err)
	}

	// Cupom de fiado com o QR do valor em fiado.
	got.PixOnReceipt = true
	if _, err := ss.Save(e.ctx, got); err != nil {
		t.Fatal(err)
	}
	c := e.customer("Ana", 10000)
	p := e.product("Caderno", 2000, 5)
	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, CustomerID: &c.ID,
		Payments: []models.PaymentInput{{Method: models.PaymentPix, AmountCents: 500}, {Method: models.PaymentCredit, AmountCents: 1500}}})
	if err != nil {
		t.Fatal(err)
	}
	rs := NewReceiptService(b)
	r, _ := rs.Build(e.ctx, models.ReceiptSale, d.Sale.ID)
	var qr *models.ReceiptLine
	for i := range r.Lines {
		if r.Lines[i].Kind == models.LineQR {
			qr = &r.Lines[i]
		}
	}
	if qr == nil || !strings.Contains(qr.Text, "540515.00") || !strings.Contains(receiptText(r), "Pague R$ 15,00 com Pix") {
		t.Fatalf("cupom de fiado deveria ter o QR de R$ 15,00:\n%s", receiptText(r))
	}
	var sent []byte
	rs.Send = func(_ string, data []byte) error { sent = data; return nil }
	got.PrinterName = "Termica"
	ss.Save(e.ctx, got)
	if err := rs.Print(e.ctx, models.ReceiptSale, d.Sale.ID); err != nil || !bytes.Contains(sent, []byte{0x1d, '(', 'k'}) {
		t.Fatalf("ESC/POS deveria ter o comando de QR: %v", err)
	}
}

func TestPrintPixQR(t *testing.T) {
	e := newEnv(t)
	b := NewBase(e.db)
	ss := NewSettingsService(b)
	rs := NewReceiptService(b)
	var sent []byte
	rs.Send = func(_ string, data []byte) error { sent = data; return nil }
	st := storeSettings()
	st.PixKey = "loja@exemplo.com.br"
	ss.Save(e.ctx, st)
	if err := rs.PrintPix(e.ctx, 1990, "VENDA1"); codeOf(err) != CodeInvalid {
		t.Fatalf("sem térmica configurada deveria avisar: %v", err)
	}
	st.PrinterName, st.OpenDrawer = "Termica", true
	ss.Save(e.ctx, st)
	if err := rs.PrintPix(e.ctx, 1990, "VENDA1"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(sent, []byte("PAGAMENTO VIA PIX")) || !bytes.Contains(sent, []byte("540519.90")) ||
		!bytes.Contains(sent, []byte{0x1d, '(', 'k'}) || bytes.Contains(sent, []byte("\x1bp")) {
		t.Fatalf("impressão do QR Pix incompleta ou abriu a gaveta: %q", sent)
	}
}

func TestReceiptsShowOperator(t *testing.T) {
	e := newEnv(t).withCash()
	rs := NewReceiptService(NewBase(e.db))
	p := e.product("Caneta", 500, 10)
	ana := e.customer("Ana", 10000)
	q, _ := NewQuoteService(NewBase(e.db)).Save(e.ctx, models.QuoteInput{Items: []models.SaleItemInput{item(p.ID, 1)}})
	if _, err := e.sell(models.PaymentCredit, &ana.ID, item(p.ID, 1)); err != nil {
		t.Fatal(err)
	}
	pay, err := e.credit.PayCustomerCredit(e.ctx, models.PayCreditInput{CustomerID: ana.ID, AmountCents: 500, PaymentMethod: models.PaymentCash, RequestID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	mv, err := e.cash.RegisterMovement(e.ctx, models.CashSupply, 1000, "troco")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		kind string
		id   int64
	}{{models.ReceiptQuote, q.Quote.ID}, {models.ReceiptCreditPayment, pay.ID}, {"movimento", mv.ID}} {
		r, err := rs.Build(e.ctx, c.kind, c.id)
		if err != nil || !strings.Contains(receiptText(r), "Operador: Admin") {
			t.Fatalf("%s sem operador:\n%s %v", c.kind, receiptText(r), err)
		}
	}
}

func TestBackgroundsSaveAndValidate(t *testing.T) {
	e := newEnv(t)
	svc := NewSettingsService(NewBase(e.db))
	img := "data:image/jpeg;base64,AAAA"
	if _, err := svc.SaveBackgrounds(e.ctx, models.Backgrounds{Login: img}); err != nil {
		t.Fatal(err)
	}
	if b, err := svc.Backgrounds(e.ctx); err != nil || b.Login != img || b.App != "" {
		t.Fatalf("fundo salvo: %v %+v", err, b)
	}
	if _, err := svc.SaveBackgrounds(e.ctx, models.Backgrounds{App: "http://x/y.jpg"}); codeOf(err) != CodeInvalid {
		t.Fatalf("só data URL PNG/JPEG: %v", err)
	}
	if _, err := svc.SaveBackgrounds(e.as(e.user("op", models.RoleOperator)), models.Backgrounds{}); err == nil {
		t.Fatal("operador sem permissão não deveria trocar o fundo")
	}
}
