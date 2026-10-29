// Caminho: pdv/internal/service/promotion_service.go

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

type PromotionService struct {
	Base
	promos repository.PromotionRepo
}

func NewPromotionService(b Base) *PromotionService { return &PromotionService{Base: b} }

func localDate(t time.Time) string { return t.Local().Format("2006-01-02") }

func promoStatus(p models.Promotion, today string) string {
	switch {
	case !p.Active:
		return "inativa"
	case today < p.StartDate:
		return "futura"
	case today > p.EndDate:
		return "encerrada"
	}
	return "vigente"
}

func (s *PromotionService) List(ctx context.Context) ([]models.Promotion, error) {
	list, err := s.promos.List(ctx, s.DB)
	today := localDate(s.Now())
	for i := range list {
		list[i].Status = promoStatus(list[i], today)
	}
	return list, err
}

// Save cadastra (ID 0) ou altera uma promoção. Exige a permissão de alterar preço.
func (s *PromotionService) Save(ctx context.Context, in models.Promotion) (models.Promotion, error) {
	v := in
	var err error
	if v.Name, err = cleanName(in.Name, "nome da promoção"); err != nil {
		return models.Promotion{}, err
	}
	v.ProductID, v.CategoryID = validCustomerOrNil(in.ProductID), validCustomerOrNil(in.CategoryID)
	if (v.ProductID == nil) == (v.CategoryID == nil) {
		return models.Promotion{}, invalid("Escolha um produto ou uma categoria para a promoção.")
	}
	for _, d := range []string{v.StartDate, v.EndDate} {
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return models.Promotion{}, invalid("Informe as datas de início e fim da promoção.")
		}
	}
	if v.StartDate > v.EndDate {
		return models.Promotion{}, invalid("A data de início não pode ser depois da data de fim.")
	}
	v.PriceCents, v.Percent, v.BuyQty, v.PayQty = 0, 0, 0, 0
	switch in.Kind {
	case models.PromoPrice:
		if err := validatePrice(in.PriceCents); err != nil || in.PriceCents <= 0 {
			return models.Promotion{}, invalid("Informe o preço promocional.")
		}
		v.PriceCents = in.PriceCents
	case models.PromoPercent:
		if in.Percent < 1 || in.Percent > 99 {
			return models.Promotion{}, invalid("O desconto da promoção deve ser de 1%% a 99%%.")
		}
		v.Percent = in.Percent
	case models.PromoBuyPay:
		if in.PayQty < 1 || in.BuyQty <= in.PayQty || in.BuyQty > 100 {
			return models.Promotion{}, invalid("No \"leve X pague Y\", X deve ser maior que Y (ex.: leve 3, pague 2).")
		}
		v.BuyQty, v.PayQty = in.BuyQty, in.PayQty
	default:
		return models.Promotion{}, invalid("Tipo de promoção inválido.")
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermPrice)
		if err != nil {
			return err
		}
		if v.ProductID != nil {
			if _, err := (repository.ProductRepo{}).GetByID(ctx, tx, *v.ProductID); err != nil {
				return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", *v.ProductID), "")
			}
		} else if _, err := (repository.CategoryRepo{}).GetByID(ctx, tx, *v.CategoryID); err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Categoria %d não encontrada.", *v.CategoryID), "")
		}
		if id, err = s.promos.Save(ctx, tx, v, now); err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Promoção %d não encontrada.", v.ID), "")
		}
		return audit(ctx, tx, now, authBy, "promocao_salva", "promocao", &id, nil,
			map[string]any{"nome": v.Name, "tipo": v.Kind, "inicio": v.StartDate, "fim": v.EndDate, "ativa": v.Active})
	})
	if err != nil {
		return models.Promotion{}, err
	}
	p, err := s.promos.GetByID(ctx, s.DB, id)
	p.Status = promoStatus(p, localDate(s.Now()))
	return p, err
}

// promoLabel é o texto que vai no cupom.
func promoLabel(p models.Promotion) string {
	if p.Kind == models.PromoBuyPay {
		return fmt.Sprintf("Leve %d pague %d", p.BuyQty, p.PayQty)
	}
	return "PROMOÇÃO " + strings.ToUpper(p.Name)
}

// applied é o efeito da melhor promoção numa linha da venda.
type applied struct {
	unit     int64 // preço unitário a usar
	discount int64 // desconto do "leve X pague Y"
	promo    *models.Promotion
}

// bestPromotion escolhe, entre as promoções vigentes, a que dá mais desconto
// para a quantidade. Promoções não se somam. allowPrice=false só considera
// "leve X pague Y" (comanda: o preço promocional já valeu ao lançar o item).
func bestPromotion(ctx context.Context, q repository.DBTX, p models.Product, qty, base int64, now time.Time, allowPrice bool) (applied, error) {
	best := applied{unit: base}
	promos, err := (repository.PromotionRepo{}).ForProduct(ctx, q, p.ID, p.CategoryID, localDate(now))
	if err != nil {
		return best, err
	}
	bestValue := int64(0)
	for i := range promos {
		pr := promos[i]
		unit, disc := base, int64(0)
		switch pr.Kind {
		case models.PromoPrice:
			if allowPrice && pr.PriceCents < base {
				unit = pr.PriceCents
			}
		case models.PromoPercent:
			if allowPrice {
				unit = base - (base*pr.Percent+50)/100
			}
		case models.PromoBuyPay:
			disc = qty / pr.BuyQty * (pr.BuyQty - pr.PayQty) * base
		}
		if value := (base-unit)*qty + disc; value > bestValue {
			bestValue, best = value, applied{unit: unit, discount: disc, promo: &pr}
		}
	}
	return best, nil
}

// productPromo descreve a promoção vigente para a busca de venda (qtd 1 para preço).
func productPromo(ctx context.Context, q repository.DBTX, p models.Product, now time.Time) *models.ProductPromo {
	promos, err := (repository.PromotionRepo{}).ForProduct(ctx, q, p.ID, p.CategoryID, localDate(now))
	if err != nil || len(promos) == 0 {
		return nil
	}
	var out *models.ProductPromo
	for _, pr := range promos {
		switch pr.Kind {
		case models.PromoPrice, models.PromoPercent:
			price := pr.PriceCents
			if pr.Kind == models.PromoPercent {
				price = p.PriceCents - (p.PriceCents*pr.Percent+50)/100
			}
			if price < p.PriceCents && (out == nil || out.PriceCents == 0 || price < out.PriceCents) {
				out = &models.ProductPromo{Label: promoLabel(pr), PriceCents: price}
			}
		case models.PromoBuyPay:
			if out == nil {
				out = &models.ProductPromo{Label: promoLabel(pr), BuyQty: pr.BuyQty, PayQty: pr.PayQty}
			}
		}
	}
	return out
}
