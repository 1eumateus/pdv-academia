// Caminho: pdv/internal/service/report_service.go

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pdv/internal/models"
	"pdv/internal/repository"
)

type ReportService struct {
	Base
	reports repository.ReportRepo
	sales   repository.SaleRepo
	credits repository.CreditRepo
	stock   repository.StockRepo
	saved   repository.SavedReportRepo
}

// reportNames: nome de cada tipo de relatório gerado (título da impressão e da planilha).
var reportNames = map[string]string{
	models.ReportGeneral:    "Relatório de vendas",
	models.ReportByPayment:  "Vendas por forma de pagamento",
	models.ReportByCategory: "Vendas por categoria",
	models.ReportByOperator: "Vendas por operador",
}

// Generate monta o relatório (como Sales) e guarda uma cópia na lista de
// relatórios gerados, para reimprimir depois com os mesmos números. kind
// escolhe se a impressão traz tudo (geral) ou só uma seção.
func (s *ReportService) Generate(ctx context.Context, startDate, endDate string, userID int64, kind string) (models.SalesReport, error) {
	if _, ok := reportNames[kind]; !ok {
		return models.SalesReport{}, invalid("Tipo de relatório desconhecido: %s.", kind)
	}
	r, err := s.Sales(ctx, startDate, endDate, userID)
	if err != nil {
		return r, err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	var filter *int64
	if uid := salesScope(ctx, userID); uid > 0 {
		filter = &uid
	}
	r.ID, err = s.saved.Insert(ctx, s.DB, kind, r, filter, string(data), s.Now())
	return r, err
}

// ListSaved lista os últimos relatórios gerados. Quem não vê as vendas de
// todos vê só os que ele mesmo gerou.
func (s *ReportService) ListSaved(ctx context.Context) ([]models.SavedReport, error) {
	return s.saved.List(ctx, s.DB, salesScope(ctx, 0), 100)
}

// Saved devolve um relatório gerado, com a mesma regra de acesso de ListSaved.
func (s *ReportService) Saved(ctx context.Context, id int64) (models.SavedReport, models.SalesReport, error) {
	var r models.SalesReport
	v, data, err := s.saved.Get(ctx, s.DB, id)
	notFound := newErr(CodeNotFound, "Relatório %d não encontrado.", id)
	if err != nil {
		return v, r, fromRepo(err, notFound, "")
	}
	if owner := salesScope(ctx, 0); owner != 0 && (v.UserID == nil || *v.UserID != owner) {
		return v, r, notFound
	}
	err = json.Unmarshal([]byte(data), &r)
	return v, r, err
}

// SavedXLSX exporta um relatório gerado como planilha do Excel (.xlsx).
func (s *ReportService) SavedXLSX(ctx context.Context, id int64) ([]byte, error) {
	v, r, err := s.Saved(ctx, id)
	if err != nil {
		return nil, err
	}
	st, err := NewSettingsService(s.Base).Get(ctx)
	if err != nil {
		return nil, err
	}
	t := func(s string) xlsxCell { return xlsxCell{text: s} }
	bold := func(s string) xlsxCell { return xlsxCell{text: s, style: xsBold} }
	money := func(c int64) xlsxCell { return xlsxCell{num: fmt.Sprintf("%.2f", float64(c)/100), style: xsMoney} }
	n := func(i int64) xlsxCell { return xlsxCell{num: fmt.Sprint(i), style: xsInt} }
	head := func(a, b, c string) []xlsxCell {
		return []xlsxCell{{text: a, style: xsHeader}, {text: b, style: xsHeaderRight}, {text: c, style: xsHeaderRight}}
	}
	br := func(d string) string { return strings.Join(reverse(strings.Split(d, "-")), "/") }
	operator := "Todos"
	if v.FilterUserName != nil {
		operator = *v.FilterUserName
	}
	generated := brDateTime(v.CreatedAt)
	if v.UserName != nil {
		generated += " por " + *v.UserName
	}
	total := money(r.TotalCents)
	total.style = xsMoneyBold
	rows := [][]xlsxCell{
		{{text: reportNames[v.Kind], style: xsTitle}},
		{{text: fmt.Sprintf("%s · relatório nº %d", st.StoreName, v.ID), style: xsMuted}},
		{},
		{bold("Período"), t(br(r.StartDate) + " a " + br(r.EndDate)), {}},
		{bold("Operador"), t(operator), {}},
		{bold("Gerado em"), t(generated), {}},
		{},
		head("Resumo", "Qtd.", "Valor"),
		{bold("Total vendido"), n(r.SaleCount), total},
		{t("Descontos"), {}, money(r.DiscountCents)},
		{t("Canceladas"), n(r.CancelledCount), money(r.CancelledCents)},
		{t("Recebido a prazo"), {}, money(r.CreditReceivedCents)},
	}
	section := func(title, qty string, lines [][]xlsxCell) {
		if len(lines) > 0 {
			rows = append(rows, []xlsxCell{}, head(title, qty, "Total"))
			rows = append(rows, lines...)
		}
	}
	// Relatório de uma seção só traz o resumo e aquela tabela.
	show := func(kind string) bool { return v.Kind == models.ReportGeneral || v.Kind == kind }
	var lines [][]xlsxCell
	for _, x := range r.ByPaymentMethod {
		lines = append(lines, []xlsxCell{t(methodLabels[x.PaymentMethod]), n(x.Count), money(x.TotalCents)})
	}
	if show(models.ReportByPayment) {
		section("Forma de pagamento", "Vendas", lines)
	}
	lines = nil
	for _, x := range r.ByCategory {
		lines = append(lines, []xlsxCell{t(x.CategoryName), n(x.Qty), money(x.TotalCents)})
	}
	if show(models.ReportByCategory) {
		section("Categoria", "Itens", lines)
	}
	lines = nil
	for _, x := range r.ByOperator {
		lines = append(lines, []xlsxCell{t(x.UserName), n(x.Count), money(x.TotalCents)})
	}
	if show(models.ReportByOperator) {
		section("Operador", "Vendas", lines)
	}
	lines = nil
	for _, p := range r.Products {
		lines = append(lines, []xlsxCell{t(p.Name), n(p.Qty), money(p.TotalCents)})
	}
	if show("") {
		section("Produto", "Quantidade", lines)
	}
	lines = nil
	for _, p := range r.NotSold {
		lines = append(lines, []xlsxCell{t(p.Name), n(p.StockQty), {}})
	}
	if show("") && len(lines) > 0 {
		rows = append(rows, []xlsxCell{}, head("Sem vendas no período", "Estoque", ""))
		rows = append(rows, lines...)
	}
	return writeXLSX("Relatório", []float64{44, 14, 18}, []string{"A1:C1", "A2:C2", "B4:C4", "B5:C5", "B6:C6"}, rows, false)
}

func NewReportService(b Base) *ReportService { return &ReportService{Base: b} }

// Daily é o resumo de um dia local.
func (s *ReportService) Daily(ctx context.Context, date string) (models.SalesReport, error) {
	return s.Sales(ctx, date, date, 0)
}

// Sales consolida o período entre duas datas locais, inclusive. Os totais
// consideram só vendas concluídas; canceladas aparecem separadas. Quem não
// pode ver as vendas de todos recebe só as próprias (userID é ignorado).
func (s *ReportService) Sales(ctx context.Context, startDate, endDate string, userID int64) (models.SalesReport, error) {
	r := models.SalesReport{StartDate: strings.TrimSpace(startDate), EndDate: strings.TrimSpace(endDate)}
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return r, err
	}
	uid := salesScope(ctx, userID)
	if r.SaleCount, r.TotalCents, r.DiscountCents, r.CancelledCount, r.CancelledCents, err =
		s.reports.SalesTotals(ctx, s.DB, start, end, uid); err != nil {
		return r, err
	}
	if r.ByCategory, err = s.reports.TotalsByCategory(ctx, s.DB, start, end, uid); err != nil {
		return r, err
	}
	if r.ByOperator, err = s.reports.TotalsByOperator(ctx, s.DB, start, end, uid); err != nil {
		return r, err
	}
	if r.ByPaymentMethod, err = s.reports.TotalsByMethod(ctx, s.DB, start, end, uid); err != nil {
		return r, err
	}
	if r.Products, err = s.reports.ProductsSold(ctx, s.DB, start, end, uid); err != nil {
		return r, err
	}
	if r.NotSold, err = s.reports.ProductsNotSold(ctx, s.DB, start, end, uid); err != nil {
		return r, err
	}
	st, err := NewSettingsService(s.Base).Get(ctx)
	if err != nil {
		return r, err
	}
	r.LowStockDefault = st.LowStockDefault
	if r.Sales, err = s.sales.ListBetween(ctx, s.DB, start, end, uid); err != nil {
		return r, err
	}
	if r.CreditPayments, err = s.credits.ListBetween(ctx, s.DB, start, end, uid); err != nil {
		return r, err
	}
	if r.CreditReceivedCents, err = s.reports.CreditReceived(ctx, s.DB, start, end, uid); err != nil {
		return r, err
	}
	r.StockMovements, err = s.stock.List(ctx, s.DB, 0, start, end, uid)
	r.StockMovements = hideMovementCosts(ctx, r.StockMovements)
	return r, err
}
