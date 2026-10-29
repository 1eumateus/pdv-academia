// Caminho: pdv/internal/service/sale_service.go

package service

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"pdv/internal/models"
	"pdv/internal/repository"
)

const maxReasonLen = 500

type SaleService struct {
	Base
	customers repository.CustomerRepo
	sales     repository.SaleRepo
	cash      repository.CashRepo
}

func NewSaleService(b Base) *SaleService { return &SaleService{Base: b} }

// saleLine é um item já consolidado. unitPrice nil = usar o preço atual do
// produto; comandas passam o preço gravado no lançamento.
type saleLine struct {
	productID int64
	qty       int64
	unitPrice *int64
	name      string
	discount  int64 // desconto do item, em centavos
}

// consolidate soma itens repetidos do mesmo produto e valida as quantidades.
func consolidate(items []models.SaleItemInput) ([]saleLine, error) {
	if len(items) == 0 {
		return nil, invalid("A venda precisa ter pelo menos um item.")
	}
	if len(items) > MaxItemsInSale {
		return nil, invalid("A venda pode ter no máximo %d itens.", MaxItemsInSale)
	}
	qty := map[int64]int64{}
	disc := map[int64]int64{}
	order := []int64{}
	for _, it := range items {
		if it.ProductID <= 0 {
			return nil, invalid("Item com produto inválido.")
		}
		if err := validateQty(it.Qty); err != nil {
			return nil, err
		}
		if _, seen := qty[it.ProductID]; !seen {
			order = append(order, it.ProductID)
		}
		if it.DiscountCents < 0 || it.DiscountCents > MaxPriceCents {
			return nil, invalid("Desconto do item inválido.")
		}
		qty[it.ProductID] += it.Qty
		disc[it.ProductID] += it.DiscountCents
		if qty[it.ProductID] > MaxQty {
			return nil, invalid("A quantidade máxima por produto é %d.", MaxQty)
		}
	}
	out := make([]saleLine, 0, len(order))
	for _, id := range order {
		out = append(out, saleLine{productID: id, qty: qty[id], discount: disc[id]})
	}
	return out, nil
}

func validateSaleHeader(method string, payments []models.PaymentInput, customerID *int64, discount int64) error {
	if len(payments) == 0 && !validPaymentMethod(method) {
		return invalid("Forma de pagamento desconhecida: %q.", method)
	}
	hasFiado := method == models.PaymentCredit && len(payments) == 0
	hasVale := method == models.PaymentStoreCredit && len(payments) == 0
	for _, p := range payments {
		hasFiado = hasFiado || p.Method == models.PaymentCredit
		hasVale = hasVale || p.Method == models.PaymentStoreCredit
	}
	if hasFiado && (customerID == nil || *customerID <= 0) {
		return invalid("Venda a prazo exige a seleção de um cliente.")
	}
	if hasVale && (customerID == nil || *customerID <= 0) {
		return invalid("Pagamento com vale-troca exige a seleção do cliente dono do vale.")
	}
	if discount < 0 {
		return invalid("O desconto não pode ser negativo.")
	}
	return nil
}

// CreateSale registra uma venda inteira numa única transação.
func (s *SaleService) CreateSale(ctx context.Context, in models.CreateSaleInput) (models.SaleDetails, error) {
	lines, err := consolidate(in.Items)
	if err != nil {
		return models.SaleDetails{}, err
	}
	if err := validateSaleHeader(in.PaymentMethod, in.Payments, in.CustomerID, in.DiscountCents); err != nil {
		return models.SaleDetails{}, err
	}
	reqID, err := cleanRequestID(in.RequestID)
	if err != nil {
		return models.SaleDetails{}, err
	}
	delivery, err := cleanDelivery(in.Delivery)
	if err != nil {
		return models.SaleDetails{}, err
	}
	var saleID int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		// Pedido repetido: devolve a venda existente sem gravar nada. A leitura
		// ocorre dentro da transação de escrita, então não há corrida.
		if reqID != nil {
			saleID, err = s.sales.FindByRequestID(ctx, tx, *reqID)
			if err == nil || !isNotFound(err) {
				return err
			}
		}
		if saleID, err = createSaleTx(ctx, tx, s.Now, lines, in.PaymentMethod, in.Payments, in.CustomerID, in.DiscountCents, nil, reqID); err != nil {
			return err
		}
		if delivery != nil {
			if err := s.sales.InsertDelivery(ctx, tx, saleID, *delivery); err != nil {
				return err
			}
		}
		if qid := validCustomerOrNil(in.QuoteID); qid != nil {
			return markQuoteConverted(ctx, tx, *qid, saleID, s.Now())
		}
		return nil
	})
	if err != nil {
		return models.SaleDetails{}, err
	}
	return s.Details(ctx, saleID)
}

// cleanDelivery valida o endereço de entrega (nil = retirada no balcão).
func cleanDelivery(d *models.Delivery) (*models.Delivery, error) {
	if d == nil {
		return nil, nil
	}
	out := models.Delivery{Address: strings.TrimSpace(d.Address), Phone: strings.TrimSpace(d.Phone), Notes: strings.TrimSpace(d.Notes)}
	switch {
	case out.Address == "":
		return nil, invalid("Informe o endereço da entrega.")
	case len([]rune(out.Address)) > 400:
		return nil, invalid("O endereço da entrega deve ter no máximo 400 caracteres.")
	case len([]rune(out.Phone)) > 30:
		return nil, invalid("O telefone da entrega deve ter no máximo 30 caracteres.")
	case len([]rune(out.Notes)) > 300:
		return nil, invalid("A observação da entrega deve ter no máximo 300 caracteres.")
	}
	return &out, nil
}

// createSaleTx é o núcleo compartilhado por CreateSale e CloseComanda. Toda
// escrita usa a mesma tx; qualquer erro faz o chamador desfazer tudo.
func createSaleTx(ctx context.Context, tx *sql.Tx, clock Clock, lines []saleLine, method string, payIn []models.PaymentInput,
	customerID *int64, discount int64, comandaID *int64, requestID *string) (int64, error) {
	var (
		products  repository.ProductRepo
		customers repository.CustomerRepo
		sales     repository.SaleRepo
		cash      repository.CashRepo
	)
	now := clock()

	session, err := requireOpenCash(ctx, tx)
	if err != nil {
		return 0, err
	}

	// Preços e situação vêm do banco, dentro da transação.
	var subtotal, itemsDiscount, promoDiscount int64
	items := make([]models.SaleItem, 0, len(lines))
	for _, l := range lines {
		p, err := products.GetByID(ctx, tx, l.productID)
		if err != nil {
			return 0, fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", l.productID), "")
		}
		if !p.Active {
			return 0, newErr(CodeProductInactive, "O produto %q está inativo e não pode ser vendido.", p.Name)
		}
		if p.PricePending {
			return 0, newErr(CodeProductInactive, "O produto %q está \"a precificar\" (sem preço de venda definido). Peça ao gerente para definir o preço.", p.Name)
		}
		unit, name := p.PriceCents, p.Name
		if l.unitPrice != nil {
			unit, name = *l.unitPrice, l.name
		}
		// Melhor promoção vigente (venda direta: preço e "leve X pague Y"; comanda: só "leve X pague Y").
		ap, err := bestPromotion(ctx, tx, p, l.qty, unit, now, l.unitPrice == nil)
		if err != nil {
			return 0, err
		}
		unit = ap.unit
		lineTotal, err := mulCents(l.qty, unit)
		if err != nil {
			return 0, err
		}
		if subtotal, err = addCents(subtotal, lineTotal); err != nil {
			return 0, err
		}
		if l.discount > lineTotal-ap.discount {
			return 0, invalid("O desconto de %q (%s) não pode ser maior que o valor do item (%s).",
				name, FormatBRL(l.discount), FormatBRL(lineTotal-ap.discount))
		}
		itemsDiscount += l.discount
		promoDiscount += ap.discount
		it := models.SaleItem{ProductID: p.ID, ProductNameSnapshot: name, Qty: l.qty,
			UnitPriceCents: unit, SubtotalCents: lineTotal, DiscountCents: l.discount, PromoDiscountCents: ap.discount,
			UnitCostCents: p.CostCents}
		if ap.promo != nil {
			it.PromotionID, it.PromoLabel = &ap.promo.ID, promoLabel(*ap.promo)
		}
		items = append(items, it)
	}
	// O desconto da venda soma o de cada item e o desconto geral.
	if discount, err = addCents(discount, itemsDiscount); err != nil {
		return 0, err
	}
	base := subtotal - promoDiscount // o que o cliente pagaria só com as promoções
	if discount > base {
		return 0, newErr(CodeInvalid, "O desconto (%s) não pode ser maior que o subtotal (%s).",
			FormatBRL(discount), FormatBRL(base))
	}
	total := base - discount

	// Desconto acima do limite do operador exige permissão (ou autorização de gerente).
	var discountAuth *int64
	overLimit := false
	if discount > 0 {
		v, _, err := repository.SettingsRepo{}.Get(ctx, tx, "operator_max_discount_pct")
		if err != nil {
			return 0, err
		}
		pct, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			pct = defaultMaxDiscountPct
		}
		if discount > base/100*pct+base%100*pct/100 { // base*pct/100 sem estourar int64
			overLimit = true
			if discountAuth, err = requirePerm(ctx, now, PermDiscount); err != nil {
				return 0, err
			}
		}
	}

	payments, summary, fiado, err := resolvePayments(method, payIn, total)
	if err != nil {
		return 0, err
	}
	method = summary

	if fiado > 0 {
		c, err := customers.GetByID(ctx, tx, *customerID)
		if err != nil {
			return 0, fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", *customerID), "")
		}
		if !c.Active {
			return 0, newErr(CodeCustomerInactive, "O cliente %s está inativo e não pode comprar a prazo.", c.Name)
		}
		if c.CreditBalanceCents+fiado > c.CreditLimitCents {
			return 0, creditLimitErr(c, fiado)
		}
	} else {
		customerID = validCustomerOrNil(customerID)
		if customerID != nil {
			if _, err := customers.GetByID(ctx, tx, *customerID); err != nil {
				return 0, fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", *customerID), "")
			}
		}
	}

	saleID, err := sales.Insert(ctx, tx, repository.NewSale{
		CustomerID: customerID, ComandaID: comandaID, CashSessionID: &session.ID,
		SubtotalCents: subtotal, DiscountCents: discount + promoDiscount, TotalCents: total, PaymentMethod: method,
		RequestID: requestID,
	}, now)
	if err != nil {
		return 0, err
	}
	if overLimit {
		if err := audit(ctx, tx, now, discountAuth, "desconto_acima_limite", "venda", &saleID, nil,
			map[string]any{"subtotal": subtotal, "desconto": discount}); err != nil {
			return 0, err
		}
	}

	for _, it := range items {
		it.SaleID = saleID
		if _, err := sales.InsertItem(ctx, tx, it); err != nil {
			return 0, err
		}
		// Baixa condicional: o UPDATE só passa se ainda houver saldo, mesmo que
		// outra operação tenha vendido o produto depois da leitura acima.
		if err := applyStockChange(ctx, tx, now, repository.NewMovement{
			ProductID: it.ProductID, QtyChange: -it.Qty, Reason: models.StockSale,
			ReferenceType: ptr("sale"), ReferenceID: &saleID,
		}); err != nil {
			return 0, err
		}
	}

	if fiado > 0 {
		ok, err := customers.AddDebtWithinLimit(ctx, tx, *customerID, fiado, now)
		if err != nil {
			return 0, err
		}
		if !ok {
			c, err := customers.GetByID(ctx, tx, *customerID)
			if err != nil {
				return 0, err
			}
			return 0, creditLimitErr(c, fiado)
		}
	}
	// Cada parte que não é fiado nem vale-troca entra no caixa na sua forma (o troco já ficou de fora).
	for _, p := range payments {
		if err := sales.InsertPayment(ctx, tx, saleID, p); err != nil {
			return 0, err
		}
		if p.Method == models.PaymentStoreCredit {
			// O UPDATE só passa se o saldo cobrir: não depende da leitura de antes.
			ok, err := customers.UseStoreCredit(ctx, tx, *customerID, p.AmountCents, now)
			if err != nil {
				return 0, err
			}
			if !ok {
				c, err := customers.GetByID(ctx, tx, *customerID)
				if err != nil {
					return 0, err
				}
				return 0, newErr(CodeStateConflict, "O vale-troca de %s é de %s, menor que a parte paga com ele (%s).",
					c.Name, FormatBRL(c.StoreCreditCents), FormatBRL(p.AmountCents))
			}
			continue
		}
		if p.Method == models.PaymentCredit {
			continue
		}
		if _, err := cash.InsertMovement(ctx, tx, repository.NewCashMovement{
			SessionID: session.ID, Type: models.CashSale, PaymentMethod: p.Method, AmountCents: p.AmountCents,
			Description: ptr(fmt.Sprintf("Venda #%d", saleID)), ReferenceType: ptr("sale"), ReferenceID: &saleID,
		}, now); err != nil {
			return 0, err
		}
	}
	return saleID, nil
}

const maxPaymentParts = 10

// resolvePayments confere as partes do pagamento contra o total da venda e
// devolve as partes normalizadas, o resumo para sales.payment_method (a forma
// única ou "misto") e quanto vai para o fiado. Sem partes, tudo vai na forma
// única informada (como antes). Troco só sai da parte em dinheiro.
func resolvePayments(method string, in []models.PaymentInput, total int64) ([]models.PaymentInput, string, int64, error) {
	if len(in) == 0 {
		if total == 0 {
			return nil, method, 0, nil
		}
		in = []models.PaymentInput{{Method: method, AmountCents: total}}
	}
	if len(in) > maxPaymentParts {
		return nil, "", 0, invalid("Use no máximo %d formas de pagamento na mesma venda.", maxPaymentParts)
	}
	var sum, fiado int64
	methods := map[string]bool{}
	out := make([]models.PaymentInput, 0, len(in))
	for _, p := range in {
		if !validPaymentMethod(p.Method) {
			return nil, "", 0, invalid("Forma de pagamento desconhecida: %q.", p.Method)
		}
		if p.AmountCents <= 0 || p.AmountCents > MaxPriceCents {
			return nil, "", 0, invalid("Valor inválido na parte em %s.", methodName(p.Method))
		}
		p.CardBrand = strings.TrimSpace(p.CardBrand)
		if len([]rune(p.CardBrand)) > 30 {
			return nil, "", 0, invalid("A bandeira do cartão deve ter no máximo 30 caracteres.")
		}
		if p.Installments < 0 || p.Installments > 24 || (p.Installments > 1 && p.Method != models.PaymentCreditCard) {
			return nil, "", 0, invalid("Parcelas só no crédito, de 1 a 24.")
		}
		if p.Method != models.PaymentDebit && p.Method != models.PaymentCreditCard && p.Method != models.PaymentCard {
			p.CardBrand, p.Installments = "", 0
		}
		if p.Method == models.PaymentCash {
			if p.ReceivedCents != 0 && p.ReceivedCents < p.AmountCents {
				return nil, "", 0, invalid("O valor recebido em dinheiro (%s) é menor que a parte em dinheiro (%s).",
					FormatBRL(p.ReceivedCents), FormatBRL(p.AmountCents))
			}
			if p.ReceivedCents > MaxPriceCents {
				return nil, "", 0, invalid("Valor recebido inválido.")
			}
		} else {
			p.ReceivedCents = 0
		}
		var err error
		if sum, err = addCents(sum, p.AmountCents); err != nil {
			return nil, "", 0, err
		}
		if p.Method == models.PaymentCredit {
			fiado += p.AmountCents
		}
		methods[p.Method] = true
		out = append(out, p)
	}
	if sum != total {
		return nil, "", 0, invalid("A soma dos pagamentos (%s) é diferente do total da venda (%s).", FormatBRL(sum), FormatBRL(total))
	}
	summary := models.PaymentMixed
	if len(methods) == 1 {
		summary = out[0].Method
	}
	return out, summary, fiado, nil
}

var methodNames = map[string]string{
	models.PaymentCash: "dinheiro", models.PaymentPix: "Pix", models.PaymentCard: "cartão", models.PaymentDebit: "débito",
	models.PaymentCreditCard: "crédito", models.PaymentCredit: "a prazo", models.PaymentStoreCredit: "vale-troca",
}

func methodName(m string) string {
	if n, ok := methodNames[m]; ok {
		return n
	}
	return m
}

func creditLimitErr(c models.Customer, total int64) *Error {
	avail := c.CreditLimitCents - c.CreditBalanceCents
	if avail < 0 {
		avail = 0
	}
	return newErr(CodeCreditLimitExceeded,
		"Limite de crédito excedido para %s. Limite: %s; dívida atual: %s; disponível: %s; valor da venda: %s.",
		c.Name, FormatBRL(c.CreditLimitCents), FormatBRL(c.CreditBalanceCents), FormatBRL(avail), FormatBRL(total))
}

func validCustomerOrNil(id *int64) *int64 {
	if id == nil || *id <= 0 {
		return nil
	}
	return id
}

// CancelSale cancela de forma auditável:
//   - a venda fica com status 'cancelada', data e motivo (nada é apagado);
//   - o estoque volta por movimentações 'cancelamento';
//   - fiado: a dívida do cliente é abatida pelo total. Se o cliente já pagou
//     parte e a dívida atual for menor que o total, o cancelamento é recusado:
//     estorne os recebimentos antes, para não gerar saldo negativo;
//   - dinheiro/pix/cartão: lança 'estorno_venda' no caixa aberto. Isso registra
//     a obrigação de reembolso; o sistema não faz devolução bancária;
//   - venda de comanda: a comanda continua fechada, com seus itens como
//     histórico, e a venda guarda o vínculo (comanda_id). Ela não é reaberta.
func (s *SaleService) CancelSale(ctx context.Context, saleID int64, reason string) (models.SaleDetails, error) {
	r := strings.TrimSpace(reason)
	if r == "" {
		return models.SaleDetails{}, invalid("Informe o motivo do cancelamento.")
	}
	if len([]rune(r)) > maxReasonLen {
		return models.SaleDetails{}, invalid("O motivo deve ter no máximo %d caracteres.", maxReasonLen)
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		session, err := requireOpenCash(ctx, tx)
		if err != nil {
			return err
		}
		sale, err := s.sales.GetByID(ctx, tx, saleID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeSaleNotFound, "Venda", saleID), "")
		}
		if sale.Status == models.SaleCancelled {
			return newErr(CodeSaleCancelled, "A venda #%d já foi cancelada.", saleID)
		}
		authBy, err := requirePerm(ctx, now, PermCancelSale)
		if err != nil {
			return err
		}
		// Com devolução feita, cancelar estornaria de novo o que já voltou.
		if returned, err := (repository.ReturnRepo{}).ReturnedQty(ctx, tx, saleID); err != nil {
			return err
		} else if len(returned) > 0 {
			return newErr(CodeStateConflict, "A venda #%d já teve devolução e não pode ser cancelada. Para desfazer o resto, devolva os itens que faltam.", saleID)
		}
		if err := audit(ctx, tx, now, authBy, "venda_cancelada", "venda", &saleID,
			map[string]any{"status": sale.Status, "total": sale.TotalCents, "forma": sale.PaymentMethod},
			map[string]any{"status": models.SaleCancelled, "motivo": r}); err != nil {
			return err
		}
		// Venda feita a partir de orçamento: o orçamento volta a "aberto".
		if qid, err := (repository.QuoteRepo{}).ReopenBySale(ctx, tx, saleID, now); err != nil {
			return err
		} else if qid != 0 {
			if err := audit(ctx, tx, now, nil, "orcamento_reaberto", "orcamento", &qid, nil,
				map[string]any{"motivo": "venda cancelada", "venda": saleID}); err != nil {
				return err
			}
		}
		ok, err := s.sales.MarkCancelled(ctx, tx, saleID, r, now)
		if err != nil {
			return err
		}
		if !ok {
			return newErr(CodeSaleCancelled, "A venda #%d já foi cancelada.", saleID)
		}
		// Venda de mensalidade: a mensalidade volta a ficar em aberto.
		if err := undoSubscriptionPayment(ctx, tx, saleID, now); err != nil {
			return err
		}
		items, err := s.sales.Items(ctx, tx, saleID)
		if err != nil {
			return err
		}
		for _, it := range items {
			if err := applyStockChange(ctx, tx, now, repository.NewMovement{
				ProductID: it.ProductID, QtyChange: it.Qty, Reason: models.StockCancellation,
				ReferenceType: ptr("sale"), ReferenceID: &saleID, Notes: &r,
			}); err != nil {
				return err
			}
		}
		payments, err := s.sales.Payments(ctx, tx, saleID)
		if err != nil {
			return err
		}
		var fiado int64
		for _, p := range payments {
			if p.Method == models.PaymentCredit {
				fiado += p.AmountCents
				continue
			}
			if p.Method == models.PaymentStoreCredit {
				// O vale usado volta para o saldo do cliente.
				if err := s.customers.AddStoreCredit(ctx, tx, *sale.CustomerID, p.AmountCents, now); err != nil {
					return err
				}
				continue
			}
			if _, err := s.cash.InsertMovement(ctx, tx, repository.NewCashMovement{
				SessionID: session.ID, Type: models.CashSaleRefund, PaymentMethod: p.Method,
				AmountCents:   -p.AmountCents,
				Description:   ptr(fmt.Sprintf("Estorno da venda #%d (reembolso ao cliente feito pelo operador)", saleID)),
				ReferenceType: ptr("sale"), ReferenceID: &saleID,
			}, now); err != nil {
				return err
			}
		}
		if fiado > 0 {
			ok, err := s.customers.ReduceDebt(ctx, tx, *sale.CustomerID, fiado, now)
			if err != nil {
				return err
			}
			if !ok {
				return newErr(CodeStateConflict,
					"A dívida atual do cliente é menor que a parte a prazo da venda (%s), porque já houve recebimentos. Estorne os recebimentos antes de cancelar.",
					FormatBRL(fiado))
			}
		}
		return nil
	})
	if err != nil {
		return models.SaleDetails{}, err
	}
	return s.Details(ctx, saleID)
}

func (s *SaleService) Details(ctx context.Context, saleID int64) (models.SaleDetails, error) {
	sale, err := s.sales.GetByID(ctx, s.DB, saleID)
	if err != nil {
		return models.SaleDetails{}, fromRepo(err, notFoundErr(CodeSaleNotFound, "Venda", saleID), "")
	}
	items, err := s.sales.Items(ctx, s.DB, saleID)
	if err != nil {
		return models.SaleDetails{}, err
	}
	payments, err := s.sales.Payments(ctx, s.DB, saleID)
	if err != nil {
		return models.SaleDetails{}, err
	}
	returns, err := (repository.ReturnRepo{}).ListBySale(ctx, s.DB, saleID)
	if err != nil {
		return models.SaleDetails{}, err
	}
	delivery, err := s.sales.Delivery(ctx, s.DB, saleID)
	if err != nil {
		return models.SaleDetails{}, err
	}
	out := models.SaleDetails{Sale: sale, Items: items, Payments: payments, Returns: returns, Delivery: delivery}
	if sale.CustomerID != nil {
		c, err := s.customers.GetByID(ctx, s.DB, *sale.CustomerID)
		if err != nil {
			return models.SaleDetails{}, err
		}
		out.Customer = &c
	}
	return out, nil
}

// ListByPeriod lista vendas entre duas datas locais (inclusive).
// ListByPeriod lista as vendas do período; quem não vê as de todos recebe só as próprias.
func (s *SaleService) ListByPeriod(ctx context.Context, startDate, endDate string, userID int64) ([]models.Sale, error) {
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	return s.sales.ListBetween(ctx, s.DB, start, end, salesScope(ctx, userID))
}
