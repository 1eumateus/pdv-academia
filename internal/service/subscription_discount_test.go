package service

import (
	"strings"
	"testing"
	"time"

	"pdv/internal/models"
)

// plano de R$ 90,00 com a fatura em aberto.
func openPlanInvoice(t *testing.T, e *env, name string) (*SubscriptionService, models.Subscription) {
	t.Helper()
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal " + name, PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	subs := NewSubscriptionService(NewBase(e.db))
	sub, err := subs.Create(e.ctx, e.customer(name, 0).ID, plan.ID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return subs, sub
}

func movementsOf(t *testing.T, e *env, saleID int64) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM cash_movements WHERE reference_type = 'sale' AND reference_id = ?`, saleID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPayInvoiceWithDiscount(t *testing.T) {
	e := newEnv(t).withCash()
	subs, sub := openPlanInvoice(t, e, "Ana")
	id := sub.OpenInvoice.ID

	// Pagamento precisa bater com o valor já descontado.
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: id, PaymentMethod: "pix", DiscountCents: 1000,
		Payments: []models.PaymentInput{{Method: "pix", AmountCents: 9000}}}); err == nil {
		t.Fatal("pagamento sem o desconto deveria ser recusado")
	}
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: id, PaymentMethod: "pix", DiscountCents: -1}); codeOf(err) != CodeInvalid {
		t.Fatalf("desconto negativo: %v", err)
	}
	// Desconto do tamanho da mensalidade é isenção: não passa por aqui.
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: id, PaymentMethod: "pix", DiscountCents: 9000}); codeOf(err) != CodeInvalid {
		t.Fatalf("desconto total: %v", err)
	}
	d, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: id, PaymentMethod: "pix", DiscountCents: 1000, Reason: "indicação"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Sale.SubtotalCents != 9000 || d.Sale.DiscountCents != 1000 || d.Sale.TotalCents != 8000 || d.Payments[0].AmountCents != 8000 {
		t.Fatalf("venda: %+v %+v", d.Sale, d.Payments)
	}
	var mov int64
	if err := e.db.QueryRow(`SELECT amount_cents FROM cash_movements WHERE reference_type = 'sale' AND reference_id = ?`, d.Sale.ID).Scan(&mov); err != nil || mov != 8000 {
		t.Fatalf("caixa: %d %v", mov, err)
	}
	// O desconto vale só nesta: a próxima vem com o valor cheio.
	fresh, err := subs.get(e.ctx, sub.ID)
	if err != nil || fresh.OpenInvoice == nil || fresh.OpenInvoice.AmountCents != 9000 {
		t.Fatalf("próxima fatura: %v %+v", err, fresh.OpenInvoice)
	}
	var after string
	if err := e.db.QueryRow(`SELECT after_json FROM audit_log WHERE action = 'fatura_paga' ORDER BY id DESC LIMIT 1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after, `"desconto":1000`) || !strings.Contains(after, "indicação") {
		t.Fatalf("auditoria: %s", after)
	}
}

func TestPayInvoiceDiscountLimitForOperator(t *testing.T) {
	e := newEnv(t).withCash()
	subs, sub := openPlanInvoice(t, e, "Bia")
	op := e.as(e.user("op", models.RoleOperator))
	// Limite padrão: 10% de R$ 90,00 = R$ 9,00.
	if _, err := subs.Pay(op, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "pix", DiscountCents: 901}); codeOf(err) != CodeAuthRequired {
		t.Fatalf("acima do limite: %v", err)
	}
	if d, err := subs.Pay(op, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "pix", DiscountCents: 900}); err != nil || d.Sale.TotalCents != 8100 {
		t.Fatalf("dentro do limite: %v %+v", err, d.Sale)
	}
}

func TestExemptInvoice(t *testing.T) {
	e := newEnv(t).withCash()
	subs, sub := openPlanInvoice(t, e, "Caio")
	id := sub.OpenInvoice.ID

	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: id, Exempt: true, Reason: "  "}); codeOf(err) != CodeInvalid {
		t.Fatalf("sem motivo: %v", err)
	}
	for _, role := range []string{models.RoleOperator, models.RoleManager} {
		ctx := e.as(e.user("u"+role, role))
		if _, err := subs.Pay(ctx, models.PayInvoiceInput{InvoiceID: id, Exempt: true, Reason: "x"}); codeOf(err) != CodeAuthRequired {
			t.Fatalf("%s: %v", role, err)
		}
	}
	d, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: id, Exempt: true, Reason: "cortesia", PaymentMethod: "pix",
		Payments: []models.PaymentInput{{Method: "pix", AmountCents: 9000}}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Sale.TotalCents != 0 || d.Sale.DiscountCents != 9000 || len(d.Payments) != 0 || movementsOf(t, e, d.Sale.ID) != 0 {
		t.Fatalf("isenção: %+v %+v", d.Sale, d.Payments)
	}
	if !strings.Contains(d.Items[0].ProductNameSnapshot, "ISENTA") {
		t.Fatalf("item: %s", d.Items[0].ProductNameSnapshot)
	}
	current, err := e.cash.Current(e.ctx)
	if err != nil || current.ExpectedCashCents != 10000 {
		t.Fatalf("o caixa não deveria mudar: %v %+v", err, current)
	}
	var status string
	if err := e.db.QueryRow(`SELECT status FROM subscription_invoices WHERE id = ?`, id).Scan(&status); err != nil || status != "paga" {
		t.Fatalf("fatura: %s %v", status, err)
	}
	fresh, err := subs.get(e.ctx, sub.ID)
	if err != nil || fresh.OpenInvoice == nil || fresh.OpenInvoice.AmountCents != 9000 {
		t.Fatalf("próxima fatura: %v %+v", err, fresh.OpenInvoice)
	}
	var after string
	if err := e.db.QueryRow(`SELECT after_json FROM audit_log WHERE action = 'fatura_isenta'`).Scan(&after); err != nil || !strings.Contains(after, "cortesia") {
		t.Fatalf("auditoria: %s %v", after, err)
	}
	// Cancelar a venda devolve a fatura.
	if _, err := e.sales.CancelSale(e.ctx, d.Sale.ID, "engano"); err != nil {
		t.Fatal(err)
	}
	if err := e.db.QueryRow(`SELECT status FROM subscription_invoices WHERE id = ?`, id).Scan(&status); err != nil || status != "aberta" {
		t.Fatalf("depois de cancelar: %s %v", status, err)
	}
	if movementsOf(t, e, d.Sale.ID) != 0 {
		t.Fatal("cancelar venda de R$ 0,00 não deveria mexer no caixa")
	}
}

func TestExemptUntilDate(t *testing.T) {
	e := newEnv(t).withCash()
	subs, sub := openPlanInvoice(t, e, "Davi")
	now := time.Now().In(Location())
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, Exempt: true, Reason: "x", ExemptUntil: now.AddDate(0, 0, -1).Format(dateLayout)}); codeOf(err) != CodeInvalid {
		t.Fatalf("data no passado: %v", err)
	}
	until := now.AddDate(0, 2, 0).Format(dateLayout)
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, Exempt: true, Reason: "lesão", ExemptUntil: until}); err != nil {
		t.Fatal(err)
	}
	got, err := subs.get(e.ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantDue := now.AddDate(0, 2, 1).Format(dateLayout)
	if !got.Exempt || got.ExemptUntil != until || got.ExemptReason != "lesão" || got.Overdue || got.OpenInvoice.DueDate != wantDue || got.OpenInvoice.AmountCents != 9000 {
		t.Fatalf("isenção com data: %+v %+v", got, got.OpenInvoice)
	}
	// Enquanto isenta não recebe, não lança fora do caixa, não pausa e não isenta de novo.
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: got.OpenInvoice.ID, PaymentMethod: "pix"}); codeOf(err) != CodeStateConflict {
		t.Fatalf("receber: %v", err)
	}
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: got.OpenInvoice.ID, Exempt: true, Reason: "x"}); codeOf(err) != CodeStateConflict {
		t.Fatalf("isentar de novo: %v", err)
	}
	if _, err := subs.PayOutside(e.ctx, got.OpenInvoice.ID, now.Format(dateLayout), ""); codeOf(err) != CodeStateConflict {
		t.Fatalf("fora do caixa: %v", err)
	}
	if _, err := subs.Pause(e.ctx, sub.ID, ""); codeOf(err) != CodeStateConflict {
		t.Fatalf("pausar: %v", err)
	}
	// Terminou a data: volta a cobrar sozinha, com o valor normal.
	yesterday := now.AddDate(0, 0, -1).Format(dateLayout)
	if _, err := e.db.Exec(`UPDATE subscriptions SET exempt_until = ? WHERE id = ?`, yesterday, sub.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE subscription_invoices SET due_date = ? WHERE status = 'aberta' AND subscription_id = ?`, now.Format(dateLayout), sub.ID); err != nil {
		t.Fatal(err)
	}
	if got, err = subs.get(e.ctx, sub.ID); err != nil || got.Exempt {
		t.Fatalf("depois da data: %v %+v", err, got)
	}
	d, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: got.OpenInvoice.ID, PaymentMethod: "pix"})
	if err != nil || d.Sale.TotalCents != 9000 {
		t.Fatalf("cobrança depois da isenção: %v %+v", err, d.Sale)
	}
	if got, err = subs.get(e.ctx, sub.ID); err != nil || got.RawExemptUntil != nil {
		t.Fatalf("marca da isenção ficou: %v %+v", err, got)
	}
}

func TestExemptPermanentAndEnd(t *testing.T) {
	e := newEnv(t).withCash()
	subs, sub := openPlanInvoice(t, e, "Eva")
	_, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, Exempt: true, Reason: "funcionária"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := subs.get(e.ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exempt || got.ExemptUntil != "" || got.Overdue || got.OpenInvoice.DueDate != models.PermanentExemptDue {
		t.Fatalf("permanente: %+v %+v", got, got.OpenInvoice)
	}
	// A catraca libera e diz que é isento.
	today := time.Now()
	ok, why := GateDecide(models.GateMember{Active: true, Plans: []models.GatePlan{{Name: "Mensal", DueDate: models.PermanentExemptDue, EverPaid: true}}}, today, 0)
	if !ok || !strings.Contains(why, "isento") {
		t.Fatalf("catraca: %v %s", ok, why)
	}
	// Sem a permissão de isentar, pede autorização.
	op := e.as(e.user("op", models.RoleManager))
	if _, err := subs.EndExemption(op, sub.ID); codeOf(err) != CodeAuthRequired {
		t.Fatalf("gerente: %v", err)
	}
	ended, err := subs.EndExemption(e.ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Exempt || ended.RawExemptUntil != nil || ended.OpenInvoice.DueDate != time.Now().In(Location()).Format(dateLayout) || ended.OpenInvoice.AmountCents != 9000 {
		t.Fatalf("encerrada: %+v %+v", ended, ended.OpenInvoice)
	}
	if _, err := subs.EndExemption(e.ctx, sub.ID); codeOf(err) != CodeStateConflict {
		t.Fatalf("encerrar de novo: %v", err)
	}
	// Isentar de novo e cancelar a venda desfaz a isenção.
	d2, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: ended.OpenInvoice.ID, Exempt: true, Reason: "de novo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.CancelSale(e.ctx, d2.Sale.ID, "engano"); err != nil {
		t.Fatal(err)
	}
	if got, err = subs.get(e.ctx, sub.ID); err != nil || got.Exempt || got.RawExemptUntil != nil {
		t.Fatalf("depois de cancelar: %v %+v", err, got)
	}
}
