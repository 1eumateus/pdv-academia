// Caminho: pdv/internal/service/promotion_test.go

package service

import (
	"strings"
	"testing"
	"time"

	"pdv/internal/models"
)

func TestPromotions(t *testing.T) {
	e := newEnv(t).withCash()
	ps := NewPromotionService(NewBase(e.db))
	beb, _ := e.products.SaveCategory(e.ctx, models.Category{Name: "Bebidas"})
	refri, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Refri", PriceCents: 1000, Active: true, CategoryID: &beb.ID, InitialStock: 50})
	suco, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Suco", PriceCents: 800, Active: true, CategoryID: &beb.ID, InitialStock: 50})
	today := localDate(time.Now())
	tomorrow := localDate(time.Now().AddDate(0, 0, 1))

	// Categoria inteira com 10% e refri a R$ 7,50: no refri vence o preço fixo (maior desconto).
	if _, err := ps.Save(e.ctx, models.Promotion{Name: "Semana das bebidas", Kind: models.PromoPercent, Percent: 10,
		CategoryID: &beb.ID, StartDate: today, EndDate: today, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Save(e.ctx, models.Promotion{Name: "Refri barato", Kind: models.PromoPrice, PriceCents: 750,
		ProductID: &refri.ID, StartDate: today, EndDate: today, Active: true}); err != nil {
		t.Fatal(err)
	}
	// Futura: não vale hoje.
	if _, err := ps.Save(e.ctx, models.Promotion{Name: "Amanhã", Kind: models.PromoPrice, PriceCents: 100,
		ProductID: &suco.ID, StartDate: tomorrow, EndDate: tomorrow, Active: true}); err != nil {
		t.Fatal(err)
	}
	d, err := e.sell(models.PaymentCash, nil, item(refri.ID, 2), item(suco.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if d.Items[0].UnitPriceCents != 750 || d.Items[1].UnitPriceCents != 720 || d.Sale.TotalCents != 2220 {
		t.Fatalf("preços promocionais: refri %d suco %d total %d", d.Items[0].UnitPriceCents, d.Items[1].UnitPriceCents, d.Sale.TotalCents)
	}
	if !strings.Contains(d.Items[0].PromoLabel, "REFRI BARATO") {
		t.Fatalf("item deveria guardar a promoção: %q", d.Items[0].PromoLabel)
	}

	// Leve 3 pague 2 no suco: com 7 unidades, 2 saem de graça (vence os 10% da categoria).
	if _, err := ps.Save(e.ctx, models.Promotion{Name: "Leve 3 pague 2", Kind: models.PromoBuyPay, BuyQty: 3, PayQty: 2,
		ProductID: &suco.ID, StartDate: today, EndDate: today, Active: true}); err != nil {
		t.Fatal(err)
	}
	d, err = e.sell(models.PaymentCash, nil, item(suco.ID, 7))
	if err != nil {
		t.Fatal(err)
	}
	if d.Items[0].UnitPriceCents != 800 || d.Items[0].PromoDiscountCents != 1600 || d.Sale.TotalCents != 4000 {
		t.Fatalf("leve 3 pague 2: %+v total %d", d.Items[0], d.Sale.TotalCents)
	}
	rc, _ := NewReceiptService(NewBase(e.db)).Build(e.ctx, models.ReceiptSale, d.Sale.ID)
	if !strings.Contains(receiptText(rc), "Leve 3 pague 2 -R$ 16,00") {
		t.Fatalf("cupom sem o leve X pague Y:\n%s", receiptText(rc))
	}

	// Desconto de promoção não conta no limite do operador (10%).
	op := e.user("caixa", models.RoleOperator)
	if _, err := e.sales.CreateSale(e.as(op), models.CreateSaleInput{PaymentMethod: models.PaymentCash,
		Items: []models.SaleItemInput{item(suco.ID, 6)}}); err != nil {
		t.Fatalf("operador vendendo com promoção (33%% off) não deveria precisar de gerente: %v", err)
	}

	// Busca de venda mostra a promoção.
	if list, _ := e.products.Search(e.ctx, "refri", true, 0); list[0].Promo == nil || list[0].Promo.PriceCents != 750 {
		t.Fatalf("busca sem a promoção: %+v", list[0].Promo)
	}
	// Comanda: preço promocional vale no lançamento.
	c, _ := e.comandas.Open(e.ctx, "MESA 1")
	cd, err := e.comandas.AddItem(e.ctx, c.Comanda.ID, refri.ID, 1)
	if err != nil || cd.Items[0].UnitPriceCents != 750 {
		t.Fatalf("comanda com promoção: %+v %v", cd.Items, err)
	}

	// Regras do cadastro.
	if _, err := ps.Save(e.ctx, models.Promotion{Name: "X", Kind: models.PromoBuyPay, BuyQty: 2, PayQty: 2,
		ProductID: &suco.ID, StartDate: today, EndDate: today}); codeOf(err) != CodeInvalid {
		t.Fatalf("leve 2 pague 2: %v", err)
	}
	if _, err := ps.Save(e.ctx, models.Promotion{Name: "X", Kind: models.PromoPrice, PriceCents: 1, ProductID: &suco.ID,
		CategoryID: &beb.ID, StartDate: today, EndDate: today}); codeOf(err) != CodeInvalid {
		t.Fatalf("produto e categoria juntos: %v", err)
	}
	if _, err := ps.Save(e.as(op), models.Promotion{Name: "X", Kind: models.PromoPercent, Percent: 5, ProductID: &suco.ID,
		StartDate: today, EndDate: today}); codeOf(err) != CodeAuthRequired {
		t.Fatalf("operador sem alterar preço: %v", err)
	}
	list, _ := ps.List(e.ctx)
	status := map[string]string{}
	for _, p := range list {
		status[p.Name] = p.Status
	}
	if status["Amanhã"] != "futura" || status["Refri barato"] != "vigente" {
		t.Fatalf("status das promoções: %+v", status)
	}
}
