// Caminho: catraca/internal/service/customer_purge.go

package service

import (
	"context"
	"database/sql"
	"fmt"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// PermPurgeCustomer: apagar uma pessoa com todo o histórico. De risco.
const PermPurgeCustomer = "apagar_pessoa_historico"

// Tabelas que a exclusão completa apaga (os gatilhos de não-apagar saem por um instante).
var customerPurgeTables = []string{"sales", "sale_items", "sale_payments", "sale_payment_changes", "sale_deliveries",
	"stock_movements", "cash_movements", "credit_payments", "credit_allocations", "returns", "return_items",
	"quotes", "subscription_invoices"}

// customerPurgeSets: subconsultas com os ids de tudo que é da pessoa (? = id do cliente).
const (
	cpSales    = `(SELECT id FROM sales WHERE customer_id = ?)`
	cpReturns  = `(SELECT id FROM returns WHERE customer_id = ? OR sale_id IN (SELECT id FROM sales WHERE customer_id = ?))`
	cpPayments = `(SELECT id FROM credit_payments WHERE customer_id = ?)`
	cpSubs     = `(SELECT id FROM subscriptions WHERE customer_id = ?)`
	cpInvoices = `(SELECT id FROM subscription_invoices WHERE subscription_id IN (SELECT id FROM subscriptions WHERE customer_id = ?))`
)

// PurgeCustomerPreview mostra o que some ao apagar a pessoa com todo o histórico. Não muda nada.
func (s *CatracaService) PurgeCustomerPreview(ctx context.Context, customerID int64) (models.CustomerPurge, error) {
	if _, err := requirePerm(ctx, s.Now(), PermPurgeCustomer); err != nil {
		return models.CustomerPurge{}, err
	}
	return s.purgePlan(ctx, s.DB, customerID)
}

func (s *CatracaService) purgePlan(ctx context.Context, q repository.DBTX, id int64) (models.CustomerPurge, error) {
	out := models.CustomerPurge{CustomerID: id}
	c, err := s.customers.customers.GetByID(ctx, q, id)
	if err != nil {
		return out, fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", id), "")
	}
	out.Name, out.DebtCents, out.StoreCreditCents = c.Name, c.CreditBalanceCents, c.StoreCreditCents
	err = q.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM sales WHERE customer_id = ?),
		(SELECT COALESCE(SUM(total_cents), 0) FROM sales WHERE customer_id = ? AND status = 'concluida'),
		(SELECT COUNT(*) FROM subscription_invoices WHERE id IN `+cpInvoices+`),
		(SELECT COUNT(*) FROM subscriptions WHERE customer_id = ?),
		(SELECT COUNT(*) FROM credit_payments WHERE customer_id = ?),
		(SELECT COUNT(*) FROM returns WHERE id IN `+cpReturns+`),
		(SELECT COUNT(*) FROM catraca_passagens WHERE customer_id = ?),
		(SELECT COUNT(DISTINCT session_id) FROM cash_movements WHERE
			(reference_type = 'sale' AND reference_id IN `+cpSales+`) OR
			(reference_type = 'return' AND reference_id IN `+cpReturns+`) OR
			(reference_type = 'credit_payment' AND reference_id IN `+cpPayments+`)),
		(SELECT device_user_id IS NOT NULL FROM catraca_alunos WHERE customer_id = ?)`,
		id, id, id, id, id, id, id, id, id, id, id, id, id).Scan(&out.Sales, &out.SalesTotalCents, &out.Invoices,
		&out.Subscriptions, &out.CreditPayments, &out.Returns, &out.Passages, &out.CashSessions, nullBool{&out.HasFace})
	return out, err
}

// nullBool lê um booleano que pode vir nulo (nulo = false).
type nullBool struct{ p *bool }

func (n nullBool) Scan(v any) error {
	var b sql.NullBool
	if err := b.Scan(v); err != nil {
		return err
	}
	*n.p = b.Valid && b.Bool
	return nil
}

// PurgeCustomer apaga de vez a pessoa e tudo que é dela: vendas (o estoque
// volta), mensalidades e assinaturas, fiado e recebimentos, devoluções,
// orçamentos, passagens e o rosto no leitor. Os caixas onde isso entrou perdem
// esses lançamentos. A auditoria guarda que foi apagada (e por quem).
// expectSales confere que nada mudou desde a prévia.
func (s *CatracaService) PurgeCustomer(ctx context.Context, customerID int64, expectSales int) error {
	var faceID *int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermPurgeCustomer)
		if err != nil {
			return err
		}
		plan, err := s.purgePlan(ctx, tx, customerID)
		if err != nil {
			return err
		}
		if plan.Sales != expectSales {
			return newErr(CodeStateConflict, "As vendas de %s mudaram desde a prévia. Confira de novo.", plan.Name)
		}
		m, err := s.repo.Member(ctx, tx, customerID)
		if err != nil && !isNotFound(err) {
			return err
		}
		faceID = m.DeviceUserID
		id := customerID
		err = withoutDeleteGuards(ctx, tx, customerPurgeTables, func() error {
			stmts := []struct {
				sql  string
				args int // quantas vezes o id do cliente entra
			}{
				// Estoque: desfaz o que as vendas e devoluções mexeram.
				{`UPDATE products SET stock_qty = stock_qty - (SELECT SUM(m.qty_change) FROM stock_movements m WHERE m.product_id = products.id AND
					((m.reference_type = 'sale' AND m.reference_id IN ` + cpSales + `) OR (m.reference_type = 'return' AND m.reference_id IN ` + cpReturns + `)))
				 WHERE track_stock = 1 AND id IN (SELECT product_id FROM stock_movements WHERE
					(reference_type = 'sale' AND reference_id IN ` + cpSales + `) OR (reference_type = 'return' AND reference_id IN ` + cpReturns + `))`, 6},
				{`DELETE FROM stock_movements WHERE (reference_type = 'sale' AND reference_id IN ` + cpSales + `)
					OR (reference_type = 'return' AND reference_id IN ` + cpReturns + `)`, 3},
				{`DELETE FROM cash_movements WHERE (reference_type = 'sale' AND reference_id IN ` + cpSales + `)
					OR (reference_type = 'return' AND reference_id IN ` + cpReturns + `)
					OR (reference_type = 'credit_payment' AND reference_id IN ` + cpPayments + `)`, 4},
				{`DELETE FROM credit_allocations WHERE payment_id IN ` + cpPayments + ` OR sale_id IN ` + cpSales, 2},
				{`DELETE FROM credit_payments WHERE customer_id = ?`, 1},
				{`DELETE FROM return_items WHERE return_id IN ` + cpReturns, 2},
				{`DELETE FROM returns WHERE id IN ` + cpReturns, 2},
				{`DELETE FROM fatura_fora_caixa WHERE invoice_id IN ` + cpInvoices, 1},
				{`DELETE FROM subscription_invoices WHERE id IN ` + cpInvoices, 1},
				// Mensalidade de outra pessoa paga numa venda desta (raro): só perde o vínculo.
				{`UPDATE subscription_invoices SET sale_id = NULL WHERE sale_id IN ` + cpSales, 1},
				{`DELETE FROM catraca_regras WHERE subscription_id IN ` + cpSubs, 1},
				{`DELETE FROM subscriptions WHERE customer_id = ?`, 1},
				{`DELETE FROM quote_items WHERE quote_id IN (SELECT id FROM quotes WHERE customer_id = ? OR sale_id IN ` + cpSales + `)`, 2},
				{`DELETE FROM quotes WHERE customer_id = ? OR sale_id IN ` + cpSales, 2},
				{`DELETE FROM sale_payment_changes WHERE sale_id IN ` + cpSales, 1},
				{`DELETE FROM sale_payments WHERE sale_id IN ` + cpSales, 1},
				{`DELETE FROM sale_items WHERE sale_id IN ` + cpSales, 1},
				{`DELETE FROM sale_deliveries WHERE sale_id IN ` + cpSales, 1},
				{`DELETE FROM sales WHERE customer_id = ?`, 1},
				{`DELETE FROM catraca_passagens WHERE customer_id = ?`, 1},
				{`DELETE FROM catraca_alunos WHERE customer_id = ?`, 1},
				{`DELETE FROM customers WHERE id = ?`, 1},
			}
			for i, st := range stmts {
				args := make([]any, st.args)
				for j := range args {
					args[j] = id
				}
				if _, err := tx.ExecContext(ctx, st.sql, args...); err != nil {
					return fromRepo(fmt.Errorf("apagar pessoa com histórico, passo %d: %w", i+1, err), nil, "")
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		// O aparelho por último, ainda dentro da transação: se ele falhar, nada é apagado.
		if c, dev := s.current(); faceID != nil && c.DeviceHost != "" {
			if err := dev.DeleteUser(ctx, *faceID); err != nil {
				return deviceErr(fmt.Errorf("não foi possível apagar a pessoa do aparelho: %w", err))
			}
		}
		return audit(ctx, tx, now, authBy, "pessoa_apagada_com_historico", "cliente", &id, map[string]any{
			"nome": plan.Name, "vendas": plan.Sales, "total_vendido": plan.SalesTotalCents, "mensalidades": plan.Invoices,
			"recebimentos_fiado": plan.CreditPayments, "devolucoes": plan.Returns, "passagens": plan.Passages,
			"fiado_em_aberto": plan.DebtCents, "vale_troca": plan.StoreCreditCents}, nil)
	})
	if err == nil && faceID != nil {
		s.forgetPhoto(*faceID)
	}
	return err
}
