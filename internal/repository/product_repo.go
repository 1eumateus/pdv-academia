// Caminho: pdv/internal/repository/product_repo.go

package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"pdv/internal/models"
)

type ProductRepo struct{}

// productSelect já traz o nome da categoria; filtros usam o prefixo p.
const productSelect = `SELECT p.id, p.name, p.barcode, p.price_cents, p.stock_qty, p.active, p.created_at, p.updated_at,
	p.category_id, c.name, p.model_id, pm.name, p.size, p.color, p.cost_cents, p.min_stock, p.price_pending, p.sku, p.track_stock, p.plan_days, p.description,
	p.unit, p.ncm, p.cest, p.cfop, p.origin, p.location, p.supplier_id, COALESCE(NULLIF(trim(sp.trade_name), ''), sp.legal_name)
	FROM products p LEFT JOIN categories c ON c.id = p.category_id LEFT JOIN product_models pm ON pm.id = p.model_id
	LEFT JOIN suppliers sp ON sp.id = p.supplier_id`

// ProductFields são os dados cadastrais já validados pelo serviço.
type ProductFields struct {
	Name         string
	Barcode      *string
	Sku          *string
	PriceCents   int64
	Active       bool
	CategoryID   *int64
	CostCents    int64
	MinStock     int64
	PricePending bool
	// Só no cadastro: a variação não troca de modelo, tamanho ou cor depois.
	ModelID     *int64
	Size, Color string
}

func scanProduct(s interface{ Scan(...any) error }) (models.Product, error) {
	var (
		p                models.Product
		barcode, catName sql.NullString
		sku              sql.NullString
		modelName        sql.NullString
		catID, modelID   sql.NullInt64
		active, pending  int
		track            int
		planDays         sql.NullInt64
		description      sql.NullString
		supplierID       sql.NullInt64
		supplierName     sql.NullString
		created, updated string
		err              error
	)
	if err = s.Scan(&p.ID, &p.Name, &barcode, &p.PriceCents, &p.StockQty, &active, &created, &updated, &catID, &catName, &modelID, &modelName, &p.Size, &p.Color, &p.CostCents, &p.MinStock, &pending, &sku, &track, &planDays, &description,
		&p.Unit, &p.Ncm, &p.Cest, &p.Cfop, &p.Origin, &p.Location, &supplierID, &supplierName); err != nil {
		return p, err
	}
	p.SupplierID, p.SupplierName = nullInt(supplierID), nullStr(supplierName)
	p.Barcode, p.Sku = nullStr(barcode), nullStr(sku)
	p.CategoryID, p.CategoryName = nullInt(catID), nullStr(catName)
	p.ModelID, p.ModelName = nullInt(modelID), nullStr(modelName)
	p.Active, p.PricePending, p.NoStock = active == 1, pending == 1, track == 0
	p.PlanDays, p.Description = nullInt(planDays), nullStr(description)
	if p.CreatedAt, err = parseTime(created); err != nil {
		return p, err
	}
	p.UpdatedAt, err = parseTime(updated)
	return p, err
}

func queryProducts(ctx context.Context, q DBTX, op, query string, args ...any) ([]models.Product, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, p)
	}
	return out, wrap(op, rows.Err())
}

// Insert cria o produto sempre com estoque zero; o saldo inicial entra depois
// como movimentação.
func (ProductRepo) Insert(ctx context.Context, q DBTX, f ProductFields, now time.Time) (int64, error) {
	const op = "inserir produto"
	res, err := q.ExecContext(ctx,
		`INSERT INTO products (name, barcode, sku, price_cents, stock_qty, active, category_id, model_id, size, color, cost_cents, min_stock,
		 price_pending, created_at, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.Name, f.Barcode, f.Sku, f.PriceCents, boolInt(f.Active), f.CategoryID, f.ModelID, f.Size, f.Color, f.CostCents, f.MinStock,
		boolInt(f.PricePending), FormatTime(now), FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// Update altera dados cadastrais. O estoque não é alterado aqui.
func (ProductRepo) Update(ctx context.Context, q DBTX, id int64, f ProductFields, now time.Time) error {
	const op = "atualizar produto"
	res, err := q.ExecContext(ctx,
		`UPDATE products SET name = ?, barcode = ?, sku = ?, price_cents = ?, active = ?, category_id = ?, cost_cents = ?, min_stock = ?,
		 price_pending = ?, updated_at = ? WHERE id = ?`,
		f.Name, f.Barcode, f.Sku, f.PriceCents, boolInt(f.Active), f.CategoryID, f.CostCents, f.MinStock, boolInt(f.PricePending),
		FormatTime(now), id)
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

func (ProductRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Product, error) {
	p, err := scanProduct(q.QueryRowContext(ctx, productSelect+` WHERE p.id = ?`, id))
	return p, wrap("buscar produto", err)
}

// GetByBarcode busca pelo código de barras ou, se não houver, pelo SKU.
func (ProductRepo) GetByBarcode(ctx context.Context, q DBTX, barcode string) (models.Product, error) {
	p, err := scanProduct(q.QueryRowContext(ctx, productSelect+` WHERE p.barcode = ? OR p.sku = ? ORDER BY p.barcode = ? DESC LIMIT 1`,
		barcode, barcode, barcode))
	return p, wrap("buscar produto por código de barras", err)
}

func (ProductRepo) List(ctx context.Context, q DBTX) ([]models.Product, error) {
	return queryProducts(ctx, q, "listar produtos",
		productSelect+` ORDER BY p.active DESC, p.name COLLATE NOCASE`)
}

// Search procura por parte do nome ou pelo início do código de barras ou SKU, opcionalmente
// só numa categoria (categoryID 0 = todas). Termo vazio lista a categoria.
func (ProductRepo) Search(ctx context.Context, q DBTX, term string, onlyActive bool, categoryID int64, limit int) ([]models.Product, error) {
	like := "%" + escapeLike(term) + "%"
	prefix := escapeLike(term) + "%"
	return queryProducts(ctx, q, "pesquisar produtos",
		productSelect+`
		 WHERE (p.name LIKE ? ESCAPE '\' OR p.barcode LIKE ? ESCAPE '\' OR p.sku LIKE ? ESCAPE '\') AND (? = 0 OR (p.active = 1 AND p.price_pending = 0))
		   AND (? = 0 OR p.category_id = ?)
		 ORDER BY (p.barcode = ?) DESC, p.name COLLATE NOCASE LIMIT ?`,
		like, prefix, prefix, boolInt(onlyActive), categoryID, categoryID, term, limit)
}

// SetCost grava o custo (médio) do produto.
func (ProductRepo) SetCost(ctx context.Context, q DBTX, id, cost int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE products SET cost_cents = ?, updated_at = ? WHERE id = ?`, cost, FormatTime(now), id)
	return wrap("gravar custo", err)
}

func (ProductRepo) SetPrice(ctx context.Context, q DBTX, id, price int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE products SET price_cents = ?, price_pending = 0, updated_at = ? WHERE id = ?`, price, FormatTime(now), id)
	return wrap("gravar preço", err)
}

// SetNoStock liga/desliga o controle de estoque do produto (o saldo fica como está).
func (ProductRepo) SetNoStock(ctx context.Context, q DBTX, id int64, noStock bool, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE products SET track_stock = ?, updated_at = ? WHERE id = ?`, boolInt(!noStock), FormatTime(now), id)
	return wrap("alterar controle de estoque", err)
}

// SetDetails grava unidade, dados fiscais, localização e fornecedor habitual.
func (ProductRepo) SetDetails(ctx context.Context, q DBTX, id int64, d models.ProductDetails, now time.Time) error {
	_, err := q.ExecContext(ctx,
		`UPDATE products SET unit = ?, ncm = ?, cest = ?, cfop = ?, origin = ?, location = ?, supplier_id = ?, updated_at = ? WHERE id = ?`,
		d.Unit, d.Ncm, d.Cest, d.Cfop, d.Origin, d.Location, d.SupplierID, FormatTime(now), id)
	return wrap("gravar dados do produto", err)
}

// SetDescription grava a descrição livre (nil apaga).
func (ProductRepo) SetDescription(ctx context.Context, q DBTX, id int64, text *string, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE products SET description = ?, updated_at = ? WHERE id = ?`, text, FormatTime(now), id)
	return wrap("gravar descrição do produto", err)
}

// SetPlanDays marca o produto como plano de assinatura (nil = produto comum).
func (ProductRepo) SetPlanDays(ctx context.Context, q DBTX, id int64, days *int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE products SET plan_days = ?, updated_at = ? WHERE id = ?`, days, FormatTime(now), id)
	return wrap("alterar plano de assinatura", err)
}

// ApplyStockDelta altera o saldo somente se ele não ficar negativo. Retorna
// false quando o saldo não comporta a alteração ou o produto não controla
// estoque (nada é gravado nesse caso).
// Deve ser chamado sempre junto com StockRepo.InsertMovement na mesma Tx.
func (ProductRepo) ApplyStockDelta(ctx context.Context, q DBTX, id, delta int64, now time.Time) (bool, error) {
	const op = "atualizar estoque"
	res, err := q.ExecContext(ctx,
		`UPDATE products SET stock_qty = stock_qty + ?, updated_at = ? WHERE id = ? AND track_stock = 1 AND stock_qty + ? >= 0`,
		delta, FormatTime(now), id, delta)
	if err != nil {
		return false, wrap(op, err)
	}
	n, err := affected(op, res)
	return n == 1, err
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ---------- categorias ----------

type CategoryRepo struct{}

func (CategoryRepo) List(ctx context.Context, q DBTX) ([]models.Category, error) {
	const op = "listar categorias"
	rows, err := q.QueryContext(ctx, `SELECT id, name, active FROM categories ORDER BY active DESC, name COLLATE NOCASE`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.Category{}
	for rows.Next() {
		var (
			c      models.Category
			active int
		)
		if err := rows.Scan(&c.ID, &c.Name, &active); err != nil {
			return nil, wrap(op, err)
		}
		c.Active = active == 1
		out = append(out, c)
	}
	return out, wrap(op, rows.Err())
}

func (CategoryRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.Category, error) {
	var (
		c      models.Category
		active int
	)
	err := q.QueryRowContext(ctx, `SELECT id, name, active FROM categories WHERE id = ?`, id).Scan(&c.ID, &c.Name, &active)
	c.Active = active == 1
	return c, wrap("buscar categoria", err)
}

func (CategoryRepo) Insert(ctx context.Context, q DBTX, name string, now time.Time) (int64, error) {
	const op = "cadastrar categoria"
	res, err := q.ExecContext(ctx, `INSERT INTO categories (name, active, created_at, updated_at) VALUES (?, 1, ?, ?)`,
		name, FormatTime(now), FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (CategoryRepo) Update(ctx context.Context, q DBTX, id int64, name string, active bool, now time.Time) error {
	const op = "alterar categoria"
	res, err := q.ExecContext(ctx, `UPDATE categories SET name = ?, active = ?, updated_at = ? WHERE id = ?`,
		name, boolInt(active), FormatTime(now), id)
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

// ---------- modelos de grade ----------

type ModelRepo struct{}

func (ModelRepo) Insert(ctx context.Context, q DBTX, name string, categoryID *int64, basePrice int64, now time.Time) (int64, error) {
	const op = "cadastrar modelo"
	res, err := q.ExecContext(ctx,
		`INSERT INTO product_models (name, category_id, base_price_cents, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		name, categoryID, basePrice, FormatTime(now), FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (ModelRepo) SetBasePrice(ctx context.Context, q DBTX, id, price int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE product_models SET base_price_cents = ?, updated_at = ? WHERE id = ?`, price, FormatTime(now), id)
	return wrap("alterar preço do modelo", err)
}

// SetPriceAll põe o mesmo preço em todas as variações do modelo.
func (ModelRepo) SetPriceAll(ctx context.Context, q DBTX, id, price int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE products SET price_cents = ?, price_pending = 0, updated_at = ? WHERE model_id = ?`, price, FormatTime(now), id)
	return wrap("alterar preço da grade", err)
}

func (ModelRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.ProductModel, error) {
	var m models.ProductModel
	var cat sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT id, name, category_id, base_price_cents FROM product_models WHERE id = ?`, id).
		Scan(&m.ID, &m.Name, &cat, &m.BasePriceCents)
	m.CategoryID = nullInt(cat)
	return m, wrap("buscar modelo", err)
}

// Variations devolve (tamanho, cor) de cada variação do modelo, na ordem de cadastro.
func (ModelRepo) Variations(ctx context.Context, q DBTX, id int64) ([][2]string, error) {
	const op = "listar variações"
	rows, err := q.QueryContext(ctx, `SELECT size, color FROM products WHERE model_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var v [2]string
		if err := rows.Scan(&v[0], &v[1]); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

// HasHistory: o produto aparece em algo que precisa continuar apontando para ele
// (inclusive qualquer movimento de estoque, que o banco não deixa apagar).
func (ProductRepo) HasHistory(ctx context.Context, q DBTX, id int64) (bool, error) {
	var used bool
	err := q.QueryRowContext(ctx, `SELECT
		EXISTS (SELECT 1 FROM sale_items WHERE product_id = ?1)
		OR EXISTS (SELECT 1 FROM purchase_items WHERE product_id = ?1)
		OR EXISTS (SELECT 1 FROM quote_items WHERE product_id = ?1)
		OR EXISTS (SELECT 1 FROM comanda_items WHERE product_id = ?1)
		OR EXISTS (SELECT 1 FROM return_items WHERE product_id = ?1)
		OR EXISTS (SELECT 1 FROM promotions WHERE product_id = ?1)
		OR EXISTS (SELECT 1 FROM stock_movements WHERE product_id = ?1)`,
		id).Scan(&used)
	return used, wrap("conferir histórico do produto", err)
}

// Delete apaga o produto sem histórico, junto com os vínculos de fornecedor.
func (ProductRepo) Delete(ctx context.Context, q DBTX, id int64) error {
	for _, stmt := range []string{
		`DELETE FROM supplier_products WHERE product_id = ?`,
		`DELETE FROM products WHERE id = ?`,
	} {
		if _, err := q.ExecContext(ctx, stmt, id); err != nil {
			return wrap("excluir produto", err)
		}
	}
	return nil
}
