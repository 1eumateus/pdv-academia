package service

import (
	"strings"
	"testing"
	"time"

	"pdv/internal/models"
)

func TestCashReport(t *testing.T) {
	e := newEnv(t).withCash()
	subs := NewSubscriptionService(NewBase(e.db))
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	agua := e.product("Água com gás", 500, 10)
	suely := e.customer("Suely", 0)
	sub, err := subs.Create(e.ctx, suely.ID, plan.ID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "credito"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell("dinheiro", nil, item(agua.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell("fiado", &suely.ID, item(agua.ID, 1)); err == nil {
		t.Fatal("a prazo sem limite deveria falhar") // limite 0: garante que a prazo nunca entra no caixa
	}
	if _, err := e.cash.RegisterMovement(e.ctx, models.CashWithdrawal, 500, "Água Vanderson"); err != nil {
		t.Fatal(err)
	}

	today := time.Now().Format(dateLayout)
	reports := NewReportService(NewBase(e.db))
	r, err := reports.CashReport(e.ctx, today, today, 0, nil)
	if err != nil || len(r.Rows) != 3 {
		t.Fatalf("esperava 3 lançamentos: %v %+v", err, r.Rows)
	}
	want := []string{"Mensalidade · Suely (Mensal)", "Venda", "Retirada · Água Vanderson"}
	for i, w := range want {
		if r.Rows[i].History != w {
			t.Errorf("linha %d: %q, esperava %q", i, r.Rows[i].History, w)
		}
	}
	if r.Rows[2].In || r.Rows[2].Receipt != "-" || r.Rows[1].Detail != "1 Água com gás" {
		t.Errorf("retirada ou itens da venda fora do formato: %+v", r.Rows)
	}
	if r.InCents != 9500 || r.OutCents != 500 {
		t.Errorf("entradas/saídas: %d/%d", r.InCents, r.OutCents)
	}

	// Operador sem a permissão vê só os próprios lançamentos, mesmo pedindo todos.
	op := e.as(e.user("op", models.RoleOperator))
	if r, err := reports.CashReport(e.ctx, today, today, 0, []string{"dinheiro"}); err != nil || len(r.Rows) != 2 || len(r.Totals) != 1 || r.InCents != 500 {
		t.Fatalf("só dinheiro: venda e retirada, %v %+v", err, r)
	}
	if r, err := reports.CashReport(op, today, today, 0, nil); err != nil || len(r.Rows) != 0 || r.UserName == nil {
		t.Fatalf("operador deveria ver só os próprios (nenhum): %v %+v", err, r)
	}
}

// A retirada aparece e já abate o saldo em dinheiro, inclusive depois do fechamento.
func TestCashReportWithdrawalAfterClose(t *testing.T) {
	e := newEnv(t).withCash()
	agua := e.product("Água", 3000, 10)
	if _, err := e.sell("dinheiro", nil, item(agua.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.cash.RegisterMovement(e.ctx, models.CashWithdrawal, 1000, "Depósito"); err != nil {
		t.Fatal(err)
	}
	closed, err := e.cash.Close(e.ctx, 12000, "")
	if err != nil {
		t.Fatal(err)
	}
	if closed.ExpectedCashCents != 12000 {
		t.Fatalf("esperado na gaveta: %d", closed.ExpectedCashCents)
	}

	today := time.Now().Format(dateLayout)
	r, err := NewReportService(NewBase(e.db)).CashReport(e.ctx, today, today, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 2 || r.Rows[1].History != "Retirada · Depósito" || r.Rows[1].In || r.Rows[1].AmountCents != -1000 {
		t.Fatalf("retirada fora do relatório: %+v", r.Rows)
	}
	cash := r.Totals[0]
	if cash.Method != models.PaymentCash || cash.InCents != 3000 || cash.OutCents != 1000 {
		t.Fatalf("dinheiro: %+v", cash)
	}

	// No relatório de vendas: o total vendido não muda e o dinheiro vem descontado.
	reports := NewReportService(NewBase(e.db))
	sales, err := reports.Sales(e.ctx, today, today, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sales.TotalCents != 3000 || sales.WithdrawalsCents != 1000 || sales.CashAfterWithdrawals != 2000 {
		t.Fatalf("relatório: total %d, retiradas %d, dinheiro %d", sales.TotalCents, sales.WithdrawalsCents, sales.CashAfterWithdrawals)
	}
	if len(sales.Withdrawals) != 1 || sales.Withdrawals[0].Description != "Depósito" || sales.Withdrawals[0].AmountCents != 1000 {
		t.Fatalf("retiradas: %+v", sales.Withdrawals)
	}
	// Operador sem a permissão não vê a retirada de outro usuário.
	op := e.as(e.user("op", models.RoleOperator))
	if own, err := reports.Sales(op, today, today, 0); err != nil || len(own.Withdrawals) != 0 || own.WithdrawalsCents != 0 {
		t.Fatalf("operador: %v %+v", err, own.Withdrawals)
	}
	// A impressão e a planilha do relatório gerado trazem a retirada.
	gen, err := reports.Generate(e.ctx, today, today, 0, models.ReportGeneral)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reports.SavedXLSX(e.ctx, gen.ID); err != nil {
		t.Fatal(err)
	}
	var b builder
	receipts := NewReceiptService(NewBase(e.db))
	st, err := receipts.settings.Get(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.report(e.ctx, &b, st, gen.ID); err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, l := range b.lines {
		text.WriteString(l.Text + " " + l.Right + "\n")
	}
	for _, want := range []string{"Retiradas (sangria)", "Depósito", "Dinheiro após retiradas"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("impressão sem %q:\n%s", want, text.String())
		}
	}
}
