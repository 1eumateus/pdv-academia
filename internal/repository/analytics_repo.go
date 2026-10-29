// Caminho: pdv/internal/repository/analytics_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/models"
)

// AnalyticsRepo lê os dados brutos das análises; as contas ficam no serviço.
type AnalyticsRepo struct{}

// SoldLine é um item vendido com o que é preciso para receita, custo e devoluções.
type SoldLine struct {
	SaleID, SaleDiscountCents        int64
	Item                             models.SaleItem
	CategoryName                     string
	UnitCostCents                    int64
	CostEstimated                    bool // venda antiga: custo atual do produto
	ReturnedQty, ReturnedAmountCents int64
}

// SoldLines lista os itens das vendas concluídas do período, em ordem de venda.
func (AnalyticsRepo) SoldLines(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]SoldLine, error) {
	const op = "ler itens vendidos"
	rows, err := q.QueryContext(ctx,
		`SELECT s.id, s.discount_cents, i.id, i.product_id, i.product_name_snapshot, COALESCE(c.name, 'Sem categoria'),
		        i.qty, i.subtotal_cents, i.discount_cents, i.promo_discount_cents,
		        COALESCE(i.unit_cost_cents, p.cost_cents), i.unit_cost_cents IS NULL,
		        COALESCE((SELECT SUM(ri.qty) FROM return_items ri WHERE ri.sale_item_id = i.id), 0),
		        COALESCE((SELECT SUM(ri.amount_cents) FROM return_items ri WHERE ri.sale_item_id = i.id), 0)
		 FROM sales s JOIN sale_items i ON i.sale_id = s.id JOIN products p ON p.id = i.product_id
		 LEFT JOIN categories c ON c.id = p.category_id
		 WHERE s.status = 'concluida' AND s.created_at >= ? AND s.created_at < ? AND (? = 0 OR s.user_id = ?)
		 ORDER BY s.id, i.id`,
		FormatTime(start), FormatTime(end), userID, userID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []SoldLine{}
	for rows.Next() {
		var l SoldLine
		var est int
		if err := rows.Scan(&l.SaleID, &l.SaleDiscountCents, &l.Item.ID, &l.Item.ProductID, &l.Item.ProductNameSnapshot,
			&l.CategoryName, &l.Item.Qty, &l.Item.SubtotalCents, &l.Item.DiscountCents, &l.Item.PromoDiscountCents,
			&l.UnitCostCents, &est, &l.ReturnedQty, &l.ReturnedAmountCents); err != nil {
			return nil, wrap(op, err)
		}
		l.CostEstimated = est == 1
		out = append(out, l)
	}
	return out, wrap(op, rows.Err())
}

// IdleCandidates: produtos ativos com estoque e a data da última venda (vazia = nunca).
func (AnalyticsRepo) IdleCandidates(ctx context.Context, q DBTX) ([]models.IdleProduct, error) {
	const op = "listar produtos com estoque"
	rows, err := q.QueryContext(ctx,
		`SELECT p.id, p.name, COALESCE(c.name, 'Sem categoria'), p.stock_qty, p.cost_cents, p.price_cents,
		        (SELECT MAX(s.created_at) FROM sale_items i JOIN sales s ON s.id = i.sale_id
		          WHERE i.product_id = p.id AND s.status = 'concluida')
		 FROM products p LEFT JOIN categories c ON c.id = p.category_id
		 WHERE p.active = 1 AND p.stock_qty > 0`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.IdleProduct{}
	for rows.Next() {
		var v models.IdleProduct
		var last sql.NullString
		if err := rows.Scan(&v.ProductID, &v.Name, &v.CategoryName, &v.StockQty, &v.CostCents, &v.PriceCents, &last); err != nil {
			return nil, wrap(op, err)
		}
		if last.Valid {
			t, err := parseTime(last.String)
			if err != nil {
				return nil, wrap(op, err)
			}
			v.LastSaleAt = &t
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

// FiadoPart é uma compra no fiado de um cliente que ainda deve.
type FiadoPart struct {
	CustomerID  int64
	At          time.Time
	AmountCents int64
}

// DebtorsAndFiado: clientes com dívida e as compras no fiado deles, da mais nova para a mais antiga.
func (AnalyticsRepo) DebtorsAndFiado(ctx context.Context, q DBTX) ([]models.Customer, []FiadoPart, error) {
	const op = "ler dívidas do fiado"
	rows, err := q.QueryContext(ctx, `SELECT `+customerCols+` FROM customers WHERE credit_balance_cents > 0 ORDER BY credit_balance_cents DESC`)
	if err != nil {
		return nil, nil, wrap(op, err)
	}
	customers := []models.Customer{}
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			rows.Close()
			return nil, nil, wrap(op, err)
		}
		customers = append(customers, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, wrap(op, err)
	}
	rows.Close()
	rows, err = q.QueryContext(ctx,
		`SELECT s.customer_id, s.created_at, sp.amount_cents FROM sale_payments sp JOIN sales s ON s.id = sp.sale_id
		 JOIN customers c ON c.id = s.customer_id
		 WHERE sp.method = 'fiado' AND s.status = 'concluida' AND c.credit_balance_cents > 0
		 ORDER BY s.created_at DESC, s.id DESC`)
	if err != nil {
		return nil, nil, wrap(op, err)
	}
	defer rows.Close()
	parts := []FiadoPart{}
	for rows.Next() {
		var p FiadoPart
		var at string
		if err := rows.Scan(&p.CustomerID, &at, &p.AmountCents); err != nil {
			return nil, nil, wrap(op, err)
		}
		if p.At, err = parseTime(at); err != nil {
			return nil, nil, wrap(op, err)
		}
		parts = append(parts, p)
	}
	return customers, parts, wrap(op, rows.Err())
}

// PurchasesBySupplier soma as compras não estornadas do período por fornecedor.
func (AnalyticsRepo) PurchasesBySupplier(ctx context.Context, q DBTX, start, end time.Time) ([]models.SupplierPurchases, error) {
	const op = "totalizar compras por fornecedor"
	rows, err := q.QueryContext(ctx,
		`SELECT su.id, CASE WHEN su.trade_name <> '' THEN su.trade_name ELSE su.legal_name END,
		        COUNT(*), SUM(pu.total_cents), MAX(pu.created_at)
		 FROM purchases pu JOIN suppliers su ON su.id = pu.supplier_id
		 WHERE pu.cancelled_at IS NULL AND pu.created_at >= ? AND pu.created_at < ?
		 GROUP BY su.id ORDER BY SUM(pu.total_cents) DESC`,
		FormatTime(start), FormatTime(end))
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.SupplierPurchases{}
	for rows.Next() {
		var v models.SupplierPurchases
		var last string
		if err := rows.Scan(&v.SupplierID, &v.Name, &v.Count, &v.TotalCents, &last); err != nil {
			return nil, wrap(op, err)
		}
		t, err := parseTime(last)
		if err != nil {
			return nil, wrap(op, err)
		}
		v.LastAt = &t
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

// StockValue soma o estoque dos produtos ativos por categoria, a custo e a preço de venda.
func (AnalyticsRepo) StockValue(ctx context.Context, q DBTX) ([]models.StockValueRow, error) {
	const op = "valorizar estoque"
	rows, err := q.QueryContext(ctx,
		`SELECT COALESCE(c.name, 'Sem categoria'), COUNT(*), SUM(p.stock_qty), SUM(p.stock_qty * p.cost_cents),
		        SUM(p.stock_qty * p.price_cents), SUM(p.cost_cents = 0)
		 FROM products p LEFT JOIN categories c ON c.id = p.category_id
		 WHERE p.active = 1 AND p.stock_qty > 0
		 GROUP BY 1 ORDER BY 5 DESC`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.StockValueRow{}
	for rows.Next() {
		var v models.StockValueRow
		if err := rows.Scan(&v.CategoryName, &v.Products, &v.Qty, &v.CostCents, &v.PriceCents, &v.NoCost); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

// DiscountsByOperator separa, por operador, o desconto dado na venda e o das promoções.
func (AnalyticsRepo) DiscountsByOperator(ctx context.Context, q DBTX, start, end time.Time, userID int64) ([]models.DiscountByOperator, error) {
	const op = "totalizar descontos por operador"
	rows, err := q.QueryContext(ctx,
		`SELECT COALESCE(u.name, '(sem usuário)'), COUNT(*),
		        SUM(CASE WHEN s.discount_cents - COALESCE(pr.promo, 0) > 0 THEN 1 ELSE 0 END),
		        SUM(s.subtotal_cents), SUM(s.discount_cents - COALESCE(pr.promo, 0)), SUM(COALESCE(pr.promo, 0))
		 FROM sales s LEFT JOIN users u ON u.id = s.user_id
		 LEFT JOIN (SELECT sale_id, SUM(promo_discount_cents) AS promo FROM sale_items GROUP BY sale_id) pr ON pr.sale_id = s.id
		 WHERE s.status = 'concluida' AND s.created_at >= ? AND s.created_at < ? AND (? = 0 OR s.user_id = ?)
		 GROUP BY s.user_id ORDER BY 5 DESC`,
		FormatTime(start), FormatTime(end), userID, userID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.DiscountByOperator{}
	for rows.Next() {
		var v models.DiscountByOperator
		if err := rows.Scan(&v.UserName, &v.Sales, &v.SalesWithDiscount, &v.SubtotalCents, &v.DiscountCents, &v.PromoCents); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

// FiadoNotes: compras no fiado (concluídas) de um cliente, da mais antiga para a mais nova.
func (AnalyticsRepo) FiadoNotes(ctx context.Context, q DBTX, customerID int64) ([]models.FiadoNote, error) {
	const op = "listar notas do fiado"
	rows, err := q.QueryContext(ctx,
		`SELECT s.id, s.created_at, f.amt,
		        COALESCE((SELECT GROUP_CONCAT(i.qty || 'x ' || i.product_name_snapshot, ', ') FROM sale_items i WHERE i.sale_id = s.id), '')
		 FROM sales s JOIN (SELECT sale_id, SUM(amount_cents) AS amt FROM sale_payments WHERE method = 'fiado' GROUP BY sale_id) f ON f.sale_id = s.id
		 WHERE s.customer_id = ? AND s.status = 'concluida'
		 ORDER BY s.created_at, s.id`, customerID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.FiadoNote{}
	for rows.Next() {
		var n models.FiadoNote
		var at string
		if err := rows.Scan(&n.SaleID, &at, &n.AmountCents, &n.Description); err != nil {
			return nil, wrap(op, err)
		}
		if n.CreatedAt, err = parseTime(at); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, n)
	}
	return out, wrap(op, rows.Err())
}
