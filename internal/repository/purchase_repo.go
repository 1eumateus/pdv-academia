// Caminho: pdv/internal/repository/purchase_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
)

type PurchaseRepo struct{}

const purchaseSelect = `SELECT p.id, p.supplier_id, s.legal_name, p.invoice_number, p.issue_date, p.access_key,
	p.products_cents, p.freight_cents, p.taxes_cents, p.discount_cents, p.total_cents, p.notes, u.name, p.created_at,
	p.cancelled_at, cu.name, p.cancellation_reason, p.cancelled_access_key
	FROM purchases p JOIN suppliers s ON s.id = p.supplier_id LEFT JOIN users u ON u.id = p.user_id
	LEFT JOIN users cu ON cu.id = p.cancelled_by_user_id`

func scanPurchase(s interface{ Scan(...any) error }) (models.Purchase, error) {
	var (
		v                       models.Purchase
		key, user, cancelled    sql.NullString
		cancelledBy, reason, ck sql.NullString
		created                 string
	)
	err := s.Scan(&v.ID, &v.SupplierID, &v.SupplierName, &v.InvoiceNumber, &v.IssueDate, &key, &v.ProductsCents,
		&v.FreightCents, &v.TaxesCents, &v.DiscountCents, &v.TotalCents, &v.Notes, &user, &created,
		&cancelled, &cancelledBy, &reason, &ck)
	if err != nil {
		return v, err
	}
	v.AccessKey, v.UserName = nullStr(key), nullStr(user)
	v.CancelledByName, v.CancellationReason, v.CancelledAccessKey = nullStr(cancelledBy), nullStr(reason), nullStr(ck)
	if v.CancelledAt, err = parseNullTime(cancelled); err != nil {
		return v, err
	}
	v.CreatedAt, err = parseTime(created)
	return v, err
}

func (PurchaseRepo) Insert(ctx context.Context, q DBTX, v models.Purchase, now time.Time) (int64, error) {
	const op = "registrar compra"
	res, err := q.ExecContext(ctx,
		`INSERT INTO purchases (supplier_id, invoice_number, issue_date, access_key, products_cents, freight_cents,
		 taxes_cents, discount_cents, total_cents, notes, user_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.SupplierID, v.InvoiceNumber, v.IssueDate, v.AccessKey, v.ProductsCents, v.FreightCents, v.TaxesCents,
		v.DiscountCents, v.TotalCents, v.Notes, actor.ID(ctx), FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (PurchaseRepo) InsertItem(ctx context.Context, q DBTX, purchaseID int64, it models.PurchaseItem) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO purchase_items (purchase_id, product_id, product_name, qty, unit_cost_cents, total_cents,
		 cost_before_cents, cost_after_cents) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		purchaseID, it.ProductID, it.ProductName, it.Qty, it.UnitCostCents, it.TotalCents, it.CostBeforeCents, it.CostAfterCents)
	return wrap("registrar item da compra", err)
}

func (PurchaseRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Purchase, error) {
	v, err := scanPurchase(q.QueryRowContext(ctx, purchaseSelect+` WHERE p.id = ?`, id))
	return v, wrap("buscar compra", err)
}

// MarkCancelled estorna a compra uma única vez, liberando a chave da NF-e.
func (PurchaseRepo) MarkCancelled(ctx context.Context, q DBTX, id int64, reason string, now time.Time) (bool, error) {
	const op = "estornar compra"
	res, err := q.ExecContext(ctx,
		`UPDATE purchases SET cancelled_at = ?, cancelled_by_user_id = ?, cancellation_reason = ?,
		 cancelled_access_key = access_key, access_key = NULL WHERE id = ? AND cancelled_at IS NULL`,
		FormatTime(now), actor.ID(ctx), reason, id)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

// ExistsAccessKey informa se uma NF-e com esta chave já foi lançada.
func (PurchaseRepo) ExistsAccessKey(ctx context.Context, q DBTX, key string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM purchases WHERE access_key = ?`, key).Scan(&n)
	return n > 0, wrap("verificar nota já lançada", err)
}

func (PurchaseRepo) Items(ctx context.Context, q DBTX, id int64) ([]models.PurchaseItem, error) {
	const op = "listar itens da compra"
	rows, err := q.QueryContext(ctx,
		`SELECT id, product_id, product_name, qty, unit_cost_cents, total_cents, cost_before_cents, cost_after_cents
		 FROM purchase_items WHERE purchase_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.PurchaseItem{}
	for rows.Next() {
		var it models.PurchaseItem
		if err := rows.Scan(&it.ID, &it.ProductID, &it.ProductName, &it.Qty, &it.UnitCostCents, &it.TotalCents,
			&it.CostBeforeCents, &it.CostAfterCents); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, it)
	}
	return out, wrap(op, rows.Err())
}

// ListBetween lista compras registradas em [start, end). Com termo, procura em
// todas as datas pelo nº da nota, pela chave ou pelo fornecedor.
func (PurchaseRepo) ListBetween(ctx context.Context, q DBTX, start, end time.Time, term string) ([]models.Purchase, error) {
	const op = "listar compras"
	like := "%" + escapeLike(term) + "%"
	rows, err := q.QueryContext(ctx, purchaseSelect+`
		WHERE (? <> '' OR (p.created_at >= ? AND p.created_at < ?))
		  AND (? = '' OR p.invoice_number LIKE ? ESCAPE '\' OR IFNULL(p.access_key, p.cancelled_access_key) LIKE ? ESCAPE '\'
		       OR s.legal_name LIKE ? ESCAPE '\' OR s.trade_name LIKE ? ESCAPE '\' OR s.document LIKE ? ESCAPE '\')
		ORDER BY p.created_at DESC, p.id DESC LIMIT 500`,
		term, FormatTime(start), FormatTime(end), term, like, like, like, like, "%"+escapeLike(normalizeDoc(term))+"%")
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Purchase{}
	for rows.Next() {
		v, err := scanPurchase(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}
