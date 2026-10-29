// Caminho: pdv/internal/service/return_service.go

package service

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"strings"

	"pdv/internal/models"
	"pdv/internal/repository"
)

type ReturnService struct {
	Base
	sales     repository.SaleRepo
	returns   repository.ReturnRepo
	customers repository.CustomerRepo
	cash      repository.CashRepo
}

func NewReturnService(b Base) *ReturnService { return &ReturnService{Base: b} }

// mulDiv calcula a*b/c arredondando para baixo, sem estourar int64 no meio.
func mulDiv(a, b, c int64) int64 {
	r := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	return r.Quo(r, big.NewInt(c)).Int64()
}

// paidByItem diz quanto o cliente pagou por cada item inteiro: o valor do item
// já com o desconto dele e a promoção, menos a parte do desconto geral da venda
// (rateado pelo valor de cada item; a soma bate com o total da venda).
func paidByItem(sale models.Sale, items []models.SaleItem) map[int64]int64 {
	nets := make([]int64, len(items))
	var sumNet, itemDisc int64
	for i, it := range items {
		nets[i] = it.SubtotalCents - it.DiscountCents - it.PromoDiscountCents
		sumNet += nets[i]
		itemDisc += it.DiscountCents + it.PromoDiscountCents
	}
	general := sale.DiscountCents - itemDisc
	out := make(map[int64]int64, len(items))
	var cum, given int64
	for i, it := range items {
		cum += nets[i]
		share := int64(0)
		if sumNet > 0 && general > 0 {
			share = mulDiv(general, cum, sumNet) - given
		}
		given += share
		out[it.ID] = nets[i] - share
	}
	return out
}

// refundFor: quanto devolver por q unidades quando a já foram devolvidas.
// Pela diferença acumulada, devolver tudo em partes soma exatamente o pago.
func refundFor(paid, soldQty, already, q int64) int64 {
	return mulDiv(paid, already+q, soldQty) - mulDiv(paid, already, soldQty)
}

// Info monta a tela de devolução: itens com o que ainda pode voltar.
func (s *ReturnService) Info(ctx context.Context, saleID int64) (models.ReturnInfo, error) {
	var out models.ReturnInfo
	sale, err := s.sales.GetByID(ctx, s.DB, saleID)
	if err != nil {
		return out, fromRepo(err, notFoundErr(CodeSaleNotFound, "Venda", saleID), "")
	}
	items, err := s.sales.Items(ctx, s.DB, saleID)
	if err != nil {
		return out, err
	}
	returned, err := s.returns.ReturnedQty(ctx, s.DB, saleID)
	if err != nil {
		return out, err
	}
	paid := paidByItem(sale, items)
	out.Sale = sale
	for _, it := range items {
		out.Items = append(out.Items, models.ReturnableItem{SaleItemID: it.ID, ProductName: it.ProductNameSnapshot,
			Qty: it.Qty, ReturnedQty: returned[it.ID], PaidCents: paid[it.ID]})
	}
	payments, err := s.sales.Payments(ctx, s.DB, saleID)
	if err != nil {
		return out, err
	}
	for _, p := range payments {
		out.HasFiado = out.HasFiado || p.Method == models.PaymentCredit
		out.Methods = append(out.Methods, p.Method)
	}
	if out.CardLeftCents, _, err = s.cardLeft(ctx, s.DB, saleID, payments); err != nil {
		return out, err
	}
	if sale.CustomerID != nil {
		c, err := s.customers.GetByID(ctx, s.DB, *sale.CustomerID)
		if err != nil {
			return out, err
		}
		out.Customer = &c
	}
	out.Returns, err = s.returns.ListBySale(ctx, s.DB, saleID)
	return out, err
}

func isCard(m string) bool {
	return m == models.PaymentDebit || m == models.PaymentCreditCard || m == models.PaymentCard
}

// cardLeft: quanto da parte em cartão ainda pode ser estornado e em qual forma
// (débito/crédito) o estorno entra no caixa.
func (s *ReturnService) cardLeft(ctx context.Context, q repository.DBTX, saleID int64, payments []models.SalePayment) (int64, string, error) {
	var paid int64
	method := ""
	for _, p := range payments {
		if isCard(p.Method) {
			paid += p.AmountCents
			if method == "" {
				method = p.Method
			}
		}
	}
	if paid == 0 {
		return 0, "", nil
	}
	done, err := s.returns.RefundedBy(ctx, q, saleID, models.RefundCard)
	return max(0, paid-done), method, err
}

// Create registra a devolução numa transação: os itens voltam ao estoque e o
// valor pago por eles é reembolsado em vale-troca, dinheiro ou Pix (saem do
// caixa), estorno no cartão ou abatendo o fiado. Não deixa devolver mais do
// que foi vendido.
func (s *ReturnService) Create(ctx context.Context, in models.ReturnInput) (models.Return, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return models.Return{}, invalid("Informe o motivo da devolução.")
	}
	if len([]rune(reason)) > maxReasonLen {
		return models.Return{}, invalid("O motivo deve ter no máximo %d caracteres.", maxReasonLen)
	}
	switch in.RefundMethod {
	case models.RefundStoreCredit, models.RefundCash, models.RefundCredit, models.RefundPix, models.RefundCard:
	default:
		return models.Return{}, invalid("Escolha como devolver o valor: vale-troca, dinheiro, Pix, estorno no cartão ou abater na dívida a prazo.")
	}
	want := map[int64]int64{}
	for _, it := range in.Items {
		if it.Qty == 0 {
			continue
		}
		if err := validateQty(it.Qty); err != nil {
			return models.Return{}, err
		}
		want[it.SaleItemID] += it.Qty
	}
	if len(want) == 0 {
		return models.Return{}, invalid("Escolha pelo menos um item e a quantidade a devolver.")
	}

	var returnID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermReturns)
		if err != nil {
			return err
		}
		sale, err := s.sales.GetByID(ctx, tx, in.SaleID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeSaleNotFound, "Venda", in.SaleID), "")
		}
		if sale.Status == models.SaleCancelled {
			return newErr(CodeSaleCancelled, "A venda #%d foi cancelada; não há o que devolver.", sale.ID)
		}
		items, err := s.sales.Items(ctx, tx, sale.ID)
		if err != nil {
			return err
		}
		returned, err := s.returns.ReturnedQty(ctx, tx, sale.ID)
		if err != nil {
			return err
		}
		paid := paidByItem(sale, items)
		var lines []repository.NewReturnItem
		var total int64
		for _, it := range items {
			q, ok := want[it.ID]
			if !ok {
				continue
			}
			delete(want, it.ID)
			if left := it.Qty - returned[it.ID]; q > left {
				return invalid("De %q só %s para devolver (vendido %d, já devolvido %d).",
					it.ProductNameSnapshot, plural(left, "resta 1", fmt.Sprintf("restam %d", left)), it.Qty, returned[it.ID])
			}
			amount := refundFor(paid[it.ID], it.Qty, returned[it.ID], q)
			total += amount
			lines = append(lines, repository.NewReturnItem{SaleItemID: it.ID, ProductID: it.ProductID,
				ProductName: it.ProductNameSnapshot, Qty: q, AmountCents: amount})
		}
		if len(want) > 0 {
			return invalid("Item que não pertence à venda #%d.", sale.ID)
		}

		nr := repository.NewReturn{SaleID: sale.ID, RefundMethod: in.RefundMethod, TotalCents: total, Reason: reason}
		cashMethod := "" // forma do movimento de caixa (vazio = não mexe no caixa)
		switch in.RefundMethod {
		case models.RefundStoreCredit:
			nr.CustomerID = validCustomerOrNil(in.CustomerID)
			if nr.CustomerID == nil {
				nr.CustomerID = sale.CustomerID
			}
			if nr.CustomerID == nil {
				return invalid("Escolha o cliente que vai receber o vale-troca.")
			}
			if total > 0 {
				if err := s.customers.AddStoreCredit(ctx, tx, *nr.CustomerID, total, now); err != nil {
					return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", *nr.CustomerID), "")
				}
			}
		case models.RefundCash, models.RefundPix, models.RefundCard:
			nr.CustomerID = sale.CustomerID
			session, err := requireOpenCash(ctx, tx)
			if err != nil {
				return err
			}
			nr.CashSessionID = &session.ID
			cashMethod = in.RefundMethod
			if in.RefundMethod == models.RefundCard {
				payments, err := s.sales.Payments(ctx, tx, sale.ID)
				if err != nil {
					return err
				}
				left, method, err := s.cardLeft(ctx, tx, sale.ID, payments)
				if err != nil {
					return err
				}
				if method == "" {
					return invalid("A venda #%d não teve pagamento no cartão. Devolva em dinheiro, Pix ou vale-troca.", sale.ID)
				}
				if total > left {
					return invalid("Só %s ainda podem ser estornados no cartão nesta venda; a devolução é de %s. Use outra forma para o restante.", FormatBRL(left), FormatBRL(total))
				}
				cashMethod = method
			}
		case models.RefundCredit:
			payments, err := s.sales.Payments(ctx, tx, sale.ID)
			if err != nil {
				return err
			}
			hasFiado := false
			for _, p := range payments {
				hasFiado = hasFiado || p.Method == models.PaymentCredit
			}
			if !hasFiado || sale.CustomerID == nil {
				return invalid("Só dá para abater na dívida uma venda feita a prazo.")
			}
			nr.CustomerID = sale.CustomerID
			if total > 0 {
				ok, err := s.customers.ReduceDebt(ctx, tx, *sale.CustomerID, total, now)
				if err != nil {
					return err
				}
				if !ok {
					return newErr(CodeStateConflict, "A dívida atual do cliente é menor que o valor da devolução (%s). Use vale-troca ou dinheiro.", FormatBRL(total))
				}
			}
		}

		if returnID, err = s.returns.Insert(ctx, tx, nr, now); err != nil {
			return err
		}
		if nr.CashSessionID != nil && total > 0 {
			if _, err := s.cash.InsertMovement(ctx, tx, repository.NewCashMovement{
				SessionID: *nr.CashSessionID, Type: models.CashReturn, PaymentMethod: cashMethod, AmountCents: -total,
				Description:   ptr(fmt.Sprintf("Devolução nº %d da venda #%d", returnID, sale.ID)),
				ReferenceType: ptr("return"), ReferenceID: &returnID,
			}, now); err != nil {
				return err
			}
		}
		for _, l := range lines {
			if err := s.returns.InsertItem(ctx, tx, returnID, l); err != nil {
				return err
			}
			if err := applyStockChange(ctx, tx, now, repository.NewMovement{
				ProductID: l.ProductID, QtyChange: l.Qty, Reason: models.StockReturn,
				ReferenceType: ptr("return"), ReferenceID: &returnID, Notes: &reason,
			}); err != nil {
				return err
			}
		}
		// Venda de mensalidade (um item só): devolver desfaz o pagamento.
		if err := undoSubscriptionPayment(ctx, tx, sale.ID, now); err != nil {
			return err
		}
		return audit(ctx, tx, now, authBy, "devolucao", "venda", &sale.ID, nil,
			map[string]any{"devolucao": returnID, "reembolso": in.RefundMethod, "total": total, "motivo": reason})
	})
	if err != nil {
		return models.Return{}, err
	}
	return s.returns.GetByID(ctx, s.DB, returnID)
}

// List lista as devoluções do período; quem não vê as vendas de todos vê só as próprias.
func (s *ReturnService) List(ctx context.Context, startDate, endDate string) ([]models.Return, error) {
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	return s.returns.ListBetween(ctx, s.DB, start, end, salesScope(ctx, 0))
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
