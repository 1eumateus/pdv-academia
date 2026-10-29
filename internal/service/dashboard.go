// Caminho: pdv/internal/service/dashboard.go

package service

import (
	"context"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

const dashboardDays = 30

// Dashboard reúne os números da tela inicial numa leitura só.
func (s *ReportService) Dashboard(ctx context.Context) (models.Dashboard, error) {
	var d models.Dashboard
	uid := salesScope(ctx, 0)
	d.AllSales = uid == 0
	now := s.Now().In(Location())
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	prevMonth := month.AddDate(0, -1, 0)
	// Mês passado até o mesmo ponto, sem passar do fim dele (dia 31 contra um mês de 30).
	prevEnd := prevMonth.Add(now.Sub(month))
	if prevEnd.After(month) {
		prevEnd = month
	}
	upToNow := now.Add(time.Second)
	totals := func(start, end time.Time) (models.PeriodTotals, error) {
		var t models.PeriodTotals
		var err error
		t.Count, t.TotalCents, _, _, _, err = s.reports.SalesTotals(ctx, s.DB, start, end, uid)
		if t.Count > 0 {
			t.AvgCents = (t.TotalCents + t.Count/2) / t.Count
		}
		return t, err
	}
	var err error
	if d.Today, err = totals(day, upToNow); err != nil {
		return d, err
	}
	if d.Yesterday, err = totals(day.AddDate(0, 0, -1), now.AddDate(0, 0, -1)); err != nil {
		return d, err
	}
	if d.Month, err = totals(month, upToNow); err != nil {
		return d, err
	}
	if d.PrevMonth, err = totals(prevMonth, prevEnd); err != nil {
		return d, err
	}

	// Gráfico: agrupa por dia local aqui (no banco as datas ficam em UTC).
	first := day.AddDate(0, 0, -(dashboardDays - 1))
	amounts, err := s.reports.SaleAmounts(ctx, s.DB, first, day.AddDate(0, 0, 1), uid)
	if err != nil {
		return d, err
	}
	idx := map[string]int{}
	for i := 0; i < dashboardDays; i++ {
		date := first.AddDate(0, 0, i).Format("2006-01-02")
		idx[date] = i
		d.Days = append(d.Days, models.DayTotal{Date: date})
	}
	for _, a := range amounts {
		if i, ok := idx[a.At.In(Location()).Format("2006-01-02")]; ok {
			d.Days[i].Count++
			d.Days[i].TotalCents += a.Cents
		}
	}

	top, err := s.reports.ProductsSold(ctx, s.DB, month, upToNow, uid)
	if err != nil {
		return d, err
	}
	d.TopProducts = top[:min(10, len(top))]
	st, err := NewSettingsService(s.Base).Get(ctx)
	if err != nil {
		return d, err
	}
	if d.StockAlerts, d.LowStockCount, d.ZeroStockCount, err = s.reports.StockAlerts(ctx, s.DB, st.LowStockDefault, 12); err != nil {
		return d, err
	}
	if d.CreditOpenCents, d.CreditCustomers, err = s.reports.CreditOpen(ctx, s.DB); err != nil {
		return d, err
	}
	if d.BillsDueToday, d.BillsOverdue, err = (repository.BillRepo{}).DueCounts(ctx, s.DB, localDate(now)); err != nil {
		return d, err
	}
	if d.OpenQuotes, d.OpenQuotesCents, err = s.reports.OpenQuotes(ctx, s.DB, localDate(now)); err != nil {
		return d, err
	}
	cash, err := NewCashService(s.Base).Current(ctx)
	if err != nil {
		return d, err
	}
	if cash != nil {
		d.Cash, d.CashExpected = &cash.Session, cash.ExpectedCashCents
	}
	return d, nil
}
