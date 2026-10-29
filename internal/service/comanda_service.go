// Caminho: pdv/internal/service/comanda_service.go

package service

import (
	"context"
	"database/sql"
	"errors"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// Regras das comandas:
//   - itens só entram em comanda aberta (o banco também recusa, via trigger);
//   - o preço do item é gravado no lançamento e é o preço cobrado no fechamento;
//   - produto desativado depois de lançado: o item continua na comanda, é
//     sinalizado na tela (productActive=false) e bloqueia o fechamento até ser
//     removido, porque produto inativo não pode ser vendido;
//   - o estoque não é reservado no lançamento; é baixado no fechamento;
//   - comanda sem itens não pode ser fechada; ela pode ser descartada;
//   - o fechamento gera uma venda normal, numa única transação.
type ComandaService struct {
	Base
	comandas repository.ComandaRepo
	products repository.ProductRepo
}

func NewComandaService(b Base) *ComandaService { return &ComandaService{Base: b} }

func (s *ComandaService) Open(ctx context.Context, tableNo string) (models.ComandaDetails, error) {
	table, err := cleanName(tableNo, "identificador da mesa ou comanda")
	if err != nil {
		return models.ComandaDetails{}, err
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = s.comandas.Insert(ctx, tx, table, s.Now())
		return fromRepo(err, nil, "Já existe uma comanda aberta para "+table+".")
	})
	if err != nil {
		return models.ComandaDetails{}, err
	}
	return s.Details(ctx, id)
}

func (s *ComandaService) getOpen(ctx context.Context, q repository.DBTX, id int64) (models.Comanda, error) {
	c, err := s.comandas.GetByID(ctx, q, id)
	if err != nil {
		return c, fromRepo(err, notFoundErr(CodeComandaNotFound, "Comanda", id), "")
	}
	if c.Status != models.ComandaOpen {
		return c, newErr(CodeComandaClosed, "A comanda %s já foi %s.", c.TableNo, c.Status)
	}
	return c, nil
}

// AddItem lança um produto. Se o mesmo produto já foi lançado com o mesmo
// preço, a quantidade é somada na linha existente.
func (s *ComandaService) AddItem(ctx context.Context, comandaID, productID, qty int64) (models.ComandaDetails, error) {
	if err := validateQty(qty); err != nil {
		return models.ComandaDetails{}, err
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if _, err := s.getOpen(ctx, tx, comandaID); err != nil {
			return err
		}
		p, err := s.products.GetByID(ctx, tx, productID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", productID), "")
		}
		if !p.Active {
			return newErr(CodeProductInactive, "O produto %q está inativo.", p.Name)
		}
		if p.PricePending {
			return newErr(CodeProductInactive, "O produto %q está \"a precificar\" (sem preço de venda definido). Peça ao gerente para definir o preço.", p.Name)
		}
		if err := s.checkStock(ctx, tx, comandaID, p, 0, qty); err != nil {
			return err
		}
		ap, err := bestPromotion(ctx, tx, p, qty, p.PriceCents, now, true)
		if err != nil {
			return err
		}
		price := ap.unit // promoção de preço vale no lançamento; "leve X pague Y" no fechamento
		itemID, current, err := s.comandas.FindItemByProduct(ctx, tx, comandaID, productID, price)
		switch {
		case err == nil:
			if current+qty > MaxQty {
				return invalid("A quantidade máxima por item é %d.", MaxQty)
			}
			return s.comandas.SetItemQty(ctx, tx, itemID, current+qty)
		case isNotFound(err):
			_, err = s.comandas.InsertItem(ctx, tx, comandaID, productID, p.Name, qty, price, now)
			return err
		default:
			return err
		}
	})
	if err != nil {
		return models.ComandaDetails{}, closedConflict(err)
	}
	return s.Details(ctx, comandaID)
}

// SetItemQty altera a quantidade de um item; zero remove o item.
func (s *ComandaService) SetItemQty(ctx context.Context, itemID, qty int64) (models.ComandaDetails, error) {
	if qty != 0 {
		if err := validateQty(qty); err != nil {
			return models.ComandaDetails{}, err
		}
	}
	var comandaID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var (
			productID int64
			err       error
		)
		comandaID, productID, err = s.comandas.GetItem(ctx, tx, itemID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Item %d não encontrado na comanda.", itemID), "")
		}
		if _, err := s.getOpen(ctx, tx, comandaID); err != nil {
			return err
		}
		if qty == 0 {
			return s.comandas.DeleteItem(ctx, tx, itemID)
		}
		p, err := s.products.GetByID(ctx, tx, productID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", productID), "")
		}
		if err := s.checkStock(ctx, tx, comandaID, p, itemID, qty); err != nil {
			return err
		}
		return s.comandas.SetItemQty(ctx, tx, itemID, qty)
	})
	if err != nil {
		return models.ComandaDetails{}, closedConflict(err)
	}
	return s.Details(ctx, comandaID)
}

// checkStock: o produto na comanda (outros itens + qty) não pode passar do
// estoque. O fechamento confere de novo ao dar baixa.
func (s *ComandaService) checkStock(ctx context.Context, tx *sql.Tx, comandaID int64, p models.Product, exceptItemID, qty int64) error {
	if p.NoStock {
		return nil
	}
	others, err := s.comandas.ProductQty(ctx, tx, comandaID, p.ID, exceptItemID)
	if err != nil {
		return err
	}
	if others+qty > p.StockQty {
		return insufficientStock(p.ID, p.Name, p.StockQty, others+qty)
	}
	return nil
}

// Discard encerra uma comanda aberta por engano. Só é permitido sem itens.
func (s *ComandaService) Discard(ctx context.Context, comandaID int64) (models.ComandaDetails, error) {
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		c, err := s.getOpen(ctx, tx, comandaID)
		if err != nil {
			return err
		}
		if c.ItemCount > 0 {
			return invalid("Só é possível descartar uma comanda sem itens. Remova os itens ou feche a comanda.")
		}
		ok, err := s.comandas.SetStatus(ctx, tx, comandaID, models.ComandaDiscarded, s.Now())
		if err == nil && !ok {
			err = newErr(CodeComandaClosed, "A comanda já foi encerrada em outra ação.")
		}
		return err
	})
	if err != nil {
		return models.ComandaDetails{}, err
	}
	return s.Details(ctx, comandaID)
}

// Close fecha a comanda gerando uma venda, tudo na mesma transação.
func (s *ComandaService) Close(ctx context.Context, in models.CloseComandaInput) (models.SaleDetails, error) {
	if err := validateSaleHeader(in.PaymentMethod, in.Payments, in.CustomerID, in.DiscountCents); err != nil {
		return models.SaleDetails{}, err
	}
	var saleID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		c, err := s.getOpen(ctx, tx, in.ComandaID)
		if err != nil {
			return err
		}
		items, err := s.comandas.Items(ctx, tx, in.ComandaID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return invalid("A comanda %s não possui itens. Adicione itens ou descarte a comanda.", c.TableNo)
		}
		lines := make([]saleLine, 0, len(items))
		for _, it := range items {
			lines = append(lines, saleLine{productID: it.ProductID, qty: it.Qty,
				unitPrice: ptr(it.UnitPriceCents), name: it.ProductNameSnapshot})
		}
		saleID, err = createSaleTx(ctx, tx, s.Now, lines, in.PaymentMethod, in.Payments, in.CustomerID, in.DiscountCents, &in.ComandaID, nil)
		if err != nil {
			if errors.Is(err, repository.ErrUnique) {
				return newErr(CodeComandaClosed, "Esta comanda já gerou uma venda.")
			}
			return err
		}
		ok, err := s.comandas.SetStatus(ctx, tx, in.ComandaID, models.ComandaClosed, s.Now())
		if err == nil && !ok {
			err = newErr(CodeComandaClosed, "A comanda %s foi fechada em outra ação.", c.TableNo)
		}
		return err
	})
	if err != nil {
		return models.SaleDetails{}, err
	}
	return NewSaleService(s.Base).Details(ctx, saleID)
}

func (s *ComandaService) ListOpen(ctx context.Context) ([]models.Comanda, error) {
	return s.comandas.ListOpen(ctx, s.DB)
}

func (s *ComandaService) Details(ctx context.Context, id int64) (models.ComandaDetails, error) {
	c, err := s.comandas.GetByID(ctx, s.DB, id)
	if err != nil {
		return models.ComandaDetails{}, fromRepo(err, notFoundErr(CodeComandaNotFound, "Comanda", id), "")
	}
	items, err := s.comandas.Items(ctx, s.DB, id)
	return models.ComandaDetails{Comanda: c, Items: items}, err
}

// closedConflict traduz a recusa do trigger de comanda fechada (corrida entre
// duas ações) para o erro de domínio.
func closedConflict(err error) error {
	if errors.Is(err, repository.ErrTrigger) {
		return newErr(CodeComandaClosed, "A comanda foi encerrada em outra ação. Atualize a tela.")
	}
	return err
}
