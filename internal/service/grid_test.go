// Caminho: pdv/internal/service/grid_test.go

package service

import (
	"slices"
	"testing"

	"pdv/internal/models"
)

func TestGrid(t *testing.T) {
	e := newEnv(t).withCash()
	roupas, _ := e.products.SaveCategory(e.ctx, models.Category{Name: "Roupas"})

	m, err := e.products.SaveGrid(e.ctx, models.GridInput{Name: "Camiseta básica", CategoryID: &roupas.ID, BasePriceCents: 4990,
		Sizes: []string{"P", "M", " g ", "m"}, Colors: []string{"Azul", "Preto"}})
	if err != nil {
		t.Fatal(err)
	}
	if m.VariationCount != 6 || !slices.Equal(m.Sizes, []string{"P", "M", "g"}) || !slices.Equal(m.Colors, []string{"Azul", "Preto"}) {
		t.Fatalf("grade 3×2 errada: %+v", m)
	}

	list, _ := e.products.List(e.ctx)
	var mPreto models.Product
	for _, p := range list {
		if p.ModelID != nil && *p.ModelID == m.ID && p.Size == "M" && p.Color == "Preto" {
			mPreto = p
		}
	}
	if mPreto.Name != "Camiseta básica M Preto" || mPreto.PriceCents != 4990 || mPreto.CategoryName == nil || *mPreto.CategoryName != "Roupas" {
		t.Fatalf("variação gerada errada: %+v", mPreto)
	}

	// A variação vende e baixa estoque como qualquer produto.
	if _, err := e.stock.RegisterEntry(e.ctx, mPreto.ID, 5, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell(models.PaymentCash, nil, item(mPreto.ID, 2)); err != nil {
		t.Fatal(err)
	}
	if e.stockOf(mPreto.ID) != 3 {
		t.Fatalf("estoque da variação = %d", e.stockOf(mPreto.ID))
	}

	// Ampliar: só cria as combinações que faltam.
	m, err = e.products.SaveGrid(e.ctx, models.GridInput{ModelID: m.ID, Sizes: []string{"GG"}, Colors: []string{"preto", "Branco"}})
	if err != nil {
		t.Fatal(err)
	}
	if m.VariationCount != 12 { // 4 tamanhos × 3 cores
		t.Fatalf("ampliação deveria chegar a 12 variações: %d", m.VariationCount)
	}
	if _, err := e.products.SaveGrid(e.ctx, models.GridInput{ModelID: m.ID, Sizes: []string{"p"}}); codeOf(err) != CodeInvalid {
		t.Fatalf("sem combinação nova deveria avisar: %v", err)
	}
	if _, err := e.products.SaveGrid(e.ctx, models.GridInput{Name: "Vazia", BasePriceCents: 1}); codeOf(err) != CodeInvalid {
		t.Fatalf("grade sem tamanho e cor: %v", err)
	}
	if _, err := e.products.SaveGrid(e.ctx, models.GridInput{Name: "CAMISETA básica", BasePriceCents: 1, Sizes: []string{"U"}}); codeOf(err) != CodeDuplicate {
		t.Fatalf("modelo repetido: %v", err)
	}

	// Preço para todas exige permissão de alterar preço.
	op := e.user("caixa", models.RoleOperator)
	if _, err := e.products.ApplyGridPrice(e.as(op), m.ID, 3990); codeOf(err) != CodeAuthRequired {
		t.Fatalf("operador sem permissão de preço: %v", err)
	}
	if _, err := e.products.ApplyGridPrice(e.ctx, m.ID, 3990); err != nil {
		t.Fatal(err)
	}
	if p, _ := e.products.Get(e.ctx, mPreto.ID); p.PriceCents != 3990 {
		t.Fatalf("preço da variação = %d", p.PriceCents)
	}
}
