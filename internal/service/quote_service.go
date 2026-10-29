// Caminho: pdv/internal/service/quote_service.go

package service

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

type QuoteService struct {
	Base
	quotes    repository.QuoteRepo
	products  repository.ProductRepo
	customers repository.CustomerRepo
	settings  repository.SettingsRepo
	sales     repository.SaleRepo
}

func NewQuoteService(b Base) *QuoteService { return &QuoteService{Base: b} }

// withExpiry marca como "expirado" o orçamento em aberto/aprovado vencido.
func withExpiry(q models.Quote, today string) models.Quote {
	if (q.Status == models.QuoteOpen || q.Status == models.QuoteApproved) && q.ValidUntil < today {
		q.Status = models.QuoteExpired
	}
	return q
}

// price calcula os itens com o preço de agora (já com promoção de preço e "leve X pague Y").
func (s *QuoteService) price(ctx context.Context, q repository.DBTX, in []models.SaleItemInput, now time.Time) ([]models.QuoteItem, int64, int64, error) {
	lines, err := consolidate(in)
	if err != nil {
		return nil, 0, 0, err
	}
	var subtotal, itemsDiscount int64
	out := make([]models.QuoteItem, 0, len(lines))
	for _, l := range lines {
		p, err := s.products.GetByID(ctx, q, l.productID)
		if err != nil {
			return nil, 0, 0, fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", l.productID), "")
		}
		if !p.Active || p.PricePending {
			return nil, 0, 0, invalid("O produto %q não está disponível para venda (inativo ou a precificar).", p.Name)
		}
		ap, err := bestPromotion(ctx, q, p, l.qty, p.PriceCents, now, true)
		if err != nil {
			return nil, 0, 0, err
		}
		line, err := mulCents(l.qty, ap.unit)
		if err != nil {
			return nil, 0, 0, err
		}
		if l.discount > line-ap.discount {
			return nil, 0, 0, invalid("O desconto de %q não pode ser maior que o valor do item.", p.Name)
		}
		it := models.QuoteItem{ProductID: p.ID, ProductName: p.Name, Qty: l.qty, UnitPriceCents: ap.unit,
			DiscountCents: l.discount, PromoDiscountCents: ap.discount}
		if ap.promo != nil {
			it.PromoLabel = promoLabel(*ap.promo)
		}
		subtotal += line - ap.discount
		itemsDiscount += l.discount
		out = append(out, it)
	}
	return out, subtotal, itemsDiscount, nil
}

// Save cria (ID 0) ou altera um orçamento em aberto/aprovado. Não mexe no estoque.
func (s *QuoteService) Save(ctx context.Context, in models.QuoteInput) (models.QuoteDetails, error) {
	notes, err := optText(&in.Notes, "observações", maxNotesLen)
	if err != nil {
		return models.QuoteDetails{}, err
	}
	if in.DiscountCents < 0 {
		return models.QuoteDetails{}, invalid("O desconto não pode ser negativo.")
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		items, subtotal, itemsDiscount, err := s.price(ctx, tx, in.Items, now)
		if err != nil {
			return err
		}
		discount := itemsDiscount + in.DiscountCents
		if discount > subtotal {
			return invalid("O desconto (%s) não pode ser maior que o subtotal (%s).", FormatBRL(discount), FormatBRL(subtotal))
		}
		customerID := validCustomerOrNil(in.CustomerID)
		if customerID != nil {
			if _, err := s.customers.GetByID(ctx, tx, *customerID); err != nil {
				return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", *customerID), "")
			}
		}
		valid := strings.TrimSpace(in.ValidUntil)
		if valid == "" {
			v, _, err := s.settings.Get(ctx, tx, "quote_validity_days")
			if err != nil {
				return err
			}
			days, perr := strconv.Atoi(v)
			if perr != nil || days < 1 {
				days = defaultQuoteDays
			}
			valid = localDate(now.AddDate(0, 0, days))
		} else if _, err := time.Parse("2006-01-02", valid); err != nil {
			return invalid("Data de validade inválida.")
		}
		q := models.Quote{ID: in.ID, CustomerID: customerID, Status: models.QuoteOpen, ValidUntil: valid, Notes: str(notes),
			SubtotalCents: subtotal, DiscountCents: discount, TotalCents: subtotal - discount}
		if in.ID == 0 {
			if id, err = s.quotes.Insert(ctx, tx, q, now); err != nil {
				return err
			}
		} else {
			old, err := s.quotes.GetByID(ctx, tx, in.ID)
			if err != nil {
				return fromRepo(err, newErr(CodeNotFound, "Orçamento %d não encontrado.", in.ID), "")
			}
			if old.Status == models.QuoteConverted || old.Status == models.QuoteCancelled {
				return newErr(CodeStateConflict, "Orçamento %s não pode ser alterado.", old.Status)
			}
			id = in.ID
			if err := s.quotes.Update(ctx, tx, q, now); err != nil {
				return err
			}
		}
		return s.quotes.ReplaceItems(ctx, tx, id, items)
	})
	if err != nil {
		return models.QuoteDetails{}, err
	}
	return s.Get(ctx, id)
}

func (s *QuoteService) Get(ctx context.Context, id int64) (models.QuoteDetails, error) {
	q, err := s.quotes.GetByID(ctx, s.DB, id)
	if err != nil {
		return models.QuoteDetails{}, fromRepo(err, newErr(CodeNotFound, "Orçamento %d não encontrado.", id), "")
	}
	items, err := s.quotes.Items(ctx, s.DB, id)
	if err != nil {
		return models.QuoteDetails{}, err
	}
	out := models.QuoteDetails{Quote: withExpiry(q, localDate(s.Now())), Items: items}
	if q.CustomerID != nil {
		c, err := s.customers.GetByID(ctx, s.DB, *q.CustomerID)
		if err != nil {
			return out, err
		}
		out.Customer = &c
	}
	return out, nil
}

func (s *QuoteService) List(ctx context.Context, startDate, endDate, term string) ([]models.Quote, error) {
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	list, err := s.quotes.List(ctx, s.DB, start, end, strings.TrimSpace(term))
	today := localDate(s.Now())
	for i := range list {
		list[i] = withExpiry(list[i], today)
	}
	return list, err
}

// SetStatus aprova, reabre ou cancela um orçamento que ainda não virou venda.
func (s *QuoteService) SetStatus(ctx context.Context, id int64, status string) (models.QuoteDetails, error) {
	switch status {
	case models.QuoteOpen, models.QuoteApproved, models.QuoteCancelled:
	default:
		return models.QuoteDetails{}, invalid("Situação inválida.")
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		q, err := s.quotes.GetByID(ctx, tx, id)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Orçamento %d não encontrado.", id), "")
		}
		if q.Status == models.QuoteConverted || q.Status == models.QuoteCancelled {
			return newErr(CodeStateConflict, "Orçamento %s não pode mudar de situação.", q.Status)
		}
		return s.quotes.SetStatus(ctx, tx, id, status, nil, s.Now())
	})
	if err != nil {
		return models.QuoteDetails{}, err
	}
	return s.Get(ctx, id)
}

// Duplicate cria um orçamento novo, em aberto, com os mesmos itens a preços de agora.
func (s *QuoteService) Duplicate(ctx context.Context, id int64) (models.QuoteDetails, error) {
	d, err := s.Get(ctx, id)
	if err != nil {
		return d, err
	}
	in := models.QuoteInput{CustomerID: d.Quote.CustomerID, Notes: d.Quote.Notes}
	var itemsDiscount int64
	for _, it := range d.Items {
		in.Items = append(in.Items, models.SaleItemInput{ProductID: it.ProductID, Qty: it.Qty, DiscountCents: it.DiscountCents})
		itemsDiscount += it.DiscountCents
	}
	in.DiscountCents = d.Quote.DiscountCents - itemsDiscount
	return s.Save(ctx, in)
}

// Differences compara o orçamento com o que a venda seria agora: preço,
// estoque, produto inativo e validade. Blocking impede a conversão.
func (s *QuoteService) Check(ctx context.Context, id int64) (models.QuoteCheck, error) {
	d, err := s.Get(ctx, id)
	if err != nil {
		return models.QuoteCheck{}, err
	}
	out := []models.QuoteDifference{}
	if d.Quote.Status == models.QuoteExpired {
		out = append(out, models.QuoteDifference{Message: fmt.Sprintf("O orçamento venceu em %s; os preços serão os de hoje.",
			strings.Join(reverse(strings.Split(d.Quote.ValidUntil, "-")), "/"))})
	}
	now := s.Now()
	for _, it := range d.Items {
		p, err := s.products.GetByID(ctx, s.DB, it.ProductID)
		if err != nil {
			return models.QuoteCheck{}, err
		}
		switch {
		case !p.Active || p.PricePending:
			out = append(out, models.QuoteDifference{ProductName: it.ProductName, Message: "produto inativo ou a precificar", Blocking: true})
			continue
		case p.StockQty < it.Qty:
			out = append(out, models.QuoteDifference{ProductName: it.ProductName, Blocking: true,
				Message: fmt.Sprintf("estoque insuficiente: há %d, o orçamento tem %d", p.StockQty, it.Qty)})
		}
		ap, err := bestPromotion(ctx, s.DB, p, it.Qty, p.PriceCents, now, true)
		if err != nil {
			return models.QuoteCheck{}, err
		}
		if ap.unit != it.UnitPriceCents || ap.discount != it.PromoDiscountCents {
			out = append(out, models.QuoteDifference{ProductName: it.ProductName,
				Message: fmt.Sprintf("preço mudou de %s para %s", FormatBRL(it.UnitPriceCents), FormatBRL(ap.unit))})
		}
	}
	// Total da venda se fosse feita agora (mesma conta da conversão).
	total := d.Quote.TotalCents
	in := make([]models.SaleItemInput, 0, len(d.Items))
	var itemsDiscount int64
	for _, it := range d.Items {
		in = append(in, models.SaleItemInput{ProductID: it.ProductID, Qty: it.Qty, DiscountCents: it.DiscountCents})
		itemsDiscount += it.DiscountCents
	}
	if _, subtotal, itemDisc, err := s.price(ctx, s.DB, in, now); err == nil {
		total = subtotal - itemDisc - (d.Quote.DiscountCents - itemsDiscount)
	}
	return models.QuoteCheck{Differences: out, TotalCents: max(total, 0)}, nil
}

func reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// Convert transforma o orçamento em venda com preços e estoque de agora,
// mantendo os descontos manuais (em R$) do orçamento.
func (s *QuoteService) Convert(ctx context.Context, in models.QuoteConvertInput) (models.SaleDetails, error) {
	reqID, err := cleanRequestID(in.RequestID)
	if err != nil {
		return models.SaleDetails{}, err
	}
	var saleID int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		if reqID != nil {
			if saleID, err = s.sales.FindByRequestID(ctx, tx, *reqID); err == nil || !isNotFound(err) {
				return err
			}
		}
		q, err := s.quotes.GetByID(ctx, tx, in.QuoteID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Orçamento %d não encontrado.", in.QuoteID), "")
		}
		if q.Status == models.QuoteConverted || q.Status == models.QuoteCancelled {
			return newErr(CodeStateConflict, "Orçamento %s não pode virar venda.", q.Status)
		}
		items, err := s.quotes.Items(ctx, tx, q.ID)
		if err != nil {
			return err
		}
		lines := make([]saleLine, 0, len(items))
		var itemsDiscount int64
		for _, it := range items {
			lines = append(lines, saleLine{productID: it.ProductID, qty: it.Qty, discount: it.DiscountCents})
			itemsDiscount += it.DiscountCents
		}
		customerID := q.CustomerID
		if c := validCustomerOrNil(in.CustomerID); c != nil {
			customerID = c
		}
		if err := validateSaleHeader(in.PaymentMethod, in.Payments, customerID, 0); err != nil {
			return err
		}
		if saleID, err = createSaleTx(ctx, tx, s.Now, lines, in.PaymentMethod, in.Payments, customerID,
			q.DiscountCents-itemsDiscount, nil, reqID); err != nil {
			return err
		}
		now := s.Now()
		if err := s.quotes.SetStatus(ctx, tx, q.ID, models.QuoteConverted, &saleID, now); err != nil {
			return err
		}
		return audit(ctx, tx, now, nil, "orcamento_convertido", "orcamento", &q.ID, nil, map[string]any{"venda": saleID})
	})
	if err != nil {
		return models.SaleDetails{}, err
	}
	return NewSaleService(s.Base).Details(ctx, saleID)
}

// markQuoteConverted liga o orçamento à venda feita a partir dele na tela de vendas.
func markQuoteConverted(ctx context.Context, tx *sql.Tx, quoteID, saleID int64, now time.Time) error {
	var quotes repository.QuoteRepo
	q, err := quotes.GetByID(ctx, tx, quoteID)
	if err != nil {
		return fromRepo(err, newErr(CodeNotFound, "Orçamento %d não encontrado.", quoteID), "")
	}
	if q.Status == models.QuoteConverted || q.Status == models.QuoteCancelled {
		return newErr(CodeStateConflict, "O orçamento nº %d já está %s; tire-o da venda para continuar.", quoteID, q.Status)
	}
	if err := quotes.SetStatus(ctx, tx, quoteID, models.QuoteConverted, &saleID, now); err != nil {
		return err
	}
	return audit(ctx, tx, now, nil, "orcamento_convertido", "orcamento", &quoteID, nil, map[string]any{"venda": saleID})
}
