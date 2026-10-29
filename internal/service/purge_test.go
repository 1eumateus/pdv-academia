package service

import (
	"strings"
	"testing"
	"time"

	"pdv/internal/models"
)

func TestPurgeTodayTests(t *testing.T) {
	e := newEnv(t).withCash()
	purge := NewPurgeService(NewBase(e.db))
	today := time.Now().Format(dateLayout)
	p := e.product("Água", 300, 10)
	ana := e.customer("Ana", 10000)
	triggers := func() (n int64) {
		e.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name LIKE '%no_delete'`).Scan(&n)
		return n
	}
	before := triggers()

	if _, err := e.sell("dinheiro", nil, item(p.ID, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell("fiado", &ana.ID, item(p.ID, 1)); err != nil {
		t.Fatal(err)
	}
	c, err := e.sell("pix", nil, item(p.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.CancelSale(e.ctx, c.Sale.ID, "teste"); err != nil {
		t.Fatal(err)
	}

	if _, err := purge.Preview(e.as(e.user("op", models.RoleOperator)), today, e.admin.ID); err == nil {
		t.Fatal("só o administrador apaga vendas de teste")
	}
	pv, err := purge.Preview(e.ctx, today, e.admin.ID)
	if err != nil || len(pv.Sales) != 3 || len(pv.Sessions) != 1 || len(pv.Blockers) != 0 || pv.TotalCents != 900 {
		t.Fatalf("prévia: %v %+v", err, pv)
	}
	if _, err := purge.Apply(e.ctx, today, e.admin.ID, 2, 1); codeOf(err) != CodeStateConflict {
		t.Fatalf("prévia diferente do banco deveria recusar: %v", err)
	}
	res, err := purge.Apply(e.ctx, today, e.admin.ID, 3, 1)
	if err != nil || !res.Applied {
		t.Fatalf("apagar: %v %+v", err, res)
	}
	for _, tb := range []string{"sales", "sale_items", "sale_payments", "cash_sessions", "cash_movements"} {
		if n := e.count(tb); n != 0 {
			t.Errorf("%s deveria estar vazia: %d", tb, n)
		}
	}
	if s := e.stockOf(p.ID); s != 10 {
		t.Errorf("estoque deveria voltar a 10: %d", s)
	}
	if b := e.balanceOf(ana.ID); b != 0 {
		t.Errorf("dívida da Ana deveria zerar: %d", b)
	}
	if n := triggers(); n != before {
		t.Fatalf("as travas do banco deveriam voltar: %d de %d", n, before)
	}
	var audits int
	e.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'vendas_teste_apagadas'`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("a limpeza deveria ficar na auditoria: %d", audits)
	}
	// Travas de volta: apagar venda direto continua proibido.
	if _, err := e.cash.Open(e.ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell("dinheiro", nil, item(p.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`DELETE FROM sales`); err == nil {
		t.Fatal("depois da limpeza, apagar venda deveria voltar a ser bloqueado")
	}
}

func TestPurgeBlockedByFiadoPayment(t *testing.T) {
	e := newEnv(t).withCash()
	purge := NewPurgeService(NewBase(e.db))
	today := time.Now().Format(dateLayout)
	p := e.product("Água", 300, 10)
	ana := e.customer("Ana", 10000)
	if _, err := e.sell("fiado", &ana.ID, item(p.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.credit.PayCustomerCredit(e.ctx, models.PayCreditInput{CustomerID: ana.ID, AmountCents: 300, PaymentMethod: "dinheiro"}); err != nil {
		t.Fatal(err)
	}
	pv, err := purge.Preview(e.ctx, today, e.admin.ID)
	if err != nil || len(pv.Blockers) == 0 || !strings.Contains(strings.Join(pv.Blockers, " "), "fiado") {
		t.Fatalf("recebimento de fiado deveria bloquear: %v %+v", err, pv.Blockers)
	}
	sales := e.count("sales")
	if _, err := purge.Apply(e.ctx, today, e.admin.ID, len(pv.Sales), len(pv.Sessions)); codeOf(err) != CodeStateConflict {
		t.Fatalf("com bloqueio, não apaga: %v", err)
	}
	if n := e.count("sales"); n != sales {
		t.Fatal("nada deveria ter sido apagado")
	}
}

// Venda do operador no caixa aberto por outro: sai a venda e o lançamento; o caixa fica.
func TestPurgeSaleInOtherCash(t *testing.T) {
	e := newEnv(t).withCash()
	purge := NewPurgeService(NewBase(e.db))
	today := time.Now().Format(dateLayout)
	p := e.product("Água", 300, 10)
	if _, err := e.sell("dinheiro", nil, item(p.ID, 1)); err != nil { // do admin
		t.Fatal(err)
	}
	op := e.user("op", models.RoleOperator)
	if _, err := e.sales.CreateSale(e.as(op), models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 2)}, PaymentMethod: "dinheiro"}); err != nil {
		t.Fatal(err)
	}
	if _, err := purge.Preview(e.ctx, time.Now().AddDate(0, 0, 1).Format(dateLayout), op.ID); codeOf(err) != CodeInvalid {
		t.Fatalf("dia futuro deveria ser recusado: %v", err)
	}
	pv, err := purge.Preview(e.ctx, today, op.ID)
	if err != nil || len(pv.Sales) != 1 || len(pv.Sessions) != 0 || len(pv.TouchedSessions) != 1 || len(pv.Blockers) != 0 {
		t.Fatalf("prévia do operador: %v %+v", err, pv)
	}
	if _, err := purge.Apply(e.ctx, today, op.ID, 1, 0); err != nil {
		t.Fatal(err)
	}
	if n := e.count("sales"); n != 1 {
		t.Fatalf("só a venda do admin deveria ficar: %d", n)
	}
	if n := e.count("cash_sessions"); n != 1 {
		t.Fatalf("o caixa do admin fica: %d", n)
	}
	if n := e.count("cash_movements"); n != 1 {
		t.Fatalf("só o lançamento da venda do admin fica: %d", n)
	}
	if s := e.stockOf(p.ID); s != 9 {
		t.Fatalf("estoque: volta a venda do operador (2), fica a do admin (1): %d", s)
	}
	// Todos os operadores: sai o resto, inclusive o caixa.
	pv, err = purge.Preview(e.ctx, today, 0)
	if err != nil || len(pv.Sales) != 1 || len(pv.Sessions) != 1 {
		t.Fatalf("prévia de todos: %v %+v", err, pv)
	}
	if _, err := purge.Apply(e.ctx, today, 0, 1, 1); err != nil {
		t.Fatal(err)
	}
	if e.count("sales")+e.count("cash_sessions")+e.count("cash_movements") != 0 || e.stockOf(p.ID) != 10 {
		t.Fatal("com todos, o dia deveria ficar limpo")
	}
}
