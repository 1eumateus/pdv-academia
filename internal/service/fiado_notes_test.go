// Caminho: pdv/internal/service/fiado_notes_test.go

package service

import (
	"strings"
	"testing"
	"time"

	"pdv/internal/models"
)

func TestFiadoNotesPaidOldestFirst(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	note := func(id int64, daysAgo int, cents int64) models.FiadoNote {
		return models.FiadoNote{SaleID: id, CreatedAt: now.AddDate(0, 0, -daysAgo), AmountCents: cents}
	}
	parts := []models.FiadoNote{note(1, 40, 500), note(2, 20, 300), note(3, 5, 200)}
	// Comprou 1000 e deve 450: pagou 550 → nota 1 paga, nota 2 parcial (50), nota 3 aberta.
	got := fiadoNotes(parts, nil, 450, now)
	if got[0].Status != "paga" || got[1].Status != "parcial" || got[1].PaidCents != 50 || got[1].OpenCents != 250 ||
		got[2].Status != "aberta" || got[2].DaysOpen != 5 || got[0].DaysOpen != 0 {
		t.Fatalf("notas: %+v", got)
	}
	// Dívida maior que as compras: aparece o saldo anterior como nota em aberto.
	got = fiadoNotes(parts, nil, 1200, now)
	if len(got) != 4 || got[0].SaleID != 0 || got[0].OpenCents != 200 || got[1].Status != "aberta" {
		t.Fatalf("saldo anterior: %+v", got)
	}
}

func TestCustomerHistoryHasNotes(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Pão", 1000, 10)
	ana := e.customer("Ana", 100000)
	if _, err := e.sell(models.PaymentCredit, &ana.ID, item(p.ID, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell(models.PaymentCredit, &ana.ID, item(p.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.credit.PayCustomerCredit(e.ctx, models.PayCreditInput{CustomerID: ana.ID, AmountCents: 2500, PaymentMethod: models.PaymentCash}); err != nil {
		t.Fatal(err)
	}
	h, err := e.customers.History(e.ctx, ana.ID)
	if err != nil || len(h.Notes) != 2 || h.Notes[0].Status != "paga" || h.Notes[1].OpenCents != 500 || h.Notes[0].Description != "2x Pão" {
		t.Fatalf("histórico: %+v %v", h.Notes, err)
	}
}

func TestPayChosenNote(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Pão", 1000, 10)
	ana := e.customer("Ana", 100000)
	var sales []models.SaleDetails
	for _, q := range []int64{1, 2, 3} { // notas de R$ 10, R$ 20 e R$ 30
		d, err := e.sell(models.PaymentCredit, &ana.ID, item(p.ID, q))
		if err != nil {
			t.Fatal(err)
		}
		sales = append(sales, d)
	}
	notes := func() []models.FiadoNote {
		h, err := e.customers.History(e.ctx, ana.ID)
		if err != nil {
			t.Fatal(err)
		}
		return h.Notes
	}
	// Paga só a nota mais nova (R$ 30): ela fica paga e as antigas continuam em aberto.
	pay, err := e.credit.PayCustomerCredit(e.ctx, models.PayCreditInput{CustomerID: ana.ID, AmountCents: 3000,
		PaymentMethod: models.PaymentCash, SaleIDs: []int64{sales[2].Sale.ID}})
	if err != nil {
		t.Fatal(err)
	}
	n := notes()
	if n[2].Status != "paga" || n[0].Status != "aberta" || n[1].Status != "aberta" {
		t.Fatalf("depois de pagar a nota escolhida: %+v", n)
	}
	// Sem nota escolhida, o pagamento quita a mais antiga.
	if _, err := e.credit.PayCustomerCredit(e.ctx, models.PayCreditInput{CustomerID: ana.ID, AmountCents: 1000, PaymentMethod: models.PaymentPix}); err != nil {
		t.Fatal(err)
	}
	if n = notes(); n[0].Status != "paga" || n[1].OpenCents != 2000 {
		t.Fatalf("pagamento sem nota: %+v", n)
	}
	// Nota já paga não pode ser escolhida de novo.
	_, err = e.credit.PayCustomerCredit(e.ctx, models.PayCreditInput{CustomerID: ana.ID, AmountCents: 100,
		PaymentMethod: models.PaymentCash, SaleIDs: []int64{sales[2].Sale.ID}})
	wantCode(t, err, CodeInvalid)
	// Estornar o pagamento da nota escolhida reabre a dívida (R$ 30) nas notas.
	if _, err := e.credit.ReversePayment(e.ctx, pay.ID, "engano"); err != nil {
		t.Fatal(err)
	}
	if n = notes(); n[2].Status != "aberta" || n[1].OpenCents+n[2].OpenCents != 5000 {
		t.Fatalf("depois do estorno: %+v", n)
	}
}

func TestSaleForDelivery(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Gás 13kg", 11000, 5)
	ana := e.customer("Ana", 0)
	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)},
		PaymentMethod: models.PaymentPix, CustomerID: &ana.ID,
		Delivery: &models.Delivery{Address: "Rua das Flores, 10\nCentro", Phone: " (11) 99999-0000 ", Notes: "portão azul"}})
	if err != nil || d.Delivery == nil || d.Delivery.Phone != "(11) 99999-0000" {
		t.Fatalf("venda para entrega: %+v %v", d.Delivery, err)
	}
	rec, err := NewReceiptService(NewBase(e.db)).Build(e.ctx, models.ReceiptSale, d.Sale.ID)
	txt := receiptText(rec)
	for _, want := range []string{"PARA ENTREGA", "Para: Ana", "Rua das Flores, 10", "Centro", "portão azul", "VENDA JÁ PAGA"} {
		if err != nil || !strings.Contains(txt, want) {
			t.Fatalf("cupom sem %q: %v\n%s", want, err, txt)
		}
	}
	// Endereço vazio é recusado; venda comum continua sem entrega.
	_, err = e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)},
		PaymentMethod: models.PaymentPix, Delivery: &models.Delivery{Address: "  "}})
	wantCode(t, err, CodeInvalid)
	if c, _ := e.sell(models.PaymentCash, nil, item(p.ID, 1)); c.Delivery != nil {
		t.Fatal("venda comum com entrega")
	}
}
