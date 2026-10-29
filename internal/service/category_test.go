// Caminho: pdv/internal/service/category_test.go

package service

import (
	"testing"

	"pdv/internal/models"
)

func TestCategories(t *testing.T) {
	e := newEnv(t)
	bebidas, err := e.products.SaveCategory(e.ctx, models.Category{Name: " Bebidas "})
	if err != nil || bebidas.Name != "Bebidas" || !bebidas.Active {
		t.Fatalf("criar categoria: %+v %v", bebidas, err)
	}
	if _, err := e.products.SaveCategory(e.ctx, models.Category{Name: "bebidas"}); codeOf(err) != CodeDuplicate {
		t.Fatalf("nome repetido (sem diferenciar maiúsculas): %v", err)
	}

	p, err := e.products.Create(e.ctx, models.ProductInput{Name: "Refri", PriceCents: 500, Active: true, CategoryID: &bebidas.ID})
	if err != nil || p.CategoryName == nil || *p.CategoryName != "Bebidas" {
		t.Fatalf("produto com categoria: %+v %v", p, err)
	}
	semCat := e.product("Pão", 100, 0)
	if semCat.CategoryID != nil {
		t.Fatal("produto sem categoria deveria ficar nulo")
	}

	// Desativada: produto que já estava nela continua; novo vínculo é recusado.
	bebidas.Active = false
	if _, err := e.products.SaveCategory(e.ctx, bebidas); err != nil {
		t.Fatal(err)
	}
	if _, err := e.products.Update(e.ctx, models.ProductInput{ID: p.ID, Name: "Refri lata", PriceCents: 500, Active: true, CategoryID: &bebidas.ID}); err != nil {
		t.Fatalf("manter categoria desativada já vinculada: %v", err)
	}
	if _, err := e.products.Update(e.ctx, models.ProductInput{ID: semCat.ID, Name: "Pão", PriceCents: 100, Active: true, CategoryID: &bebidas.ID}); codeOf(err) != CodeInvalid {
		t.Fatalf("vincular categoria desativada: %v", err)
	}
	missing := int64(999)
	if _, err := e.products.Create(e.ctx, models.ProductInput{Name: "X", PriceCents: 1, Active: true, CategoryID: &missing}); codeOf(err) != CodeNotFound {
		t.Fatalf("categoria inexistente: %v", err)
	}

	op := e.user("caixa", models.RoleOperator)
	e.auth.UpdateUser(e.ctx, models.UserInput{ID: op.ID, Name: op.Name, Login: op.Login, Role: op.Role, Active: true, Permissions: []string{}})
	if _, err := e.products.SaveCategory(e.as(op), models.Category{Name: "Doces"}); codeOf(err) != CodeAuthRequired {
		t.Fatalf("sem permissão de cadastrar produto não cria categoria: %v", err)
	}
}

func TestCategorySearchAndReport(t *testing.T) {
	e := newEnv(t).withCash()
	beb, _ := e.products.SaveCategory(e.ctx, models.Category{Name: "Bebidas"})
	refri, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Refri", PriceCents: 500, Active: true, CategoryID: &beb.ID, InitialStock: 10})
	pao := e.product("Pão", 100, 10)

	if list, _ := e.products.Search(e.ctx, "", true, beb.ID); len(list) != 1 || list[0].ID != refri.ID {
		t.Fatalf("categoria sem termo deveria listar a categoria: %+v", list)
	}
	if list, _ := e.products.Search(e.ctx, "pão", true, beb.ID); len(list) != 0 {
		t.Fatal("filtro de categoria ignorado")
	}

	if _, err := e.sell(models.PaymentCash, nil, item(refri.ID, 2), item(pao.ID, 3)); err != nil {
		t.Fatal(err)
	}
	cancelled, _ := e.sell(models.PaymentCash, nil, item(refri.ID, 1))
	if _, err := e.sales.CancelSale(e.ctx, cancelled.Sale.ID, "teste"); err != nil {
		t.Fatal(err)
	}
	r, err := e.reports.Sales(e.ctx, today(), today(), 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]models.CategoryTotal{}
	for _, c := range r.ByCategory {
		got[c.CategoryName] = c
	}
	if got["Bebidas"].Qty != 2 || got["Bebidas"].TotalCents != 1000 || got["Sem categoria"].TotalCents != 300 {
		t.Fatalf("relatório por categoria (cancelada fora): %+v", r.ByCategory)
	}
}
