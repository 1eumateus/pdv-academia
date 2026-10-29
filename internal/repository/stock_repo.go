// Caminho: pdv/internal/repository/stock_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
)

type StockRepo struct{}

// NewMovement descreve uma movimentação. Use sempre junto com
// ProductRepo.ApplyStockDelta na mesma transação.
type NewMovement struct {
	ProductID     int64
	QtyChange     int64
	Reason        string
	ReferenceType *string
	ReferenceID   *int64
	Notes         *string
}

func (StockRepo) InsertMovement(ctx context.Context, q DBTX, m NewMovement, now time.Time) (int64, error) {
	const op = "registrar movimentação de estoque"
	res, err := q.ExecContext(ctx,
		`INSERT INTO stock_movements (product_id, qty_change, reason, reference_type, reference_id, notes, created_at, user_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ProductID, m.QtyChange, m.Reason, m.ReferenceType, m.ReferenceID, m.Notes, FormatTime(now), actor.ID(ctx))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// List filtra por produto (0 = todos) e período [start, end).
func (StockRepo) List(ctx context.Context, q DBTX, productID int64, start, end time.Time, userID int64) ([]models.StockMovement, error) {
	const op = "listar movimentações de estoque"
	rows, err := q.QueryContext(ctx,
		`SELECT m.id, m.product_id, p.name, m.qty_change, m.reason, m.reference_type, m.reference_id, m.notes, m.created_at, u.name,
		        CASE m.reference_type
		          WHEN 'sale' THEN (SELECT SUM(subtotal_cents) / SUM(qty) FROM sale_items WHERE sale_id = m.reference_id AND product_id = m.product_id)
		          WHEN 'return' THEN (SELECT SUM(amount_cents) / SUM(qty) FROM return_items WHERE return_id = m.reference_id AND product_id = m.product_id)
		          WHEN 'purchase' THEN (SELECT SUM(total_cents) / SUM(qty) FROM purchase_items WHERE purchase_id = m.reference_id AND product_id = m.product_id)
		          ELSE p.cost_cents END,
		        COALESCE(m.reference_type, '') NOT IN ('sale', 'return')
		 FROM stock_movements m JOIN products p ON p.id = m.product_id LEFT JOIN users u ON u.id = m.user_id
		 WHERE (? = 0 OR m.product_id = ?) AND m.created_at >= ? AND m.created_at < ? AND (? = 0 OR m.user_id = ?)
		 ORDER BY m.created_at DESC, m.id DESC`,
		productID, productID, FormatTime(start), FormatTime(end), userID, userID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.StockMovement{}
	for rows.Next() {
		var (
			m             models.StockMovement
			refType, note sql.NullString
			user          sql.NullString
			refID, unit   sql.NullInt64
			created       string
		)
		if err := rows.Scan(&m.ID, &m.ProductID, &m.ProductName, &m.QtyChange, &m.Reason, &refType, &refID, &note, &created, &user, &unit, &m.IsCost); err != nil {
			return nil, wrap(op, err)
		}
		m.ReferenceType, m.ReferenceID, m.Notes = nullStr(refType), nullInt(refID), nullStr(note)
		m.UserName, m.UnitCents = nullStr(user), nullInt(unit)
		if m.CreatedAt, err = parseTime(created); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, m)
	}
	return out, wrap(op, rows.Err())
}

// Audit devolve os produtos cujo saldo gravado difere da soma das movimentações.
func (StockRepo) Audit(ctx context.Context, q DBTX) ([]models.StockAuditEntry, error) {
	const op = "auditar estoque"
	rows, err := q.QueryContext(ctx,
		`SELECT p.id, p.name, p.stock_qty, COALESCE(SUM(m.qty_change), 0) AS calc
		 FROM products p LEFT JOIN stock_movements m ON m.product_id = p.id
		 GROUP BY p.id HAVING p.stock_qty <> calc ORDER BY p.name COLLATE NOCASE`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.StockAuditEntry{}
	for rows.Next() {
		var e models.StockAuditEntry
		if err := rows.Scan(&e.ProductID, &e.ProductName, &e.StoredQty, &e.CalculatedQty); err != nil {
			return nil, wrap(op, err)
		}
		e.DifferenceQty = e.StoredQty - e.CalculatedQty
		out = append(out, e)
	}
	return out, wrap(op, rows.Err())
}

// CalculatedQty soma as movimentações de um produto.
func (StockRepo) CalculatedQty(ctx context.Context, q DBTX, productID int64) (int64, error) {
	var n int64
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty_change), 0) FROM stock_movements WHERE product_id = ?`, productID).Scan(&n)
	return n, wrap("calcular saldo do produto", err)
}
