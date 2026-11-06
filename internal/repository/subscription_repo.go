// Caminho: pdv/internal/repository/subscription_repo.go

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"pdv/internal/models"
)

type SubscriptionRepo struct{}

// subSelect traz a assinatura com a fatura aberta (se houver) e o último pagamento.
const subSelect = `SELECT s.id, s.customer_id, c.name, c.phone, s.product_id, p.name, COALESCE(p.plan_days, 0), s.price_cents, s.status, s.created_at, s.cancelled_at,
	i.id, i.due_date, i.amount_cents, s.paused_at, s.pause_reason,
	(SELECT MAX(paid_at) FROM subscription_invoices WHERE subscription_id = s.id AND status = 'paga'), r.dias, r.vezes_semana, r.entradas_dia,
	s.exempt_until, s.exempt_reason
	FROM subscriptions s JOIN customers c ON c.id = s.customer_id JOIN products p ON p.id = s.product_id
	LEFT JOIN subscription_invoices i ON i.subscription_id = s.id AND i.status = 'aberta'
	LEFT JOIN catraca_regras r ON r.subscription_id = s.id`

func scanSub(s interface{ Scan(...any) error }) (models.Subscription, error) {
	var (
		v                   models.Subscription
		created             string
		cancelled, lastPaid sql.NullString
		invID, invAmount    sql.NullInt64
		invDue              sql.NullString
		phone               sql.NullString
		paused, reason      sql.NullString
		price               sql.NullInt64
		days                sql.NullString
		weekly, daily       sql.NullInt64
		exemptUntil, exempt sql.NullString
		err                 error
	)
	if err = s.Scan(&v.ID, &v.CustomerID, &v.CustomerName, &phone, &v.ProductID, &v.PlanName, &v.PlanDays, &price, &v.Status, &created, &cancelled,
		&invID, &invDue, &invAmount, &paused, &reason, &lastPaid, &days, &weekly, &daily, &exemptUntil, &exempt); err != nil {
		return v, err
	}
	v.RawExemptUntil, v.ExemptReason = nullStr(exemptUntil), exempt.String
	if days.Valid { // linha em catraca_regras: exceção da assinatura
		r := rulesFrom(days, weekly, daily)
		v.Rules = &r
	}
	v.CustomerPhone, v.PauseReason, v.PriceCents = nullStr(phone), nullStr(reason), nullInt(price)
	if v.PausedAt, err = parseNullTime(paused); err != nil {
		return v, err
	}
	if invID.Valid {
		v.OpenInvoice = &models.SubscriptionInvoice{ID: invID.Int64, DueDate: invDue.String, AmountCents: invAmount.Int64, Status: "aberta"}
	}
	if v.CreatedAt, err = parseTime(created); err != nil {
		return v, err
	}
	if v.CancelledAt, err = parseNullTime(cancelled); err != nil {
		return v, err
	}
	v.LastPaidAt, err = parseNullTime(lastPaid)
	return v, err
}

// List: ativas primeiro (fatura que vence antes no topo), depois as canceladas (sem as excluídas).
func (SubscriptionRepo) List(ctx context.Context, q DBTX) ([]models.Subscription, error) {
	const op = "listar assinaturas"
	rows, err := q.QueryContext(ctx, subSelect+` WHERE s.deleted_at IS NULL ORDER BY s.status = 'cancelada', i.due_date IS NULL, i.due_date, c.name COLLATE NOCASE`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Subscription{}
	for rows.Next() {
		v, err := scanSub(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

func (SubscriptionRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Subscription, error) {
	v, err := scanSub(q.QueryRowContext(ctx, subSelect+` WHERE s.id = ?`, id))
	return v, wrap("buscar assinatura", err)
}

// ActiveExists: o cliente já tem assinatura ativa deste plano.
func (SubscriptionRepo) ActiveExists(ctx context.Context, q DBTX, customerID, productID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions WHERE customer_id = ? AND product_id = ? AND status = 'ativa'`,
		customerID, productID).Scan(&n)
	return n > 0, wrap("conferir assinatura ativa", err)
}

// CustomerHas: o cliente já tem alguma assinatura (ativa ou não).
func (SubscriptionRepo) CustomerHas(ctx context.Context, q DBTX, customerID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions WHERE customer_id = ?`, customerID).Scan(&n)
	return n > 0, wrap("conferir assinatura do cliente", err)
}

// Insert cria a assinatura ativa; price nil = preço do plano.
func (SubscriptionRepo) Insert(ctx context.Context, q DBTX, customerID, productID int64, price *int64, now time.Time) (int64, error) {
	const op = "criar assinatura"
	res, err := q.ExecContext(ctx, `INSERT INTO subscriptions (customer_id, product_id, price_cents, status, created_at) VALUES (?, ?, ?, 'ativa', ?)`,
		customerID, productID, price, FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// PaidBySale devolve a fatura paga pela venda (ErrNotFound se a venda não é de mensalidade).
func (SubscriptionRepo) PaidBySale(ctx context.Context, q DBTX, saleID int64) (invoiceID, subID int64, subStatus string, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT i.id, i.subscription_id, s.status FROM subscription_invoices i JOIN subscriptions s ON s.id = i.subscription_id
		 WHERE i.sale_id = ? AND i.status = 'paga'`, saleID).Scan(&invoiceID, &subID, &subStatus)
	return invoiceID, subID, subStatus, wrap("buscar mensalidade da venda", err)
}

// LaterPaid conta as mensalidades pagas depois desta, na mesma assinatura.
func (SubscriptionRepo) LaterPaid(ctx context.Context, q DBTX, subID, invoiceID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_invoices WHERE subscription_id = ? AND status = 'paga' AND id > ?`,
		subID, invoiceID).Scan(&n)
	return n, wrap("conferir pagamentos seguintes", err)
}

// UndoPayment desfaz o pagamento da fatura: cancela a próxima gerada no pagamento
// e devolve esta a "em aberto" (ou "cancelada" se a assinatura já foi encerrada).
func (SubscriptionRepo) UndoPayment(ctx context.Context, q DBTX, subID, invoiceID int64, reopen bool) error {
	const op = "desfazer pagamento da mensalidade"
	if _, err := q.ExecContext(ctx, `UPDATE subscription_invoices SET status = 'cancelada' WHERE subscription_id = ? AND status = 'aberta' AND id > ?`,
		subID, invoiceID); err != nil {
		return wrap(op, err)
	}
	status := "cancelada"
	if reopen {
		status = "aberta"
	}
	_, err := q.ExecContext(ctx, `UPDATE subscription_invoices SET status = ?, sale_id = NULL, paid_at = NULL, imported = 0 WHERE id = ? AND status = 'paga'`,
		status, invoiceID)
	return wrap(op, err)
}

// SetExempt grava a isenção: until vazio = permanente, data = isenta até o dia.
func (SubscriptionRepo) SetExempt(ctx context.Context, q DBTX, subID int64, until, reason string) error {
	_, err := q.ExecContext(ctx, `UPDATE subscriptions SET exempt_until = ?, exempt_reason = ? WHERE id = ?`, until, reason, subID)
	return wrap("gravar isenção da assinatura", err)
}

// ClearExempt tira a marca de isenção da assinatura.
func (SubscriptionRepo) ClearExempt(ctx context.Context, q DBTX, subID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE subscriptions SET exempt_until = NULL, exempt_reason = NULL WHERE id = ?`, subID)
	return wrap("tirar isenção da assinatura", err)
}

// SetOpenDue muda o vencimento da mensalidade em aberto.
func (SubscriptionRepo) SetOpenDue(ctx context.Context, q DBTX, subID int64, due string) (bool, error) {
	res, err := q.ExecContext(ctx, `UPDATE subscription_invoices SET due_date = ? WHERE subscription_id = ? AND status = 'aberta'`, due, subID)
	if err != nil {
		return false, wrap("mudar vencimento da mensalidade", err)
	}
	n, err := res.RowsAffected()
	return n > 0, wrap("mudar vencimento da mensalidade", err)
}

// InsertImported cria a assinatura vinda de outro sistema (cancelledAt vazio = ativa).
func (SubscriptionRepo) InsertImported(ctx context.Context, q DBTX, customerID, productID int64, price *int64, createdAt, cancelledAt string) (int64, error) {
	const op = "importar assinatura"
	status, cancelled := "ativa", any(nil)
	if cancelledAt != "" {
		status, cancelled = "cancelada", cancelledAt
	}
	res, err := q.ExecContext(ctx,
		`INSERT INTO subscriptions (customer_id, product_id, price_cents, status, created_at, cancelled_at) VALUES (?, ?, ?, ?, ?, ?)`,
		customerID, productID, price, status, createdAt, cancelled)
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// InsertImportedInvoice grava a fatura do outro sistema: paga (sem venda), em aberto ou "não paga" antiga.
func (SubscriptionRepo) InsertImportedInvoice(ctx context.Context, q DBTX, subID int64, dueDate string, amount int64, status, paidAt string, now time.Time) error {
	var paid any
	if paidAt != "" {
		paid = paidAt
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO subscription_invoices (subscription_id, due_date, amount_cents, status, paid_at, created_at, imported) VALUES (?, ?, ?, ?, ?, ?, 1)`,
		subID, dueDate, amount, status, paid, FormatTime(now))
	return wrap("importar fatura", err)
}

// Restore volta a assinatura cancelada (e não excluída) para ativa; false se não estava.
func (SubscriptionRepo) Restore(ctx context.Context, q DBTX, id int64) (bool, error) {
	const op = "reativar assinatura"
	res, err := q.ExecContext(ctx, `UPDATE subscriptions SET status = 'ativa', cancelled_at = NULL
		WHERE id = ? AND status = 'cancelada' AND deleted_at IS NULL`, id)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n > 0, err
}

// CancelledOpen acha a mensalidade que estava em aberto quando a assinatura foi
// cancelada (0 se não há): a cancelada mais nova, depois de todas as pagas, que é a
// primeira da assinatura ou veio logo depois de uma paga (ou de uma "não utilizou"
// da importação). A cancelada ao desfazer um pagamento vem depois de uma que deixou
// de ser paga, então não conta.
func (SubscriptionRepo) CancelledOpen(ctx context.Context, q DBTX, subID int64) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT c.id FROM subscription_invoices c
		WHERE c.subscription_id = ? AND c.status = 'cancelada'
		  AND c.id > COALESCE((SELECT MAX(id) FROM subscription_invoices WHERE subscription_id = c.subscription_id AND status = 'paga'), 0)
		  AND COALESCE((SELECT p.status FROM subscription_invoices p WHERE p.subscription_id = c.subscription_id AND p.id < c.id
		                ORDER BY p.id DESC LIMIT 1), 'paga') IN ('paga', 'nao_paga')
		ORDER BY c.id DESC LIMIT 1`, subID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, wrap("buscar mensalidade da assinatura cancelada", err)
}

// ReopenInvoice volta a mensalidade cancelada para em aberto.
func (SubscriptionRepo) ReopenInvoice(ctx context.Context, q DBTX, invoiceID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE subscription_invoices SET status = 'aberta' WHERE id = ? AND status = 'cancelada'`, invoiceID)
	return wrap("reabrir mensalidade", err)
}

// LastPaidDue: o vencimento mais novo entre as mensalidades pagas ("" se nenhuma).
func (SubscriptionRepo) LastPaidDue(ctx context.Context, q DBTX, subID int64) (string, error) {
	var due sql.NullString
	err := q.QueryRowContext(ctx, `SELECT MAX(due_date) FROM subscription_invoices WHERE subscription_id = ? AND status = 'paga'`, subID).Scan(&due)
	return due.String, wrap("buscar último vencimento pago", err)
}

// SoftDelete exclui a assinatura cancelada (some das telas; o histórico fica); false se não estava cancelada.
func (SubscriptionRepo) SoftDelete(ctx context.Context, q DBTX, id int64, now time.Time) (bool, error) {
	const op = "excluir assinatura"
	res, err := q.ExecContext(ctx, `UPDATE subscriptions SET deleted_at = ? WHERE id = ? AND status = 'cancelada' AND deleted_at IS NULL`,
		FormatTime(now), id)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n > 0, err
}

// CancelledOf: as assinaturas canceladas (e não excluídas) do cliente.
func (SubscriptionRepo) CancelledOf(ctx context.Context, q DBTX, customerID int64) ([]int64, error) {
	const op = "listar assinaturas canceladas"
	rows, err := q.QueryContext(ctx, `SELECT id FROM subscriptions WHERE customer_id = ? AND status = 'cancelada' AND deleted_at IS NULL`, customerID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, wrap(op, err)
		}
		ids = append(ids, id)
	}
	return ids, wrap(op, rows.Err())
}

// Cancel encerra a assinatura e cancela a fatura aberta; false se já estava cancelada.
func (SubscriptionRepo) Cancel(ctx context.Context, q DBTX, id int64, now time.Time) (bool, error) {
	const op = "cancelar assinatura"
	res, err := q.ExecContext(ctx, `UPDATE subscriptions SET status = 'cancelada', cancelled_at = ? WHERE id = ? AND status = 'ativa'`,
		FormatTime(now), id)
	if err != nil {
		return false, wrap(op, err)
	}
	if n, err := affected(op, res); err != nil || n == 0 {
		return false, err
	}
	_, err = q.ExecContext(ctx, `UPDATE subscription_invoices SET status = 'cancelada' WHERE subscription_id = ? AND status = 'aberta'`, id)
	return true, wrap(op, err)
}

// ActiveIDs lista as assinaturas ativas do cliente.
func (SubscriptionRepo) ActiveIDs(ctx context.Context, q DBTX, customerID int64) ([]int64, error) {
	const op = "listar assinaturas ativas"
	rows, err := q.QueryContext(ctx, `SELECT id FROM subscriptions WHERE customer_id = ? AND status = 'ativa'`, customerID)
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

// Pause marca a assinatura ativa como pausada; false se não está ativa e correndo.
func (SubscriptionRepo) Pause(ctx context.Context, q DBTX, id int64, reason *string, now time.Time) (bool, error) {
	const op = "pausar assinatura"
	res, err := q.ExecContext(ctx, `UPDATE subscriptions SET paused_at = ?, pause_reason = ? WHERE id = ? AND status = 'ativa' AND paused_at IS NULL`,
		FormatTime(now), reason, id)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

// Resume tira a pausa e empurra o vencimento da fatura aberta em days dias.
func (SubscriptionRepo) Resume(ctx context.Context, q DBTX, id int64, days int) (bool, error) {
	const op = "retomar assinatura"
	res, err := q.ExecContext(ctx, `UPDATE subscriptions SET paused_at = NULL, pause_reason = NULL WHERE id = ? AND paused_at IS NOT NULL`, id)
	if err != nil {
		return false, wrap(op, err)
	}
	if n, err := affected(op, res); err != nil || n == 0 {
		return false, err
	}
	_, err = q.ExecContext(ctx, `UPDATE subscription_invoices SET due_date = date(due_date, ?) WHERE subscription_id = ? AND status = 'aberta'`,
		fmt.Sprintf("+%d days", days), id)
	return true, wrap(op, err)
}

// ChangePlan troca o plano da assinatura ativa e ajusta a fatura aberta
// (valor do plano novo, vencendo em dueDate). false se ela não está ativa.
func (SubscriptionRepo) ChangePlan(ctx context.Context, q DBTX, id, productID int64, price *int64, dueDate string, amount int64) (bool, error) {
	const op = "alterar plano da assinatura"
	res, err := q.ExecContext(ctx, `UPDATE subscriptions SET product_id = ?, price_cents = ? WHERE id = ? AND status = 'ativa'`, productID, price, id)
	if err != nil {
		return false, wrap(op, err)
	}
	if n, err := affected(op, res); err != nil || n == 0 {
		return false, err
	}
	_, err = q.ExecContext(ctx, `UPDATE subscription_invoices SET due_date = ?, amount_cents = ? WHERE subscription_id = ? AND status = 'aberta'`,
		dueDate, amount, id)
	return true, wrap(op, err)
}

func (SubscriptionRepo) InsertInvoice(ctx context.Context, q DBTX, subID int64, dueDate string, amount int64, now time.Time) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO subscription_invoices (subscription_id, due_date, amount_cents, status, created_at) VALUES (?, ?, ?, 'aberta', ?)`,
		subID, dueDate, amount, FormatTime(now))
	return wrap("gerar fatura", err)
}

// OpenInvoice devolve a fatura aberta e a assinatura dela, ou ErrNotFound.
func (SubscriptionRepo) OpenInvoice(ctx context.Context, q DBTX, invoiceID int64) (inv models.SubscriptionInvoice, subID int64, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT id, subscription_id, due_date, amount_cents, status FROM subscription_invoices WHERE id = ? AND status = 'aberta'`, invoiceID).
		Scan(&inv.ID, &subID, &inv.DueDate, &inv.AmountCents, &inv.Status)
	return inv, subID, wrap("buscar fatura", err)
}

// MarkPaidOutside baixa a fatura aberta sem venda (paga fora do caixa em paidAt)
// e registra quem lançou; false se já não estava aberta. imported = 1 é a marca
// de "paga sem venda" que o banco exige (fatura_fora_caixa diz que foi lançada aqui).
func (SubscriptionRepo) MarkPaidOutside(ctx context.Context, q DBTX, invoiceID int64, paidAt time.Time, userID any, now time.Time) (bool, error) {
	const op = "baixar fatura fora do caixa"
	res, err := q.ExecContext(ctx, `UPDATE subscription_invoices SET status = 'paga', sale_id = NULL, imported = 1, paid_at = ? WHERE id = ? AND status = 'aberta'`,
		FormatTime(paidAt), invoiceID)
	if err != nil {
		return false, wrap(op, err)
	}
	if n, err := affected(op, res); err != nil || n == 0 {
		return false, err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO fatura_fora_caixa (invoice_id, user_id, created_at) VALUES (?, ?, ?)`, invoiceID, userID, FormatTime(now))
	return true, wrap(op, err)
}

// PaidOutside devolve a assinatura (e a situação dela) de uma fatura paga fora do caixa, ou ErrNotFound.
func (SubscriptionRepo) PaidOutside(ctx context.Context, q DBTX, invoiceID int64) (subID int64, subStatus string, err error) {
	err = q.QueryRowContext(ctx, `SELECT i.subscription_id, s.status FROM subscription_invoices i
		JOIN subscriptions s ON s.id = i.subscription_id JOIN fatura_fora_caixa f ON f.invoice_id = i.id
		WHERE i.id = ? AND i.status = 'paga'`, invoiceID).Scan(&subID, &subStatus)
	return subID, subStatus, wrap("buscar fatura paga fora do caixa", err)
}

// ForgetOutside apaga a marca de fatura paga fora do caixa (ao desfazer).
func (SubscriptionRepo) ForgetOutside(ctx context.Context, q DBTX, invoiceID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM fatura_fora_caixa WHERE invoice_id = ?`, invoiceID)
	return wrap("desfazer fatura fora do caixa", err)
}

// MarkPaid: só passa de aberta para paga; false se já não estava aberta.
func (SubscriptionRepo) MarkPaid(ctx context.Context, q DBTX, invoiceID, saleID int64, now time.Time) (bool, error) {
	const op = "baixar fatura"
	res, err := q.ExecContext(ctx, `UPDATE subscription_invoices SET status = 'paga', sale_id = ?, paid_at = ? WHERE id = ? AND status = 'aberta'`,
		saleID, FormatTime(now), invoiceID)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

// Invoices lista as faturas pagas e a aberta da assinatura (as canceladas somem).
func (SubscriptionRepo) Invoices(ctx context.Context, q DBTX, subID int64) ([]models.SubscriptionInvoice, error) {
	const op = "listar faturas"
	rows, err := q.QueryContext(ctx,
		`SELECT id, due_date, amount_cents, status, sale_id, paid_at, imported,
		 EXISTS (SELECT 1 FROM fatura_fora_caixa f WHERE f.invoice_id = subscription_invoices.id)
		 FROM subscription_invoices
		 WHERE subscription_id = ? AND status <> 'cancelada' ORDER BY due_date DESC, id DESC`, subID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.SubscriptionInvoice{}
	for rows.Next() {
		var (
			v    models.SubscriptionInvoice
			sale sql.NullInt64
			paid sql.NullString
		)
		var imported int
		if err := rows.Scan(&v.ID, &v.DueDate, &v.AmountCents, &v.Status, &sale, &paid, &imported, &v.OutsideCash); err != nil {
			return nil, wrap(op, err)
		}
		v.SaleID, v.Imported = nullInt(sale), imported == 1
		if v.PaidAt, err = parseNullTime(paid); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}
