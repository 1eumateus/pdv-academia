// Caminho: pdv/internal/repository/return_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
)

type ReturnRepo struct{}

type NewReturn struct {
	SaleID        int64
	CustomerID    *int64
	RefundMethod  string
	TotalCents    int64
	Reason        string
	CashSessionID *int64
}

type NewReturnItem struct {
	SaleItemID  int64
	ProductID   int64
	ProductName string
	Qty         int64
	AmountCents int64
}

func (ReturnRepo) Insert(ctx context.Context, q DBTX, r NewReturn, now time.Time) (int64, error) {
	const op = "registrar devolução"
	res, err := q.ExecContext(ctx,
		`INSERT INTO returns (sale_id, customer_id, refund_method, total_cents, reason, cash_session_id, user_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.SaleID, r.CustomerID, r.RefundMethod, r.TotalCents, r.Reason, r.CashSessionID, actor.ID(ctx), FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (ReturnRepo) InsertItem(ctx context.Context, q DBTX, returnID int64, it NewReturnItem) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO return_items (return_id, sale_item_id, product_id, product_name, qty, amount_cents) VALUES (?, ?, ?, ?, ?, ?)`,
		returnID, it.SaleItemID, it.ProductID, it.ProductName, it.Qty, it.AmountCents)
	return wrap("registrar item devolvido", err)
}

// ReturnedQty soma o que já foi devolvido de cada item da venda.
func (ReturnRepo) ReturnedQty(ctx context.Context, q DBTX, saleID int64) (map[int64]int64, error) {
	const op = "somar itens devolvidos"
	rows, err := q.QueryContext(ctx,
		`SELECT ri.sale_item_id, SUM(ri.qty) FROM return_items ri JOIN returns r ON r.id = ri.return_id
		 WHERE r.sale_id = ? GROUP BY ri.sale_item_id`, saleID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id, qty int64
		if err := rows.Scan(&id, &qty); err != nil {
			return nil, wrap(op, err)
		}
		out[id] = qty
	}
	return out, wrap(op, rows.Err())
}

// RefundedBy soma o que já foi devolvido da venda numa forma de reembolso.
func (ReturnRepo) RefundedBy(ctx context.Context, q DBTX, saleID int64, method string) (int64, error) {
	var v int64
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(total_cents), 0) FROM returns WHERE sale_id = ? AND refund_method = ?`,
		saleID, method).Scan(&v)
	return v, wrap("somar devoluções", err)
}

const returnSelect = `SELECT r.id, r.sale_id, r.customer_id, c.name, r.refund_method, r.total_cents, r.reason, u.name, r.created_at
	FROM returns r LEFT JOIN customers c ON c.id = r.customer_id LEFT JOIN users u ON u.id = r.user_id`

func scanReturn(s interface{ Scan(...any) error }) (models.Return, error) {
	var (
		v            models.Return
		cid          sql.NullInt64
		cname, uname sql.NullString
		created      string
	)
	if err := s.Scan(&v.ID, &v.SaleID, &cid, &cname, &v.RefundMethod, &v.TotalCents, &v.Reason, &uname, &created); err != nil {
		return v, err
	}
	v.CustomerID, v.CustomerName, v.UserName = nullInt(cid), nullStr(cname), nullStr(uname)
	var err error
	v.CreatedAt, err = parseTime(created)
	return v, err
}

func (ReturnRepo) items(ctx context.Context, q DBTX, returnID int64) ([]models.ReturnItem, error) {
	const op = "listar itens devolvidos"
	rows, err := q.QueryContext(ctx, `SELECT sale_item_id, product_name, qty, amount_cents FROM return_items WHERE return_id = ? ORDER BY id`, returnID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.ReturnItem{}
	for rows.Next() {
		var it models.ReturnItem
		if err := rows.Scan(&it.SaleItemID, &it.ProductName, &it.Qty, &it.AmountCents); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, it)
	}
	return out, wrap(op, rows.Err())
}

func (r ReturnRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Return, error) {
	v, err := scanReturn(q.QueryRowContext(ctx, returnSelect+` WHERE r.id = ?`, id))
	if err != nil {
		return v, wrap("buscar devolução", err)
	}
	v.Items, err = r.items(ctx, q, id)
	return v, err
}

// ListBySale lista as devoluções de uma venda, com os itens.
func (r ReturnRepo) ListBySale(ctx context.Context, q DBTX, saleID int64) ([]models.Return, error) {
	return r.list(ctx, q, returnSelect+` WHERE r.sale_id = ? ORDER BY r.id`, saleID)
}

// ListBetween lista as devoluções do período [start, end), mais recentes
// primeiro. userID 0 = de todos; senão só as registradas por ele.
func (r ReturnRepo) ListBetween(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]models.Return, error) {
	return r.list(ctx, q, returnSelect+` WHERE r.created_at >= ? AND r.created_at < ? AND (? = 0 OR r.user_id = ?) ORDER BY r.id DESC`,
		FormatTime(start), FormatTime(end), userID, userID)
}

func (r ReturnRepo) list(ctx context.Context, q DBTX, query string, args ...any) ([]models.Return, error) {
	const op = "listar devoluções"
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrap(op, err)
	}
	out := []models.Return{}
	for rows.Next() {
		v, err := scanReturn(rows)
		if err != nil {
			rows.Close()
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	// Fecha antes de ler os itens: a conexão é a mesma (SQLite).
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, wrap(op, err)
	}
	if err := rows.Close(); err != nil {
		return nil, wrap(op, err)
	}
	for i := range out {
		if out[i].Items, err = r.items(ctx, q, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}
