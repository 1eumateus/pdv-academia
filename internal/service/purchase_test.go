// Caminho: pdv/internal/service/purchase_test.go

package service

import (
	"testing"

	"pdv/internal/models"
)

func TestAverageCost(t *testing.T) {
	cases := []struct{ stock, cost, qty, unit, want int64 }{
		{0, 0, 10, 500, 500}, // sem estoque: custo da entrada
		{5, 0, 5, 800, 800},  // estoque sem custo conhecido: custo da entrada
		{10, 400, 10, 600, 500},
		{3, 100, 1, 101, 100}, // 401/4 = 100,25 → 100
		{1, 100, 2, 101, 101}, // 302/3 = 100,67 → 101
	}
	for _, c := range cases {
		if got := averageCost(c.stock, c.cost, c.qty, c.unit); got != c.want {
			t.Errorf("averageCost(%d,%d,%d,%d) = %d, queria %d", c.stock, c.cost, c.qty, c.unit, got, c.want)
		}
	}
}

func TestPurchase(t *testing.T) {
	e := newEnv(t)
	sup, err := NewSupplierService(NewBase(e.db)).Save(e.ctx, models.Supplier{LegalName: "Atacado X"})
	if err != nil {
		t.Fatal(err)
	}
	cost := int64(400)
	caneta, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Caneta", PriceCents: 1000, Active: true, InitialStock: 10, CostCents: &cost})
	lapis := e.product("Lápis", 300, 0)
	svc := NewPurchaseService(NewBase(e.db))

	newPrice := int64(1200)
	d, err := svc.Create(e.ctx, models.PurchaseInput{SupplierID: sup.ID, InvoiceNumber: "123", IssueDate: "2026-10-17",
		Items: []models.PurchaseItemInput{
			{ProductID: caneta.ID, Qty: 10, UnitCostCents: 600, NewPriceCents: &newPrice},
			{ProductID: lapis.ID, Qty: 20, UnitCostCents: 150},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Purchase.TotalCents != 9000 || len(d.Items) != 2 || d.Items[0].CostAfterCents != 500 {
		t.Fatalf("compra: %+v", d)
	}
	c, _ := e.products.Get(e.ctx, caneta.ID)
	l, _ := e.products.Get(e.ctx, lapis.ID)
	if c.StockQty != 20 || c.CostCents != 500 || c.PriceCents != 1200 || l.StockQty != 20 || l.CostCents != 150 {
		t.Fatalf("estoque/custo/preço depois da compra: caneta %+v lápis %+v", c, l)
	}

	// Falha no meio (produto inexistente na 2ª linha) não grava nada.
	before := e.count("purchases")
	_, err = svc.Create(e.ctx, models.PurchaseInput{SupplierID: sup.ID, IssueDate: "2026-10-17",
		Items: []models.PurchaseItemInput{{ProductID: caneta.ID, Qty: 1, UnitCostCents: 1}, {ProductID: 999, Qty: 1, UnitCostCents: 1}}})
	if codeOf(err) != CodeProductNotFound || e.count("purchases") != before || e.stockOf(caneta.ID) != 20 {
		t.Fatalf("falha deveria desfazer tudo: %v", err)
	}
	if _, err := svc.Create(e.ctx, models.PurchaseInput{SupplierID: sup.ID, IssueDate: "2026-10-17",
		Items: []models.PurchaseItemInput{{ProductID: lapis.ID, Qty: 1}, {ProductID: lapis.ID, Qty: 2}}}); codeOf(err) != CodeInvalid {
		t.Fatalf("produto repetido: %v", err)
	}

	op := e.user("caixa", models.RoleOperator)
	if _, err := svc.Create(e.as(op), models.PurchaseInput{SupplierID: sup.ID, IssueDate: "2026-10-17",
		Items: []models.PurchaseItemInput{{ProductID: lapis.ID, Qty: 1}}}); codeOf(err) != CodeAuthRequired {
		t.Fatalf("operador sem compras: %v", err)
	}
	list, err := svc.List(e.ctx, today(), today(), "")
	if err != nil || len(list) != 1 || list[0].SupplierName != "Atacado X" {
		t.Fatalf("lista de compras: %+v %v", list, err)
	}
}
