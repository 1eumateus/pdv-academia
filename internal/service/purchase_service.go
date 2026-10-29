// Caminho: pdv/internal/service/purchase_service.go

package service

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

const maxPurchaseItems = 500

type PurchaseService struct {
	Base
	purchases repository.PurchaseRepo
	suppliers repository.SupplierRepo
	products  repository.ProductRepo
}

func NewPurchaseService(b Base) *PurchaseService { return &PurchaseService{Base: b} }

// averageCost é o custo médio ponderado depois da entrada, arredondado ao
// centavo. Sem custo anterior conhecido (0) vale o custo da entrada, para não
// diluir o custo real com estoque que nunca teve custo informado.
func averageCost(stock, cost, qty, unit int64) int64 {
	if stock <= 0 || cost == 0 {
		return unit
	}
	total := stock + qty
	return (stock*cost + qty*unit + total/2) / total
}

// validatePurchase confere a entrada antes de abrir a transação.
func validatePurchase(in models.PurchaseInput) (invoice, notes *string, err error) {
	if len(in.Items) == 0 {
		return nil, nil, invalid("Inclua ao menos um item na entrada.")
	}
	if len(in.Items) > maxPurchaseItems {
		return nil, nil, invalid("A entrada pode ter no máximo %d itens.", maxPurchaseItems)
	}
	if _, err := time.Parse("2006-01-02", in.IssueDate); err != nil {
		return nil, nil, invalid("Data da nota inválida.")
	}
	if invoice, err = optText(&in.InvoiceNumber, "número da nota", 60); err != nil {
		return nil, nil, err
	}
	if notes, err = optText(&in.Notes, "observações", maxNotesLen); err != nil {
		return nil, nil, err
	}
	for _, v := range []int64{in.FreightCents, in.TaxesCents, in.DiscountCents} {
		if v < 0 || v > MaxPriceCents {
			return nil, nil, invalid("Valores de frete, impostos e desconto inválidos.")
		}
	}
	seen := map[int64]bool{}
	for _, it := range in.Items {
		if err := validateQty(it.Qty); err != nil {
			return nil, nil, err
		}
		if it.UnitCostCents < 0 || it.UnitCostCents > MaxPriceCents {
			return nil, nil, invalid("Custo unitário inválido.")
		}
		if it.NewPriceCents != nil {
			if err := validatePrice(*it.NewPriceCents); err != nil {
				return nil, nil, err
			}
		}
		if seen[it.ProductID] {
			return nil, nil, invalid("O mesmo produto aparece em mais de uma linha. Junte as quantidades em uma só.")
		}
		seen[it.ProductID] = true
	}
	return invoice, notes, nil
}

// Create registra a compra: cada item dá entrada no estoque e recalcula o custo
// médio do produto, tudo numa transação.
func (s *PurchaseService) Create(ctx context.Context, in models.PurchaseInput) (models.PurchaseDetails, error) {
	invoice, notes, err := validatePurchase(in)
	if err != nil {
		return models.PurchaseDetails{}, err
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = s.createTx(ctx, tx, in, invoice, notes)
		return err
	})
	if err != nil {
		return models.PurchaseDetails{}, err
	}
	return s.Details(ctx, id)
}

// createTx grava a compra dentro de uma transação já aberta (também usado pela
// importação do XML, que cria fornecedor e vínculos na mesma transação).
func (s *PurchaseService) createTx(ctx context.Context, tx *sql.Tx, in models.PurchaseInput, invoice, notes *string) (id int64, err error) {
	err = func() error {
		now := s.Now()
		if _, err := requirePerm(ctx, now, PermPurchases); err != nil {
			return err
		}
		sup, err := s.suppliers.GetByID(ctx, tx, in.SupplierID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Escolha o fornecedor da entrada."), "")
		}
		if !sup.Active {
			return invalid("O fornecedor %s está desativado.", sup.LegalName)
		}
		var key *string
		if k := strings.TrimSpace(in.AccessKey); k != "" {
			if dup, err := s.purchases.ExistsAccessKey(ctx, tx, k); err != nil {
				return err
			} else if dup {
				return newErr(CodeDuplicate, "Esta nota (chave %s) já foi lançada.", k)
			}
			key = &k
		}

		items := make([]models.PurchaseItem, 0, len(in.Items))
		var productsTotal int64
		priceChecked := false
		var priceAuth *int64
		for _, it := range in.Items {
			p, err := s.products.GetByID(ctx, tx, it.ProductID)
			if err != nil {
				return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", it.ProductID), "")
			}
			lineTotal, err := mulCents(it.Qty, it.UnitCostCents)
			if err != nil {
				return err
			}
			if productsTotal, err = addCents(productsTotal, lineTotal); err != nil {
				return err
			}
			newCost := averageCost(p.StockQty, p.CostCents, it.Qty, it.UnitCostCents)
			items = append(items, models.PurchaseItem{ProductID: p.ID, ProductName: p.Name, Qty: it.Qty,
				UnitCostCents: it.UnitCostCents, TotalCents: lineTotal, CostBeforeCents: p.CostCents, CostAfterCents: newCost})

			if it.NewPriceCents != nil && *it.NewPriceCents != p.PriceCents {
				if !priceChecked {
					if priceAuth, err = requirePerm(ctx, now, PermPrice); err != nil {
						return err
					}
					priceChecked = true
				}
				if err := s.products.SetPrice(ctx, tx, p.ID, *it.NewPriceCents, now); err != nil {
					return err
				}
				if err := audit(ctx, tx, now, priceAuth, "preco_alterado", "produto", &p.ID,
					map[string]any{"preco": p.PriceCents}, map[string]any{"preco": *it.NewPriceCents, "origem": "entrada de mercadoria"}); err != nil {
					return err
				}
			}
		}
		// Cada item já traz o custo final por unidade (no XML, com frete/impostos/desconto
		// rateados), então o total é a soma dos itens; os demais valores são informativos.
		total := productsTotal
		if id, err = s.purchases.Insert(ctx, tx, models.Purchase{SupplierID: sup.ID, InvoiceNumber: str(invoice),
			IssueDate: in.IssueDate, AccessKey: key, ProductsCents: productsTotal, FreightCents: in.FreightCents,
			TaxesCents: in.TaxesCents, DiscountCents: in.DiscountCents, TotalCents: total, Notes: str(notes)}, now); err != nil {
			return fromRepo(err, nil, "Esta nota já foi lançada.")
		}
		for _, it := range items {
			if err := s.purchases.InsertItem(ctx, tx, id, it); err != nil {
				return err
			}
			if err := applyStockChange(ctx, tx, now, repository.NewMovement{
				ProductID: it.ProductID, QtyChange: it.Qty, Reason: models.StockEntry,
				ReferenceType: ptr("purchase"), ReferenceID: &id, Notes: ptr("Compra de " + sup.LegalName),
			}); err != nil {
				return err
			}
			if err := s.products.SetCost(ctx, tx, it.ProductID, it.CostAfterCents, now); err != nil {
				return err
			}
		}
		return audit(ctx, tx, now, nil, "compra_registrada", "compra", &id, nil,
			map[string]any{"fornecedor": sup.LegalName, "nota": str(invoice), "itens": len(items), "total": total})
	}()
	return id, err
}

// Cancel estorna a entrada: tira do estoque o que ela colocou, devolve o custo
// anterior de cada produto (se o custo não mudou depois) e libera a chave da
// NF-e para importar de novo. Se parte já foi vendida, recusa com o produto.
func (s *PurchaseService) Cancel(ctx context.Context, id int64, reason string) (models.PurchaseDetails, error) {
	r := strings.TrimSpace(reason)
	if r == "" {
		return models.PurchaseDetails{}, invalid("Informe o motivo do estorno.")
	}
	if len([]rune(r)) > maxReasonLen {
		return models.PurchaseDetails{}, invalid("O motivo deve ter no máximo %d caracteres.", maxReasonLen)
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if _, err := requirePerm(ctx, now, PermPurchases); err != nil {
			return err
		}
		p, err := s.purchases.GetByID(ctx, tx, id)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Entrada %d não encontrada.", id), "")
		}
		if p.CancelledAt != nil {
			return newErr(CodeStateConflict, "A entrada nº %d já foi estornada.", id)
		}
		items, err := s.purchases.Items(ctx, tx, id)
		if err != nil {
			return err
		}
		kept := 0
		for _, it := range items {
			if err := applyStockChange(ctx, tx, now, repository.NewMovement{
				ProductID: it.ProductID, QtyChange: -it.Qty, Reason: models.StockCancellation,
				ReferenceType: ptr("purchase"), ReferenceID: &id, Notes: ptr("Estorno da entrada: " + r),
			}); err != nil {
				return err
			}
			cur, err := s.products.GetByID(ctx, tx, it.ProductID)
			if err != nil {
				return err
			}
			if cur.CostCents == it.CostAfterCents {
				if err := s.products.SetCost(ctx, tx, it.ProductID, it.CostBeforeCents, now); err != nil {
					return err
				}
			} else {
				kept++ // custo mudou depois (outra entrada ou edição): mantém o atual
			}
		}
		ok, err := s.purchases.MarkCancelled(ctx, tx, id, r, now)
		if err != nil {
			return err
		}
		if !ok {
			return newErr(CodeStateConflict, "A entrada nº %d já foi estornada.", id)
		}
		return audit(ctx, tx, now, nil, "compra_estornada", "compra", &id, nil,
			map[string]any{"fornecedor": p.SupplierName, "nota": p.InvoiceNumber, "motivo": r, "custos_mantidos": kept})
	})
	if err != nil {
		return models.PurchaseDetails{}, err
	}
	return s.Details(ctx, id)
}

func (s *PurchaseService) Details(ctx context.Context, id int64) (models.PurchaseDetails, error) {
	if _, err := requirePerm(ctx, s.Now(), PermPurchases); err != nil {
		return models.PurchaseDetails{}, err
	}
	p, err := s.purchases.GetByID(ctx, s.DB, id)
	if err != nil {
		return models.PurchaseDetails{}, fromRepo(err, newErr(CodeNotFound, "Compra %d não encontrada.", id), "")
	}
	items, err := s.purchases.Items(ctx, s.DB, id)
	return models.PurchaseDetails{Purchase: p, Items: items}, err
}

func (s *PurchaseService) List(ctx context.Context, startDate, endDate, term string) ([]models.Purchase, error) {
	if _, err := requirePerm(ctx, s.Now(), PermPurchases); err != nil {
		return nil, err
	}
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	return s.purchases.ListBetween(ctx, s.DB, start, end, strings.TrimSpace(term))
}
