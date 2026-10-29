// Caminho: pdv/internal/service/analytics_test.go

package service

import (
	"testing"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

func TestProfitUsesCostAtSaleTimeDiscountsAndReturns(t *testing.T) {
	e := newEnv(t).withCash()
	as := NewAnalyticsService(NewBase(e.db))
	cost := int64(400)
	caneca, err := e.products.Create(e.ctx, models.ProductInput{Name: "Caneca", PriceCents: 1000, Active: true, InitialStock: 10, CostCents: &cost})
	if err != nil {
		t.Fatal(err)
	}
	// 3 canecas com R$ 3,00 de desconto geral: receita 2700, custo 1200.
	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(caneca.ID, 3)},
		PaymentMethod: models.PaymentCash, DiscountCents: 300})
	if err != nil {
		t.Fatal(err)
	}
	// O custo muda depois: a venda continua com o custo da hora (400).
	newCost := int64(900)
	if _, err := e.products.Update(e.ctx, models.ProductInput{ID: caneca.ID, Name: "Caneca", PriceCents: 1000, Active: true, CostCents: &newCost}); err != nil {
		t.Fatal(err)
	}
	rep, err := as.Profit(e.ctx, today(), today())
	if err != nil || rep.RevenueCents != 2700 || rep.CostCents != 1200 || rep.ProfitCents != 1500 || rep.Products[0].Estimated {
		t.Fatalf("lucro: %+v %v", rep, err)
	}
	// Devolver 1: sai a receita paga por ela (900) e o custo dela (400).
	if _, err := NewReturnService(NewBase(e.db)).Create(e.ctx, models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundCash,
		Reason: "quebrada", Items: []models.ReturnItemInput{{SaleItemID: d.Items[0].ID, Qty: 1}}}); err != nil {
		t.Fatal(err)
	}
	rep, _ = as.Profit(e.ctx, today(), today())
	if rep.RevenueCents != 1800 || rep.CostCents != 800 || rep.Products[0].Qty != 2 || rep.Categories[0].ProfitCents != 1000 {
		t.Fatalf("lucro após devolução: %+v", rep)
	}

	// Sem ver_custo, o lucro é recusado; os descontos aparecem por operador.
	op := e.user("caixa", models.RoleOperator)
	if _, err := as.Profit(e.as(op), today(), today()); err == nil {
		t.Fatal("operador viu o lucro")
	}
	disc, err := as.Discounts(e.ctx, today(), today())
	if err != nil || len(disc) != 1 || disc[0].DiscountCents != 300 || disc[0].SalesWithDiscount != 1 {
		t.Fatalf("descontos: %+v %v", disc, err)
	}

	// Parados: um produto nunca vendido aparece; o vendido hoje não.
	e.product("Encalhado", 500, 4)
	idle, err := as.Idle(e.ctx, 1)
	if err != nil || len(idle) != 1 || idle[0].Name != "Encalhado" || idle[0].IdleDays != -1 {
		t.Fatalf("parados: %+v %v", idle, err)
	}
	sv, err := as.StockValue(e.ctx)
	if err != nil || len(sv) != 1 || sv[0].Qty != 12 || sv[0].CostCents != 8*900 || sv[0].NoCost != 1 {
		t.Fatalf("estoque valorizado: %+v %v", sv, err)
	}
}

func TestClassifyABC(t *testing.T) {
	rows := []models.ABCRow{{Name: "c", RevenueCents: 50}, {Name: "a", RevenueCents: 700}, {Name: "b", RevenueCents: 200}, {Name: "d", RevenueCents: 50}}
	classifyABC(rows, 1000)
	got := ""
	for _, r := range rows {
		got += r.Name + r.Class
	}
	// a: 0% antes → A; b: 70% antes → A; c: 90% → B; d: 95% → C.
	if got != "aAbAcBdC" || rows[1].CumulativePct != 90 {
		t.Fatalf("ABC: %s %+v", got, rows)
	}
}

func TestAgingBucketsFIFO(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	ago := func(d int) time.Time { return now.AddDate(0, 0, -d) }
	c := []models.Customer{{ID: 1, Name: "Ana", CreditBalanceCents: 700}, {ID: 2, Name: "Bia", CreditBalanceCents: 300}}
	// Ana comprou 500 há 100 dias, 400 há 45 e 200 há 10 (da mais nova para a mais antiga) e pagou 400:
	// a dívida de 700 é 200 (10 dias) + 400 (45 dias) + 100 (100 dias).
	part := func(days int, cents int64) repository.FiadoPart {
		return repository.FiadoPart{CustomerID: 1, At: ago(days), AmountCents: cents}
	}
	parts := []repository.FiadoPart{part(10, 200), part(45, 400), part(100, 500)}
	got := agingBuckets(c, parts, now)
	a, b := got[0], got[1]
	if a.Upto30Cents != 200 || a.Upto60Cents != 400 || a.Over90Cents != 100 || a.OldestDays != 100 {
		t.Fatalf("Ana: %+v", a)
	}
	// Bia sem compra de origem: tudo no mais atrasado.
	if b.Over90Cents != 300 {
		t.Fatalf("Bia: %+v", b)
	}
}

func TestExportXLSXValidates(t *testing.T) {
	as := NewAnalyticsService(Base{})
	cols := []models.ExportColumn{{Label: "Produto", Kind: "text"}, {Label: "Total", Kind: "money"}, {Label: "Margem", Kind: "pct"}}
	if _, err := as.ExportXLSX(models.ExportTable{Title: "Lucro: bruto/teste", Columns: cols, Rows: [][]string{{"Caneca", "1250", "0.25"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := as.ExportXLSX(models.ExportTable{Title: "x", Columns: cols, Rows: [][]string{{"Caneca", "doze", "0"}}}); err == nil {
		t.Fatal("aceitou valor que não é número")
	}
	if _, err := as.ExportXLSX(models.ExportTable{Title: "x", Columns: cols, Rows: [][]string{{"Caneca"}}}); err == nil {
		t.Fatal("aceitou linha com colunas faltando")
	}
	if sheetName("Lucro: bruto/teste com nome muito comprido demais") != "Lucro  bruto teste com nome mui" {
		t.Fatalf("nome da aba: %q", sheetName("Lucro: bruto/teste com nome muito comprido demais"))
	}
}
