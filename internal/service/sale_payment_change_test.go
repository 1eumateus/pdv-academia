package service

import (
	"slices"
	"testing"
	"time"

	"pdv/internal/models"
)

func paymentMethods(t *testing.T, e *env, saleID int64) (sale, payment, movement string) {
	t.Helper()
	err := e.db.QueryRow(`SELECT s.payment_method, p.method, m.payment_method FROM sales s
		JOIN sale_payments p ON p.sale_id = s.id
		JOIN cash_movements m ON m.reference_type = 'sale' AND m.reference_id = s.id AND m.type = 'venda'
		WHERE s.id = ?`, saleID).Scan(&sale, &payment, &movement)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestChangePaymentOnlyOnce(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Água", 3000, 10)
	d, err := e.sell("credito", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.PaymentChangeMethods) != 3 || d.PaymentChangeBlocked != "" {
		t.Fatalf("opções: %+v %q", d.PaymentChangeMethods, d.PaymentChangeBlocked)
	}
	if _, err := e.sales.ChangePaymentMethod(e.ctx, d.Sale.ID, "debito", "  "); codeOf(err) != CodeInvalid {
		t.Fatalf("motivo é obrigatório: %v", err)
	}
	out, err := e.sales.ChangePaymentMethod(e.ctx, d.Sale.ID, "debito", "passou errado")
	if err != nil {
		t.Fatal(err)
	}
	if s, pay, m := paymentMethods(t, e, d.Sale.ID); s != "debito" || pay != "debito" || m != "debito" {
		t.Fatalf("formas: %s %s %s", s, pay, m)
	}
	if out.Payments[0].Installments != 0 || out.Sale.TotalCents != 3000 {
		t.Fatalf("pagamento: %+v", out)
	}
	if out.PaymentChange == nil || out.PaymentChange.FromMethod != "credito" || out.PaymentChange.Reason != "passou errado" ||
		len(out.PaymentChangeMethods) != 0 || out.PaymentChangeBlocked == "" {
		t.Fatalf("registro da troca: %+v", out)
	}
	var audited int
	e.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'forma_pagamento_alterada' AND entity_id = ?`, d.Sale.ID).Scan(&audited)
	if audited != 1 {
		t.Fatalf("auditoria: %d", audited)
	}

	// Segunda troca: recusada no serviço e também por SQL direto em cada tabela.
	if _, err := e.sales.ChangePaymentMethod(e.ctx, d.Sale.ID, "pix", "de novo"); codeOf(err) != CodeStateConflict {
		t.Fatalf("segunda troca: %v", err)
	}
	for _, q := range []string{
		`UPDATE sales SET payment_method = 'pix' WHERE id = ?`,
		`UPDATE sales SET payment_method = 'credito' WHERE id = ?`,
		`UPDATE sale_payments SET method = 'pix' WHERE sale_id = ?`,
		`UPDATE sale_payments SET method = 'credito' WHERE sale_id = ?`,
		`UPDATE cash_movements SET payment_method = 'pix' WHERE reference_type = 'sale' AND reference_id = ?`,
		`DELETE FROM sale_payment_changes WHERE sale_id = ?`,
		`UPDATE sale_payment_changes SET to_method = 'pix' WHERE sale_id = ?`,
	} {
		if _, err := e.db.Exec(q, d.Sale.ID); err == nil {
			t.Errorf("o banco deveria recusar: %s", q)
		}
	}
	// Sem o registro da troca, as travas continuam valendo.
	other, err := e.sell("pix", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE sales SET payment_method = 'debito' WHERE id = ?`, other.Sale.ID); err == nil {
		t.Error("o banco deveria recusar a troca sem registro")
	}
	if _, err := e.db.Exec(`UPDATE sales SET total_cents = 1, subtotal_cents = 1 WHERE id = ?`, other.Sale.ID); err == nil {
		t.Error("o banco deveria recusar a mudança de valor")
	}
}

func TestChangePaymentRequiresTodaysOpenCash(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Água", 3000, 10)
	d, err := e.sell("pix", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	// Caixa aberto ontem e ainda aberto: não é o caixa do dia.
	old := e.ctx
	var opened string
	if err := e.db.QueryRow(`SELECT opened_at FROM cash_sessions WHERE id = ?`, *d.Sale.CashSessionID).Scan(&opened); err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().Add(-48 * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
	if _, err := e.db.Exec(`UPDATE cash_sessions SET opened_at = ? WHERE id = ?`, yesterday, *d.Sale.CashSessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.ChangePaymentMethod(old, d.Sale.ID, "debito", "x"); codeOf(err) != CodeStateConflict {
		t.Fatalf("caixa de outro dia: %v", err)
	}
	if _, err := e.db.Exec(`UPDATE cash_sessions SET opened_at = ? WHERE id = ?`, opened, *d.Sale.CashSessionID); err != nil {
		t.Fatal(err)
	}
	// Caixa fechado hoje: também não.
	if _, err := e.cash.Close(e.ctx, 10000, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.ChangePaymentMethod(e.ctx, d.Sale.ID, "debito", "x"); codeOf(err) != CodeStateConflict {
		t.Fatalf("caixa fechado: %v", err)
	}
	details, err := e.sales.Details(e.ctx, d.Sale.ID)
	if err != nil || len(details.PaymentChangeMethods) != 0 || details.PaymentChangeBlocked == "" {
		t.Fatalf("detalhes de caixa fechado: %v %+v", err, details)
	}
	if s, pay, m := paymentMethods(t, e, d.Sale.ID); s != "pix" || pay != "pix" || m != "pix" {
		t.Fatalf("nada deveria mudar: %s %s %s", s, pay, m)
	}
}

func TestChangePaymentUpdatesExpectedCash(t *testing.T) {
	e := newEnv(t).withCash() // fundo de R$ 100,00
	p := e.product("Água", 3000, 10)
	expected := func() int64 {
		t.Helper()
		c, err := e.cash.Current(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		return c.ExpectedCashCents
	}
	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, PaymentMethod: "dinheiro",
		Payments: []models.PaymentInput{{Method: "dinheiro", AmountCents: 3000, ReceivedCents: 5000}}})
	if err != nil {
		t.Fatal(err)
	}
	if expected() != 13000 {
		t.Fatalf("antes: %d", expected())
	}
	out, err := e.sales.ChangePaymentMethod(e.ctx, d.Sale.ID, "pix", "foi Pix")
	if err != nil {
		t.Fatal(err)
	}
	if expected() != 10000 {
		t.Fatalf("de dinheiro para Pix: %d", expected())
	}
	if out.Payments[0].ReceivedCents != 0 || out.Payments[0].ChangeCents != 0 {
		t.Fatalf("troco ficou: %+v", out.Payments[0])
	}
	// Para dinheiro entra na gaveta.
	d2, err := e.sell("credito", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.ChangePaymentMethod(e.ctx, d2.Sale.ID, "dinheiro", "foi dinheiro"); err != nil {
		t.Fatal(err)
	}
	if expected() != 13000 {
		t.Fatalf("de crédito para dinheiro: %d", expected())
	}
	// Dinheiro que já saiu em sangria não pode virar outra forma.
	d3, err := e.sell("dinheiro", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.cash.RegisterMovement(e.ctx, models.CashWithdrawal, 15000, "banco"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.ChangePaymentMethod(e.ctx, d3.Sale.ID, "pix", "x"); codeOf(err) != CodeStateConflict {
		t.Fatalf("gaveta ficaria negativa: %v", err)
	}
	// Cancelar depois da troca estorna na forma nova.
	if _, err := e.sales.CancelSale(e.ctx, d2.Sale.ID, "erro"); err != nil {
		t.Fatal(err)
	}
	var refund string
	if err := e.db.QueryRow(`SELECT payment_method FROM cash_movements WHERE type = 'estorno_venda' AND reference_id = ?`, d2.Sale.ID).Scan(&refund); err != nil {
		t.Fatal(err)
	}
	if refund != "dinheiro" {
		t.Fatalf("estorno em %s", refund)
	}
}

func TestChangePaymentBlockedCases(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Água", 3000, 10)
	cust := e.customer("Suely", 100000)

	// Em duas formas.
	mixed, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, PaymentMethod: "misto",
		Payments: []models.PaymentInput{{Method: "pix", AmountCents: 1000}, {Method: "debito", AmountCents: 2000}}})
	if err != nil {
		t.Fatal(err)
	}
	// A prazo.
	fiado, err := e.sell("fiado", &cust.ID, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	// Cancelada.
	cancelled, err := e.sell("pix", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.CancelSale(e.ctx, cancelled.Sale.ID, "erro"); err != nil {
		t.Fatal(err)
	}
	// Com devolução.
	returned, err := e.sell("dinheiro", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReturnService(NewBase(e.db)).Create(e.ctx, models.ReturnInput{SaleID: returned.Sale.ID, RefundMethod: models.RefundCash,
		Reason: "quebrada", Items: []models.ReturnItemInput{{SaleItemID: returned.Items[0].ID, Qty: 1}}}); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]int64{"misto": mixed.Sale.ID, "fiado": fiado.Sale.ID, "cancelada": cancelled.Sale.ID, "devolução": returned.Sale.ID} {
		if _, err := e.sales.ChangePaymentMethod(e.ctx, id, "debito", "x"); codeOf(err) != CodeStateConflict {
			t.Errorf("%s: %v", name, err)
		}
	}

	ok, err := e.sell("pix", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	// Mesma forma, forma desconhecida ou a prazo como destino.
	for _, m := range []string{"pix", "fiado", "vale_troca", "cartao", "xyz"} {
		if _, err := e.sales.ChangePaymentMethod(e.ctx, ok.Sale.ID, m, "x"); codeOf(err) != CodeStateConflict {
			t.Errorf("destino %s: %v", m, err)
		}
	}
	// Permissão de risco: fora do padrão de operador e gerente (pede autorização).
	for _, role := range []string{models.RoleOperator, models.RoleManager} {
		ctx := e.as(e.user("u"+role, role))
		if _, err := e.sales.ChangePaymentMethod(ctx, ok.Sale.ID, "debito", "x"); codeOf(err) != CodeAuthRequired {
			t.Fatalf("%s: %v", role, err)
		}
	}
	if s, _, _ := paymentMethods(t, e, ok.Sale.ID); s != "pix" {
		t.Fatalf("a venda mudou: %s", s)
	}
}

func TestRiskyPermsOnlyWhenAdminGrants(t *testing.T) {
	for _, role := range []string{models.RoleOperator, models.RoleManager} {
		for _, p := range configurablePerms {
			if p.Risky && slices.Contains(DefaultPerms(role), p.Key) {
				t.Errorf("%s não pode vir no padrão de %s", p.Key, role)
			}
		}
	}
	if !slices.Contains(EffectivePerms(models.User{Role: models.RoleAdmin}), PermChangePayment) {
		t.Fatal("o administrador deve ter a permissão")
	}
	// Marcada pelo administrador, vale para o gerente.
	u := models.User{Role: models.RoleManager, Permissions: []string{PermChangePayment, permsVersion}}
	if !slices.Contains(EffectivePerms(u), PermChangePayment) {
		t.Fatal("permissão marcada pelo administrador deve valer")
	}
	if isConfigurable(PermUsers) {
		t.Fatal("gerenciar usuários continua só do administrador")
	}
}

func TestLegacyGatePermExpands(t *testing.T) {
	// Cadastro gravado antes da divisão: "catraca" vale pelas novas e a liberação manual continua.
	old := EffectivePerms(models.User{Role: models.RoleManager, Permissions: []string{PermGate}})
	for _, k := range []string{PermGateMember, PermGateFace, PermGateRemove, PermGateStaff, PermGateManual} {
		if !slices.Contains(old, k) {
			t.Errorf("cadastro antigo com catraca perdeu %s", k)
		}
	}
	if slices.Contains(old, PermGateConfig) {
		t.Fatal("configurar a catraca é só do administrador, mesmo para quem tinha a catraca antiga")
	}
	if slices.Contains(EffectivePerms(models.User{Role: models.RoleManager, Permissions: []string{PermGateConfig, permsVersion}}), PermGateConfig) {
		t.Fatal("configurar a catraca gravado no cadastro de gerente não pode valer")
	}
	if !slices.Contains(EffectivePerms(models.User{Role: models.RoleAdmin}), PermGateConfig) {
		t.Fatal("o administrador configura a catraca")
	}
	// Gravado na versão nova: só o que foi marcado.
	if slices.Contains(EffectivePerms(models.User{Role: models.RoleOperator, Permissions: []string{permsVersion}}), PermGateManual) {
		t.Fatal("desmarcada na versão nova, a liberação manual não pode valer")
	}
}

func TestPurgeSaleWithPaymentChange(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Água", 3000, 10)
	d, err := e.sell("credito", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.ChangePaymentMethod(e.ctx, d.Sale.ID, "debito", "x"); err != nil {
		t.Fatal(err)
	}
	purge := NewPurgeService(NewBase(e.db))
	today := time.Now().In(Location()).Format(dateLayout)
	prev, err := purge.Preview(e.ctx, today, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := purge.Apply(e.ctx, today, 0, len(prev.Sales), len(prev.Sessions)); err != nil {
		t.Fatal(err)
	}
	if e.count("sale_payment_changes") != 0 || e.count("sales") != 0 {
		t.Fatalf("sobrou: %d trocas, %d vendas", e.count("sale_payment_changes"), e.count("sales"))
	}
	var triggers int
	e.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name = 'tr_sale_payment_changes_no_delete'`).Scan(&triggers)
	if triggers != 1 {
		t.Fatal("a trava deveria voltar depois da limpeza")
	}
}
