// Caminho: pdv/internal/service/dashboard_test.go

package service

import (
	"testing"

	"pdv/internal/models"
)

func TestDashboard(t *testing.T) {
	e := newEnv(t).withCash()
	caneta := e.product("Caneta", 500, 10)
	e.product("Lápis", 200, 0) // zerado
	ana := e.customer("Ana", 10000)
	if _, err := e.sell(models.PaymentCash, nil, item(caneta.ID, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell(models.PaymentCredit, &ana.ID, item(caneta.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewQuoteService(NewBase(e.db)).Save(e.ctx, models.QuoteInput{Items: []models.SaleItemInput{item(caneta.ID, 4)}}); err != nil {
		t.Fatal(err)
	}

	d, err := e.reports.Dashboard(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !d.AllSales || d.Today.Count != 2 || d.Today.TotalCents != 1500 || d.Today.AvgCents != 750 || d.Month.TotalCents != 1500 {
		t.Fatalf("totais do dia/mês: %+v %+v", d.Today, d.Month)
	}
	if len(d.Days) != dashboardDays || d.Days[dashboardDays-1].TotalCents != 1500 || d.Days[dashboardDays-1].Date != today() {
		t.Fatalf("gráfico: último dia %+v", d.Days[len(d.Days)-1])
	}
	if len(d.TopProducts) != 1 || d.TopProducts[0].Qty != 3 {
		t.Fatalf("mais vendidos: %+v", d.TopProducts)
	}
	if d.ZeroStockCount != 1 || len(d.StockAlerts) != 1 || d.StockAlerts[0].Name != "Lápis" {
		t.Fatalf("alertas de estoque: %+v", d.StockAlerts)
	}
	if d.CreditOpenCents != 500 || d.CreditCustomers != 1 || d.OpenQuotes != 1 || d.OpenQuotesCents != 2000 {
		t.Fatalf("fiado/orçamentos: %+v", d)
	}
	if d.Cash == nil || d.CashExpected != 10000+1000 {
		t.Fatalf("caixa: %+v esperado %d", d.Cash, d.CashExpected)
	}

	// Operador sem a permissão vê só as próprias vendas.
	op := e.user("caixa", models.RoleOperator)
	od, err := e.reports.Dashboard(e.as(op))
	if err != nil {
		t.Fatal(err)
	}
	if od.AllSales || od.Today.Count != 0 || len(od.TopProducts) != 0 || od.ZeroStockCount != 1 {
		t.Fatalf("operador: %+v", od.Today)
	}
}
