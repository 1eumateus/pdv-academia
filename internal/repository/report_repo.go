// Caminho: pdv/internal/repository/report_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/models"
)

type ReportRepo struct{}

// SalesTotals soma as vendas concluídas e as canceladas no período [start, end).
func (ReportRepo) SalesTotals(ctx context.Context, q DBTX, start, end time.Time, userID int64) (count, total, discount, cancelledCount, cancelledTotal int64, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(status = 'concluida'), 0),
		   COALESCE(SUM(CASE WHEN status = 'concluida' THEN total_cents END), 0),
		   COALESCE(SUM(CASE WHEN status = 'concluida' THEN discount_cents END), 0),
		   COALESCE(SUM(status = 'cancelada'), 0),
		   COALESCE(SUM(CASE WHEN status = 'cancelada' THEN total_cents END), 0)
		 FROM sales WHERE created_at >= ? AND created_at < ? AND (? = 0 OR user_id = ?)`,
		FormatTime(start), FormatTime(end), userID, userID).Scan(&count, &total, &discount, &cancelledCount, &cancelledTotal)
	err = wrap("totalizar vendas", err)
	return
}

// TotalsByMethod soma vendas concluídas do período por forma de pagamento.
func (ReportRepo) TotalsByMethod(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]models.PaymentTotal, error) {
	const op = "totalizar vendas por forma de pagamento"
	rows, err := q.QueryContext(ctx,
		`SELECT p.method, COUNT(DISTINCT s.id), SUM(p.amount_cents) FROM sale_payments p JOIN sales s ON s.id = p.sale_id
		 WHERE s.status = 'concluida' AND s.created_at >= ? AND s.created_at < ? AND (? = 0 OR s.user_id = ?)
		 GROUP BY p.method ORDER BY p.method`, FormatTime(start), FormatTime(end), userID, userID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.PaymentTotal{}
	for rows.Next() {
		var t models.PaymentTotal
		if err := rows.Scan(&t.PaymentMethod, &t.Count, &t.TotalCents); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, t)
	}
	return out, wrap(op, rows.Err())
}

// TotalsByCategory soma os itens de vendas concluídas por categoria do produto.
// Itens são somados pelo subtotal do item (antes do desconto da venda).
func (ReportRepo) TotalsByCategory(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]models.CategoryTotal, error) {
	const op = "totalizar vendas por categoria"
	rows, err := q.QueryContext(ctx,
		`SELECT COALESCE(c.name, 'Sem categoria'), SUM(i.qty), SUM(i.subtotal_cents - i.discount_cents)
		 FROM sale_items i JOIN sales s ON s.id = i.sale_id JOIN products p ON p.id = i.product_id
		 LEFT JOIN categories c ON c.id = p.category_id
		 WHERE s.status = 'concluida' AND s.created_at >= ? AND s.created_at < ? AND (? = 0 OR s.user_id = ?)
		 GROUP BY p.category_id ORDER BY SUM(i.subtotal_cents - i.discount_cents) DESC`, FormatTime(start), FormatTime(end), userID, userID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.CategoryTotal{}
	for rows.Next() {
		var t models.CategoryTotal
		if err := rows.Scan(&t.CategoryName, &t.Qty, &t.TotalCents); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, t)
	}
	return out, wrap(op, rows.Err())
}

// TotalsByOperator soma vendas concluídas do período por operador.
func (ReportRepo) TotalsByOperator(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]models.OperatorTotal, error) {
	const op = "totalizar vendas por operador"
	rows, err := q.QueryContext(ctx,
		`SELECT COALESCE(u.name, '(sem operador)'), COUNT(*), SUM(s.total_cents)
		 FROM sales s LEFT JOIN users u ON u.id = s.user_id
		 WHERE s.status = 'concluida' AND s.created_at >= ? AND s.created_at < ? AND (? = 0 OR s.user_id = ?)
		 GROUP BY s.user_id ORDER BY SUM(s.total_cents) DESC`, FormatTime(start), FormatTime(end), userID, userID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.OperatorTotal{}
	for rows.Next() {
		var t models.OperatorTotal
		if err := rows.Scan(&t.UserName, &t.Count, &t.TotalCents); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, t)
	}
	return out, wrap(op, rows.Err())
}

// ProductsSold agrupa os itens de vendas concluídas do período.
func (ReportRepo) ProductsSold(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]models.ProductSold, error) {
	const op = "listar produtos vendidos"
	rows, err := q.QueryContext(ctx,
		`SELECT i.product_id, p.name, SUM(i.qty), SUM(i.subtotal_cents - i.discount_cents), p.stock_qty, p.min_stock
		 FROM sale_items i JOIN sales s ON s.id = i.sale_id JOIN products p ON p.id = i.product_id
		 WHERE s.status = 'concluida' AND s.created_at >= ? AND s.created_at < ? AND (? = 0 OR s.user_id = ?)
		 GROUP BY i.product_id ORDER BY SUM(i.qty) DESC, p.name COLLATE NOCASE`,
		FormatTime(start), FormatTime(end), userID, userID)
	return scanProductsSold(op, rows, err)
}

// ProductsNotSold lista os produtos ativos sem nenhuma venda concluída no período.
func (ReportRepo) ProductsNotSold(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]models.ProductSold, error) {
	const op = "listar produtos sem venda"
	rows, err := q.QueryContext(ctx,
		`SELECT p.id, p.name, 0, 0, p.stock_qty, p.min_stock FROM products p
		 WHERE p.active = 1 AND NOT EXISTS (
			SELECT 1 FROM sale_items i JOIN sales s ON s.id = i.sale_id
			WHERE i.product_id = p.id AND s.status = 'concluida' AND s.created_at >= ? AND s.created_at < ? AND (? = 0 OR s.user_id = ?))
		 ORDER BY p.stock_qty DESC, p.name COLLATE NOCASE`,
		FormatTime(start), FormatTime(end), userID, userID)
	return scanProductsSold(op, rows, err)
}

func scanProductsSold(op string, rows *sql.Rows, err error) ([]models.ProductSold, error) {
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.ProductSold{}
	for rows.Next() {
		var p models.ProductSold
		if err := rows.Scan(&p.ProductID, &p.Name, &p.Qty, &p.TotalCents, &p.StockQty, &p.MinStock); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, p)
	}
	return out, wrap(op, rows.Err())
}

// CreditReceived soma recebimentos menos estornos no período.
func (ReportRepo) CreditReceived(ctx context.Context, q DBTX, start, end time.Time, userID int64) (int64, error) {
	var v int64
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(CASE WHEN kind = 'recebimento' THEN amount_cents ELSE -amount_cents END), 0)
		 FROM credit_payments WHERE created_at >= ? AND created_at < ? AND (? = 0 OR user_id = ?)`,
		FormatTime(start), FormatTime(end), userID, userID).Scan(&v)
	return v, wrap("totalizar recebimentos", err)
}
