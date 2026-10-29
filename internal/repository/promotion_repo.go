// Caminho: pdv/internal/repository/promotion_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/models"
)

type PromotionRepo struct{}

const promoSelect = `SELECT pr.id, pr.name, pr.kind, pr.product_id, p.name, pr.category_id, c.name, pr.price_cents, pr.percent,
	pr.buy_qty, pr.pay_qty, pr.start_date, pr.end_date, pr.active
	FROM promotions pr LEFT JOIN products p ON p.id = pr.product_id LEFT JOIN categories c ON c.id = pr.category_id`

func scanPromo(s interface{ Scan(...any) error }) (models.Promotion, error) {
	var (
		v            models.Promotion
		pid, cid     sql.NullInt64
		pname, cname sql.NullString
		active       int
	)
	err := s.Scan(&v.ID, &v.Name, &v.Kind, &pid, &pname, &cid, &cname, &v.PriceCents, &v.Percent, &v.BuyQty, &v.PayQty,
		&v.StartDate, &v.EndDate, &active)
	v.ProductID, v.ProductName, v.CategoryID, v.CategoryName = nullInt(pid), nullStr(pname), nullInt(cid), nullStr(cname)
	v.Active = active == 1
	return v, err
}

func queryPromos(ctx context.Context, q DBTX, op, query string, args ...any) ([]models.Promotion, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Promotion{}
	for rows.Next() {
		v, err := scanPromo(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

func (PromotionRepo) List(ctx context.Context, q DBTX) ([]models.Promotion, error) {
	return queryPromos(ctx, q, "listar promoções", promoSelect+` ORDER BY pr.active DESC, pr.end_date DESC, pr.id DESC`)
}

func (PromotionRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Promotion, error) {
	v, err := scanPromo(q.QueryRowContext(ctx, promoSelect+` WHERE pr.id = ?`, id))
	return v, wrap("buscar promoção", err)
}

// ForProduct devolve as promoções ativas e vigentes na data para o produto
// (diretas ou da categoria dele).
func (PromotionRepo) ForProduct(ctx context.Context, q DBTX, productID int64, categoryID *int64, date string) ([]models.Promotion, error) {
	var cat any
	if categoryID != nil {
		cat = *categoryID
	}
	return queryPromos(ctx, q, "buscar promoções do produto", promoSelect+`
		WHERE pr.active = 1 AND pr.start_date <= ? AND pr.end_date >= ? AND (pr.product_id = ? OR pr.category_id = ?)`,
		date, date, productID, cat)
}

func (PromotionRepo) Save(ctx context.Context, q DBTX, v models.Promotion, now time.Time) (int64, error) {
	const op = "salvar promoção"
	args := []any{v.Name, v.Kind, v.ProductID, v.CategoryID, v.PriceCents, v.Percent, v.BuyQty, v.PayQty, v.StartDate, v.EndDate,
		boolInt(v.Active), FormatTime(now)}
	if v.ID == 0 {
		res, err := q.ExecContext(ctx, `INSERT INTO promotions (name, kind, product_id, category_id, price_cents, percent, buy_qty,
			pay_qty, start_date, end_date, active, updated_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			append(args, FormatTime(now))...)
		if err != nil {
			return 0, wrap(op, err)
		}
		return lastID(op, res)
	}
	res, err := q.ExecContext(ctx, `UPDATE promotions SET name = ?, kind = ?, product_id = ?, category_id = ?, price_cents = ?,
		percent = ?, buy_qty = ?, pay_qty = ?, start_date = ?, end_date = ?, active = ?, updated_at = ? WHERE id = ?`,
		append(args, v.ID)...)
	if err != nil {
		return 0, wrap(op, err)
	}
	if n, err := affected(op, res); err != nil || n == 0 {
		if err == nil {
			err = ErrNotFound
		}
		return 0, wrap(op, err)
	}
	return v.ID, nil
}
