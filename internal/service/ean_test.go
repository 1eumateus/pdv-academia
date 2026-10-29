// Caminho: pdv/internal/service/ean_test.go

package service

import (
	"testing"

	"pdv/internal/models"
)

func TestEAN13AndInternalBarcode(t *testing.T) {
	// Códigos reais conhecidos.
	for _, c := range []string{"4006381333931", "7891000100103", "5901234123457"} {
		if !ValidEAN13(c) {
			t.Fatalf("%s deveria ser válido", c)
		}
	}
	if ValidEAN13("4006381333932") || ValidEAN13("400638133393") {
		t.Fatal("aceitou EAN inválido")
	}

	e := newEnv(t)
	bc := "7891000100103"
	com, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Com código", Barcode: &bc, PriceCents: 100, Active: true})
	sem, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Sem código", PriceCents: 100, Active: true})
	// Alguém já usou à mão o código que seria gerado: pula para o próximo bloco.
	taken := internalEAN(sem.ID)
	e.products.Create(e.ctx, models.ProductInput{Name: "Ocupa", Barcode: &taken, PriceCents: 100, Active: true})

	out, err := e.products.AssignInternalBarcodes(e.ctx, []int64{com.ID, sem.ID})
	if err != nil || len(out) != 2 {
		t.Fatal(out, err)
	}
	if *out[0].Barcode != bc {
		t.Fatal("trocou o código de quem já tinha")
	}
	got := *out[1].Barcode
	if !ValidEAN13(got) || got[0] != '2' || got == taken {
		t.Fatalf("código interno: %s", got)
	}
	if p, err := e.products.SearchByBarcode(e.ctx, got); err != nil || p.ID != sem.ID {
		t.Fatal("código não foi gravado", err)
	}
}

func TestProductCodes(t *testing.T) {
	e := newEnv(t)
	// Ocupa à mão o SKU "2200" e o EAN da sequência 2201: a sugestão pula as duas.
	one, two := "2200", internalEAN(2201)
	e.products.Create(e.ctx, models.ProductInput{Name: "A", Sku: &one, PriceCents: 100, Active: true})
	e.products.Create(e.ctx, models.ProductInput{Name: "B", Barcode: &two, PriceCents: 100, Active: true})
	c, err := e.products.SuggestCode(e.ctx)
	if err != nil || c.Sku != "2202" || c.Barcode != internalEAN(2202) || !ValidEAN13(c.Barcode) {
		t.Fatal(c, err)
	}
	// Sugerir não reserva; cadastrar com Auto usa e avança.
	if again, _ := e.products.SuggestCode(e.ctx); again != c {
		t.Fatal("sugestão reservou o código", again)
	}
	p, err := e.products.Create(e.ctx, models.ProductInput{Name: "C", PriceCents: 100, Active: true, AutoSku: true, AutoBarcode: true})
	if err != nil || *p.Sku != "2202" || *p.Barcode != internalEAN(2202) {
		t.Fatal(p, err)
	}
	// Só o SKU automático: o código de fábrica fica.
	bc := "7891000100103"
	p, _ = e.products.Create(e.ctx, models.ProductInput{Name: "D", Barcode: &bc, PriceCents: 100, Active: true, AutoSku: true})
	if *p.Sku != "2203" || *p.Barcode != bc {
		t.Fatal(p)
	}
	// Reservados para a planilha não voltam na sugestão.
	r, _ := e.products.ReserveCodes(e.ctx, 2)
	if r[0].Sku != "2204" || r[1].Sku != "2205" {
		t.Fatal(r)
	}
	if c, _ = e.products.SuggestCode(e.ctx); c.Sku != "2206" {
		t.Fatal(c)
	}
}

func TestDeleteProduct(t *testing.T) {
	e := newEnv(t)
	novo, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Errado", PriceCents: 100, Active: true})
	if ok, err := e.products.Delete(e.ctx, novo.ID); err != nil || !ok {
		t.Fatal("não excluiu produto sem histórico", err)
	}
	if _, err := e.products.Get(e.ctx, novo.ID); err == nil {
		t.Fatal("produto continua cadastrado")
	}
	// Com movimento de estoque (até o saldo inicial do cadastro): só desativa.
	usado, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Usado", PriceCents: 100, Active: true, InitialStock: 3})
	if ok, err := e.products.Delete(e.ctx, usado.ID); err != nil || ok {
		t.Fatal("excluiu produto com histórico", err)
	}
	if p, _ := e.products.Get(e.ctx, usado.ID); p.Active {
		t.Fatal("não desativou")
	}
	if _, err := e.products.Delete(e.ctx, usado.ID); err == nil {
		t.Fatal("deveria avisar que já está desativado")
	}
}
