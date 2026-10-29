// Caminho: pdv/internal/service/quote_test.go

package service

import (
	"strings"
	"testing"
	"time"

	"pdv/internal/models"
)

func TestQuoteLifecycle(t *testing.T) {
	e := newEnv(t).withCash()
	qs := NewQuoteService(NewBase(e.db))
	caneta := e.product("Caneta", 500, 10)
	caderno := e.product("Caderno", 2000, 3)
	ana := e.customer("Ana", 0)

	d, err := qs.Save(e.ctx, models.QuoteInput{CustomerID: &ana.ID, DiscountCents: 100, Notes: "Lista escolar",
		Items: []models.SaleItemInput{{ProductID: caneta.ID, Qty: 4, DiscountCents: 200}, {ProductID: caderno.ID, Qty: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	// 20,00 + 40,00 = 60,00; desconto 2,00 no item + 1,00 = 3,00 → 57,00
	q := d.Quote
	if q.Status != models.QuoteOpen || q.SubtotalCents != 6000 || q.DiscountCents != 300 || q.TotalCents != 5700 {
		t.Fatalf("orçamento: %+v", q)
	}
	want := localDate(time.Now().AddDate(0, 0, defaultQuoteDays))
	if q.ValidUntil != want {
		t.Fatalf("validade padrão: %s, queria %s", q.ValidUntil, want)
	}
	if e.stockOf(caneta.ID) != 10 {
		t.Fatal("orçamento não pode mexer no estoque")
	}

	// Comprovante marcado como orçamento.
	r, err := NewReceiptService(NewBase(e.db)).Build(e.ctx, models.ReceiptQuote, q.ID)
	if err != nil || !strings.Contains(receiptText(r), "NÃO É DOCUMENTO FISCAL") || !strings.Contains(receiptText(r), "Cliente: Ana") {
		t.Fatalf("comprovante do orçamento:\n%s %v", receiptText(r), err)
	}

	// Mudou o preço e o estoque não cobre: aparecem as diferenças.
	if _, err := e.products.Update(e.ctx, models.ProductInput{ID: caneta.ID, Name: "Caneta", PriceCents: 600, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.stock.Adjust(e.ctx, caderno.ID, -2, "quebra"); err != nil {
		t.Fatal(err)
	}
	check, _ := qs.Check(e.ctx, q.ID)
	var priceMsg, blocked bool
	for _, df := range check.Differences {
		priceMsg = priceMsg || strings.Contains(df.Message, "preço mudou")
		blocked = blocked || df.Blocking
	}
	if !priceMsg || !blocked {
		t.Fatalf("diferenças esperadas (preço e estoque): %+v", check.Differences)
	}
	if check.TotalCents != 6100 {
		t.Fatalf("total com o preço de hoje: %d", check.TotalCents)
	}
	if _, err := qs.Convert(e.ctx, models.QuoteConvertInput{QuoteID: q.ID, PaymentMethod: models.PaymentCash}); codeOf(err) != CodeInsufficientStock {
		t.Fatalf("converter sem estoque: %v", err)
	}
	if _, err := e.stock.RegisterEntry(e.ctx, caderno.ID, 5, ""); err != nil {
		t.Fatal(err)
	}

	// Duplicar mantém itens e descontos, com preço de hoje.
	dup, err := qs.Duplicate(e.ctx, q.ID)
	if err != nil || dup.Quote.ID == q.ID || len(dup.Items) != 2 || dup.Items[0].UnitPriceCents != 600 || dup.Quote.DiscountCents != 300 {
		t.Fatalf("duplicar: %+v %v", dup, err)
	}

	// Converter: preço atual (24,00 + 40,00 − 3,00 = 61,00), estoque baixa, orçamento fica convertido.
	sale, err := qs.Convert(e.ctx, models.QuoteConvertInput{QuoteID: q.ID, PaymentMethod: models.PaymentPix, RequestID: "orc-1"})
	if err != nil {
		t.Fatal(err)
	}
	if sale.Sale.TotalCents != 6100 || sale.Sale.CustomerName == nil || e.stockOf(caneta.ID) != 6 {
		t.Fatalf("venda do orçamento: %+v estoque %d", sale.Sale, e.stockOf(caneta.ID))
	}
	got, _ := qs.Get(e.ctx, q.ID)
	if got.Quote.Status != models.QuoteConverted || got.Quote.SaleID == nil || *got.Quote.SaleID != sale.Sale.ID {
		t.Fatalf("orçamento depois de convertido: %+v", got.Quote)
	}
	// Mesmo pedido de novo devolve a mesma venda; orçamento convertido não muda mais.
	again, err := qs.Convert(e.ctx, models.QuoteConvertInput{QuoteID: q.ID, PaymentMethod: models.PaymentPix, RequestID: "orc-1"})
	if err != nil || again.Sale.ID != sale.Sale.ID {
		t.Fatalf("pedido repetido: %v", err)
	}
	if _, err := qs.SetStatus(e.ctx, q.ID, models.QuoteCancelled); codeOf(err) != CodeStateConflict {
		t.Fatalf("cancelar convertido: %v", err)
	}
	if _, err := qs.Save(e.ctx, models.QuoteInput{ID: q.ID, Items: []models.SaleItemInput{item(caneta.ID, 1)}}); codeOf(err) != CodeStateConflict {
		t.Fatalf("editar convertido: %v", err)
	}

	// Vencido aparece como expirado.
	old, _ := qs.Save(e.ctx, models.QuoteInput{ValidUntil: "2020-01-01", Items: []models.SaleItemInput{item(caneta.ID, 1)}})
	if old.Quote.Status != models.QuoteExpired {
		t.Fatalf("vencido deveria aparecer expirado: %s", old.Quote.Status)
	}
	list, _ := qs.List(e.ctx, today(), today(), "")
	if len(list) != 3 {
		t.Fatalf("lista de orçamentos: %d", len(list))
	}
	if found, _ := qs.List(e.ctx, today(), today(), "ana"); len(found) != 2 {
		t.Fatalf("busca por cliente: %d", len(found))
	}
}

func TestSaleFromQuoteInSalesScreen(t *testing.T) {
	e := newEnv(t).withCash()
	qs := NewQuoteService(NewBase(e.db))
	p := e.product("Caneta", 500, 10)
	d, _ := qs.Save(e.ctx, models.QuoteInput{Items: []models.SaleItemInput{item(p.ID, 2)}})
	// Na tela de vendas o operador acrescentou mais um item; a venda leva o orçamento junto.
	q := e.product("Lápis", 200, 10)
	sale, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{PaymentMethod: models.PaymentCash, QuoteID: &d.Quote.ID,
		Items: []models.SaleItemInput{item(p.ID, 2), item(q.ID, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := qs.Get(e.ctx, d.Quote.ID)
	if got.Quote.Status != models.QuoteConverted || got.Quote.SaleID == nil || *got.Quote.SaleID != sale.Sale.ID {
		t.Fatalf("orçamento deveria ficar convertido: %+v", got.Quote)
	}
	// Usar o mesmo orçamento de novo é recusado e não grava a segunda venda.
	before := e.count("sales")
	if _, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{PaymentMethod: models.PaymentCash, QuoteID: &d.Quote.ID,
		Items: []models.SaleItemInput{item(p.ID, 1)}}); codeOf(err) != CodeStateConflict || e.count("sales") != before {
		t.Fatalf("orçamento já convertido: %v", err)
	}
}

func TestCancelledSaleReopensQuote(t *testing.T) {
	e := newEnv(t).withCash()
	qs := NewQuoteService(NewBase(e.db))
	p := e.product("Caneta", 500, 10)
	d, _ := qs.Save(e.ctx, models.QuoteInput{Items: []models.SaleItemInput{item(p.ID, 2)}})
	sale, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{PaymentMethod: models.PaymentCash, QuoteID: &d.Quote.ID,
		Items: []models.SaleItemInput{item(p.ID, 2)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.CancelSale(e.ctx, sale.Sale.ID, "cliente desistiu"); err != nil {
		t.Fatal(err)
	}
	got, _ := qs.Get(e.ctx, d.Quote.ID)
	if got.Quote.Status != models.QuoteOpen || got.Quote.SaleID != nil {
		t.Fatalf("orçamento deveria voltar a aberto: %+v", got.Quote)
	}
	// Pode virar venda de novo.
	if _, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{PaymentMethod: models.PaymentCash, QuoteID: &d.Quote.ID,
		Items: []models.SaleItemInput{item(p.ID, 2)}}); err != nil {
		t.Fatalf("vender o orçamento reaberto: %v", err)
	}
	// Cancelado continua definitivo no banco.
	c, _ := qs.Save(e.ctx, models.QuoteInput{Items: []models.SaleItemInput{item(p.ID, 1)}})
	qs.SetStatus(e.ctx, c.Quote.ID, models.QuoteCancelled)
	if _, err := e.db.Exec(`UPDATE quotes SET status = 'aberto' WHERE id = ?`, c.Quote.ID); err == nil {
		t.Fatal("orçamento cancelado não pode ser reaberto")
	}
}
