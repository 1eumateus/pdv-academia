// Caminho: pdv/internal/service/sale_payment_change.go

package service

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// paymentChangeTargets: formas para as quais uma venda pode ser trocada.
var paymentChangeTargets = []string{models.PaymentCash, models.PaymentPix, models.PaymentDebit, models.PaymentCreditCard}

// paymentChangeOptions diz para quais formas a venda ainda pode ser trocada e,
// se para nenhuma, o motivo. Só vale com o caixa da venda aberto e aberto hoje:
// caixa fechado já foi conferido e caixa de outro dia já não é o do movimento atual.
func paymentChangeOptions(ctx context.Context, q repository.DBTX, now time.Time, sale models.Sale, payments []models.SalePayment) ([]string, string, error) {
	repo := repository.SaleRepo{}
	if sale.Status == models.SaleCancelled {
		return nil, "A venda está cancelada.", nil
	}
	change, err := repo.PaymentChange(ctx, q, sale.ID)
	if err != nil {
		return nil, "", err
	}
	if change != nil {
		return nil, "A forma de pagamento desta venda já foi alterada uma vez.", nil
	}
	if len(payments) != 1 {
		return nil, "Só dá para trocar a forma de venda paga em uma única forma.", nil
	}
	from := payments[0].Method
	if from != models.PaymentCard && !slices.Contains(paymentChangeTargets, from) {
		return nil, "Venda a prazo ou com vale-troca não pode trocar de forma.", nil
	}
	returned, err := (repository.ReturnRepo{}).ReturnedQty(ctx, q, sale.ID)
	if err != nil {
		return nil, "", err
	}
	if len(returned) > 0 {
		return nil, "A venda já teve devolução.", nil
	}
	if sale.CashSessionID == nil {
		return nil, "A venda não está ligada a um caixa.", nil
	}
	if n, err := repo.SaleCashMovements(ctx, q, sale.ID); err != nil {
		return nil, "", err
	} else if n != 1 {
		return nil, "O lançamento da venda no caixa não confere.", nil
	}
	cash := repository.CashRepo{}
	session, err := cash.GetByID(ctx, q, *sale.CashSessionID)
	if err != nil {
		return nil, "", err
	}
	if session.Status != models.CashOpen {
		return nil, "O caixa desta venda já foi fechado.", nil
	}
	if session.OpenedAt.In(Location()).Format(dateLayout) != now.In(Location()).Format(dateLayout) {
		return nil, "O caixa desta venda não é de hoje.", nil
	}
	if from == models.PaymentCash {
		expected, err := cash.ExpectedCash(ctx, q, session.ID)
		if err != nil {
			return nil, "", err
		}
		if expected < payments[0].AmountCents {
			return nil, "O dinheiro desta venda já saiu do caixa (sangria).", nil
		}
	}
	out := []string{}
	for _, m := range paymentChangeTargets {
		if m != from {
			out = append(out, m)
		}
	}
	return out, "", nil
}

// ChangePaymentMethod troca a forma de pagamento de uma venda já lançada, uma
// única vez: na venda, no pagamento e no lançamento do caixa em que ela entrou.
func (s *SaleService) ChangePaymentMethod(ctx context.Context, saleID int64, method, reason string) (models.SaleDetails, error) {
	r := strings.TrimSpace(reason)
	if r == "" {
		return models.SaleDetails{}, invalid("Informe o motivo da troca.")
	}
	if len([]rune(r)) > maxReasonLen {
		return models.SaleDetails{}, invalid("O motivo deve ter no máximo %d caracteres.", maxReasonLen)
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		sale, err := s.sales.GetByID(ctx, tx, saleID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeSaleNotFound, "Venda", saleID), "")
		}
		authBy, err := requirePerm(ctx, now, PermChangePayment)
		if err != nil {
			return err
		}
		payments, err := s.sales.Payments(ctx, tx, saleID)
		if err != nil {
			return err
		}
		options, blocked, err := paymentChangeOptions(ctx, tx, now, sale, payments)
		if err != nil {
			return err
		}
		if blocked != "" {
			return newErr(CodeStateConflict, "%s", blocked)
		}
		if !slices.Contains(options, method) {
			return newErr(CodeStateConflict, "A venda #%d não pode ser trocada para %s.", saleID, methodName(method))
		}
		var installments int64
		if method == models.PaymentCreditCard {
			installments = 1
		}
		from := payments[0].Method
		if err := s.sales.ApplyPaymentChange(ctx, tx, saleID, from, method, installments, r, authBy, now); err != nil {
			return err
		}
		return audit(ctx, tx, now, authBy, "forma_pagamento_alterada", "venda", &saleID,
			map[string]any{"forma": from}, map[string]any{"forma": method, "motivo": r})
	})
	if err != nil {
		return models.SaleDetails{}, err
	}
	return s.Details(ctx, saleID)
}
