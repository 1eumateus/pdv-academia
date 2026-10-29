// Caminho: pdv/internal/service/cash_report.go

package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// cashReportMethods: ordem das formas no quadro de totais (cartão antigo só se houver).
var cashReportMethods = []string{models.PaymentCash, models.PaymentDebit, models.PaymentCreditCard, models.PaymentPix, models.PaymentCard}

// CashReport é o "Controle do caixa": cada entrada e saída dos caixas no
// período, com um histórico curto e os totais por forma. Sem a permissão de
// ver todos, só os lançamentos do próprio usuário.
// methods vazio = todas as formas.
func (s *ReportService) CashReport(ctx context.Context, startDate, endDate string, userID int64, methods []string) (models.CashReport, error) {
	r := models.CashReport{StartDate: strings.TrimSpace(startDate), EndDate: strings.TrimSpace(endDate),
		Rows: []models.CashReportRow{}, Totals: []models.CashReportTotal{}}
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return r, err
	}
	uid := salesScope(ctx, userID)
	if uid > 0 {
		u, err := repository.UserRepo{}.GetByID(ctx, s.DB, uid)
		if err != nil {
			return r, err
		}
		r.UserName = &u.Name
	}
	raw, err := repository.CashRepo{}.ReportRows(ctx, s.DB, start, end, uid)
	if err != nil {
		return r, err
	}
	totals := map[string]*models.CashReportTotal{}
	for _, m := range raw {
		if len(methods) > 0 && !slices.Contains(methods, m.Method) {
			continue
		}
		row := cashReportRow(m)
		r.Rows = append(r.Rows, row)
		t := totals[m.Method]
		if t == nil {
			t = &models.CashReportTotal{Method: m.Method}
			totals[m.Method] = t
		}
		if row.In {
			t.InCents += row.AmountCents
			r.InCents += row.AmountCents
		} else {
			t.OutCents -= row.AmountCents
			r.OutCents -= row.AmountCents
		}
	}
	for _, m := range cashReportMethods {
		if len(methods) > 0 && !slices.Contains(methods, m) {
			continue
		}
		if t := totals[m]; t != nil || m != models.PaymentCard {
			if t == nil {
				t = &models.CashReportTotal{Method: m}
			}
			r.Totals = append(r.Totals, *t)
		}
	}
	return r, nil
}

// cashReportRow monta o histórico curto: "Mensalidade · Suely (Mensal)",
// "Venda · Mateus" (itens no detalhe), "Retirada · motivo" etc.
func cashReportRow(m repository.CashReportRaw) models.CashReportRow {
	row := models.CashReportRow{Receipt: "-", At: m.CreatedAt, AmountCents: m.AmountCents, Method: m.Method, In: m.AmountCents > 0}
	with := func(label, extra string) string {
		if extra = strings.TrimSpace(extra); extra == "" {
			return label
		}
		return label + " · " + extra
	}
	if m.SaleID > 0 {
		row.Receipt = fmt.Sprint(m.SaleID)
	}
	switch {
	case m.Type == models.CashSale && m.PlanName != "":
		row.History = with("Mensalidade", m.CustomerName+" ("+m.PlanName+")")
	case m.Type == models.CashSale:
		row.History = with("Venda", m.CustomerName)
		row.Detail = m.Items
	case m.Type == models.CashSupply:
		row.History = with("Suprimento", m.Description)
	case m.Type == models.CashWithdrawal:
		row.History = with("Retirada", m.Description)
	default: // recebimento a prazo, estornos e devoluções já têm descrição
		row.History = strings.TrimSpace(m.Description)
	}
	return row
}

// cashMethodShort: forma abreviada como no modelo (DIN, DEB, CRE, PIX, CAR).
func cashMethodShort(m string) string {
	switch m {
	case models.PaymentCash:
		return "DIN"
	case models.PaymentDebit:
		return "DEB"
	case models.PaymentCreditCard:
		return "CRE"
	case models.PaymentPix:
		return "PIX"
	}
	return "CAR"
}

// PrintCashReport imprime o controle do caixa na térmica configurada.
func (s *ReceiptService) PrintCashReport(ctx context.Context, startDate, endDate string, userID int64, methods []string) error {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return err
	}
	if st.PaperWidth == "a4" || st.PrinterName == "" {
		return invalid("Nenhuma impressora térmica configurada. Escolha a impressora em Configurações.")
	}
	r, err := NewReportService(s.Base).CashReport(ctx, startDate, endDate, userID, methods)
	if err != nil {
		return err
	}
	br := func(d string) string { return strings.Join(reverse(strings.Split(d, "-")), "/") }
	var b builder
	const title = "CONTROLE DO CAIXA"
	b.header(st, title)
	period := br(r.StartDate)
	if r.EndDate != r.StartDate {
		period += " a " + br(r.EndDate)
	}
	b.pair("Entradas e saídas", period, true)
	if r.UserName != nil {
		b.text("Operador: " + *r.UserName)
	} else {
		b.text("Todos os operadores")
	}
	b.sep()
	for _, row := range r.Rows {
		b.text(row.At.Local().Format("02/01 15:04") + " " + row.History)
		if row.Detail != "" {
			b.text("  " + row.Detail)
		}
		es := "E"
		if !row.In {
			es = "S"
		}
		b.pair("", fmt.Sprintf("%s %s %s", brl(row.AmountCents), cashMethodShort(row.Method), es), false)
	}
	if len(r.Rows) == 0 {
		b.text("Nenhuma entrada ou saída no período.")
	}
	b.sep()
	b.text("Forma: entrada / saída = saldo")
	for _, t := range r.Totals {
		b.text(methodLabels[t.Method] + ":")
		b.pair(fmt.Sprintf("  %s / %s", brl(t.InCents), brl(t.OutCents)), brl(t.InCents-t.OutCents), false)
	}
	b.sep()
	b.pair("Entradas", brl(r.InCents), false)
	b.pair("Saídas", brl(r.OutCents), false)
	b.pair("SALDO", brl(r.InCents-r.OutCents), true)
	b.footer(st)
	return s.send(st, models.Receipt{Title: title, Logo: st.Logo, Lines: b.lines}, false)
}
