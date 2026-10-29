package service

import (
	"testing"
	"time"

	"pdv/internal/models"
)

func TestSubscriptionCycle(t *testing.T) {
	e := newEnv(t).withCash()
	subs := NewSubscriptionService(NewBase(e.db))
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal 5x", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil || plan.PlanDays == nil || !plan.NoStock {
		t.Fatalf("plano: %v %+v", err, plan)
	}
	comum := e.product("Água", 300, 5)
	joao := e.customer("João", 0)

	if _, err := subs.Create(e.ctx, joao.ID, comum.ID, 0, nil); codeOf(err) != CodeInvalid {
		t.Fatalf("produto comum não é plano: esperava OPERACAO_INVALIDA, veio %v", err)
	}
	sub, err := subs.Create(e.ctx, joao.ID, plan.ID, 0, nil)
	today := time.Now().Format(dateLayout)
	if err != nil || sub.OpenInvoice == nil || sub.OpenInvoice.DueDate != today || sub.OpenInvoice.AmountCents != 9000 {
		t.Fatalf("assinatura nova deveria ter fatura vencendo hoje: %v %+v", err, sub.OpenInvoice)
	}
	if _, err := subs.Create(e.ctx, joao.ID, plan.ID, 0, nil); codeOf(err) != CodeDuplicate {
		t.Fatalf("mesmo plano duas vezes: esperava RECURSO_DUPLICADO, veio %v", err)
	}

	// Vencida aparece marcada.
	if _, err := e.db.Exec(`UPDATE subscription_invoices SET due_date = '2000-01-01' WHERE id = ?`, sub.OpenInvoice.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := subs.List(e.ctx); len(list) != 1 || !list[0].Overdue {
		t.Fatalf("fatura vencida deveria aparecer como vencida: %+v", list)
	}

	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "fiado"}); codeOf(err) != CodeInvalid {
		t.Fatalf("fatura a prazo: esperava OPERACAO_INVALIDA, veio %v", err)
	}
	in := models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "pix", RequestID: "fat-1"}
	sale, err := subs.Pay(e.ctx, in)
	if err != nil || sale.Sale.TotalCents != 9000 || sale.Sale.CustomerID == nil || *sale.Sale.CustomerID != joao.ID {
		t.Fatalf("pagar fatura deveria gerar venda de R$ 90 para o João: %v %+v", err, sale.Sale)
	}
	if again, err := subs.Pay(e.ctx, in); err != nil || again.Sale.ID != sale.Sale.ID {
		t.Fatalf("repetir o pedido deveria devolver a mesma venda: %v", err)
	}
	sub, _ = subs.get(e.ctx, sub.ID)
	next := time.Now().AddDate(0, 0, 30).Format(dateLayout)
	if sub.OpenInvoice == nil || sub.OpenInvoice.DueDate != next || sub.Overdue || sub.LastPaidAt == nil {
		t.Fatalf("próxima fatura deveria vencer em %s: %+v", next, sub.OpenInvoice)
	}
	if inv, _ := subs.Invoices(e.ctx, sub.ID); len(inv) != 2 {
		t.Fatalf("deveria haver a paga e a aberta: %+v", inv)
	}

	// Troca de plano: a mensalidade aberta passa ao valor novo e vence hoje.
	tri := int64(90)
	plan3, err := e.products.Create(e.ctx, models.ProductInput{Name: "Trimestral 5x", PriceCents: 24000, Active: true, PlanDays: &tri})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subs.ChangePlan(e.ctx, sub.ID, plan.ID, 0); codeOf(err) != CodeInvalid {
		t.Fatalf("mesmo plano: esperava OPERACAO_INVALIDA, veio %v", err)
	}
	if sub, err = subs.ChangePlan(e.ctx, sub.ID, plan3.ID, 0); err != nil || sub.PlanName != "Trimestral 5x" ||
		sub.OpenInvoice == nil || sub.OpenInvoice.AmountCents != 24000 || sub.OpenInvoice.DueDate != today {
		t.Fatalf("troca de plano: %v %+v %+v", err, sub, sub.OpenInvoice)
	}

	if sub, err = subs.Cancel(e.ctx, sub.ID); err != nil || sub.Status != "cancelada" || sub.OpenInvoice != nil {
		t.Fatalf("cancelar deveria encerrar e sumir com a fatura aberta: %v %+v", err, sub)
	}
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: 999, PaymentMethod: "pix"}); codeOf(err) != CodeStateConflict {
		t.Fatalf("fatura inexistente: esperava CONFLITO_DE_ESTADO, veio %v", err)
	}
}

func TestServiceDescription(t *testing.T) {
	e := newEnv(t)
	desc := "3x por semana"
	p, err := e.products.Create(e.ctx, models.ProductInput{Name: "Plano 3x", PriceCents: 7000, Active: true, Description: &desc})
	if err != nil || p.Description == nil || *p.Description != desc {
		t.Fatalf("descrição no cadastro: %v %+v", err, p.Description)
	}
	// Edição que não manda a descrição (nil) mantém a atual; "" apaga.
	in := models.ProductInput{ID: p.ID, Name: p.Name, PriceCents: p.PriceCents, Active: true}
	if p, _ = e.products.Update(e.ctx, in); p.Description == nil {
		t.Fatal("nil deveria manter a descrição")
	}
	empty := ""
	in.Description = &empty
	if p, _ = e.products.Update(e.ctx, in); p.Description != nil {
		t.Fatalf("vazio deveria apagar: %q", *p.Description)
	}
}

func TestSubscriptionPause(t *testing.T) {
	e := newEnv(t).withCash()
	now := time.Now()
	b := NewBase(e.db)
	b.Now = func() time.Time { return now }
	subs := NewSubscriptionService(b)
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := subs.Create(e.ctx, e.customer("Ana", 0).ID, plan.ID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	due := sub.OpenInvoice.DueDate

	op := e.as(e.user("op", models.RoleOperator))
	if _, err := subs.Pause(op, sub.ID, "viagem"); codeOf(err) != CodeAuthRequired {
		t.Fatalf("operador sem a permissão: esperava AUTORIZACAO_NECESSARIA, veio %v", err)
	}
	if sub, err = subs.Pause(e.ctx, sub.ID, "viagem"); err != nil || sub.PausedAt == nil || *sub.PauseReason != "viagem" {
		t.Fatalf("pausar: %v %+v", err, sub)
	}
	now = now.AddDate(0, 0, 10) // 10 dias pausada: a mensalidade de hoje não vence
	if sub, _ = subs.get(e.ctx, sub.ID); sub.Overdue {
		t.Fatal("pausada não pode aparecer como vencida")
	}
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "pix"}); codeOf(err) != CodeStateConflict {
		t.Fatalf("receber pausada: esperava CONFLITO_DE_ESTADO, veio %v", err)
	}
	if sub, err = subs.Resume(e.ctx, sub.ID); err != nil || sub.PausedAt != nil {
		t.Fatalf("retomar: %v %+v", err, sub)
	}
	d, _ := time.Parse(dateLayout, due)
	if want := d.AddDate(0, 0, 10).Format(dateLayout); sub.OpenInvoice.DueDate != want {
		t.Fatalf("vencimento depois de 10 dias pausada: %s, esperava %s", sub.OpenInvoice.DueDate, want)
	}
}

func TestSubscriptionPaymentUndo(t *testing.T) {
	e := newEnv(t).withCash()
	subs := NewSubscriptionService(NewBase(e.db))
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := subs.Create(e.ctx, e.customer("Ana", 0).ID, plan.ID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	first := *sub.OpenInvoice
	sale, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: first.ID, PaymentMethod: "dinheiro"})
	if err != nil {
		t.Fatal(err)
	}
	// Cancelar a venda: a mensalidade volta a ficar em aberto, com o vencimento original.
	if _, err := e.sales.CancelSale(e.ctx, sale.Sale.ID, "lançado errado"); err != nil {
		t.Fatal(err)
	}
	sub, _ = subs.get(e.ctx, sub.ID)
	if sub.OpenInvoice == nil || sub.OpenInvoice.ID != first.ID || sub.OpenInvoice.DueDate != first.DueDate {
		t.Fatalf("depois do estorno a mensalidade original deveria voltar a ficar em aberto: %+v", sub.OpenInvoice)
	}
	// Paga de novo, paga a seguinte: cancelar a primeira venda é recusado.
	s1, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: first.ID, PaymentMethod: "dinheiro"})
	if err != nil {
		t.Fatal(err)
	}
	sub, _ = subs.get(e.ctx, sub.ID)
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "dinheiro"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.CancelSale(e.ctx, s1.Sale.ID, "teste"); codeOf(err) != CodeStateConflict {
		t.Fatalf("cancelar a mais antiga com a seguinte paga: esperava CONFLITO_DE_ESTADO, veio %v", err)
	}
}

func TestSubscriptionPayOutsideAndNextDue(t *testing.T) {
	e := newEnv(t).withCash()
	subs := NewSubscriptionService(NewBase(e.db))
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	ana := e.customer("Ana", 0)
	sub, err := subs.Create(e.ctx, ana.ID, plan.ID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	sales := e.count("sales")
	today := time.Now()
	paid := today.AddDate(0, 0, -18).Format(dateLayout)

	if _, err := subs.PayOutside(e.as(e.user("op", models.RoleOperator)), sub.OpenInvoice.ID, paid, ""); err == nil {
		t.Fatal("operador sem a permissão não lança fora do caixa")
	}
	if _, err := subs.PayOutside(e.ctx, sub.OpenInvoice.ID, today.AddDate(0, 0, 1).Format(dateLayout), ""); codeOf(err) != CodeInvalid {
		t.Fatalf("data futura deveria ser recusada: %v", err)
	}
	// Retroativo: sem venda; a próxima vence na data do pagamento + 30 dias.
	first := sub.OpenInvoice.ID
	got, err := subs.PayOutside(e.ctx, first, paid, "")
	want := today.AddDate(0, 0, -18+30).Format(dateLayout)
	if err != nil || got.OpenInvoice == nil || got.OpenInvoice.DueDate != want {
		t.Fatalf("próxima deveria vencer em %s: %v %+v", want, err, got.OpenInvoice)
	}
	if n := e.count("sales"); n != sales {
		t.Fatalf("fora do caixa não cria venda: %d -> %d", sales, n)
	}
	invs, _ := subs.Invoices(e.ctx, sub.ID)
	if len(invs) != 2 || !invs[1].OutsideCash || invs[1].SaleID != nil || invs[1].Status != "paga" {
		t.Fatalf("a paga deveria estar marcada fora do caixa: %+v", invs)
	}
	// Desfazer: volta a ficar em aberto e a seguinte some.
	if got, err := subs.UndoOutside(e.ctx, first); err != nil || got.OpenInvoice == nil || got.OpenInvoice.ID != first {
		t.Fatalf("desfazer: %v %+v", err, got.OpenInvoice)
	}
	if _, err := subs.UndoOutside(e.ctx, first); codeOf(err) != CodeStateConflict {
		t.Fatalf("desfazer de novo deveria recusar: %v", err)
	}
	// Vencimento escolhido: fora do caixa e no caixa.
	chosen := today.AddDate(0, 0, 10).Format(dateLayout)
	if _, err := subs.PayOutside(e.ctx, first, paid, paid); codeOf(err) != CodeInvalid {
		t.Fatalf("vencimento na data do pagamento deveria ser recusado: %v", err)
	}
	got, err = subs.PayOutside(e.ctx, first, paid, chosen)
	if err != nil || got.OpenInvoice.DueDate != chosen {
		t.Fatalf("vencimento escolhido (fora do caixa): %v %+v", err, got.OpenInvoice)
	}
	// Com a seguinte paga, não desfaz a anterior.
	chosen2 := today.AddDate(0, 0, 45).Format(dateLayout)
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: got.OpenInvoice.ID, PaymentMethod: "pix", NextDue: chosen2}); err != nil {
		t.Fatal(err)
	}
	if s, _ := subs.get(e.ctx, sub.ID); s.OpenInvoice == nil || s.OpenInvoice.DueDate != chosen2 {
		t.Fatalf("vencimento escolhido (no caixa): %+v", s.OpenInvoice)
	}
	if _, err := subs.UndoOutside(e.ctx, first); codeOf(err) != CodeStateConflict {
		t.Fatalf("com a seguinte paga, não desfaz: %v", err)
	}
}
