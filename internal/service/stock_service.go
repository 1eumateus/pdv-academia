// Caminho: pdv/internal/service/stock_service.go

package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// Regra única de escrita: todo movimento de estoque passa por applyStockChange,
// que atualiza products.stock_qty e insere em stock_movements na mesma Tx.
func applyStockChange(ctx context.Context, tx *sql.Tx, now time.Time, m repository.NewMovement) error {
	var (
		products repository.ProductRepo
		stock    repository.StockRepo
	)
	ok, err := products.ApplyStockDelta(ctx, tx, m.ProductID, m.QtyChange, now)
	if err != nil {
		return err
	}
	if !ok {
		// Nada foi gravado: descobrir o motivo lendo o estado atual dentro da Tx.
		p, err := products.GetByID(ctx, tx, m.ProductID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", m.ProductID), "")
		}
		if p.NoStock {
			// Venda, cancelamento e devolução passam sem mexer no saldo; entrada e ajuste não fazem sentido.
			if m.Reason == models.StockSale || m.Reason == models.StockCancellation || m.Reason == models.StockReturn {
				return nil
			}
			return invalid("O produto %q não controla estoque.", p.Name)
		}
		return insufficientStock(p.ID, p.Name, p.StockQty, -m.QtyChange)
	}
	_, err = stock.InsertMovement(ctx, tx, m, now)
	return err
}

type StockService struct {
	Base
	products repository.ProductRepo
	stock    repository.StockRepo
}

func NewStockService(b Base) *StockService { return &StockService{Base: b} }

// RegisterEntry registra entrada de mercadoria (quantidade positiva).
func (s *StockService) RegisterEntry(ctx context.Context, productID, qty int64, notes string) (models.Product, error) {
	if err := validateQty(qty); err != nil {
		return models.Product{}, err
	}
	return s.change(ctx, productID, qty, models.StockEntry, notes, false)
}

// Adjust corrige o saldo (positivo ou negativo). Exige justificativa.
func (s *StockService) Adjust(ctx context.Context, productID, qtyChange int64, notes string) (models.Product, error) {
	if qtyChange == 0 {
		return models.Product{}, invalid("O ajuste não pode ser zero.")
	}
	abs := qtyChange
	if abs < 0 {
		abs = -abs
	}
	if err := validateQty(abs); err != nil {
		return models.Product{}, err
	}
	return s.change(ctx, productID, qtyChange, models.StockAdjust, notes, true)
}

// RegisterReturn registra devolução de mercadoria ao estoque fora de um
// cancelamento (ex.: troca). Exige justificativa.
func (s *StockService) RegisterReturn(ctx context.Context, productID, qty int64, notes string) (models.Product, error) {
	if err := validateQty(qty); err != nil {
		return models.Product{}, err
	}
	return s.change(ctx, productID, qty, models.StockReturn, notes, true)
}

func (s *StockService) change(ctx context.Context, productID, delta int64, reason, notes string, notesRequired bool) (models.Product, error) {
	n := trimPtr(&notes)
	if notesRequired && n == nil {
		return models.Product{}, invalid("Informe o motivo da movimentação.")
	}
	var out models.Product
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		// Ajuste manual corrige o saldo sem venda/entrada: exige gerente e fica na auditoria.
		var authBy *int64
		var before models.Product
		switch reason {
		case models.StockEntry, models.StockReturn:
			perm := PermStockEntry
			if reason == models.StockReturn {
				perm = PermStockReturn
			}
			if _, err := requirePerm(ctx, now, perm); err != nil {
				return err
			}
		case models.StockAdjust:
			var err error
			if before, err = s.products.GetByID(ctx, tx, productID); err != nil {
				return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", productID), "")
			}
			if authBy, err = requirePerm(ctx, now, PermStockAdjust); err != nil {
				return err
			}
		}
		if err := applyStockChange(ctx, tx, now, repository.NewMovement{
			ProductID: productID, QtyChange: delta, Reason: reason, ReferenceType: ptr("manual"), Notes: n,
		}); err != nil {
			return err
		}
		p, err := s.products.GetByID(ctx, tx, productID)
		if err != nil {
			return err
		}
		out = p
		if reason != models.StockAdjust {
			return nil
		}
		return audit(ctx, tx, now, authBy, "ajuste_estoque", "produto", &productID,
			map[string]any{"estoque": before.StockQty}, map[string]any{"estoque": p.StockQty, "motivo": *n})
	})
	return hideCost(ctx, out), err
}

func (s *StockService) ListMovements(ctx context.Context, productID int64, startDate, endDate string) ([]models.StockMovement, error) {
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	out, err := s.stock.List(ctx, s.DB, productID, start, end, 0)
	return hideMovementCosts(ctx, out), err
}

// hideMovementCosts: sem a permissão de ver custo, o valor de custo não sai.
func hideMovementCosts(ctx context.Context, ms []models.StockMovement) []models.StockMovement {
	if !canSeeCost(ctx) {
		for i := range ms {
			if ms[i].IsCost {
				ms[i].UnitCents = nil
			}
		}
	}
	return ms
}

// Audit lista divergências entre products.stock_qty e a soma das movimentações.
func (s *StockService) Audit(ctx context.Context) ([]models.StockAuditEntry, error) {
	return s.stock.Audit(ctx, s.DB)
}

// ReconcileDivergence é o procedimento de correção de uma divergência. Uma
// divergência só surge se alguém alterou o banco por fora do sistema. O saldo
// gravado é o que o operador vê e confere na prateleira, então ele é mantido e
// o histórico ganha um movimento 'ajuste' (referência 'reconciliacao') com a
// diferença. É a única escrita de estoque que não altera stock_qty, porque o
// objetivo é justamente fazer o histórico voltar a explicar o saldo. Nada do
// histórico original é apagado. Depois, recomenda-se contar o produto e usar
// Adjust se a contagem física for diferente.
func (s *StockService) ReconcileDivergence(ctx context.Context, productID int64, notes string) (models.StockAuditEntry, error) {
	n := trimPtr(&notes)
	if n == nil {
		return models.StockAuditEntry{}, invalid("Informe o motivo da correção.")
	}
	var out models.StockAuditEntry
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		p, err := s.products.GetByID(ctx, tx, productID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", productID), "")
		}
		calc, err := s.stock.CalculatedQty(ctx, tx, productID)
		if err != nil {
			return err
		}
		diff := p.StockQty - calc
		if diff == 0 {
			return newErr(CodeStateConflict, "O produto %q não possui divergência.", p.Name)
		}
		authBy, err := requirePerm(ctx, s.Now(), PermStockAdjust)
		if err != nil {
			return err
		}
		note := "Reconciliação de divergência: " + strings.TrimSpace(*n)
		if err := audit(ctx, tx, s.Now(), authBy, "reconciliacao_estoque", "produto", &productID,
			map[string]any{"historico": calc}, map[string]any{"historico": p.StockQty, "motivo": *n}); err != nil {
			return err
		}
		if _, err := s.stock.InsertMovement(ctx, tx, repository.NewMovement{
			ProductID: productID, QtyChange: diff, Reason: models.StockAdjust,
			ReferenceType: ptr("reconciliacao"), Notes: &note,
		}, s.Now()); err != nil {
			return err
		}
		out = models.StockAuditEntry{ProductID: p.ID, ProductName: p.Name, StoredQty: p.StockQty, CalculatedQty: p.StockQty}
		return nil
	})
	return out, err
}

func isNotFound(err error) bool { return errors.Is(err, repository.ErrNotFound) }
