// Caminho: pdv/internal/service/analytics_service.go

package service

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
	"pdv/internal/repository"
)

// AnalyticsService: lucro, curva ABC, produtos parados, fiado por idade,
// compras por fornecedor, estoque valorizado, descontos e exportação.
type AnalyticsService struct {
	Base
	repo repository.AnalyticsRepo
}

func NewAnalyticsService(b Base) *AnalyticsService { return &AnalyticsService{Base: b} }

// userHas: consulta de leitura (não consome autorização de gerente).
func userHas(ctx context.Context, perm string) bool {
	u, ok := actor.From(ctx)
	return ok && (u.Role == models.RoleAdmin || slices.Contains(u.Perms, perm))
}

// productTotals soma, por produto, a receita líquida (o que o cliente pagou,
// menos as devoluções) e o custo do que não voltou.
func (s *AnalyticsService) productTotals(ctx context.Context, startDate, endDate string) (map[int64]*models.ProfitRow, error) {
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	lines, err := s.repo.SoldLines(ctx, s.DB, start, end, salesScope(ctx, 0))
	if err != nil {
		return nil, err
	}
	out := map[int64]*models.ProfitRow{}
	for i := 0; i < len(lines); {
		// Itens da mesma venda juntos: o desconto geral é rateado entre eles.
		j := i
		var items []models.SaleItem
		for ; j < len(lines) && lines[j].SaleID == lines[i].SaleID; j++ {
			items = append(items, lines[j].Item)
		}
		paid := paidByItem(models.Sale{DiscountCents: lines[i].SaleDiscountCents}, items)
		for _, l := range lines[i:j] {
			r := out[l.Item.ProductID]
			if r == nil {
				r = &models.ProfitRow{ProductID: l.Item.ProductID, Name: l.Item.ProductNameSnapshot, CategoryName: l.CategoryName}
				out[l.Item.ProductID] = r
			}
			qty := l.Item.Qty - l.ReturnedQty
			r.Qty += qty
			r.RevenueCents += paid[l.Item.ID] - l.ReturnedAmountCents
			r.CostCents += l.UnitCostCents * qty
			r.Estimated = r.Estimated || (l.CostEstimated && qty > 0)
			r.NoCost = r.NoCost || (l.UnitCostCents == 0 && qty > 0)
		}
		i = j
	}
	return out, nil
}

// Profit: lucro bruto por produto e categoria. Só para quem vê custo.
func (s *AnalyticsService) Profit(ctx context.Context, startDate, endDate string) (models.ProfitReport, error) {
	var rep models.ProfitReport
	if !userHas(ctx, PermCost) {
		return rep, newErr(CodeForbidden, "Para ver o lucro é preciso a permissão \"%s\".", permLabel(PermCost))
	}
	totals, err := s.productTotals(ctx, startDate, endDate)
	if err != nil {
		return rep, err
	}
	cats := map[string]*models.ProfitRow{}
	rep.Products = []models.ProfitRow{}
	for _, r := range totals {
		if r.Qty == 0 && r.RevenueCents == 0 {
			continue // tudo devolvido
		}
		r.ProfitCents = r.RevenueCents - r.CostCents
		rep.Products = append(rep.Products, *r)
		c := cats[r.CategoryName]
		if c == nil {
			c = &models.ProfitRow{Name: r.CategoryName, CategoryName: r.CategoryName}
			cats[r.CategoryName] = c
		}
		c.Qty += r.Qty
		c.RevenueCents += r.RevenueCents
		c.CostCents += r.CostCents
		c.ProfitCents += r.ProfitCents
		c.Estimated = c.Estimated || r.Estimated
		c.NoCost = c.NoCost || r.NoCost
		rep.RevenueCents += r.RevenueCents
		rep.CostCents += r.CostCents
	}
	rep.ProfitCents = rep.RevenueCents - rep.CostCents
	rep.Categories = []models.ProfitRow{}
	for _, c := range cats {
		rep.Categories = append(rep.Categories, *c)
	}
	byProfit := func(l []models.ProfitRow) {
		sort.Slice(l, func(a, b int) bool {
			if l[a].ProfitCents != l[b].ProfitCents {
				return l[a].ProfitCents > l[b].ProfitCents
			}
			return l[a].Name < l[b].Name
		})
	}
	byProfit(rep.Products)
	byProfit(rep.Categories)
	return rep, nil
}

// ABC classifica os produtos pela receita: A até 80% acumulado, B até 95%, C o resto.
func (s *AnalyticsService) ABC(ctx context.Context, startDate, endDate string) (models.ABCReport, error) {
	rep := models.ABCReport{Rows: []models.ABCRow{}}
	totals, err := s.productTotals(ctx, startDate, endDate)
	if err != nil {
		return rep, err
	}
	for _, r := range totals {
		if r.RevenueCents > 0 {
			rep.Rows = append(rep.Rows, models.ABCRow{ProductID: r.ProductID, Name: r.Name, Qty: r.Qty, RevenueCents: r.RevenueCents})
			rep.TotalCents += r.RevenueCents
		}
	}
	classifyABC(rep.Rows, rep.TotalCents)
	return rep, nil
}

func classifyABC(rows []models.ABCRow, total int64) {
	sort.Slice(rows, func(a, b int) bool {
		if rows[a].RevenueCents != rows[b].RevenueCents {
			return rows[a].RevenueCents > rows[b].RevenueCents
		}
		return rows[a].Name < rows[b].Name
	})
	var cum int64
	for i := range rows {
		before := float64(cum) * 100 / float64(total)
		cum += rows[i].RevenueCents
		rows[i].SharePct = float64(rows[i].RevenueCents) * 100 / float64(total)
		rows[i].CumulativePct = float64(cum) * 100 / float64(total)
		// A classe vem do acumulado antes do item: o 1º item é sempre A.
		switch {
		case before < 80:
			rows[i].Class = "A"
		case before < 95:
			rows[i].Class = "B"
		default:
			rows[i].Class = "C"
		}
	}
}

// Idle: produtos com estoque sem venda há pelo menos `days` dias (ou nunca vendidos).
func (s *AnalyticsService) Idle(ctx context.Context, days int64) ([]models.IdleProduct, error) {
	if days < 1 || days > 3650 {
		return nil, invalid("Informe de 1 a 3650 dias.")
	}
	list, err := s.repo.IdleCandidates(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	now, cost := s.Now(), userHas(ctx, PermCost)
	out := []models.IdleProduct{}
	for _, p := range list {
		p.IdleDays = -1
		if p.LastSaleAt != nil {
			p.IdleDays = int64(now.Sub(*p.LastSaleAt) / (24 * time.Hour))
			if p.IdleDays < days {
				continue
			}
		}
		if !cost {
			p.CostCents = 0
		}
		out = append(out, p)
	}
	// Nunca vendidos primeiro; depois os parados há mais tempo.
	sort.Slice(out, func(a, b int) bool {
		da, db := out[a].IdleDays, out[b].IdleDays
		if da == -1 {
			da = 1 << 30
		}
		if db == -1 {
			db = 1 << 30
		}
		if da != db {
			return da > db
		}
		return out[a].Name < out[b].Name
	})
	return out, nil
}

// Aging separa a dívida de cada cliente pela idade das compras no fiado. O que
// já foi pago abate as compras mais antigas, então a dívida é das mais novas.
func (s *AnalyticsService) Aging(ctx context.Context) ([]models.AgingCustomer, error) {
	customers, parts, err := s.repo.DebtorsAndFiado(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	return agingBuckets(customers, parts, s.Now()), nil
}

func agingBuckets(customers []models.Customer, parts []repository.FiadoPart, now time.Time) []models.AgingCustomer {
	byCustomer := map[int64][]repository.FiadoPart{}
	for _, p := range parts {
		byCustomer[p.CustomerID] = append(byCustomer[p.CustomerID], p)
	}
	out := make([]models.AgingCustomer, 0, len(customers))
	for _, c := range customers {
		a := models.AgingCustomer{CustomerID: c.ID, Name: c.Name, Phone: c.Phone, BalanceCents: c.CreditBalanceCents}
		left := c.CreditBalanceCents
		for _, p := range byCustomer[c.ID] { // da mais nova para a mais antiga
			if left <= 0 {
				break
			}
			take := min(p.AmountCents, left)
			left -= take
			age := int64(now.Sub(p.At) / (24 * time.Hour))
			a.OldestDays = age
			switch {
			case age <= 30:
				a.Upto30Cents += take
			case age <= 60:
				a.Upto60Cents += take
			case age <= 90:
				a.Upto90Cents += take
			default:
				a.Over90Cents += take
			}
		}
		// Saldo sem compra de origem (dados antigos): conta como o mais atrasado.
		a.Over90Cents += left
		out = append(out, a)
	}
	return out
}

// Suppliers: compras por fornecedor no período (permissão de compras).
func (s *AnalyticsService) Suppliers(ctx context.Context, startDate, endDate string) ([]models.SupplierPurchases, error) {
	if !userHas(ctx, PermPurchases) {
		return nil, newErr(CodeForbidden, "Para ver as compras é preciso a permissão \"%s\".", permLabel(PermPurchases))
	}
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	return s.repo.PurchasesBySupplier(ctx, s.DB, start, end)
}

// StockValue: estoque a preço de venda e, para quem vê custo, a custo.
func (s *AnalyticsService) StockValue(ctx context.Context) ([]models.StockValueRow, error) {
	rows, err := s.repo.StockValue(ctx, s.DB)
	if err != nil || userHas(ctx, PermCost) {
		return rows, err
	}
	for i := range rows {
		rows[i].CostCents, rows[i].NoCost = 0, 0
	}
	return rows, nil
}

// Discounts: descontos dados por operador no período (escopo de vendas do usuário).
func (s *AnalyticsService) Discounts(ctx context.Context, startDate, endDate string) ([]models.DiscountByOperator, error) {
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	return s.repo.DiscountsByOperator(ctx, s.DB, start, end, salesScope(ctx, 0))
}

const maxExportRows = 100000

// ExportXLSX monta a planilha de uma listagem da tela. money vem em centavos e
// pct como fração (0.25 = 25%); tudo é conferido antes de virar número.
func (s *AnalyticsService) ExportXLSX(in models.ExportTable) ([]byte, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, invalid("A exportação precisa de um título.")
	}
	n := len(in.Columns)
	if n == 0 || n > 26 {
		return nil, invalid("A planilha deve ter de 1 a 26 colunas.")
	}
	if len(in.Rows) > maxExportRows {
		return nil, invalid("A planilha pode ter no máximo %d linhas.", maxExportRows)
	}
	widths := make([]float64, n)
	head := make([]xlsxCell, n)
	for i, c := range in.Columns {
		widths[i] = 16
		if c.Kind == "text" || c.Kind == "" {
			widths[i] = 32
		}
		style := xsHeaderRight
		if c.Kind == "text" || c.Kind == "" {
			style = xsHeader
		}
		head[i] = xlsxCell{text: c.Label, style: style}
	}
	rows := [][]xlsxCell{{{text: title, style: xsTitle}}}
	if in.Subtitle != "" {
		rows = append(rows, []xlsxCell{{text: in.Subtitle, style: xsMuted}})
	}
	rows = append(rows, []xlsxCell{}, head)
	for r, row := range in.Rows {
		if len(row) != n {
			return nil, invalid("A linha %d tem %d colunas; esperado %d.", r+1, len(row), n)
		}
		cells := make([]xlsxCell, n)
		for i, v := range row {
			cell, err := exportCell(in.Columns[i].Kind, strings.TrimSpace(v))
			if err != nil {
				return nil, invalid("Valor inválido na linha %d, coluna %q.", r+1, in.Columns[i].Label)
			}
			cells[i] = cell
		}
		rows = append(rows, cells)
	}
	last := fmt.Sprintf("%c1", 'A'+n-1)
	return writeXLSX(sheetName(title), widths, []string{"A1:" + last}, rows, false)
}

func exportCell(kind, v string) (xlsxCell, error) {
	if v == "" {
		return xlsxCell{}, nil
	}
	switch kind {
	case "money":
		c, err := strconv.ParseInt(v, 10, 64)
		return xlsxCell{num: fmt.Sprintf("%.2f", float64(c)/100), style: xsMoney}, err
	case "int":
		c, err := strconv.ParseInt(v, 10, 64)
		return xlsxCell{num: strconv.FormatInt(c, 10), style: xsInt}, err
	case "pct":
		f, err := strconv.ParseFloat(v, 64)
		return xlsxCell{num: strconv.FormatFloat(f, 'f', -1, 64), style: xsPct}, err
	}
	return xlsxCell{text: v}, nil
}

// sheetName: o Excel recusa nomes de aba com mais de 31 caracteres ou com : \ / ? * [ ].
func sheetName(title string) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`:\/?*[]`, r) {
			return ' '
		}
		return r
	}, title)
	if r := []rune(name); len(r) > 31 {
		name = string(r[:31])
	}
	return name
}
