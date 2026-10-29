// Caminho: pdv/internal/service/payments_test.go

package service

import (
	"strings"
	"testing"

	"pdv/internal/models"
)

func part(method string, amount int64) models.PaymentInput {
	return models.PaymentInput{Method: method, AmountCents: amount}
}

func TestMixedPayments(t *testing.T) {
	e := newEnv(t).withCash() // abre com R$ 100,00
	p := e.product("Camisa", 10000, 10)
	ana := e.customer("Ana", 5000)

	// R$ 100: 30 dinheiro (recebeu 50, troco 20) + 40 crédito 2x + 20 Pix + 10 fiado.
	cash := part(models.PaymentCash, 3000)
	cash.ReceivedCents = 5000
	card := part(models.PaymentCreditCard, 4000)
	card.CardBrand, card.Installments = "Visa", 2
	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, CustomerID: &ana.ID,
		Payments: []models.PaymentInput{cash, card, part(models.PaymentPix, 2000), part(models.PaymentCredit, 1000)}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Sale.PaymentMethod != models.PaymentMixed || len(d.Payments) != 4 || d.Payments[0].ChangeCents != 2000 {
		t.Fatalf("venda mista: %+v %+v", d.Sale.PaymentMethod, d.Payments)
	}
	if e.balanceOf(ana.ID) != 1000 {
		t.Fatalf("só a parte em fiado vira dívida: %d", e.balanceOf(ana.ID))
	}
	cur, _ := e.cash.Current(e.ctx)
	if cur.ExpectedCashCents != 13000 { // 100 de abertura + 30 em dinheiro (o troco não entra)
		t.Fatalf("dinheiro esperado na gaveta: %d", cur.ExpectedCashCents)
	}
	byMethod := map[string]int64{}
	for _, x := range cur.ByPaymentMethod {
		byMethod[x.PaymentMethod] = x.TotalCents
	}
	if byMethod[models.PaymentCreditCard] != 4000 || byMethod[models.PaymentPix] != 2000 || byMethod[models.PaymentCash] != 3000 {
		t.Fatalf("caixa por forma: %+v", byMethod)
	}
	r, _ := e.reports.Sales(e.ctx, today(), today(), 0)
	rep := map[string]int64{}
	for _, x := range r.ByPaymentMethod {
		rep[x.PaymentMethod] = x.TotalCents
	}
	if rep[models.PaymentCredit] != 1000 || rep[models.PaymentCreditCard] != 4000 || r.TotalCents != 10000 {
		t.Fatalf("relatório por forma: %+v", rep)
	}
	if h, _ := e.customers.History(e.ctx, ana.ID); len(h.Sales) != 1 {
		t.Fatal("venda mista com fiado deveria aparecer no histórico do cliente")
	}

	// Cupom lista as formas e o troco.
	rc, _ := NewReceiptService(NewBase(e.db)).Build(e.ctx, models.ReceiptSale, d.Sale.ID)
	txt := receiptText(rc)
	for _, w := range []string{"Crédito Visa 2x R$ 40,00", "Troco R$ 20,00", "A prazo R$ 10,00", "Saldo devedor atual R$ 10,00"} {
		if !strings.Contains(txt, w) {
			t.Errorf("cupom sem %q:\n%s", w, txt)
		}
	}

	// Cancelar estorna cada parte e devolve só o fiado da venda.
	if _, err := e.sales.CancelSale(e.ctx, d.Sale.ID, "teste"); err != nil {
		t.Fatal(err)
	}
	cur, _ = e.cash.Current(e.ctx)
	if cur.ExpectedCashCents != 10000 || e.balanceOf(ana.ID) != 0 {
		t.Fatalf("cancelamento: gaveta %d dívida %d", cur.ExpectedCashCents, e.balanceOf(ana.ID))
	}
}

func TestPaymentRules(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Bala", 1000, 50)
	sell := func(customer *int64, parts ...models.PaymentInput) error {
		_, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, CustomerID: customer, Payments: parts})
		return err
	}
	if err := sell(nil, part(models.PaymentCash, 500), part(models.PaymentPix, 400)); codeOf(err) != CodeInvalid {
		t.Fatalf("soma menor que o total: %v", err)
	}
	if err := sell(nil, part(models.PaymentPix, 1000), part(models.PaymentCash, 100)); codeOf(err) != CodeInvalid {
		t.Fatalf("soma maior que o total: %v", err)
	}
	low := part(models.PaymentCash, 1000)
	low.ReceivedCents = 500
	if err := sell(nil, low); codeOf(err) != CodeInvalid {
		t.Fatalf("recebido menor que a parte em dinheiro: %v", err)
	}
	debit := part(models.PaymentDebit, 1000)
	debit.Installments = 3
	if err := sell(nil, debit); codeOf(err) != CodeInvalid {
		t.Fatalf("parcela no débito: %v", err)
	}
	if err := sell(nil, part(models.PaymentPix, 500), part(models.PaymentCredit, 500)); codeOf(err) != CodeInvalid {
		t.Fatalf("fiado sem cliente: %v", err)
	}
	bia := e.customer("Bia", 300)
	if err := sell(&bia.ID, part(models.PaymentPix, 500), part(models.PaymentCredit, 500)); codeOf(err) != CodeCreditLimitExceeded {
		t.Fatalf("parte em fiado acima do limite: %v", err)
	}
	if e.count("sales") != 0 || e.stockOf(p.ID) != 50 {
		t.Fatal("pagamento recusado não pode gravar nada")
	}
	// Só uma forma nas partes: resumo é essa forma.
	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)},
		Payments: []models.PaymentInput{part(models.PaymentDebit, 600), part(models.PaymentDebit, 400)}})
	if err != nil || d.Sale.PaymentMethod != models.PaymentDebit {
		t.Fatalf("resumo com forma única: %v %v", d.Sale.PaymentMethod, err)
	}
}

func TestComandaMixedPayment(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Suco", 800, 10)
	c, _ := e.comandas.Open(e.ctx, "MESA 5")
	if _, err := e.comandas.AddItem(e.ctx, c.Comanda.ID, p.ID, 2); err != nil {
		t.Fatal(err)
	}
	d, err := e.comandas.Close(e.ctx, models.CloseComandaInput{ComandaID: c.Comanda.ID,
		Payments: []models.PaymentInput{part(models.PaymentCash, 600), part(models.PaymentPix, 1000)}})
	if err != nil || d.Sale.PaymentMethod != models.PaymentMixed || len(d.Payments) != 2 {
		t.Fatalf("comanda com pagamento misto: %+v %v", d.Sale, err)
	}
}

func TestItemDiscount(t *testing.T) {
	e := newEnv(t).withCash()
	a := e.product("Camisa", 10000, 10)
	b := e.product("Meia", 1000, 10)
	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{PaymentMethod: models.PaymentCash, DiscountCents: 100,
		Items: []models.SaleItemInput{{ProductID: a.ID, Qty: 1, DiscountCents: 500}, {ProductID: b.ID, Qty: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	// 100,00 + 20,00 = 120,00; desconto 5,00 no item + 1,00 na venda = 6,00 → 114,00
	if d.Sale.SubtotalCents != 12000 || d.Sale.DiscountCents != 600 || d.Sale.TotalCents != 11400 || d.Items[0].DiscountCents != 500 {
		t.Fatalf("venda com desconto no item: %+v %+v", d.Sale, d.Items)
	}
	r, _ := e.reports.Sales(e.ctx, today(), today(), 0)
	for _, p := range r.Products {
		if p.ProductID == a.ID && p.TotalCents != 9500 {
			t.Fatalf("produto vendido deveria somar já com o desconto do item: %d", p.TotalCents)
		}
	}
	rc, _ := NewReceiptService(NewBase(e.db)).Build(e.ctx, models.ReceiptSale, d.Sale.ID)
	if !strings.Contains(receiptText(rc), "Desconto no item -R$ 5,00") {
		t.Fatalf("cupom sem o desconto do item:\n%s", receiptText(rc))
	}
	if _, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{PaymentMethod: models.PaymentCash,
		Items: []models.SaleItemInput{{ProductID: b.ID, Qty: 1, DiscountCents: 1001}}}); codeOf(err) != CodeInvalid {
		t.Fatalf("desconto maior que o item: %v", err)
	}
	// O limite do operador vale sobre o desconto total (itens + venda).
	op := e.user("caixa", models.RoleOperator)
	if _, err := e.sales.CreateSale(e.as(op), models.CreateSaleInput{PaymentMethod: models.PaymentCash, DiscountCents: 600,
		Items: []models.SaleItemInput{{ProductID: a.ID, Qty: 1, DiscountCents: 500}}}); codeOf(err) != CodeAuthRequired {
		t.Fatalf("11%% de desconto somado deveria passar do limite do operador: %v", err)
	}
}
