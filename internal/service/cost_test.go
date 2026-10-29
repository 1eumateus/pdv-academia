// Caminho: pdv/internal/service/cost_test.go

package service

import (
	"testing"

	"pdv/internal/models"
)

func TestCostVisibilityAndMinStock(t *testing.T) {
	e := newEnv(t)
	cost := int64(600)
	p, err := e.products.Create(e.ctx, models.ProductInput{Name: "Caderno", PriceCents: 1000, Active: true, CostCents: &cost, MinStock: 5})
	if err != nil || p.CostCents != 600 || p.MinStock != 5 {
		t.Fatalf("admin cadastra com custo: %+v %v", p, err)
	}

	op := e.user("caixa", models.RoleOperator)
	mgr := e.user("gerente", models.RoleManager)
	opCtx := e.as(op)

	// Operador não recebe o custo em nenhuma consulta.
	if got, _ := e.products.Get(opCtx, p.ID); got.CostCents != 0 {
		t.Fatalf("operador viu o custo no Get: %d", got.CostCents)
	}
	if list, _ := e.products.List(opCtx); list[0].CostCents != 0 {
		t.Fatal("operador viu o custo na lista")
	}
	if list, _ := e.products.Search(opCtx, "cad", true, 0); list[0].CostCents != 0 {
		t.Fatal("operador viu o custo na busca")
	}
	if got, _ := e.stock.RegisterEntry(opCtx, p.ID, 1, ""); got.CostCents != 0 {
		t.Fatal("operador viu o custo na entrada de estoque")
	}
	if got, _ := e.products.Get(e.as(mgr), p.ID); got.CostCents != 600 {
		t.Fatalf("gerente deveria ver o custo: %d", got.CostCents)
	}

	// Operador edita sem mandar custo: o custo gravado continua.
	if _, err := e.products.Update(opCtx, models.ProductInput{ID: p.ID, Name: "Caderno 10 matérias", PriceCents: 1000, Active: true, MinStock: 3}); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.products.Get(e.ctx, p.ID); got.CostCents != 600 || got.MinStock != 3 {
		t.Fatalf("custo perdido ou mínimo errado: %+v", got)
	}
	other := int64(700)
	if _, err := e.products.Update(opCtx, models.ProductInput{ID: p.ID, Name: "Caderno", PriceCents: 1000, Active: true, CostCents: &other}); codeOf(err) != CodeAuthRequired {
		t.Fatalf("operador alterar custo: %v", err)
	}
	if _, err := e.products.Create(opCtx, models.ProductInput{Name: "Lápis", PriceCents: 100, Active: true, CostCents: &other}); codeOf(err) != CodeAuthRequired {
		t.Fatalf("operador cadastrar com custo: %v", err)
	}
	if _, err := e.products.Create(e.ctx, models.ProductInput{Name: "X", PriceCents: 1, Active: true, MinStock: -1}); codeOf(err) != CodeInvalid {
		t.Fatalf("estoque mínimo negativo: %v", err)
	}
}
