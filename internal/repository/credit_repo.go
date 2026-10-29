// Caminho: pdv/internal/repository/credit_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
)

type CreditRepo struct{}

const creditSelect = `SELECT p.id, p.customer_id, c.name, p.amount_cents, p.payment_method, p.kind,
	p.reverses_payment_id, EXISTS (SELECT 1 FROM credit_payments r WHERE r.reverses_payment_id = p.id),
	p.created_at, p.notes
	FROM credit_payments p JOIN customers c ON c.id = p.customer_id`

func scanCredit(s interface{ Scan(...any) error }) (models.CreditPayment, error) {
	var (
		p        models.CreditPayment
		reverses sql.NullInt64
		reversed int
		notes    sql.NullString
		created  string
		err      error
	)
	if err = s.Scan(&p.ID, &p.CustomerID, &p.CustomerName, &p.AmountCents, &p.PaymentMethod, &p.Kind,
		&reverses, &reversed, &created, &notes); err != nil {
		return p, err
	}
	p.ReversesPaymentID = nullInt(reverses)
	p.Reversed = reversed == 1
	p.Notes = nullStr(notes)
	p.CreatedAt, err = parseTime(created)
	return p, err
}

func queryCredits(ctx context.Context, q DBTX, op, query string, args ...any) ([]models.CreditPayment, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.CreditPayment{}
	for rows.Next() {
		p, err := scanCredit(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, p)
	}
	return out, wrap(op, rows.Err())
}

func (CreditRepo) Insert(ctx context.Context, q DBTX, customerID, amount int64, method, kind string, reverses *int64, notes, requestID *string, now time.Time) (int64, error) {
	const op = "registrar recebimento"
	res, err := q.ExecContext(ctx,
		`INSERT INTO credit_payments (customer_id, amount_cents, payment_method, kind, reverses_payment_id, created_at, notes, request_id, user_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		customerID, amount, method, kind, reverses, FormatTime(now), notes, requestID, actor.ID(ctx))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// FindByRequestID devolve o id do recebimento já criado com essa chave, ou ErrNotFound.
func (CreditRepo) FindByRequestID(ctx context.Context, q DBTX, requestID string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM credit_payments WHERE request_id = ?`, requestID).Scan(&id)
	return id, wrap("buscar recebimento pela chave do pedido", err)
}

func (CreditRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.CreditPayment, error) {
	p, err := scanCredit(q.QueryRowContext(ctx, creditSelect+` WHERE p.id = ?`, id))
	return p, wrap("buscar recebimento", err)
}

func (CreditRepo) ListByCustomer(ctx context.Context, q DBTX, customerID int64) ([]models.CreditPayment, error) {
	return queryCredits(ctx, q, "listar recebimentos do cliente",
		creditSelect+` WHERE p.customer_id = ? ORDER BY p.created_at DESC, p.id DESC`, customerID)
}

func (CreditRepo) ListBetween(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]models.CreditPayment, error) {
	return queryCredits(ctx, q, "listar recebimentos do período",
		creditSelect+` WHERE p.created_at >= ? AND p.created_at < ? AND (? = 0 OR p.user_id = ?) ORDER BY p.created_at DESC, p.id DESC`,
		FormatTime(start), FormatTime(end), userID, userID)
}

// InsertAllocation registra quanto de um recebimento foi para uma nota (venda).
func (CreditRepo) InsertAllocation(ctx context.Context, q DBTX, paymentID, saleID, amount int64) error {
	_, err := q.ExecContext(ctx, `INSERT INTO credit_allocations (payment_id, sale_id, amount_cents) VALUES (?, ?, ?)`,
		paymentID, saleID, amount)
	return wrap("registrar pagamento da nota", err)
}

// PaidBySale: o que já foi pago de cada nota do cliente de forma direta —
// recebimentos com nota escolhida (não estornados) e devoluções abatidas no fiado.
func (CreditRepo) PaidBySale(ctx context.Context, q DBTX, customerID int64) (map[int64]int64, error) {
	const op = "somar pagamentos por nota"
	rows, err := q.QueryContext(ctx,
		`SELECT a.sale_id, a.amount_cents FROM credit_allocations a JOIN credit_payments p ON p.id = a.payment_id
		 WHERE p.customer_id = ? AND NOT EXISTS (SELECT 1 FROM credit_payments r WHERE r.reverses_payment_id = p.id)
		 UNION ALL
		 SELECT sale_id, total_cents FROM returns WHERE customer_id = ? AND refund_method = 'fiado'`, customerID, customerID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var sale, cents int64
		if err := rows.Scan(&sale, &cents); err != nil {
			return nil, wrap(op, err)
		}
		out[sale] += cents
	}
	return out, wrap(op, rows.Err())
}

// AllocatedSales: as notas (vendas) pagas por um recebimento.
func (CreditRepo) AllocatedSales(ctx context.Context, q DBTX, paymentID int64) ([]int64, error) {
	const op = "listar notas do recebimento"
	rows, err := q.QueryContext(ctx, `SELECT sale_id FROM credit_allocations WHERE payment_id = ? ORDER BY sale_id`, paymentID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, id)
	}
	return out, wrap(op, rows.Err())
}
