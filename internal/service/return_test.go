// Caminho: pdv/internal/service/return_test.go

package service

import (
	"strings"
	"testing"

	"pdv/internal/models"
	"pdv/internal/repository"
)

func TestPaidByItemSplitsGeneralDiscount(t *testing.T) {
	sale := models.Sale{DiscountCents: 350}
	items := []models.SaleItem{{ID: 1, Qty: 3, SubtotalCents: 3000}, {ID: 2, Qty: 1, SubtotalCents: 500}}
	paid := paidByItem(sale, items)
	if paid[1] != 2700 || paid[2] != 450 || paid[1]+paid[2] != 3150 {
		t.Fatalf("rateio: %v", paid)
	}
	// Devolver 1 + 1 + 1 de 3 soma exatamente o pago, sem sobrar centavo.
	if a, b, c := refundFor(1000, 3, 0, 1), refundFor(1000, 3, 1, 1), refundFor(1000, 3, 2, 1); a+b+c != 1000 {
		t.Fatalf("partes: %d %d %d", a, b, c)
	}
}

func TestReturnByPixAndCard(t *testing.T) {
	e := newEnv(t).withCash()
	rs := NewReturnService(NewBase(e.db))
	p := e.product("Tênis", 10000, 5)
	// R$ 200: R$ 120 no débito + R$ 80 no Pix.
	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 2)},
		Payments: []models.PaymentInput{part(models.PaymentDebit, 12000), part(models.PaymentPix, 8000)}})
	if err != nil {
		t.Fatal(err)
	}
	sum := func() models.CashSummary {
		s, err := e.cash.Current(e.ctx)
		if err != nil || s == nil {
			t.Fatal(err)
		}
		return *s
	}
	byMethod := func(s models.CashSummary, m string) int64 {
		for _, x := range s.ByPaymentMethod {
			if x.PaymentMethod == m {
				return x.TotalCents
			}
		}
		return 0
	}
	before := sum()
	info, err := rs.Info(e.ctx, d.Sale.ID)
	if err != nil || info.CardLeftCents != 12000 {
		t.Fatalf("info: %d %v", info.CardLeftCents, err)
	}
	// 1 tênis (R$ 100) estornado no cartão: sai do débito, não do dinheiro.
	r, err := rs.Create(e.ctx, models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundCard, Reason: "apertado",
		Items: []models.ReturnItemInput{{SaleItemID: d.Items[0].ID, Qty: 1}}})
	after := sum()
	if err != nil || r.TotalCents != 10000 || after.ExpectedCashCents != before.ExpectedCashCents ||
		byMethod(after, models.PaymentDebit) != byMethod(before, models.PaymentDebit)-10000 {
		t.Fatalf("cartão: %+v %v", r, err)
	}
	// O outro tênis no cartão passaria do que foi pago no cartão (só restam R$ 20).
	_, err = rs.Create(e.ctx, models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundCard, Reason: "x",
		Items: []models.ReturnItemInput{{SaleItemID: d.Items[0].ID, Qty: 1}}})
	wantCode(t, err, CodeInvalid)
	// Por Pix vai.
	if _, err := rs.Create(e.ctx, models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundPix, Reason: "x",
		Items: []models.ReturnItemInput{{SaleItemID: d.Items[0].ID, Qty: 1}}}); err != nil ||
		byMethod(sum(), models.PaymentPix) != byMethod(before, models.PaymentPix)-10000 || sum().ExpectedCashCents != before.ExpectedCashCents {
		t.Fatalf("pix: %v", err)
	}
	// Venda só em dinheiro não estorna no cartão.
	c, _ := e.sell(models.PaymentCash, nil, item(p.ID, 1))
	_, err = rs.Create(e.ctx, models.ReturnInput{SaleID: c.Sale.ID, RefundMethod: models.RefundCard, Reason: "x",
		Items: []models.ReturnItemInput{{SaleItemID: c.Items[0].ID, Qty: 1}}})
	wantCode(t, err, CodeInvalid)
}

func TestReturnsStockCashStoreCreditAndLimits(t *testing.T) {
	e := newEnv(t).withCash()
	rs := NewReturnService(NewBase(e.db))
	camisa := e.product("Camisa", 1000, 10)
	meia := e.product("Meia", 500, 10)
	ana := e.customer("Ana", 0)
	cust := func() models.Customer {
		c, err := (repository.CustomerRepo{}).GetByID(e.ctx, e.db, ana.ID)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	expected := func() int64 {
		s, err := e.cash.Current(e.ctx)
		if err != nil || s == nil {
			t.Fatal(err)
		}
		return s.ExpectedCashCents
	}

	// 3 camisas + 1 meia com R$ 3,50 de desconto geral, em dinheiro, sem cliente.
	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(camisa.ID, 3), item(meia.ID, 1)},
		PaymentMethod: models.PaymentCash, DiscountCents: 350})
	if err != nil || d.Sale.TotalCents != 3150 {
		t.Fatalf("venda: %+v %v", d.Sale, err)
	}
	camisaItem := d.Items[0].ID
	before := expected()

	// Vale-troca de venda sem cliente exige escolher o cliente.
	_, err = rs.Create(e.ctx, models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundStoreCredit, Reason: "tamanho",
		Items: []models.ReturnItemInput{{SaleItemID: camisaItem, Qty: 1}}})
	wantCode(t, err, CodeInvalid)

	r1, err := rs.Create(e.ctx, models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundStoreCredit, Reason: "tamanho",
		CustomerID: &ana.ID, Items: []models.ReturnItemInput{{SaleItemID: camisaItem, Qty: 1}}})
	if err != nil || r1.TotalCents != 900 || cust().StoreCreditCents != 900 || e.stockOf(camisa.ID) != 8 {
		t.Fatalf("vale: %+v %v vale=%d estoque=%d", r1, err, cust().StoreCreditCents, e.stockOf(camisa.ID))
	}
	if expected() != before {
		t.Fatal("vale-troca mexeu no caixa")
	}

	// As outras 2 em dinheiro: o resto do pago (2700 - 900) sai do caixa.
	r2, err := rs.Create(e.ctx, models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundCash, Reason: "defeito",
		Items: []models.ReturnItemInput{{SaleItemID: camisaItem, Qty: 2}}})
	if err != nil || r2.TotalCents != 1800 || expected() != before-1800 || e.stockOf(camisa.ID) != 10 {
		t.Fatalf("dinheiro: %+v %v caixa=%d", r2, err, expected())
	}

	// Não devolve mais do que foi vendido nem cancela venda com devolução.
	_, err = rs.Create(e.ctx, models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundCash, Reason: "x",
		Items: []models.ReturnItemInput{{SaleItemID: camisaItem, Qty: 1}}})
	wantCode(t, err, CodeInvalid)
	_, err = e.sales.CancelSale(e.ctx, d.Sale.ID, "teste")
	wantCode(t, err, CodeStateConflict)

	// Vale-troca como pagamento: desconta do saldo e não entra no caixa.
	cx := expected()
	v, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(meia.ID, 1)}, CustomerID: &ana.ID,
		PaymentMethod: models.PaymentStoreCredit})
	if err != nil || cust().StoreCreditCents != 400 || expected() != cx {
		t.Fatalf("usar vale: %v vale=%d", err, cust().StoreCreditCents)
	}
	_, err = e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(meia.ID, 1)}, CustomerID: &ana.ID,
		PaymentMethod: models.PaymentStoreCredit})
	wantCode(t, err, CodeStateConflict) // saldo 400 < 500
	if _, err := e.sales.CancelSale(e.ctx, v.Sale.ID, "desistiu"); err != nil || cust().StoreCreditCents != 900 {
		t.Fatalf("cancelar venda com vale: %v vale=%d", err, cust().StoreCreditCents)
	}

	// Fiado: a devolução abate a dívida.
	bia := e.customer("Bia", 10000)
	f, err := e.sell(models.PaymentCredit, &bia.ID, item(meia.ID, 2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rs.Create(e.ctx, models.ReturnInput{SaleID: f.Sale.ID, RefundMethod: models.RefundCredit, Reason: "sobrou",
		Items: []models.ReturnItemInput{{SaleItemID: f.Items[0].ID, Qty: 1}}}); err != nil || e.balanceOf(bia.ID) != 500 {
		t.Fatalf("fiado: %v dívida=%d", err, e.balanceOf(bia.ID))
	}
	_, err = rs.Create(e.ctx, models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundCredit, Reason: "x",
		Items: []models.ReturnItemInput{{SaleItemID: d.Items[1].ID, Qty: 1}}})
	wantCode(t, err, CodeInvalid) // venda em dinheiro não abate fiado

	// Comprovante e permissão.
	rec, err := NewReceiptService(NewBase(e.db)).Build(e.ctx, models.ReceiptReturn, r1.ID)
	if err != nil || !strings.Contains(receiptText(rec), "COMPROVANTE DE DEVOLUÇÃO") || !strings.Contains(receiptText(rec), "Saldo de vale-troca") {
		t.Fatalf("comprovante: %v\n%s", err, receiptText(rec))
	}
	op := e.user("caixa", models.RoleOperator)
	_, err = rs.Create(e.as(op), models.ReturnInput{SaleID: d.Sale.ID, RefundMethod: models.RefundCash, Reason: "x",
		Items: []models.ReturnItemInput{{SaleItemID: d.Items[1].ID, Qty: 1}}})
	wantCode(t, err, CodeAuthRequired)

	if l, err := rs.List(e.ctx, today(), today()); err != nil || len(l) != 3 || l[0].RefundMethod != models.RefundCredit {
		t.Fatalf("lista: %+v %v", l, err)
	}
	if l, _ := rs.List(e.as(op), today(), today()); len(l) != 0 {
		t.Fatalf("operador viu devoluções de outro: %+v", l)
	}

	// O cupom da venda mostra o que foi devolvido.
	sr, err := NewReceiptService(NewBase(e.db)).Build(e.ctx, models.ReceiptSale, d.Sale.ID)
	if txt := receiptText(sr); err != nil || !strings.Contains(txt, "ITENS DEVOLVIDOS") || !strings.Contains(txt, " 2  -") ||
		!strings.Contains(txt, "VALOR FINAL DA VENDA R$ 4,50") {
		t.Fatalf("cupom da venda com devolução: %v\n%s", err, txt)
	}
	if dd, err := e.sales.Details(e.ctx, d.Sale.ID); err != nil || len(dd.Returns) != 2 || dd.Returns[1].Items[0].SaleItemID != camisaItem {
		t.Fatalf("detalhes com devoluções: %+v %v", dd.Returns, err)
	}

	info, err := rs.Info(e.ctx, d.Sale.ID)
	if err != nil || len(info.Returns) != 2 || info.Items[0].ReturnedQty != 3 || info.Items[0].PaidCents != 2700 {
		t.Fatalf("info: %+v %v", info, err)
	}
}
