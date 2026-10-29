// Caminho: pdv/internal/repository/dashboard_repo.go

package repository

import (
	"context"
	"time"

	"pdv/internal/models"
)

// SaleAmount é uma venda concluída (hora e valor), para agrupar por dia local no serviço.
type SaleAmount struct {
	At    time.Time
	Cents int64
}

// SaleAmounts lista as vendas concluídas em [start, end) (userID 0 = todas).
func (ReportRepo) SaleAmounts(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]SaleAmount, error) {
	const op = "listar vendas por dia"
	rows, err := q.QueryContext(ctx,
		`SELECT created_at, total_cents FROM sales
		 WHERE status = 'concluida' AND created_at >= ? AND created_at < ? AND (? = 0 OR user_id = ?)`,
		FormatTime(start), FormatTime(end), userID, userID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	var out []SaleAmount
	for rows.Next() {
		var at string
		var v SaleAmount
		if err := rows.Scan(&at, &v.Cents); err != nil {
			return nil, wrap(op, err)
		}
		if v.At, err = parseTime(at); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

// StockAlerts lista produtos ativos zerados ou no estoque mínimo (zerados
// primeiro) e conta quantos há de cada.
// lowDefault vale para produto sem estoque mínimo próprio (0 = só os zerados).
func (ReportRepo) StockAlerts(ctx context.Context, q DBTX, lowDefault int64, limit int) (list []models.StockAlert, low, zero int64, err error) {
	const op = "listar alertas de estoque"
	const where = `active = 1 AND track_stock = 1 AND stock_qty <= MAX(0, CASE WHEN min_stock > 0 THEN min_stock ELSE ? END)`
	if err = q.QueryRowContext(ctx, `SELECT COALESCE(SUM(stock_qty > 0), 0), COALESCE(SUM(stock_qty <= 0), 0) FROM products WHERE `+where, lowDefault).
		Scan(&low, &zero); err != nil {
		return nil, 0, 0, wrap(op, err)
	}
	rows, err := q.QueryContext(ctx, `SELECT id, name, stock_qty, min_stock FROM products WHERE `+where+`
		ORDER BY stock_qty > 0, stock_qty, name COLLATE NOCASE LIMIT ?`, lowDefault, limit)
	if err != nil {
		return nil, 0, 0, wrap(op, err)
	}
	defer rows.Close()
	list = []models.StockAlert{}
	for rows.Next() {
		var a models.StockAlert
		if err = rows.Scan(&a.ProductID, &a.Name, &a.StockQty, &a.MinStock); err != nil {
			return nil, 0, 0, wrap(op, err)
		}
		list = append(list, a)
	}
	return list, low, zero, wrap(op, rows.Err())
}

// CreditOpen soma o fiado em aberto e conta os clientes devendo.
func (ReportRepo) CreditOpen(ctx context.Context, q DBTX) (cents, customers int64, err error) {
	err = q.QueryRowContext(ctx, `SELECT COALESCE(SUM(credit_balance_cents), 0), COUNT(*) FROM customers WHERE credit_balance_cents > 0`).
		Scan(&cents, &customers)
	return cents, customers, wrap("totalizar fiado em aberto", err)
}

// OpenQuotes conta os orçamentos abertos/aprovados ainda na validade (today = AAAA-MM-DD local).
func (ReportRepo) OpenQuotes(ctx context.Context, q DBTX, today string) (count, cents int64, err error) {
	err = q.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(total_cents), 0) FROM quotes
		WHERE status IN ('aberto', 'aprovado') AND valid_until >= ?`, today).Scan(&count, &cents)
	return count, cents, wrap("contar orçamentos abertos", err)
}
