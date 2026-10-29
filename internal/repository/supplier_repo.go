// Caminho: pdv/internal/repository/supplier_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/models"
)

type SupplierRepo struct{}

const supplierCols = `id, legal_name, trade_name, document, state_registration, contact, phone, email, zip_code,
	street, number, complement, district, city, state, notes, active`

func scanSupplier(s interface{ Scan(...any) error }) (models.Supplier, error) {
	var (
		v      models.Supplier
		doc    sql.NullString
		active int
	)
	err := s.Scan(&v.ID, &v.LegalName, &v.TradeName, &doc, &v.StateRegistration, &v.Contact, &v.Phone, &v.Email,
		&v.ZipCode, &v.Street, &v.Number, &v.Complement, &v.District, &v.City, &v.State, &v.Notes, &active)
	v.Document = doc.String
	v.Active = active == 1
	return v, err
}

// args devolve os campos na ordem de supplierCols, sem o id; documento vazio vira NULL.
func supplierArgs(v models.Supplier) []any {
	var doc any
	if v.Document != "" {
		doc = v.Document
	}
	return []any{v.LegalName, v.TradeName, doc, v.StateRegistration, v.Contact, v.Phone, v.Email, v.ZipCode,
		v.Street, v.Number, v.Complement, v.District, v.City, v.State, v.Notes, boolInt(v.Active)}
}

func (SupplierRepo) Insert(ctx context.Context, q DBTX, v models.Supplier, now time.Time) (int64, error) {
	const op = "cadastrar fornecedor"
	res, err := q.ExecContext(ctx,
		`INSERT INTO suppliers (legal_name, trade_name, document, state_registration, contact, phone, email, zip_code,
		 street, number, complement, district, city, state, notes, active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append(supplierArgs(v), FormatTime(now), FormatTime(now))...)
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (SupplierRepo) Update(ctx context.Context, q DBTX, v models.Supplier, now time.Time) error {
	const op = "alterar fornecedor"
	res, err := q.ExecContext(ctx,
		`UPDATE suppliers SET legal_name = ?, trade_name = ?, document = ?, state_registration = ?, contact = ?, phone = ?,
		 email = ?, zip_code = ?, street = ?, number = ?, complement = ?, district = ?, city = ?, state = ?, notes = ?,
		 active = ?, updated_at = ? WHERE id = ?`,
		append(supplierArgs(v), FormatTime(now), v.ID)...)
	if err != nil {
		return wrap(op, err)
	}
	if n, err := affected(op, res); err != nil || n == 0 {
		if err == nil {
			err = ErrNotFound
		}
		return wrap(op, err)
	}
	return nil
}

func (SupplierRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Supplier, error) {
	v, err := scanSupplier(q.QueryRowContext(ctx, `SELECT `+supplierCols+` FROM suppliers WHERE id = ?`, id))
	return v, wrap("buscar fornecedor", err)
}

// GetByDocument localiza pelo CPF/CNPJ (usado na importação da NF-e).
func (SupplierRepo) GetByDocument(ctx context.Context, q DBTX, doc string) (models.Supplier, error) {
	v, err := scanSupplier(q.QueryRowContext(ctx, `SELECT `+supplierCols+` FROM suppliers WHERE document = ?`, doc))
	return v, wrap("buscar fornecedor pelo documento", err)
}

// Link devolve o produto e o fator ligados ao código do fornecedor (ErrNotFound se não houver).
func (SupplierRepo) Link(ctx context.Context, q DBTX, supplierID int64, code string) (productID, factor int64, err error) {
	err = q.QueryRowContext(ctx, `SELECT product_id, factor FROM supplier_products WHERE supplier_id = ? AND supplier_code = ?`,
		supplierID, code).Scan(&productID, &factor)
	return productID, factor, wrap("buscar vínculo do fornecedor", err)
}

func (SupplierRepo) SaveLink(ctx context.Context, q DBTX, supplierID int64, code string, productID, factor int64, now time.Time) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO supplier_products (supplier_id, supplier_code, product_id, factor, updated_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (supplier_id, supplier_code) DO UPDATE SET product_id = excluded.product_id, factor = excluded.factor,
		 updated_at = excluded.updated_at`, supplierID, code, productID, factor, FormatTime(now))
	return wrap("gravar vínculo do fornecedor", err)
}

func (SupplierRepo) List(ctx context.Context, q DBTX, term string) ([]models.Supplier, error) {
	const op = "listar fornecedores"
	like := "%" + escapeLike(term) + "%"
	rows, err := q.QueryContext(ctx, `SELECT `+supplierCols+` FROM suppliers
		WHERE ? = '' OR legal_name LIKE ? ESCAPE '\' OR trade_name LIKE ? ESCAPE '\' OR document LIKE ? ESCAPE '\'
		   OR contact LIKE ? ESCAPE '\' OR phone LIKE ? ESCAPE '\' OR city LIKE ? ESCAPE '\'
		ORDER BY active DESC, legal_name COLLATE NOCASE`, term, like, like, "%"+escapeLike(normalizeDoc(term))+"%", like, like, like)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Supplier{}
	for rows.Next() {
		v, err := scanSupplier(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}
