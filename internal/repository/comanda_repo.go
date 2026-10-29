// Caminho: pdv/internal/repository/comanda_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/models"
)

type ComandaRepo struct{}

const comandaSelect = `SELECT c.id, c.table_no, c.status, c.opened_at, c.closed_at,
	COALESCE((SELECT SUM(i.qty * i.unit_price_cents) FROM comanda_items i WHERE i.comanda_id = c.id), 0),
	(SELECT COUNT(*) FROM comanda_items i WHERE i.comanda_id = c.id),
	(SELECT s.id FROM sales s WHERE s.comanda_id = c.id)
	FROM comandas c`

func scanComanda(s interface{ Scan(...any) error }) (models.Comanda, error) {
	var (
		c      models.Comanda
		closed sql.NullString
		saleID sql.NullInt64
		opened string
		err    error
	)
	if err = s.Scan(&c.ID, &c.TableNo, &c.Status, &opened, &closed, &c.TotalCents, &c.ItemCount, &saleID); err != nil {
		return c, err
	}
	c.SaleID = nullInt(saleID)
	if c.OpenedAt, err = parseTime(opened); err != nil {
		return c, err
	}
	c.ClosedAt, err = parseNullTime(closed)
	return c, err
}

func (ComandaRepo) Insert(ctx context.Context, q DBTX, tableNo string, now time.Time) (int64, error) {
	const op = "abrir comanda"
	res, err := q.ExecContext(ctx,
		`INSERT INTO comandas (table_no, status, opened_at) VALUES (?, 'aberta', ?)`, tableNo, FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (ComandaRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Comanda, error) {
	c, err := scanComanda(q.QueryRowContext(ctx, comandaSelect+` WHERE c.id = ?`, id))
	return c, wrap("buscar comanda", err)
}

func (ComandaRepo) ListOpen(ctx context.Context, q DBTX) ([]models.Comanda, error) {
	const op = "listar comandas abertas"
	rows, err := q.QueryContext(ctx, comandaSelect+` WHERE c.status = 'aberta' ORDER BY c.opened_at, c.id`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Comanda{}
	for rows.Next() {
		c, err := scanComanda(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, c)
	}
	return out, wrap(op, rows.Err())
}

func (ComandaRepo) Items(ctx context.Context, q DBTX, comandaID int64) ([]models.ComandaItem, error) {
	const op = "listar itens da comanda"
	rows, err := q.QueryContext(ctx,
		`SELECT i.id, i.comanda_id, i.product_id, i.product_name_snapshot, i.qty, i.unit_price_cents, p.active
		 FROM comanda_items i JOIN products p ON p.id = i.product_id
		 WHERE i.comanda_id = ? ORDER BY i.id`, comandaID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.ComandaItem{}
	for rows.Next() {
		var (
			it     models.ComandaItem
			active int
		)
		if err := rows.Scan(&it.ID, &it.ComandaID, &it.ProductID, &it.ProductNameSnapshot, &it.Qty, &it.UnitPriceCents, &active); err != nil {
			return nil, wrap(op, err)
		}
		it.ProductActive = active == 1
		it.SubtotalCents = it.Qty * it.UnitPriceCents
		out = append(out, it)
	}
	return out, wrap(op, rows.Err())
}

// FindItemByProduct procura um item já lançado do mesmo produto e preço, para
// somar a quantidade em vez de criar outra linha.
func (ComandaRepo) FindItemByProduct(ctx context.Context, q DBTX, comandaID, productID, unitPrice int64) (id, qty int64, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT id, qty FROM comanda_items WHERE comanda_id = ? AND product_id = ? AND unit_price_cents = ? ORDER BY id LIMIT 1`,
		comandaID, productID, unitPrice).Scan(&id, &qty)
	return id, qty, wrap("buscar item da comanda", err)
}

func (ComandaRepo) InsertItem(ctx context.Context, q DBTX, comandaID, productID int64, name string, qty, unitPrice int64, now time.Time) (int64, error) {
	const op = "adicionar item à comanda"
	res, err := q.ExecContext(ctx,
		`INSERT INTO comanda_items (comanda_id, product_id, product_name_snapshot, qty, unit_price_cents, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`, comandaID, productID, name, qty, unitPrice, FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (ComandaRepo) GetItem(ctx context.Context, q DBTX, itemID int64) (comandaID, productID int64, err error) {
	err = q.QueryRowContext(ctx, `SELECT comanda_id, product_id FROM comanda_items WHERE id = ?`, itemID).Scan(&comandaID, &productID)
	return comandaID, productID, wrap("buscar item da comanda", err)
}

// ProductQty soma a quantidade do produto na comanda, sem contar o item exceptItemID.
func (ComandaRepo) ProductQty(ctx context.Context, q DBTX, comandaID, productID, exceptItemID int64) (int64, error) {
	var n int64
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty), 0) FROM comanda_items WHERE comanda_id = ? AND product_id = ? AND id <> ?`,
		comandaID, productID, exceptItemID).Scan(&n)
	return n, wrap("somar quantidade do produto na comanda", err)
}

func (ComandaRepo) SetItemQty(ctx context.Context, q DBTX, itemID, qty int64) error {
	const op = "alterar quantidade do item"
	res, err := q.ExecContext(ctx, `UPDATE comanda_items SET qty = ? WHERE id = ?`, qty, itemID)
	if err != nil {
		return wrap(op, err)
	}
	n, err := affected(op, res)
	if err == nil && n == 0 {
		err = wrap(op, ErrNotFound)
	}
	return err
}

func (ComandaRepo) DeleteItem(ctx context.Context, q DBTX, itemID int64) error {
	const op = "remover item da comanda"
	res, err := q.ExecContext(ctx, `DELETE FROM comanda_items WHERE id = ?`, itemID)
	if err != nil {
		return wrap(op, err)
	}
	n, err := affected(op, res)
	if err == nil && n == 0 {
		err = wrap(op, ErrNotFound)
	}
	return err
}

// SetStatus muda o status somente de comandas abertas. Devolve false se a
// comanda já não estava aberta (fechada em outra ação).
func (ComandaRepo) SetStatus(ctx context.Context, q DBTX, id int64, status string, now time.Time) (bool, error) {
	const op = "alterar status da comanda"
	res, err := q.ExecContext(ctx,
		`UPDATE comandas SET status = ?, closed_at = ? WHERE id = ? AND status = 'aberta'`,
		status, FormatTime(now), id)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}
