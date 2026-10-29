// Caminho: pdv/internal/repository/bill_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/models"
)

// BillRepo: contas a pagar (boletos, aluguel, energia…) e os arquivos anexados.
type BillRepo struct{}

const billSelect = `SELECT b.id, b.description, b.category, b.payee, b.amount_cents, b.due_date, b.barcode, b.notes, b.recurring,
	b.status, b.paid_at, b.paid_cents, b.pay_method, b.from_cash, b.created_at,
	(SELECT COUNT(*) FROM bill_files f WHERE f.bill_id = b.id AND f.kind = 'boleto'),
	(SELECT COUNT(*) FROM bill_files f WHERE f.bill_id = b.id AND f.kind = 'comprovante')
	FROM bills b`

func scanBill(s interface{ Scan(...any) error }) (models.Bill, error) {
	var (
		v                 models.Bill
		recurring, cash   int
		paidAt, payMethod sql.NullString
		paidCents         sql.NullInt64
		created           string
		err               error
	)
	if err = s.Scan(&v.ID, &v.Description, &v.Category, &v.Payee, &v.AmountCents, &v.DueDate, &v.Barcode, &v.Notes, &recurring,
		&v.Status, &paidAt, &paidCents, &payMethod, &cash, &created, &v.BoletoFiles, &v.ReceiptFiles); err != nil {
		return v, err
	}
	v.Recurring, v.FromCash = recurring == 1, cash == 1
	v.PaidDate, v.PaidCents, v.PayMethod = nullStr(paidAt), nullInt(paidCents), nullStr(payMethod)
	v.CreatedAt, err = parseTime(created)
	return v, err
}

// List: abertas pelo vencimento, depois as pagas mais recentes (até 500).
func (BillRepo) List(ctx context.Context, q DBTX) ([]models.Bill, error) {
	const op = "listar contas a pagar"
	rows, err := q.QueryContext(ctx, billSelect+` WHERE b.status <> 'cancelada'
		ORDER BY b.status = 'paga', CASE WHEN b.status = 'paga' THEN '' ELSE b.due_date END, b.paid_at DESC, b.id DESC LIMIT 500`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Bill{}
	for rows.Next() {
		v, err := scanBill(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

func (BillRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Bill, error) {
	v, err := scanBill(q.QueryRowContext(ctx, billSelect+` WHERE b.id = ?`, id))
	return v, wrap("buscar conta a pagar", err)
}

func (BillRepo) Insert(ctx context.Context, q DBTX, b models.BillInput, userID any, now time.Time) (int64, error) {
	const op = "cadastrar conta a pagar"
	res, err := q.ExecContext(ctx,
		`INSERT INTO bills (description, category, payee, amount_cents, due_date, barcode, notes, recurring, status, created_at, user_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'aberta', ?, ?)`,
		b.Description, b.Category, b.Payee, b.AmountCents, b.DueDate, b.Barcode, b.Notes, boolInt(b.Recurring), FormatTime(now), userID)
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// Update altera uma conta ainda aberta; false se já foi paga ou cancelada.
func (BillRepo) Update(ctx context.Context, q DBTX, b models.BillInput) (bool, error) {
	const op = "alterar conta a pagar"
	res, err := q.ExecContext(ctx,
		`UPDATE bills SET description = ?, category = ?, payee = ?, amount_cents = ?, due_date = ?, barcode = ?, notes = ?, recurring = ?
		 WHERE id = ? AND status = 'aberta'`,
		b.Description, b.Category, b.Payee, b.AmountCents, b.DueDate, b.Barcode, b.Notes, boolInt(b.Recurring), b.ID)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

// MarkPaid baixa a conta aberta; false se ela já não estava aberta.
func (BillRepo) MarkPaid(ctx context.Context, q DBTX, id int64, paidDate string, cents int64, method string, fromCash bool) (bool, error) {
	const op = "pagar conta"
	res, err := q.ExecContext(ctx,
		`UPDATE bills SET status = 'paga', paid_at = ?, paid_cents = ?, pay_method = ?, from_cash = ? WHERE id = ? AND status = 'aberta'`,
		paidDate, cents, method, boolInt(fromCash), id)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

// Cancel tira da lista uma conta aberta (lançada por engano); false se não estava aberta.
func (BillRepo) Cancel(ctx context.Context, q DBTX, id int64) (bool, error) {
	const op = "excluir conta a pagar"
	res, err := q.ExecContext(ctx, `UPDATE bills SET status = 'cancelada' WHERE id = ? AND status = 'aberta'`, id)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

// PaidByCategory soma o que foi pago por categoria com data de pagamento em [start, end] (AAAA-MM-DD).
func (BillRepo) PaidByCategory(ctx context.Context, q DBTX, start, end string) ([]models.BillCategoryTotal, error) {
	const op = "totalizar contas pagas"
	rows, err := q.QueryContext(ctx,
		`SELECT category, COUNT(*), SUM(paid_cents) FROM bills WHERE status = 'paga' AND paid_at >= ? AND paid_at <= ?
		 GROUP BY category ORDER BY SUM(paid_cents) DESC`, start, end)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.BillCategoryTotal{}
	for rows.Next() {
		var t models.BillCategoryTotal
		if err := rows.Scan(&t.Category, &t.Count, &t.TotalCents); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, t)
	}
	return out, wrap(op, rows.Err())
}

// DueCounts: contas abertas que vencem hoje e as já vencidas (today = AAAA-MM-DD local).
func (BillRepo) DueCounts(ctx context.Context, q DBTX, today string) (dueToday, overdue int64, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(due_date = ?), 0), COALESCE(SUM(due_date < ?), 0) FROM bills WHERE status = 'aberta'`, today, today).
		Scan(&dueToday, &overdue)
	return dueToday, overdue, wrap("contar contas a vencer", err)
}

// ---------- arquivos (boleto e comprovante) ----------

func (BillRepo) InsertFile(ctx context.Context, q DBTX, billID int64, kind, name, mime string, data []byte, now time.Time) (int64, error) {
	const op = "anexar arquivo"
	res, err := q.ExecContext(ctx, `INSERT INTO bill_files (bill_id, kind, name, mime, data, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		billID, kind, name, mime, data, FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// Files lista os arquivos da conta, sem o conteúdo.
func (BillRepo) Files(ctx context.Context, q DBTX, billID int64) ([]models.BillFile, error) {
	const op = "listar arquivos da conta"
	rows, err := q.QueryContext(ctx, `SELECT id, kind, name, mime, length(data), created_at FROM bill_files WHERE bill_id = ? ORDER BY id`, billID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.BillFile{}
	for rows.Next() {
		var (
			f       models.BillFile
			created string
		)
		if err := rows.Scan(&f.ID, &f.Kind, &f.Name, &f.Mime, &f.Size, &created); err != nil {
			return nil, wrap(op, err)
		}
		if f.CreatedAt, err = parseTime(created); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, f)
	}
	return out, wrap(op, rows.Err())
}

// FileData devolve nome e conteúdo de um arquivo anexado.
func (BillRepo) FileData(ctx context.Context, q DBTX, id int64) (name string, data []byte, err error) {
	err = q.QueryRowContext(ctx, `SELECT name, data FROM bill_files WHERE id = ?`, id).Scan(&name, &data)
	return name, data, wrap("abrir arquivo", err)
}

func (BillRepo) DeleteFile(ctx context.Context, q DBTX, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM bill_files WHERE id = ?`, id)
	return wrap("remover arquivo", err)
}
