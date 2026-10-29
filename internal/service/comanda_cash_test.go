// Caminho: pdv/internal/service/comanda_cash_test.go

package service

import (
	"errors"
	"testing"

	"pdv/internal/models"
)

func TestComandaFlow(t *testing.T) {
	e := newEnv(t).withCash()
	cerveja := e.product("Cerveja", 1200, 10)
	porcao := e.product("Porção", 3500, 5)

	c, err := e.comandas.Open(e.ctx, "Mesa 4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.comandas.Open(e.ctx, "Mesa 4"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duas comandas abertas para a mesma mesa: %v", err)
	}
	e.comandas.AddItem(e.ctx, c.Comanda.ID, cerveja.ID, 2)
	e.comandas.AddItem(e.ctx, c.Comanda.ID, cerveja.ID, 1) // soma na mesma linha
	d, err := e.comandas.AddItem(e.ctx, c.Comanda.ID, porcao.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) != 2 || d.Items[0].Qty != 3 || d.Comanda.TotalCents != 3*1200+3500 {
		t.Fatalf("comanda incorreta: %+v", d)
	}
	// Preço alterado depois do lançamento não muda o item.
	e.products.Update(e.ctx, models.ProductInput{ID: cerveja.ID, Name: "Cerveja", PriceCents: 1500, Active: true})

	sale, err := e.comandas.Close(e.ctx, models.CloseComandaInput{ComandaID: c.Comanda.ID, PaymentMethod: "pix"})
	if err != nil {
		t.Fatal(err)
	}
	if sale.Sale.TotalCents != 7100 || sale.Sale.ComandaID == nil || *sale.Sale.ComandaID != c.Comanda.ID {
		t.Fatalf("venda da comanda incorreta: %+v", sale.Sale)
	}
	if e.stockOf(cerveja.ID) != 7 || e.stockOf(porcao.ID) != 4 {
		t.Fatal("estoque não baixado no fechamento")
	}
	got, _ := e.comandas.Details(e.ctx, c.Comanda.ID)
	if got.Comanda.Status != models.ComandaClosed || len(got.Items) != 2 {
		t.Fatal("comanda não foi fechada ou perdeu o histórico")
	}

	_, err = e.comandas.Close(e.ctx, models.CloseComandaInput{ComandaID: c.Comanda.ID, PaymentMethod: "pix"})
	wantCode(t, err, CodeComandaClosed)
	_, err = e.comandas.AddItem(e.ctx, c.Comanda.ID, cerveja.ID, 1)
	wantCode(t, err, CodeComandaClosed)
	if e.count("sales") != 1 {
		t.Fatal("fechamento duplicado gerou outra venda")
	}
	// A mesa fica livre para uma nova comanda.
	if _, err := e.comandas.Open(e.ctx, "Mesa 4"); err != nil {
		t.Fatal(err)
	}
}

func TestComandaInsufficientStockOnClose(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Refrigerante", 600, 2)
	c, _ := e.comandas.Open(e.ctx, "Balcão")
	e.comandas.AddItem(e.ctx, c.Comanda.ID, p.ID, 2)
	// Outra venda leva 1 depois do lançamento: ao fechar, falta estoque.
	if _, err := e.sell("dinheiro", nil, item(p.ID, 1)); err != nil {
		t.Fatal(err)
	}
	before := e.snap([]int64{p.ID}, nil)
	_, err := e.comandas.Close(e.ctx, models.CloseComandaInput{ComandaID: c.Comanda.ID, PaymentMethod: "dinheiro"})
	wantCode(t, err, CodeInsufficientStock)
	e.assertUnchanged(before, []int64{p.ID}, nil)
	got, _ := e.comandas.Details(e.ctx, c.Comanda.ID)
	if got.Comanda.Status != models.ComandaOpen {
		t.Fatal("comanda fechou apesar da falha")
	}
}

func TestComandaCloseRollback(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Água", 300, 10)
	c, _ := e.comandas.Open(e.ctx, "Mesa 1")
	e.comandas.AddItem(e.ctx, c.Comanda.ID, p.ID, 2)
	before := e.snap([]int64{p.ID}, nil)
	// Falha no último passo (marcar a comanda como fechada).
	e.failOn("UPDATE", "comandas")
	if _, err := e.comandas.Close(e.ctx, models.CloseComandaInput{ComandaID: c.Comanda.ID, PaymentMethod: "dinheiro"}); err == nil {
		t.Fatal("fechamento deveria falhar")
	}
	e.assertUnchanged(before, []int64{p.ID}, nil)
}

func TestComandaEmptyAndInactive(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Suco", 800, 10)
	c, _ := e.comandas.Open(e.ctx, "Mesa 2")
	_, err := e.comandas.Close(e.ctx, models.CloseComandaInput{ComandaID: c.Comanda.ID, PaymentMethod: "dinheiro"})
	wantCode(t, err, CodeInvalid)

	d, _ := e.comandas.AddItem(e.ctx, c.Comanda.ID, p.ID, 1)
	e.products.Update(e.ctx, models.ProductInput{ID: p.ID, Name: p.Name, PriceCents: 800, Active: false})
	_, err = e.comandas.Close(e.ctx, models.CloseComandaInput{ComandaID: c.Comanda.ID, PaymentMethod: "dinheiro"})
	wantCode(t, err, CodeProductInactive)

	// Remove o item inativo (quantidade zero) e descarta a comanda vazia.
	if _, err := e.comandas.SetItemQty(e.ctx, d.Items[0].ID, 0); err != nil {
		t.Fatal(err)
	}
	got, err := e.comandas.Discard(e.ctx, c.Comanda.ID)
	if err != nil || got.Comanda.Status != models.ComandaDiscarded {
		t.Fatalf("descarte: %v %+v", err, got.Comanda)
	}
}

func TestComandaCreditClose(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Prato", 2500, 10)
	cust := e.customer("Bruno", 3000)
	c, _ := e.comandas.Open(e.ctx, "Mesa 9")
	e.comandas.AddItem(e.ctx, c.Comanda.ID, p.ID, 2) // 50,00 > limite
	_, err := e.comandas.Close(e.ctx, models.CloseComandaInput{ComandaID: c.Comanda.ID, PaymentMethod: "fiado", CustomerID: &cust.ID})
	wantCode(t, err, CodeCreditLimitExceeded)
	_, err = e.comandas.Close(e.ctx, models.CloseComandaInput{ComandaID: c.Comanda.ID, PaymentMethod: "fiado"})
	wantCode(t, err, CodeInvalid)
}

func TestCashOpenMovementsClose(t *testing.T) {
	e := newEnv(t)
	if cur, _ := e.cash.Current(e.ctx); cur != nil {
		t.Fatal("caixa deveria iniciar fechado")
	}
	if _, err := e.cash.Open(e.ctx, 5000); err != nil {
		t.Fatal(err)
	}
	_, err := e.cash.Open(e.ctx, 1000)
	wantCode(t, err, CodeCashAlreadyOpen)

	p := e.product("Item", 1000, 10)
	e.sell("dinheiro", nil, item(p.ID, 2)) // +20,00 dinheiro
	e.sell("pix", nil, item(p.ID, 1))      // pix não conta no dinheiro esperado
	if _, err := e.cash.RegisterMovement(e.ctx, models.CashSupply, 3000, "Troco"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.cash.RegisterMovement(e.ctx, models.CashWithdrawal, 4000, "Depósito"); err != nil {
		t.Fatal(err)
	}
	_, err = e.cash.RegisterMovement(e.ctx, models.CashWithdrawal, 1_000_000, "Demais")
	wantCode(t, err, CodeInvalid)
	_, err = e.cash.RegisterMovement(e.ctx, models.CashSupply, 100, "")
	wantCode(t, err, CodeInvalid)

	cur, _ := e.cash.Current(e.ctx)
	// 50 + 20 + 30 - 40 = 60,00
	if cur.ExpectedCashCents != 6000 {
		t.Fatalf("esperado = %d", cur.ExpectedCashCents)
	}
	byMethod := map[string]int64{}
	for _, t := range cur.ByPaymentMethod {
		byMethod[t.PaymentMethod] = t.TotalCents
	}
	if byMethod["dinheiro"] != 2000 || byMethod["pix"] != 1000 {
		t.Fatalf("resumo por forma: %+v", cur.ByPaymentMethod)
	}

	closed, err := e.cash.Close(e.ctx, 5850, "Faltou troco")
	if err != nil {
		t.Fatal(err)
	}
	s := closed.Session
	if s.Status != models.CashClosed || *s.ExpectedCents != 6000 || *s.CountedCents != 5850 || *s.DifferenceCents != -150 {
		t.Fatalf("fechamento incorreto: %+v", s)
	}
	_, err = e.cash.Close(e.ctx, 0, "")
	wantCode(t, err, CodeCashNotOpen)
	_, err = e.sell("dinheiro", nil, item(p.ID, 1))
	wantCode(t, err, CodeCashNotOpen)
	if list, _ := e.cash.ListSessions(e.ctx); len(list) != 1 {
		t.Fatal("histórico de sessões incorreto")
	}
}

func TestReports(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Item", 1000, 10)
	c := e.customer("Ana", 10000)
	e.sell("dinheiro", nil, item(p.ID, 2))
	d, _ := e.sell("pix", nil, item(p.ID, 1))
	e.sell("fiado", &c.ID, item(p.ID, 3))
	e.sales.CancelSale(e.ctx, d.Sale.ID, "teste")
	pay(e, c.ID, 1000)

	today := e.sales.Now().In(Location()).Format("2006-01-02")
	r, err := e.reports.Daily(e.ctx, today)
	if err != nil {
		t.Fatal(err)
	}
	if r.SaleCount != 2 || r.TotalCents != 5000 || r.CancelledCount != 1 || r.CancelledCents != 1000 {
		t.Fatalf("totais: %+v", r)
	}
	if len(r.Products) != 1 || r.Products[0].Qty != 5 {
		t.Fatalf("produtos: %+v", r.Products)
	}
	if r.CreditReceivedCents != 1000 || len(r.CreditPayments) != 1 || len(r.Sales) != 3 {
		t.Fatalf("recebimentos/vendas: %+v", r)
	}
	if _, err := e.reports.Sales(e.ctx, "2026-02-10", "2026-02-01", 0); !errors.Is(err, ErrInvalid) {
		t.Fatal("período invertido aceito")
	}
}
