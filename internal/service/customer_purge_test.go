// Caminho: catraca/internal/service/customer_purge_test.go

package service

import (
	"testing"

	"pdv/internal/models"
)

func TestPurgeCustomerWithHistory(t *testing.T) {
	e := newEnv(t).withCash()
	b := NewBase(e.db)
	gate := NewCatracaService(b)
	subs, sub := openPlanInvoice(t, e, "Carla")
	cid := sub.CustomerID
	other := e.customer("Outro", 0)

	// Histórico: mensalidade paga, venda a prazo, recebimento de parte do fiado, passagem.
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "pix",
		Payments: []models.PaymentInput{{Method: "pix", AmountCents: 9000}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE customers SET credit_limit_cents = 100000 WHERE id = ?`, cid); err != nil {
		t.Fatal(err)
	}
	p := e.product("Água", 500, 10)
	if _, err := e.sell("fiado", &cid, item(p.ID, 4)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.credit.PayCustomerCredit(e.ctx, models.PayCreditInput{CustomerID: cid, AmountCents: 500, PaymentMethod: "dinheiro"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell("pix", &other.ID, item(p.ID, 1)); err != nil { // de outra pessoa: fica
		t.Fatal(err)
	}
	if err := gate.ManualRelease(e.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`INSERT INTO catraca_passagens (at, customer_id, name, allowed, reason, source) VALUES ('2026-11-01T10:00:00.000Z', ?, 'Carla', 1, 'ok', 'leitor')`, cid); err != nil {
		t.Fatal(err)
	}

	// Sem a permissão: pede autorização.
	op := e.as(e.user("op", models.RoleManager))
	if _, err := gate.PurgeCustomerPreview(op, cid); codeOf(err) != CodeAuthRequired {
		t.Fatalf("sem permissão: %v", err)
	}
	prev, err := gate.PurgeCustomerPreview(e.ctx, cid)
	if err != nil {
		t.Fatal(err)
	}
	if prev.Sales != 2 || prev.Invoices < 1 || prev.CreditPayments != 1 || prev.Passages != 1 || prev.DebtCents != 1500 || prev.CashSessions != 1 {
		t.Fatalf("prévia: %+v", prev)
	}
	if err := gate.PurgeCustomer(e.ctx, cid, prev.Sales+1); codeOf(err) != CodeStateConflict {
		t.Fatalf("prévia desatualizada: %v", err)
	}
	if err := gate.PurgeCustomer(e.ctx, cid, prev.Sales); err != nil {
		t.Fatal(err)
	}

	for table, where := range map[string]string{
		"customers": "id = ?", "sales": "customer_id = ?", "subscriptions": "customer_id = ?", "credit_payments": "customer_id = ?",
		"catraca_passagens": "customer_id = ?",
	} {
		var n int
		e.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+where, cid).Scan(&n)
		if n != 0 {
			t.Errorf("sobrou em %s: %d", table, n)
		}
	}
	if e.count("sales") != 1 || e.stockOf(p.ID) != 9 { // só a venda do outro; o estoque das 4 voltou
		t.Fatalf("vendas %d, estoque %d", e.count("sales"), e.stockOf(p.ID))
	}
	var guards int
	e.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name IN ('tr_sales_no_delete', 'tr_return_items_no_change', 'tr_subscription_invoices_no_delete')`).Scan(&guards)
	if guards != 3 {
		t.Fatalf("as travas deveriam voltar: %d", guards)
	}
	var audits int
	e.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'pessoa_apagada_com_historico'`).Scan(&audits)
	if audits != 1 {
		t.Fatal("a exclusão deveria ficar na auditoria")
	}
}
