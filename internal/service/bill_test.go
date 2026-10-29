package service

import (
	"encoding/base64"
	"testing"
	"time"

	"pdv/internal/models"
)

func TestBills(t *testing.T) {
	e := newEnv(t).withCash()
	bills := NewBillService(NewBase(e.db))
	in := models.BillInput{Description: "Aluguel", Category: "Aluguel", Payee: "Imobiliária", AmountCents: 150000, DueDate: "2026-01-31", Recurring: true}

	op := e.as(e.user("op", models.RoleOperator))
	if _, err := bills.Save(op, in); codeOf(err) != CodeAuthRequired {
		t.Fatalf("operador sem a permissão: esperava AUTORIZACAO_NECESSARIA, veio %v", err)
	}
	b, err := bills.Save(e.ctx, in)
	if err != nil || b.Status != "aberta" {
		t.Fatalf("cadastrar: %v %+v", err, b)
	}

	// Anexo: aceita PDF, recusa outro tipo.
	pdf := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 teste"))
	if _, err := bills.AddFile(e.ctx, b.ID, "boleto", "boleto.exe", "application/x-msdownload", pdf); codeOf(err) != CodeInvalid {
		t.Fatalf("arquivo .exe: esperava OPERACAO_INVALIDA, veio %v", err)
	}
	if files, err := bills.AddFile(e.ctx, b.ID, "boleto", "boleto.pdf", "application/pdf", pdf); err != nil || len(files) != 1 {
		t.Fatalf("anexar boleto: %v %+v", err, files)
	}

	// Pagar em dinheiro do caixa: vira sangria e gera a de fevereiro (31/01 -> 28/02).
	before, _ := e.cash.Current(e.ctx)
	b, err = bills.Pay(e.ctx, models.PayBillInput{BillID: b.ID, PaidDate: "2026-01-30", PaidCents: 5000, Method: "dinheiro", FromCash: true})
	if err != nil || b.Status != "paga" || !b.FromCash || b.BoletoFiles != 1 {
		t.Fatalf("pagar: %v %+v", err, b)
	}
	after, _ := e.cash.Current(e.ctx)
	if after.ExpectedCashCents != before.ExpectedCashCents-5000 {
		t.Fatalf("o caixa deveria ter saído R$ 50: %d -> %d", before.ExpectedCashCents, after.ExpectedCashCents)
	}
	list, _ := bills.List(e.ctx)
	if len(list) != 2 || list[0].Status != "aberta" || list[0].DueDate != "2026-02-28" {
		t.Fatalf("a conta mensal deveria gerar a de 28/02: %+v", list)
	}
	if _, err := bills.Pay(e.ctx, models.PayBillInput{BillID: b.ID, PaidDate: "2026-01-30", PaidCents: 5000, Method: "pix"}); codeOf(err) != CodeStateConflict {
		t.Fatalf("pagar de novo: esperava CONFLITO_DE_ESTADO, veio %v", err)
	}
	if sum, err := bills.Summary(e.ctx, "2026-01"); err != nil || len(sum) != 1 || sum[0].TotalCents != 5000 {
		t.Fatalf("resumo do mês: %v %+v", err, sum)
	}
	if got := addMonthClamped(time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)); got.Format(time.DateOnly) != "2026-04-30" {
		t.Fatalf("31/03 + 1 mês: %v", got)
	}
}
