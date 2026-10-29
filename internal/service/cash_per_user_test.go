package service

import (
	"testing"

	"pdv/internal/models"
)

func TestCashPerUser(t *testing.T) {
	e := newEnv(t)
	settings := NewSettingsService(NewBase(e.db))
	st, err := settings.Get(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	st.CashPerUser = true
	if _, err := settings.Save(e.ctx, st); err != nil {
		t.Fatal(err)
	}
	p := e.product("Pão", 100, 10)
	ana, beto := e.as(e.user("ana", models.RoleOperator)), e.as(e.user("beto", models.RoleOperator))

	anaCash, err := e.cash.Open(ana, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.cash.Open(ana, 0); codeOf(err) != CodeCashAlreadyOpen {
		t.Fatalf("segundo caixa da mesma pessoa: esperava CAIXA_JA_ABERTO, veio %v", err)
	}
	// O caixa da Ana não serve para o Beto.
	in := models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, PaymentMethod: "dinheiro"}
	if _, err := e.sales.CreateSale(beto, in); codeOf(err) != CodeCashNotOpen {
		t.Fatalf("Beto sem caixa: esperava CAIXA_NAO_ABERTO, veio %v", err)
	}
	betoCash, err := e.cash.Open(beto, 0)
	if err != nil {
		t.Fatal(err)
	}
	sale, err := e.sales.CreateSale(beto, in)
	if err != nil || sale.Sale.CashSessionID == nil || *sale.Sale.CashSessionID != betoCash.Session.ID {
		t.Fatalf("a venda do Beto deveria cair no caixa dele: %v %+v", err, sale.Sale.CashSessionID)
	}

	// Trocar o modo com caixa aberto é recusado.
	st.CashPerUser = false
	if _, err := settings.Save(e.ctx, st); codeOf(err) != CodeStateConflict {
		t.Fatalf("trocar o modo com caixa aberto: esperava CONFLITO_DE_ESTADO, veio %v", err)
	}

	// Fechar o caixa de outro exige a permissão; o administrador tem.
	if _, err := e.cash.CloseOther(beto, anaCash.Session.ID, 1000, ""); codeOf(err) != CodeAuthRequired {
		t.Fatalf("Beto fechando o caixa da Ana: esperava AUTORIZACAO_NECESSARIA, veio %v", err)
	}
	if _, err := e.cash.CloseOther(e.ctx, anaCash.Session.ID, 1000, "foi embora sem fechar"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.cash.Close(beto, 100, ""); err != nil {
		t.Fatal(err)
	}
	if totals, err := e.cash.DayTotals(e.ctx); err != nil || len(totals) != 1 || totals[0].TotalCents != 100 {
		t.Fatalf("total do dia: %v %+v", err, totals)
	}
	if _, err := settings.Save(e.ctx, st); err != nil {
		t.Fatalf("com tudo fechado, deveria trocar o modo: %v", err)
	}
}
