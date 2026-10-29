// Caminho: pdv/internal/service/credit_service.go

package service

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// CreditService trata recebimentos de fiado. Recebimentos entram no caixa da
// sessão aberta, então exigem caixa aberto. Não geram movimentação de estoque e
// não alteram a venda original.
type CreditService struct {
	Base
	customers repository.CustomerRepo
	credits   repository.CreditRepo
	cash      repository.CashRepo
}

func NewCreditService(b Base) *CreditService { return &CreditService{Base: b} }

// PayCustomerCredit registra um recebimento e abate a dívida na mesma Tx.
// Um RequestID repetido devolve o recebimento já registrado.
func (s *CreditService) PayCustomerCredit(ctx context.Context, in models.PayCreditInput) (models.CreditPayment, error) {
	customerID, amountCents, method := in.CustomerID, in.AmountCents, in.PaymentMethod
	if err := validateAmount(amountCents, "O valor do recebimento"); err != nil {
		return models.CreditPayment{}, err
	}
	if !validCashMethod(method) {
		return models.CreditPayment{}, invalid("Forma de pagamento inválida para recebimento: use dinheiro, pix ou cartão.")
	}
	reqID, err := cleanRequestID(in.RequestID)
	if err != nil {
		return models.CreditPayment{}, err
	}
	n := trimPtr(&in.Notes)
	var out models.CreditPayment
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if reqID != nil {
			id, err := s.credits.FindByRequestID(ctx, tx, *reqID)
			if err == nil {
				out, err = s.credits.GetByID(ctx, tx, id)
				return err
			}
			if !isNotFound(err) {
				return err
			}
		}
		session, err := requireOpenCash(ctx, tx)
		if err != nil {
			return err
		}
		c, err := s.customers.GetByID(ctx, tx, customerID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", customerID), "")
		}
		if c.CreditBalanceCents == 0 {
			return newErr(CodeInvalid, "O cliente %s não possui dívida em aberto.", c.Name)
		}
		if amountCents > c.CreditBalanceCents {
			return newErr(CodeInvalid, "O valor recebido (%s) é maior que a dívida atual (%s).",
				FormatBRL(amountCents), FormatBRL(c.CreditBalanceCents))
		}
		// Notas escolhidas: o que está em aberto em cada uma, antes deste recebimento.
		var chosen []models.FiadoNote
		if len(in.SaleIDs) > 0 {
			notes, err := currentNotes(ctx, tx, c, now)
			if err != nil {
				return err
			}
			for _, sid := range in.SaleIDs {
				idx := slices.IndexFunc(notes, func(x models.FiadoNote) bool { return x.SaleID == sid && sid != 0 })
				if idx < 0 || notes[idx].OpenCents == 0 {
					return invalid("A nota nº %d não está em aberto para %s.", sid, c.Name)
				}
				if !slices.ContainsFunc(chosen, func(x models.FiadoNote) bool { return x.SaleID == sid }) {
					chosen = append(chosen, notes[idx])
				}
			}
			slices.SortFunc(chosen, func(a, b models.FiadoNote) int { return a.CreatedAt.Compare(b.CreatedAt) })
		}
		id, err := s.credits.Insert(ctx, tx, customerID, amountCents, method, models.CreditPaymentReceipt, nil, n, reqID, now)
		if err != nil {
			return err
		}
		// O valor vai para as notas escolhidas (mais antigas primeiro); se sobrar,
		// quita as demais notas pela regra de sempre.
		left := amountCents
		for _, note := range chosen {
			take := min(left, note.OpenCents)
			if take == 0 {
				break
			}
			if err := s.credits.InsertAllocation(ctx, tx, id, note.SaleID, take); err != nil {
				return err
			}
			left -= take
		}
		ok, err := s.customers.ReduceDebt(ctx, tx, customerID, amountCents, now)
		if err != nil {
			return err
		}
		if !ok {
			return newErr(CodeStateConflict, "A dívida do cliente mudou durante a operação. Confira o saldo e tente novamente.")
		}
		if _, err := s.cash.InsertMovement(ctx, tx, repository.NewCashMovement{
			SessionID: session.ID, Type: models.CashCreditPayment, PaymentMethod: method, AmountCents: amountCents,
			Description: ptr("Recebimento a prazo: " + c.Name), ReferenceType: ptr("credit_payment"), ReferenceID: &id,
		}, now); err != nil {
			return err
		}
		out, err = s.credits.GetByID(ctx, tx, id)
		return err
	})
	return out, err
}

// ReversePayment corrige um recebimento lançado por engano: registra um estorno
// que aponta para o original, restaura a dívida e lança a saída no caixa. O
// recebimento original nunca é apagado.
func (s *CreditService) ReversePayment(ctx context.Context, paymentID int64, reason string) (models.CreditPayment, error) {
	r := trimPtr(&reason)
	if r == nil {
		return models.CreditPayment{}, invalid("Informe o motivo do estorno.")
	}
	var out models.CreditPayment
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		session, err := requireOpenCash(ctx, tx)
		if err != nil {
			return err
		}
		p, err := s.credits.GetByID(ctx, tx, paymentID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Recebimento %d não encontrado.", paymentID), "")
		}
		if p.Kind != models.CreditPaymentReceipt {
			return invalid("Um estorno não pode ser estornado.")
		}
		if p.Reversed {
			return newErr(CodeStateConflict, "Este recebimento já foi estornado.")
		}
		authBy, err := requirePerm(ctx, now, PermReverseCredit)
		if err != nil {
			return err
		}
		id, err := s.credits.Insert(ctx, tx, p.CustomerID, p.AmountCents, p.PaymentMethod,
			models.CreditPaymentReversal, &p.ID, r, nil, now)
		if err != nil {
			if errors.Is(err, repository.ErrUnique) {
				return newErr(CodeStateConflict, "Este recebimento já foi estornado.")
			}
			return err
		}
		if err := s.customers.AddDebt(ctx, tx, p.CustomerID, p.AmountCents, now); err != nil {
			return err
		}
		if _, err := s.cash.InsertMovement(ctx, tx, repository.NewCashMovement{
			SessionID: session.ID, Type: models.CashPaymentReversal, PaymentMethod: p.PaymentMethod,
			AmountCents: -p.AmountCents, Description: ptr("Estorno de recebimento: " + *r),
			ReferenceType: ptr("credit_payment"), ReferenceID: &id,
		}, now); err != nil {
			return err
		}
		if err := audit(ctx, tx, now, authBy, "estorno_fiado", "recebimento", &p.ID, nil,
			map[string]any{"cliente": p.CustomerName, "valor": p.AmountCents, "motivo": *r}); err != nil {
			return err
		}
		out, err = s.credits.GetByID(ctx, tx, id)
		return err
	})
	return out, err
}
