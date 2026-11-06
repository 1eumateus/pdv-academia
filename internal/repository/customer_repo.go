// Caminho: pdv/internal/repository/customer_repo.go

package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"pdv/internal/models"
)

type CustomerRepo struct{}

const customerCols = `id, name, phone, document, email, zip_code, street, number, complement, district, city,
	state, reference, notes, credit_limit_cents, credit_balance_cents, store_credit_cents, active, created_at, updated_at, last_charge_at, birth_date, gender`

// CustomerFields são os dados cadastrais já validados pelo serviço.
type CustomerFields struct {
	Name                                         string
	Phone, Document, Email, ZipCode, Street      *string
	Number                                       *string
	Complement, District, City, State, Reference *string
	Notes                                        *string
	LimitCents                                   int64
	Active                                       bool
}

func scanCustomer(s interface{ Scan(...any) error }) (models.Customer, error) {
	var (
		c                models.Customer
		opt              [12]sql.NullString
		active           int
		created, updated string
		lastCharge       sql.NullString
		birth, gender    sql.NullString
		err              error
	)
	if err = s.Scan(&c.ID, &c.Name, &opt[0], &opt[1], &opt[2], &opt[3], &opt[4], &opt[5], &opt[6], &opt[7],
		&opt[8], &opt[9], &opt[10], &opt[11], &c.CreditLimitCents, &c.CreditBalanceCents, &c.StoreCreditCents, &active, &created, &updated, &lastCharge, &birth, &gender); err != nil {
		return c, err
	}
	c.Phone, c.Document, c.Email, c.ZipCode = nullStr(opt[0]), nullStr(opt[1]), nullStr(opt[2]), nullStr(opt[3])
	c.Street, c.Number, c.Complement, c.District = nullStr(opt[4]), nullStr(opt[5]), nullStr(opt[6]), nullStr(opt[7])
	c.City, c.State, c.Reference, c.Notes = nullStr(opt[8]), nullStr(opt[9]), nullStr(opt[10]), nullStr(opt[11])
	c.Active = active == 1
	c.BirthDate = nullStr(birth)
	c.Gender = nullStr(gender)
	if lastCharge.Valid {
		t, err := parseTime(lastCharge.String)
		if err != nil {
			return c, err
		}
		c.LastChargeAt = &t
	}
	if c.CreatedAt, err = parseTime(created); err != nil {
		return c, err
	}
	c.UpdatedAt, err = parseTime(updated)
	return c, err
}

// SetBirthDate grava a data de nascimento (nil apaga).
func (CustomerRepo) SetBirthDate(ctx context.Context, q DBTX, id int64, date *string) error {
	_, err := q.ExecContext(ctx, `UPDATE customers SET birth_date = ? WHERE id = ?`, date, id)
	return wrap("gravar data de nascimento", err)
}

// SetGender grava o gênero (nil apaga).
func (CustomerRepo) SetGender(ctx context.Context, q DBTX, id int64, gender *string) error {
	_, err := q.ExecContext(ctx, `UPDATE customers SET gender = ? WHERE id = ?`, gender, id)
	return wrap("gravar gênero", err)
}

// SetActive ativa ou desativa o cliente.
func (CustomerRepo) SetActive(ctx context.Context, q DBTX, id int64, active bool, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE customers SET active = ?, updated_at = ? WHERE id = ?`, boolInt(active), FormatTime(now), id)
	return wrap("ativar cliente", err)
}

// Delete apaga o cliente. Com histórico (vendas, fiado, assinaturas…) o banco
// recusa: ErrForeignKey.
func (CustomerRepo) Delete(ctx context.Context, q DBTX, id int64) error {
	const op = "excluir cliente"
	res, err := q.ExecContext(ctx, `DELETE FROM customers WHERE id = ?`, id)
	if err != nil {
		return wrap(op, err)
	}
	if n, err := affected(op, res); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetLastCharge registra quando o cliente foi cobrado pelo WhatsApp.
func (CustomerRepo) SetLastCharge(ctx context.Context, q DBTX, id int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE customers SET last_charge_at = ? WHERE id = ?`, FormatTime(now), id)
	return wrap("registrar cobrança", err)
}

func (f CustomerFields) args() []any {
	return []any{f.Name, f.Phone, f.Document, f.Email, f.ZipCode, f.Street, f.Number, f.Complement,
		f.District, f.City, f.State, f.Reference, f.Notes, f.LimitCents, boolInt(f.Active)}
}

func (CustomerRepo) Insert(ctx context.Context, q DBTX, f CustomerFields, now time.Time) (int64, error) {
	const op = "inserir cliente"
	args := append(f.args(), FormatTime(now), FormatTime(now))
	res, err := q.ExecContext(ctx,
		`INSERT INTO customers (name, phone, document, email, zip_code, street, number, complement, district, city,
		 state, reference, notes, credit_limit_cents, active, credit_balance_cents, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`, args...)
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// Update altera dados cadastrais. O saldo devedor só muda pelas operações de
// crédito.
func (CustomerRepo) Update(ctx context.Context, q DBTX, id int64, f CustomerFields, now time.Time) error {
	const op = "atualizar cliente"
	args := append(f.args(), FormatTime(now), id)
	res, err := q.ExecContext(ctx,
		`UPDATE customers SET name = ?, phone = ?, document = ?, email = ?, zip_code = ?, street = ?, number = ?,
		 complement = ?, district = ?, city = ?, state = ?, reference = ?, notes = ?, credit_limit_cents = ?,
		 active = ?, updated_at = ? WHERE id = ?`, args...)
	if err != nil {
		return wrap(op, err)
	}
	n, err := affected(op, res)
	if err != nil {
		return err
	}
	if n == 0 {
		return wrap(op, ErrNotFound)
	}
	return nil
}

func (CustomerRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Customer, error) {
	c, err := scanCustomer(q.QueryRowContext(ctx, `SELECT `+customerCols+` FROM customers WHERE id = ?`, id))
	return c, wrap("buscar cliente", err)
}

func (CustomerRepo) List(ctx context.Context, q DBTX, term string) ([]models.Customer, error) {
	const op = "listar clientes"
	like := "%" + escapeLike(term) + "%"
	// O documento é gravado sem pontuação; a busca aceita "12.345" ou "12345".
	digits := "%" + escapeLike(normalizeDoc(term)) + "%"
	rows, err := q.QueryContext(ctx,
		`SELECT `+customerCols+` FROM customers
		 WHERE ? = '' OR name LIKE ? ESCAPE '\' OR phone LIKE ? ESCAPE '\' OR email LIKE ? ESCAPE '\'
		    OR street LIKE ? ESCAPE '\' OR district LIKE ? ESCAPE '\' OR city LIKE ? ESCAPE '\'
		    OR (? <> '%%' AND document LIKE ? ESCAPE '\')
		 ORDER BY active DESC, name COLLATE NOCASE`, term, like, like, like, like, like, like, digits, digits)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Customer{}
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, c)
	}
	return out, wrap(op, rows.Err())
}

// AddDebtWithinLimit soma a dívida somente se o cliente estiver ativo e o novo
// saldo couber no limite. A condição fica no próprio UPDATE, então não depende
// de uma leitura anterior.
func (CustomerRepo) AddDebtWithinLimit(ctx context.Context, q DBTX, id, amount int64, now time.Time) (bool, error) {
	const op = "lançar dívida"
	res, err := q.ExecContext(ctx,
		`UPDATE customers SET credit_balance_cents = credit_balance_cents + ?, updated_at = ?
		 WHERE id = ? AND active = 1 AND credit_balance_cents + ? <= credit_limit_cents`,
		amount, FormatTime(now), id, amount)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

// AddDebt soma a dívida sem checar limite (usado no estorno de um recebimento,
// que apenas restaura uma dívida que já existia).
func (CustomerRepo) AddDebt(ctx context.Context, q DBTX, id, amount int64, now time.Time) error {
	const op = "restaurar dívida"
	res, err := q.ExecContext(ctx,
		`UPDATE customers SET credit_balance_cents = credit_balance_cents + ?, updated_at = ? WHERE id = ?`,
		amount, FormatTime(now), id)
	if err != nil {
		return wrap(op, err)
	}
	n, err := affected(op, res)
	if err == nil && n == 0 {
		err = wrap(op, ErrNotFound)
	}
	return err
}

// ReduceDebt abate a dívida somente se ela for maior ou igual ao valor.
func (CustomerRepo) ReduceDebt(ctx context.Context, q DBTX, id, amount int64, now time.Time) (bool, error) {
	const op = "abater dívida"
	res, err := q.ExecContext(ctx,
		`UPDATE customers SET credit_balance_cents = credit_balance_cents - ?, updated_at = ?
		 WHERE id = ? AND credit_balance_cents >= ?`,
		amount, FormatTime(now), id, amount)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

func normalizeDoc(s string) string {
	b := make([]byte, 0, len(s))
	for _, c := range []byte(strings.ToUpper(s)) {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') {
			b = append(b, c)
		}
	}
	return string(b)
}

// AddStoreCredit soma vale-troca ao saldo do cliente.
func (CustomerRepo) AddStoreCredit(ctx context.Context, q DBTX, id, amount int64, now time.Time) error {
	const op = "lançar vale-troca"
	res, err := q.ExecContext(ctx,
		`UPDATE customers SET store_credit_cents = store_credit_cents + ?, updated_at = ? WHERE id = ?`,
		amount, FormatTime(now), id)
	if err != nil {
		return wrap(op, err)
	}
	n, err := affected(op, res)
	if err == nil && n == 0 {
		err = wrap(op, ErrNotFound)
	}
	return err
}

// UseStoreCredit desconta o vale-troca somente se o saldo cobrir o valor.
func (CustomerRepo) UseStoreCredit(ctx context.Context, q DBTX, id, amount int64, now time.Time) (bool, error) {
	const op = "usar vale-troca"
	res, err := q.ExecContext(ctx,
		`UPDATE customers SET store_credit_cents = store_credit_cents - ?, updated_at = ?
		 WHERE id = ? AND store_credit_cents >= ?`,
		amount, FormatTime(now), id, amount)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}
