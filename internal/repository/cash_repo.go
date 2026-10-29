// Caminho: pdv/internal/repository/cash_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
)

type CashRepo struct{}

const cashSessionCols = `id, status, opening_cents, expected_cents, counted_cents, difference_cents, notes, opened_at, closed_at,
	(SELECT name FROM users WHERE id = opened_by_user_id), (SELECT name FROM users WHERE id = closed_by_user_id), owner_user_id`

func scanCashSession(s interface{ Scan(...any) error }) (models.CashSession, error) {
	var (
		c                       models.CashSession
		expected, counted, diff sql.NullInt64
		notes, closed           sql.NullString
		openedBy, closedBy      sql.NullString
		owner                   sql.NullInt64
		opened                  string
		err                     error
	)
	if err = s.Scan(&c.ID, &c.Status, &c.OpeningCents, &expected, &counted, &diff, &notes, &opened, &closed, &openedBy, &closedBy, &owner); err != nil {
		return c, err
	}
	c.ExpectedCents, c.CountedCents, c.DifferenceCents = nullInt(expected), nullInt(counted), nullInt(diff)
	c.Notes = nullStr(notes)
	c.OpenedByName, c.ClosedByName = nullStr(openedBy), nullStr(closedBy)
	c.OwnerUserID = nullInt(owner)
	if c.OpenedAt, err = parseTime(opened); err != nil {
		return c, err
	}
	c.ClosedAt, err = parseNullTime(closed)
	return c, err
}

// Open cria a sessão do dono (nil = caixa geral). Os índices únicos parciais
// impedem dois caixas gerais ou dois do mesmo usuário abertos (ErrUnique).
func (CashRepo) Open(ctx context.Context, q DBTX, openingCents int64, owner *int64, now time.Time) (int64, error) {
	const op = "abrir caixa"
	res, err := q.ExecContext(ctx,
		`INSERT INTO cash_sessions (status, opening_cents, opened_at, opened_by_user_id, owner_user_id) VALUES ('aberta', ?, ?, ?, ?)`,
		openingCents, FormatTime(now), actor.ID(ctx), owner)
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// GetOpen devolve a sessão aberta do dono (nil = caixa geral) ou ErrNotFound.
func (CashRepo) GetOpen(ctx context.Context, q DBTX, owner *int64) (models.CashSession, error) {
	c, err := scanCashSession(q.QueryRowContext(ctx,
		`SELECT `+cashSessionCols+` FROM cash_sessions WHERE status = 'aberta' AND owner_user_id IS ?`, owner))
	return c, wrap("buscar caixa aberto", err)
}

// CountOpen conta os caixas abertos (gerais e de usuários).
func (CashRepo) CountOpen(ctx context.Context, q DBTX) (int64, error) {
	var n int64
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM cash_sessions WHERE status = 'aberta'`).Scan(&n)
	return n, wrap("contar caixas abertos", err)
}

// ListOpen lista todos os caixas abertos.
func (CashRepo) ListOpen(ctx context.Context, q DBTX) ([]models.CashSession, error) {
	return querySessions(ctx, q, "listar caixas abertos", `SELECT `+cashSessionCols+` FROM cash_sessions WHERE status = 'aberta' ORDER BY opened_at`)
}

func (CashRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.CashSession, error) {
	c, err := scanCashSession(q.QueryRowContext(ctx, `SELECT `+cashSessionCols+` FROM cash_sessions WHERE id = ?`, id))
	return c, wrap("buscar sessão de caixa", err)
}

func (CashRepo) ListSessions(ctx context.Context, q DBTX, limit int) ([]models.CashSession, error) {
	return querySessions(ctx, q, "listar sessões de caixa", `SELECT `+cashSessionCols+` FROM cash_sessions ORDER BY opened_at DESC, id DESC LIMIT ?`, limit)
}

func querySessions(ctx context.Context, q DBTX, op, query string, args ...any) ([]models.CashSession, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.CashSession{}
	for rows.Next() {
		c, err := scanCashSession(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, c)
	}
	return out, wrap(op, rows.Err())
}

type NewCashMovement struct {
	SessionID     int64
	Type          string
	PaymentMethod string
	AmountCents   int64 // com sinal: positivo entra, negativo sai
	Description   *string
	ReferenceType *string
	ReferenceID   *int64
}

func (CashRepo) InsertMovement(ctx context.Context, q DBTX, m NewCashMovement, now time.Time) (int64, error) {
	const op = "registrar movimentação de caixa"
	res, err := q.ExecContext(ctx,
		`INSERT INTO cash_movements (session_id, type, payment_method, amount_cents, description, reference_type, reference_id, created_at, user_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.SessionID, m.Type, m.PaymentMethod, m.AmountCents, m.Description, m.ReferenceType, m.ReferenceID, FormatTime(now), actor.ID(ctx))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

const movementSelect = `SELECT id, session_id, type, payment_method, amount_cents, description, reference_type, reference_id, created_at
	FROM cash_movements`

func scanMovement(s interface{ Scan(...any) error }) (models.CashMovement, error) {
	var (
		m             models.CashMovement
		desc, refType sql.NullString
		refID         sql.NullInt64
		created       string
	)
	if err := s.Scan(&m.ID, &m.SessionID, &m.Type, &m.PaymentMethod, &m.AmountCents, &desc, &refType, &refID, &created); err != nil {
		return m, err
	}
	m.Description, m.ReferenceType, m.ReferenceID = nullStr(desc), nullStr(refType), nullInt(refID)
	var err error
	m.CreatedAt, err = parseTime(created)
	return m, err
}

func (CashRepo) Movements(ctx context.Context, q DBTX, sessionID int64) ([]models.CashMovement, error) {
	const op = "listar movimentações de caixa"
	rows, err := q.QueryContext(ctx, movementSelect+` WHERE session_id = ? ORDER BY created_at DESC, id DESC`, sessionID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.CashMovement{}
	for rows.Next() {
		m, err := scanMovement(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, m)
	}
	return out, wrap(op, rows.Err())
}

func (CashRepo) GetMovement(ctx context.Context, q DBTX, id int64) (models.CashMovement, error) {
	m, err := scanMovement(q.QueryRowContext(ctx, movementSelect+` WHERE id = ?`, id))
	return m, wrap("buscar movimentação de caixa", err)
}

// ExpectedCash = saldo inicial + movimentações em dinheiro da sessão.
func (CashRepo) ExpectedCash(ctx context.Context, q DBTX, sessionID int64) (int64, error) {
	var v int64
	err := q.QueryRowContext(ctx,
		`SELECT s.opening_cents + COALESCE((SELECT SUM(m.amount_cents) FROM cash_movements m
		   WHERE m.session_id = s.id AND m.payment_method = 'dinheiro'), 0)
		 FROM cash_sessions s WHERE s.id = ?`, sessionID).Scan(&v)
	return v, wrap("calcular saldo esperado do caixa", err)
}

// CashReportRaw é um lançamento de caixa com a venda, o cliente, o plano
// (fatura de assinatura) e os itens, para o relatório de controle do caixa.
type CashReportRaw struct {
	Type, Method, Description string
	AmountCents               int64
	CreatedAt                 time.Time
	SaleID, SaleTotal         int64
	CustomerID                int64
	CustomerName, PlanName    string
	Items                     string
}

// ReportRows lista os lançamentos de [start, end) de um usuário (0 = todos).
func (CashRepo) ReportRows(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]CashReportRaw, error) {
	const op = "listar entradas e saídas do caixa"
	rows, err := q.QueryContext(ctx,
		`SELECT m.type, m.payment_method, COALESCE(m.description, ''), m.amount_cents, m.created_at,
		 COALESCE(s.id, 0), COALESCE(s.total_cents, 0), COALESCE(c.id, 0), COALESCE(c.name, ''),
		 COALESCE((SELECT p.name FROM subscription_invoices i JOIN subscriptions sb ON sb.id = i.subscription_id
		           JOIN products p ON p.id = sb.product_id WHERE i.sale_id = s.id), ''),
		 COALESCE((SELECT group_concat(qty || ' ' || product_name_snapshot, ', ') FROM sale_items WHERE sale_id = s.id), '')
		 FROM cash_movements m
		 LEFT JOIN sales s ON m.reference_type = 'sale' AND s.id = m.reference_id
		 LEFT JOIN customers c ON c.id = s.customer_id
		 WHERE m.created_at >= ? AND m.created_at < ? AND (? = 0 OR m.user_id = ?)
		 ORDER BY m.created_at, m.id`,
		FormatTime(start), FormatTime(end), userID, userID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []CashReportRaw{}
	for rows.Next() {
		var (
			v       CashReportRaw
			created string
		)
		if err := rows.Scan(&v.Type, &v.Method, &v.Description, &v.AmountCents, &created, &v.SaleID, &v.SaleTotal,
			&v.CustomerID, &v.CustomerName, &v.PlanName, &v.Items); err != nil {
			return nil, wrap(op, err)
		}
		if v.CreatedAt, err = parseTime(created); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

const totalsTypes = `type IN ('venda', 'recebimento_fiado', 'estorno_venda', 'estorno_recebimento', 'devolucao')`

// TotalsByMethod soma as movimentações da sessão por forma de pagamento.
func (CashRepo) TotalsByMethod(ctx context.Context, q DBTX, sessionID int64) ([]models.PaymentTotal, error) {
	return queryTotals(ctx, q, "totalizar caixa por forma de pagamento",
		`SELECT payment_method, COUNT(*), SUM(amount_cents) FROM cash_movements
		 WHERE session_id = ? AND `+totalsTypes+` GROUP BY payment_method ORDER BY payment_method`, sessionID)
}

// TotalsBetween soma, por forma de pagamento, as movimentações de todos os
// caixas lançadas em [start, end).
func (CashRepo) TotalsBetween(ctx context.Context, q DBTX, start, end time.Time) ([]models.PaymentTotal, error) {
	return queryTotals(ctx, q, "totalizar caixas do período",
		`SELECT payment_method, COUNT(*), SUM(amount_cents) FROM cash_movements
		 WHERE created_at >= ? AND created_at < ? AND `+totalsTypes+` GROUP BY payment_method ORDER BY payment_method`,
		FormatTime(start), FormatTime(end))
}

func queryTotals(ctx context.Context, q DBTX, op, query string, args ...any) ([]models.PaymentTotal, error) {
	rows, err := q.QueryContext(ctx, query, args...)
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

// Close fecha somente sessões abertas; devolve false se já estava fechada.
func (CashRepo) Close(ctx context.Context, q DBTX, id, expected, counted int64, notes *string, now time.Time) (bool, error) {
	const op = "fechar caixa"
	res, err := q.ExecContext(ctx,
		`UPDATE cash_sessions SET status = 'fechada', expected_cents = ?, counted_cents = ?, difference_cents = ?,
		 notes = ?, closed_at = ?, closed_by_user_id = ? WHERE id = ? AND status = 'aberta'`,
		expected, counted, counted-expected, notes, FormatTime(now), actor.ID(ctx), id)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}
