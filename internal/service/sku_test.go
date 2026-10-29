// Caminho: pdv/internal/service/sku_test.go

package service

import (
	"strings"
	"testing"

	"pdv/internal/models"
)

func TestProductSKU(t *testing.T) {
	e := newEnv(t)
	sku, bc := "CAM-001", "7891000000001"
	p, err := e.products.Create(e.ctx, models.ProductInput{Name: "Camiseta", Barcode: &bc, Sku: &sku, PriceCents: 5000, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	// O leitor acha pelo SKU e pelo código de barras.
	for _, code := range []string{"CAM-001", bc} {
		if got, err := e.products.SearchByBarcode(e.ctx, code); err != nil || got.ID != p.ID || got.Sku == nil || *got.Sku != sku {
			t.Fatalf("busca por %s: %+v %v", code, got, err)
		}
	}
	// SKU repetido é recusado com mensagem clara.
	if _, err := e.products.Create(e.ctx, models.ProductInput{Name: "Outra", Sku: &sku, PriceCents: 100, Active: true}); err == nil || !strings.Contains(err.Error(), "SKU CAM-001") {
		t.Fatalf("SKU duplicado: %v", err)
	}
	// Editar sem mandar o SKU apaga (o formulário sempre manda o valor atual).
	p2, err := e.products.Update(e.ctx, models.ProductInput{ID: p.ID, Name: "Camiseta", Barcode: &bc, PriceCents: 5000, Active: true})
	if err != nil || p2.Sku != nil {
		t.Fatalf("edição: %+v %v", p2, err)
	}
}
