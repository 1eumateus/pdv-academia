// Caminho: pdv/internal/service/credit_test.go

package service

import (
	"errors"
	"testing"

	"pdv/internal/models"
)

func pay(e *env, customerID, amount int64) (models.CreditPayment, error) {
	return e.credit.PayCustomerCredit(e.ctx, models.PayCreditInput{CustomerID: customerID, AmountCents: amount, PaymentMethod: "dinheiro"})
}

func TestCreditSaleValid(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Leite", 500, 20)
	c := e.customer("Ana", 5000)
	d, err := e.sell(models.PaymentCredit, &c.ID, item(p.ID, 4))
	if err != nil {
		t.Fatal(err)
	}
	if e.balanceOf(c.ID) != 2000 || d.Sale.CustomerID == nil || *d.Sale.CustomerID != c.ID {
		t.Fatalf("dívida = %d", e.balanceOf(c.ID))
	}
	// Venda a prazo não entra no caixa.
	if e.count("cash_movements") != 0 {
		t.Fatal("venda a prazo gerou movimentação de caixa")
	}
}

func TestCreditSaleWithoutCustomer(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Leite", 500, 20)
	_, err := e.sell(models.PaymentCredit, nil, item(p.ID, 1))
	wantCode(t, err, CodeInvalid)
}

func TestCreditLimitExceeded(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Leite", 500, 20)
	c := e.customer("Ana", 2000)
	if _, err := e.sell(models.PaymentCredit, &c.ID, item(p.ID, 3)); err != nil { // 15,00
		t.Fatal(err)
	}
	before := e.snap([]int64{p.ID}, []int64{c.ID})
	_, err := e.sell(models.PaymentCredit, &c.ID, item(p.ID, 2)) // 15 + 10 > 20
	wantCode(t, err, CodeCreditLimitExceeded)
	e.assertUnchanged(before, []int64{p.ID}, []int64{c.ID})
	// Exatamente no limite é permitido.
	if _, err := e.sell(models.PaymentCredit, &c.ID, item(p.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if e.balanceOf(c.ID) != 2000 {
		t.Fatal("saldo incorreto no limite")
	}
}

func TestCreditInactiveCustomer(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Leite", 500, 20)
	c := e.customer("Ana", 5000)
	e.customers.Update(e.ctx, models.CustomerInput{ID: c.ID, Name: c.Name, CreditLimitCents: 5000, Active: false})
	_, err := e.sell(models.PaymentCredit, &c.ID, item(p.ID, 1))
	wantCode(t, err, CodeCustomerInactive)
}

func TestCreditPayments(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Leite", 500, 20)
	c := e.customer("Ana", 10000)
	e.sell(models.PaymentCredit, &c.ID, item(p.ID, 10)) // 50,00

	if _, err := pay(e, c.ID, 2000); err != nil { // parcial
		t.Fatal(err)
	}
	if e.balanceOf(c.ID) != 3000 {
		t.Fatalf("após parcial = %d", e.balanceOf(c.ID))
	}
	_, err := pay(e, c.ID, 3001) // acima da dívida
	wantCode(t, err, CodeInvalid)
	if e.balanceOf(c.ID) != 3000 || e.count("credit_payments") != 1 {
		t.Fatal("pagamento acima da dívida alterou dados")
	}
	if _, err := pay(e, c.ID, 3000); err != nil { // total
		t.Fatal(err)
	}
	if e.balanceOf(c.ID) != 0 {
		t.Fatal("dívida não quitada")
	}
	_, err = pay(e, c.ID, 1)
	wantCode(t, err, CodeInvalid)
	for _, a := range []int64{0, -100} {
		if _, err := pay(e, c.ID, a); !errors.Is(err, ErrInvalid) {
			t.Fatalf("valor %d aceito", a)
		}
	}
	// Recebimentos não mexem no estoque.
	if e.count("stock_movements") != 2 {
		t.Fatal("recebimento gerou movimentação de estoque")
	}
	cur, _ := e.cash.Current(e.ctx)
	if cur.ExpectedCashCents != 10000+5000 {
		t.Fatalf("recebimentos não entraram no caixa: %d", cur.ExpectedCashCents)
	}
}

func TestCreditPaymentRollback(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Leite", 500, 20)
	c := e.customer("Ana", 10000)
	e.sell(models.PaymentCredit, &c.ID, item(p.ID, 4))
	before := e.snap(nil, []int64{c.ID})
	// Falha entre o registro do pagamento e a atualização do saldo.
	e.failOn("UPDATE", "customers")
	if _, err := pay(e, c.ID, 1000); err == nil {
		t.Fatal("pagamento deveria falhar")
	}
	e.assertUnchanged(before, nil, []int64{c.ID})
}

func TestCreditPaymentIdempotent(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Leite", 500, 20)
	c := e.customer("Ana", 10000)
	e.sell(models.PaymentCredit, &c.ID, item(p.ID, 4))
	in := models.PayCreditInput{CustomerID: c.ID, AmountCents: 500, PaymentMethod: "pix", RequestID: "rec-1"}
	a, err := e.credit.PayCustomerCredit(e.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.credit.PayCustomerCredit(e.ctx, in)
	if err != nil || a.ID != b.ID || e.balanceOf(c.ID) != 1500 {
		t.Fatalf("recebimento duplicado: %v %d %d saldo=%d", err, a.ID, b.ID, e.balanceOf(c.ID))
	}
}

func TestReverseCreditPayment(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Leite", 500, 20)
	c := e.customer("Ana", 10000)
	e.sell(models.PaymentCredit, &c.ID, item(p.ID, 4))
	pm, _ := pay(e, c.ID, 1500)
	r, err := e.credit.ReversePayment(e.ctx, pm.ID, "Lançado no cliente errado")
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != models.CreditPaymentReversal || e.balanceOf(c.ID) != 2000 || e.count("credit_payments") != 2 {
		t.Fatal("estorno incorreto")
	}
	_, err = e.credit.ReversePayment(e.ctx, pm.ID, "de novo")
	wantCode(t, err, CodeStateConflict)
}

func TestCancelCreditSale(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Leite", 500, 20)
	c := e.customer("Ana", 10000)
	d, _ := e.sell(models.PaymentCredit, &c.ID, item(p.ID, 4)) // 20,00
	if _, err := e.sales.CancelSale(e.ctx, d.Sale.ID, "Erro de lançamento"); err != nil {
		t.Fatal(err)
	}
	if e.balanceOf(c.ID) != 0 || e.stockOf(p.ID) != 20 {
		t.Fatalf("reversão incorreta: dívida=%d estoque=%d", e.balanceOf(c.ID), e.stockOf(p.ID))
	}

	// Se já houve pagamento parcial, o cancelamento é recusado (política explícita).
	d2, _ := e.sell(models.PaymentCredit, &c.ID, item(p.ID, 4))
	pay(e, c.ID, 1000)
	_, err := e.sales.CancelSale(e.ctx, d2.Sale.ID, "x")
	wantCode(t, err, CodeStateConflict)
	if e.balanceOf(c.ID) != 1000 || e.stockOf(p.ID) != 16 {
		t.Fatal("cancelamento recusado alterou dados")
	}
}
