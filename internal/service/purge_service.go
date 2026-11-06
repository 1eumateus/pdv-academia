// Caminho: catraca/internal/service/purge_service.go

package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// PurgeService apaga vendas de teste: as vendas de um operador (ou de todos)
// num dia e o caixa que ele abriu nesse dia, devolvendo o estoque e desfazendo
// fiado, vale-troca, orçamento e mensalidade. Só o administrador.
//
// O banco foi feito para não apagar venda (gatilhos *_no_delete). Aqui eles
// saem e voltam dentro da mesma transação: se algo falhar, nada muda. O que
// estiver ligado a outro dia, operador ou caixa (devolução, recebimento de
// fiado, estorno em outro caixa…) bloqueia: apagar deixaria aquele caixa furado.
type PurgeService struct{ Base }

func NewPurgeService(b Base) *PurgeService { return &PurgeService{Base: b} }

// Tabelas cujas linhas a limpeza apaga (e cujos gatilhos de não-apagar saem por um instante).
var purgeTables = []string{"sales", "sale_items", "sale_payments", "sale_payment_changes", "sale_deliveries", "stock_movements", "cash_movements", "cash_sessions"}

// Preview mostra o que seria apagado (e o que impede). Não muda nada.
func (s *PurgeService) Preview(ctx context.Context, date string, userID int64) (models.TestPurge, error) {
	if _, err := requirePerm(ctx, s.Now(), PermPurge); err != nil {
		return models.TestPurge{}, err
	}
	return s.plan(ctx, s.DB, date, userID)
}

func idList(ids []int64) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = fmt.Sprint(id)
	}
	return strings.Join(out, ",")
}

// plan monta a limpeza de date (AAAA-MM-DD, até hoje) para o operador
// (userID 0 = todos). Caixa aberto por ele nesse dia sai inteiro só se nele
// sobrar apenas o que está sendo apagado (as vendas, suprimento e sangria); os
// demais ficam e perdem só os lançamentos dessas vendas (TouchedSessions).
func (s *PurgeService) plan(ctx context.Context, q repository.DBTX, date string, userID int64) (models.TestPurge, error) {
	out := models.TestPurge{Date: date, UserID: userID, Sales: []models.TestPurgeSale{}, Sessions: []models.TestPurgeSession{},
		TouchedSessions: []int64{}, Blockers: []string{}}
	from, to, err := DayRange(date)
	if err != nil {
		return out, err
	}
	if date > s.Now().In(Location()).Format(dateLayout) {
		return out, invalid("O dia não pode ser no futuro.")
	}
	f, t := repository.FormatTime(from), repository.FormatTime(to)

	rows, err := q.QueryContext(ctx, `SELECT s.id, s.created_at, s.total_cents, s.payment_method, s.status, COALESCE(c.name, '')
		FROM sales s LEFT JOIN customers c ON c.id = s.customer_id
		WHERE (? = 0 OR s.user_id = ?) AND s.created_at >= ? AND s.created_at < ? ORDER BY s.id`, userID, userID, f, t)
	if err != nil {
		return out, err
	}
	saleIDs := []int64{}
	for rows.Next() {
		var (
			v  models.TestPurgeSale
			at string
		)
		if err := rows.Scan(&v.ID, &at, &v.TotalCents, &v.PaymentMethod, &v.Status, &v.CustomerName); err != nil {
			rows.Close()
			return out, err
		}
		v.At, _ = time.Parse(repository.TimeLayout, at)
		out.Sales = append(out.Sales, v)
		saleIDs = append(saleIDs, v.ID)
		if v.Status == models.SaleCompleted {
			out.TotalCents += v.TotalCents
		}
	}
	rows.Close()
	sales := "(" + idList(saleIDs) + ")"

	// Caixas abertos pelo operador no dia que só têm o que está sendo apagado.
	rows, err = q.QueryContext(ctx, `SELECT s.id, s.opened_at, s.status, (SELECT COUNT(*) FROM cash_movements m WHERE m.session_id = s.id)
		FROM cash_sessions s WHERE (? = 0 OR COALESCE(s.owner_user_id, s.opened_by_user_id) = ?) AND s.opened_at >= ? AND s.opened_at < ?
		AND NOT EXISTS (SELECT 1 FROM sales x WHERE x.cash_session_id = s.id AND x.id NOT IN `+sales+`)
		AND NOT EXISTS (SELECT 1 FROM returns r WHERE r.cash_session_id = s.id)
		AND NOT EXISTS (SELECT 1 FROM cash_movements m WHERE m.session_id = s.id AND m.type NOT IN ('suprimento', 'sangria')
			AND NOT (m.reference_type = 'sale' AND m.reference_id IN `+sales+`))
		ORDER BY s.id`, userID, userID, f, t)
	if err != nil {
		return out, err
	}
	sessIDs := []int64{}
	for rows.Next() {
		var (
			v  models.TestPurgeSession
			at string
		)
		if err := rows.Scan(&v.ID, &at, &v.Status, &v.Movements); err != nil {
			rows.Close()
			return out, err
		}
		v.OpenedAt, _ = time.Parse(repository.TimeLayout, at)
		out.Sessions = append(out.Sessions, v)
		sessIDs = append(sessIDs, v.ID)
	}
	rows.Close()

	if len(saleIDs) == 0 {
		return out, nil
	}
	// Caixas que ficam, mas perdem os lançamentos dessas vendas.
	rows, err = q.QueryContext(ctx, `SELECT DISTINCT session_id FROM cash_movements
		WHERE reference_type = 'sale' AND reference_id IN `+sales+` AND session_id NOT IN (`+idList(sessIDs)+`) ORDER BY session_id`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		out.TouchedSessions = append(out.TouchedSessions, id)
	}
	rows.Close()

	// Mexe na conta do cliente: precisa ser desfeito antes, à mão.
	rows, err = q.QueryContext(ctx, `SELECT 'A venda #' || sale_id || ' teve devolução (desfaça antes).' FROM returns WHERE sale_id IN `+sales+`
		UNION SELECT 'A venda #' || sale_id || ' a prazo já recebeu pagamento de fiado (estorne o recebimento antes).' FROM credit_allocations WHERE sale_id IN `+sales+`
		UNION SELECT 'A venda #' || s.id || ' a prazo já recebeu pagamento de fiado (estorne o recebimento antes).' FROM sales s
		 WHERE s.id IN `+sales+` AND s.status = 'concluida'
		 AND EXISTS (SELECT 1 FROM sale_payments sp WHERE sp.sale_id = s.id AND sp.method = 'fiado')
		 AND EXISTS (SELECT 1 FROM credit_payments cp WHERE cp.customer_id = s.customer_id AND cp.created_at >= s.created_at)`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return out, err
		}
		if len(out.Blockers) < 30 {
			out.Blockers = append(out.Blockers, msg)
		}
	}
	return out, rows.Err()
}

// Apply apaga de vez o que Preview mostrou. Recusa se houver bloqueio ou se
// nada mudou de lugar desde a prévia (mesmas vendas e caixas).
func (s *PurgeService) Apply(ctx context.Context, date string, userID int64, expectSales, expectSessions int) (models.TestPurge, error) {
	var out models.TestPurge
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermPurge)
		if err != nil {
			return err
		}
		if out, err = s.plan(ctx, tx, date, userID); err != nil {
			return err
		}
		if len(out.Blockers) > 0 {
			return newErr(CodeStateConflict, "%s", out.Blockers[0])
		}
		if len(out.Sales) != expectSales || len(out.Sessions) != expectSessions {
			return newErr(CodeStateConflict, "As vendas ou o caixa mudaram desde a prévia. Confira de novo.")
		}
		if len(out.Sales) == 0 && len(out.Sessions) == 0 {
			return invalid("Não há vendas nem caixa para apagar neste dia.")
		}
		var custs repository.CustomerRepo
		for _, v := range out.Sales {
			if v.Status != models.SaleCompleted {
				continue // cancelada: estoque, fiado e mensalidade já foram desfeitos no cancelamento
			}
			if err := undoSubscriptionPayment(ctx, tx, v.ID, now); err != nil {
				return err
			}
			if _, err := (repository.QuoteRepo{}).ReopenBySale(ctx, tx, v.ID, now); err != nil {
				return err
			}
			var customerID sql.NullInt64
			var fiado, vale int64
			if err := tx.QueryRowContext(ctx, `SELECT s.customer_id,
				COALESCE((SELECT SUM(amount_cents) FROM sale_payments WHERE sale_id = s.id AND method = 'fiado'), 0),
				COALESCE((SELECT SUM(amount_cents) FROM sale_payments WHERE sale_id = s.id AND method = 'vale_troca'), 0)
				FROM sales s WHERE s.id = ?`, v.ID).Scan(&customerID, &fiado, &vale); err != nil {
				return err
			}
			if customerID.Valid && fiado > 0 {
				if ok, err := custs.ReduceDebt(ctx, tx, customerID.Int64, fiado, now); err != nil {
					return err
				} else if !ok {
					return newErr(CodeStateConflict, "A venda #%d a prazo não bate com a dívida atual do cliente.", v.ID)
				}
			}
			if customerID.Valid && vale > 0 {
				if err := custs.AddStoreCredit(ctx, tx, customerID.Int64, vale, now); err != nil {
					return err
				}
			}
		}
		saleIDs, sessIDs := []int64{}, []int64{}
		for _, v := range out.Sales {
			saleIDs = append(saleIDs, v.ID)
		}
		for _, v := range out.Sessions {
			sessIDs = append(sessIDs, v.ID)
		}
		sales, sess := "("+idList(saleIDs)+")", "("+idList(sessIDs)+")"
		err = withoutDeleteGuards(ctx, tx, purgeTables, func() error {
			stmts := []string{
				// Estoque: desfaz as movimentações das vendas (venda e cancelamento).
				`UPDATE products SET stock_qty = stock_qty - (SELECT SUM(m.qty_change) FROM stock_movements m
				 WHERE m.product_id = products.id AND m.reference_type = 'sale' AND m.reference_id IN ` + sales + `)
				 WHERE track_stock = 1 AND id IN (SELECT product_id FROM stock_movements WHERE reference_type = 'sale' AND reference_id IN ` + sales + `)`,
				`DELETE FROM stock_movements WHERE reference_type = 'sale' AND reference_id IN ` + sales,
				`DELETE FROM cash_movements WHERE session_id IN ` + sess + ` OR (reference_type = 'sale' AND reference_id IN ` + sales + `)`,
				`DELETE FROM sale_payment_changes WHERE sale_id IN ` + sales,
				`DELETE FROM sale_payments WHERE sale_id IN ` + sales,
				`DELETE FROM sale_items WHERE sale_id IN ` + sales,
				`DELETE FROM sale_deliveries WHERE sale_id IN ` + sales,
				`DELETE FROM sales WHERE id IN ` + sales,
				`DELETE FROM cash_sessions WHERE id IN ` + sess,
			}
			for _, q := range stmts {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return fromRepo(fmt.Errorf("apagar vendas de teste: %w", err), nil, "")
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		out.Applied = true
		return audit(ctx, tx, now, authBy, "vendas_teste_apagadas", "usuario", &userID, nil, map[string]any{
			"dia": out.Date, "vendas": saleIDs, "caixas": sessIDs, "total": out.TotalCents})
	})
	return out, err
}

// withoutDeleteGuards tira os gatilhos que proíbem apagar nessas tabelas (os
// *_no_delete e qualquer outro BEFORE DELETE, como o dos itens de devolução),
// roda fn e os recria com o mesmo SQL. Tudo dentro de tx: se fn falhar, eles voltam.
func withoutDeleteGuards(ctx context.Context, tx *sql.Tx, tables []string, fn func() error) error {
	rows, err := tx.QueryContext(ctx, `SELECT name, sql FROM sqlite_master WHERE type = 'trigger'
		AND (name LIKE '%\_no\_delete' ESCAPE '\' OR UPPER(sql) LIKE '%BEFORE DELETE ON%')
		AND tbl_name IN ('`+strings.Join(tables, "','")+`')`)
	if err != nil {
		return err
	}
	type trigger struct{ name, sql string }
	var list []trigger
	for rows.Next() {
		var tr trigger
		if err := rows.Scan(&tr.name, &tr.sql); err != nil {
			rows.Close()
			return err
		}
		list = append(list, tr)
	}
	rows.Close()
	for _, tr := range list {
		if _, err := tx.ExecContext(ctx, `DROP TRIGGER `+tr.name); err != nil {
			return err
		}
	}
	if err := fn(); err != nil {
		return err
	}
	for _, tr := range list {
		if _, err := tx.ExecContext(ctx, tr.sql); err != nil {
			return fmt.Errorf("recriar a trava %s: %w", tr.name, err)
		}
	}
	return nil
}
