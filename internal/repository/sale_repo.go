// Caminho: pdv/internal/repository/sale_repo.go

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
)

type SaleRepo struct{}

// NewSale são os valores já calculados e validados pelo serviço.
type NewSale struct {
	CustomerID    *int64
	ComandaID     *int64
	CashSessionID *int64
	SubtotalCents int64
	DiscountCents int64
	TotalCents    int64
	PaymentMethod string
	RequestID     *string
}

const saleSelect = `SELECT s.id, s.customer_id, c.name, s.comanda_id, s.cash_session_id, s.subtotal_cents,
	s.discount_cents, s.total_cents, s.payment_method, s.status, s.created_at, s.cancelled_at, s.cancellation_reason,
	s.user_id, u.name, cu.name,
	(SELECT product_name_snapshot FROM sale_items WHERE sale_id = s.id ORDER BY id LIMIT 1),
	(SELECT qty FROM sale_items WHERE sale_id = s.id ORDER BY id LIMIT 1),
	(SELECT COUNT(*) FROM sale_items WHERE sale_id = s.id)
	FROM sales s LEFT JOIN customers c ON c.id = s.customer_id
	LEFT JOIN users u ON u.id = s.user_id LEFT JOIN users cu ON cu.id = s.cancelled_by_user_id`

func scanSale(s interface{ Scan(...any) error }) (models.Sale, error) {
	var (
		v                         models.Sale
		customerID, comandaID     sql.NullInt64
		cashID                    sql.NullInt64
		customerName, cancelledAt sql.NullString
		reason                    sql.NullString
		userID                    sql.NullInt64
		userName, cancelledBy     sql.NullString
		firstName                 sql.NullString
		firstQty                  sql.NullInt64
		itemCount                 int64
		created                   string
		err                       error
	)
	if err = s.Scan(&v.ID, &customerID, &customerName, &comandaID, &cashID, &v.SubtotalCents, &v.DiscountCents,
		&v.TotalCents, &v.PaymentMethod, &v.Status, &created, &cancelledAt, &reason, &userID, &userName, &cancelledBy,
		&firstName, &firstQty, &itemCount); err != nil {
		return v, err
	}
	v.CustomerID = nullInt(customerID)
	v.CustomerName = nullStr(customerName)
	v.ComandaID = nullInt(comandaID)
	v.CashSessionID = nullInt(cashID)
	v.CancellationReason = nullStr(reason)
	v.UserID, v.UserName, v.CancelledByName = nullInt(userID), nullStr(userName), nullStr(cancelledBy)
	v.ItemsLabel = itemsLabel(firstName.String, firstQty.Int64, itemCount)
	if v.CreatedAt, err = parseTime(created); err != nil {
		return v, err
	}
	v.CancelledAt, err = parseNullTime(cancelledAt)
	return v, err
}

// itemsLabel resume a venda pelo primeiro item: "Coca-Cola 2L", "3x Coca-Cola 2L",
// "3x Coca-Cola 2L + 2 itens". Vazio se a venda não tem itens.
func itemsLabel(name string, qty, count int64) string {
	if count == 0 {
		return ""
	}
	if qty > 1 {
		name = fmt.Sprintf("%dx %s", qty, name)
	}
	switch rest := count - 1; rest {
	case 0:
		return name
	case 1:
		return name + " + 1 item"
	default:
		return fmt.Sprintf("%s + %d itens", name, rest)
	}
}

func querySales(ctx context.Context, q DBTX, op, query string, args ...any) ([]models.Sale, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Sale{}
	for rows.Next() {
		s, err := scanSale(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, s)
	}
	return out, wrap(op, rows.Err())
}

func (SaleRepo) Insert(ctx context.Context, q DBTX, s NewSale, now time.Time) (int64, error) {
	const op = "inserir venda"
	res, err := q.ExecContext(ctx,
		`INSERT INTO sales (customer_id, comanda_id, cash_session_id, subtotal_cents, discount_cents, total_cents,
		 payment_method, status, created_at, request_id, user_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.CustomerID, s.ComandaID, s.CashSessionID, s.SubtotalCents, s.DiscountCents, s.TotalCents,
		s.PaymentMethod, models.SaleCompleted, FormatTime(now), s.RequestID, actor.ID(ctx))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (SaleRepo) InsertItem(ctx context.Context, q DBTX, it models.SaleItem) (int64, error) {
	const op = "inserir item da venda"
	res, err := q.ExecContext(ctx,
		`INSERT INTO sale_items (sale_id, product_id, product_name_snapshot, qty, unit_price_cents, subtotal_cents, discount_cents,
		 promotion_id, promo_discount_cents, promo_label, unit_cost_cents) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.SaleID, it.ProductID, it.ProductNameSnapshot, it.Qty, it.UnitPriceCents, it.SubtotalCents, it.DiscountCents,
		it.PromotionID, it.PromoDiscountCents, it.PromoLabel, it.UnitCostCents)
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (SaleRepo) InsertPayment(ctx context.Context, q DBTX, saleID int64, p models.PaymentInput) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO sale_payments (sale_id, method, amount_cents, received_cents, card_brand, installments) VALUES (?, ?, ?, ?, ?, ?)`,
		saleID, p.Method, p.AmountCents, p.ReceivedCents, p.CardBrand, p.Installments)
	return wrap("registrar pagamento da venda", err)
}

func (SaleRepo) Payments(ctx context.Context, q DBTX, saleID int64) ([]models.SalePayment, error) {
	const op = "listar pagamentos da venda"
	rows, err := q.QueryContext(ctx,
		`SELECT id, method, amount_cents, received_cents, card_brand, installments FROM sale_payments WHERE sale_id = ? ORDER BY id`, saleID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.SalePayment{}
	for rows.Next() {
		var p models.SalePayment
		if err := rows.Scan(&p.ID, &p.Method, &p.AmountCents, &p.ReceivedCents, &p.CardBrand, &p.Installments); err != nil {
			return nil, wrap(op, err)
		}
		if p.ReceivedCents > p.AmountCents {
			p.ChangeCents = p.ReceivedCents - p.AmountCents
		}
		out = append(out, p)
	}
	return out, wrap(op, rows.Err())
}

func (SaleRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Sale, error) {
	s, err := scanSale(q.QueryRowContext(ctx, saleSelect+` WHERE s.id = ?`, id))
	return s, wrap("buscar venda", err)
}

// FindByRequestID devolve o id da venda já criada com essa chave, ou ErrNotFound.
func (SaleRepo) FindByRequestID(ctx context.Context, q DBTX, requestID string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM sales WHERE request_id = ?`, requestID).Scan(&id)
	return id, wrap("buscar venda pela chave do pedido", err)
}

func (SaleRepo) Items(ctx context.Context, q DBTX, saleID int64) ([]models.SaleItem, error) {
	const op = "listar itens da venda"
	rows, err := q.QueryContext(ctx,
		`SELECT id, sale_id, product_id, product_name_snapshot, qty, unit_price_cents, subtotal_cents, discount_cents,
		 promotion_id, promo_discount_cents, promo_label FROM sale_items WHERE sale_id = ? ORDER BY id`, saleID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.SaleItem{}
	for rows.Next() {
		var it models.SaleItem
		var promo sql.NullInt64
		if err := rows.Scan(&it.ID, &it.SaleID, &it.ProductID, &it.ProductNameSnapshot, &it.Qty, &it.UnitPriceCents, &it.SubtotalCents,
			&it.DiscountCents, &promo, &it.PromoDiscountCents, &it.PromoLabel); err != nil {
			return nil, wrap(op, err)
		}
		it.PromotionID = nullInt(promo)
		out = append(out, it)
	}
	return out, wrap(op, rows.Err())
}

// ListBetween lista vendas com created_at em [start, end), de um operador (0 = todos).
func (SaleRepo) ListBetween(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]models.Sale, error) {
	return querySales(ctx, q, "listar vendas do período",
		saleSelect+` WHERE s.created_at >= ? AND s.created_at < ? AND (? = 0 OR s.user_id = ?) ORDER BY s.created_at DESC, s.id DESC`,
		FormatTime(start), FormatTime(end), userID, userID)
}

func (SaleRepo) ListCreditByCustomer(ctx context.Context, q DBTX, customerID int64) ([]models.Sale, error) {
	return querySales(ctx, q, "listar vendas a prazo do cliente",
		saleSelect+` WHERE s.customer_id = ? AND EXISTS (SELECT 1 FROM sale_payments sp WHERE sp.sale_id = s.id AND sp.method = 'fiado')
		 ORDER BY s.created_at DESC, s.id DESC`,
		customerID)
}

func (SaleRepo) ListBySession(ctx context.Context, q DBTX, sessionID int64) ([]models.Sale, error) {
	return querySales(ctx, q, "listar vendas da sessão",
		saleSelect+` WHERE s.cash_session_id = ? ORDER BY s.created_at DESC, s.id DESC`, sessionID)
}

// MarkCancelled só altera vendas concluídas; devolve false se já estava cancelada.
func (SaleRepo) MarkCancelled(ctx context.Context, q DBTX, id int64, reason string, now time.Time) (bool, error) {
	const op = "cancelar venda"
	res, err := q.ExecContext(ctx,
		`UPDATE sales SET status = 'cancelada', cancelled_at = ?, cancellation_reason = ?, cancelled_by_user_id = ?
		 WHERE id = ? AND status = 'concluida'`, FormatTime(now), reason, actor.ID(ctx), id)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

// InsertDelivery grava para onde levar a venda.
func (SaleRepo) InsertDelivery(ctx context.Context, q DBTX, saleID int64, d models.Delivery) error {
	_, err := q.ExecContext(ctx, `INSERT INTO sale_deliveries (sale_id, address, phone, notes) VALUES (?, ?, ?, ?)`,
		saleID, d.Address, d.Phone, d.Notes)
	return wrap("registrar entrega", err)
}

// Delivery devolve a entrega da venda (nil = retirada no balcão).
func (SaleRepo) Delivery(ctx context.Context, q DBTX, saleID int64) (*models.Delivery, error) {
	var d models.Delivery
	err := q.QueryRowContext(ctx, `SELECT address, phone, notes FROM sale_deliveries WHERE sale_id = ?`, saleID).
		Scan(&d.Address, &d.Phone, &d.Notes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrap("ler entrega", err)
	}
	return &d, nil
}
