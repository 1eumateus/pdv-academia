// Caminho: pdv/internal/repository/quote_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
)

type QuoteRepo struct{}

const quoteSelect = `SELECT q.id, q.customer_id, c.name, q.status, q.valid_until, q.notes, q.subtotal_cents, q.discount_cents,
	q.total_cents, q.sale_id, u.name, q.created_at
	FROM quotes q LEFT JOIN customers c ON c.id = q.customer_id LEFT JOIN users u ON u.id = q.user_id`

func scanQuote(s interface{ Scan(...any) error }) (models.Quote, error) {
	var (
		v            models.Quote
		cid, sid     sql.NullInt64
		cname, uname sql.NullString
		created      string
	)
	err := s.Scan(&v.ID, &cid, &cname, &v.Status, &v.ValidUntil, &v.Notes, &v.SubtotalCents, &v.DiscountCents,
		&v.TotalCents, &sid, &uname, &created)
	if err != nil {
		return v, err
	}
	v.CustomerID, v.CustomerName, v.SaleID, v.UserName = nullInt(cid), nullStr(cname), nullInt(sid), nullStr(uname)
	v.CreatedAt, err = parseTime(created)
	return v, err
}

func (QuoteRepo) Insert(ctx context.Context, q DBTX, v models.Quote, now time.Time) (int64, error) {
	const op = "salvar orçamento"
	res, err := q.ExecContext(ctx,
		`INSERT INTO quotes (customer_id, status, valid_until, notes, subtotal_cents, discount_cents, total_cents, user_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.CustomerID, v.Status, v.ValidUntil, v.Notes, v.SubtotalCents, v.DiscountCents, v.TotalCents, actor.ID(ctx), FormatTime(now), FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (QuoteRepo) Update(ctx context.Context, q DBTX, v models.Quote, now time.Time) error {
	_, err := q.ExecContext(ctx,
		`UPDATE quotes SET customer_id = ?, valid_until = ?, notes = ?, subtotal_cents = ?, discount_cents = ?, total_cents = ?,
		 updated_at = ? WHERE id = ?`,
		v.CustomerID, v.ValidUntil, v.Notes, v.SubtotalCents, v.DiscountCents, v.TotalCents, FormatTime(now), v.ID)
	return wrap("alterar orçamento", err)
}

func (QuoteRepo) ReplaceItems(ctx context.Context, q DBTX, quoteID int64, items []models.QuoteItem) error {
	const op = "gravar itens do orçamento"
	if _, err := q.ExecContext(ctx, `DELETE FROM quote_items WHERE quote_id = ?`, quoteID); err != nil {
		return wrap(op, err)
	}
	for _, it := range items {
		if _, err := q.ExecContext(ctx,
			`INSERT INTO quote_items (quote_id, product_id, product_name, qty, unit_price_cents, discount_cents, promo_discount_cents, promo_label)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			quoteID, it.ProductID, it.ProductName, it.Qty, it.UnitPriceCents, it.DiscountCents, it.PromoDiscountCents, it.PromoLabel); err != nil {
			return wrap(op, err)
		}
	}
	return nil
}

func (QuoteRepo) SetStatus(ctx context.Context, q DBTX, id int64, status string, saleID *int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE quotes SET status = ?, sale_id = ?, updated_at = ? WHERE id = ?`, status, saleID, FormatTime(now), id)
	return wrap("alterar situação do orçamento", err)
}

// ReopenBySale devolve para "aberto" o orçamento ligado à venda (venda cancelada).
func (QuoteRepo) ReopenBySale(ctx context.Context, q DBTX, saleID int64, now time.Time) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM quotes WHERE sale_id = ?`, saleID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, wrap("buscar orçamento da venda", err)
	}
	_, err = q.ExecContext(ctx, `UPDATE quotes SET status = 'aberto', sale_id = NULL, updated_at = ? WHERE id = ?`, FormatTime(now), id)
	return id, wrap("reabrir orçamento", err)
}

func (QuoteRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Quote, error) {
	v, err := scanQuote(q.QueryRowContext(ctx, quoteSelect+` WHERE q.id = ?`, id))
	return v, wrap("buscar orçamento", err)
}

func (QuoteRepo) Items(ctx context.Context, q DBTX, id int64) ([]models.QuoteItem, error) {
	const op = "listar itens do orçamento"
	rows, err := q.QueryContext(ctx,
		`SELECT i.id, i.product_id, i.product_name, COALESCE(p.barcode, ''), i.qty, i.unit_price_cents, i.discount_cents,
		        i.promo_discount_cents, i.promo_label
		 FROM quote_items i LEFT JOIN products p ON p.id = i.product_id WHERE i.quote_id = ? ORDER BY i.id`, id)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.QuoteItem{}
	for rows.Next() {
		var it models.QuoteItem
		if err := rows.Scan(&it.ID, &it.ProductID, &it.ProductName, &it.Barcode, &it.Qty, &it.UnitPriceCents, &it.DiscountCents,
			&it.PromoDiscountCents, &it.PromoLabel); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, it)
	}
	return out, wrap(op, rows.Err())
}

// List devolve os orçamentos criados em [start, end); com termo, procura em
// todas as datas pelo nº, cliente ou operador.
func (QuoteRepo) List(ctx context.Context, q DBTX, start, end time.Time, term string) ([]models.Quote, error) {
	const op = "listar orçamentos"
	like := "%" + escapeLike(term) + "%"
	rows, err := q.QueryContext(ctx, quoteSelect+`
		WHERE (? <> '' OR (q.created_at >= ? AND q.created_at < ?))
		  AND (? = '' OR CAST(q.id AS TEXT) = ? OR c.name LIKE ? ESCAPE '\' OR c.phone LIKE ? ESCAPE '\' OR u.name LIKE ? ESCAPE '\')
		ORDER BY q.created_at DESC, q.id DESC LIMIT 500`,
		term, FormatTime(start), FormatTime(end), term, term, like, like, like)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Quote{}
	for rows.Next() {
		v, err := scanQuote(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}
