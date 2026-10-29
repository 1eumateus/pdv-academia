// Caminho: pdv/internal/service/subscription_service.go

package service

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
	"pdv/internal/repository"
)

// SubscriptionService: assinaturas de planos (academia). O plano é um produto
// com duração (plan_days). Pagar a fatura registra uma venda do plano no caixa
// de quem recebe e gera a próxima fatura para a data do pagamento + duração.
// Sem pagamento não há fatura nova: só uma aberta por assinatura.
type SubscriptionService struct {
	Base
	subs      repository.SubscriptionRepo
	products  repository.ProductRepo
	customers repository.CustomerRepo
	salesRepo repository.SaleRepo
	gate      repository.CatracaRepo
	sales     *SaleService
}

func NewSubscriptionService(b Base) *SubscriptionService {
	return &SubscriptionService{Base: b, sales: NewSaleService(b)}
}

const dateLayout = "2006-01-02"

// List traz as assinaturas; Overdue marca fatura aberta com vencimento antes de hoje.
func (s *SubscriptionService) List(ctx context.Context) ([]models.Subscription, error) {
	list, err := s.subs.List(ctx, s.DB)
	today := s.Now().Format(dateLayout)
	for i := range list {
		list[i].Overdue = list[i].PausedAt == nil && list[i].OpenInvoice != nil && list[i].OpenInvoice.DueDate < today
	}
	return list, err
}

func (s *SubscriptionService) Invoices(ctx context.Context, subID int64) ([]models.SubscriptionInvoice, error) {
	return s.subs.Invoices(ctx, s.DB, subID)
}

// Create assina o plano para o cliente; a primeira fatura vence hoje.
// subPrice: valor especial (> 0) ou nil (preço do plano).
func subPrice(cents int64) (*int64, error) {
	if cents == 0 {
		return nil, nil
	}
	if cents < 0 || cents > MaxPriceCents {
		return nil, invalid("Valor especial inválido.")
	}
	return &cents, nil
}

// Create assina o plano; priceCents > 0 = valor especial deste aluno (0 = preço do plano).
// rules: exceção deste aluno (nil = segue as regras do plano).
func (s *SubscriptionService) Create(ctx context.Context, customerID, productID, priceCents int64, rules *models.AccessRules) (models.Subscription, error) {
	price, err := subPrice(priceCents)
	if err != nil {
		return models.Subscription{}, err
	}
	if rules, err = validateRulesPtr(rules); err != nil {
		return models.Subscription{}, err
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		id, err = s.create(ctx, tx, customerID, productID, price, rules, false)
		return err
	})
	if err != nil {
		return models.Subscription{}, err
	}
	return s.get(ctx, id)
}

// Reactivate volta uma assinatura cancelada: ativa o cliente, se inativo, e
// cria uma assinatura nova no mesmo plano e valor, com a mensalidade vencendo
// hoje (paga antes de entrar, como uma assinatura nova).
func (s *SubscriptionService) Reactivate(ctx context.Context, subID int64) (models.Subscription, error) {
	var id int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		old, err := s.subs.GetByID(ctx, tx, subID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Assinatura %d não encontrada.", subID), "")
		}
		if old.Status != "cancelada" {
			return newErr(CodeStateConflict, "A assinatura de %s não está cancelada.", old.CustomerName)
		}
		id, err = s.create(ctx, tx, old.CustomerID, old.ProductID, old.PriceCents, old.Rules, true)
		return err
	})
	if err != nil {
		return models.Subscription{}, err
	}
	return s.get(ctx, id)
}

// SetRules troca a exceção de regras da assinatura (nil = segue o plano); a mensalidade não muda.
func (s *SubscriptionService) SetRules(ctx context.Context, subID int64, rules *models.AccessRules) (models.Subscription, error) {
	rules, err := validateRulesPtr(rules)
	if err != nil {
		return models.Subscription{}, err
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		old, err := s.subs.GetByID(ctx, tx, subID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Assinatura %d não encontrada.", subID), "")
		}
		if err := s.gate.SetRules(ctx, tx, subID, rules); err != nil {
			return err
		}
		return audit(ctx, tx, s.Now(), nil, "assinatura_regras", "assinatura", &subID, old.Rules, rules)
	})
	if err != nil {
		return models.Subscription{}, err
	}
	return s.get(ctx, subID)
}

// PlanRules: as regras de acesso de cada plano que tem alguma.
func (s *SubscriptionService) PlanRules(ctx context.Context) (map[int64]models.AccessRules, error) {
	return s.gate.PlanRules(ctx, s.DB)
}

// SetPlanRules grava as regras de acesso do plano; valem para todos os assinantes sem exceção.
func (s *SubscriptionService) SetPlanRules(ctx context.Context, productID int64, rules models.AccessRules) (models.AccessRules, error) {
	rules, err := validateRules(rules)
	if err != nil {
		return rules, err
	}
	return rules, s.withTx(ctx, func(tx *sql.Tx) error {
		p, err := s.products.GetByID(ctx, tx, productID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", productID), "")
		}
		if p.PlanDays == nil {
			return invalid("%q não é um plano de assinatura.", p.Name)
		}
		if err := s.gate.SetPlanRules(ctx, tx, productID, rules); err != nil {
			return err
		}
		return audit(ctx, tx, s.Now(), nil, "plano_regras", "produto", &productID, nil, rules)
	})
}

func validateRulesPtr(r *models.AccessRules) (*models.AccessRules, error) {
	if r == nil {
		return nil, nil
	}
	v, err := validateRules(*r)
	return &v, err
}

// validateRules confere os dias (1 a 7, sem repetir, em ordem) e os limites.
func validateRules(r models.AccessRules) (models.AccessRules, error) {
	days := []int64{}
	for _, d := range r.Weekdays {
		if d < 1 || d > 7 {
			return r, invalid("Dia da semana inválido.")
		}
		if !slices.Contains(days, d) {
			days = append(days, d)
		}
	}
	slices.Sort(days)
	r.Weekdays = days
	switch {
	case r.WeeklyLimit < 0 || r.WeeklyLimit > 7:
		return r, invalid("As vezes por semana devem ficar entre 1 e 7 (vazio = sem limite).")
	case r.DailyLimit < 0 || r.DailyLimit > 20:
		return r, invalid("As entradas por dia devem ficar entre 1 e 20 (vazio = sem limite).")
	case len(days) > 0 && r.WeeklyLimit > int64(len(days)):
		return r, invalid("Foram marcados %d dia(s) da semana: as vezes por semana não podem passar disso.", len(days))
	}
	return r, nil
}

// create grava a assinatura, as regras de acesso e a primeira mensalidade (vence hoje).
// activate: cliente inativo é ativado em vez de recusado.
func (s *SubscriptionService) create(ctx context.Context, tx *sql.Tx, customerID, productID int64, price *int64, rules *models.AccessRules, activate bool) (int64, error) {
	now := s.Now()
	c, err := s.customers.GetByID(ctx, tx, customerID)
	if err != nil {
		return 0, fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", customerID), "")
	}
	if !c.Active && !activate {
		return 0, newErr(CodeCustomerInactive, "O cliente %s está inativo.", c.Name)
	}
	if !c.Active {
		if err := s.customers.SetActive(ctx, tx, customerID, true, now); err != nil {
			return 0, err
		}
	}
	p, err := s.products.GetByID(ctx, tx, productID)
	if err != nil {
		return 0, fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", productID), "")
	}
	if p.PlanDays == nil {
		return 0, invalid("%q não é um plano de assinatura. Marque a duração do plano no cadastro do produto.", p.Name)
	}
	if !p.Active || p.PricePending {
		return 0, newErr(CodeProductInactive, "O plano %q está inativo ou sem preço.", p.Name)
	}
	if dup, err := s.subs.ActiveExists(ctx, tx, customerID, productID); err != nil {
		return 0, err
	} else if dup {
		return 0, newErr(CodeDuplicate, "%s já tem o plano %q ativo.", c.Name, p.Name)
	}
	id, err := s.subs.Insert(ctx, tx, customerID, productID, price, now)
	if err != nil {
		return 0, err
	}
	if err := s.gate.SetRules(ctx, tx, id, rules); err != nil {
		return 0, err
	}
	if err := s.subs.InsertInvoice(ctx, tx, id, now.Format(dateLayout), cmpOr(price, p.PriceCents), now); err != nil {
		return 0, err
	}
	return id, audit(ctx, tx, now, nil, "assinatura_criada", "assinatura", &id, nil,
		map[string]any{"cliente": c.Name, "plano": p.Name, "valor": p.PriceCents})
}

// Pay recebe a fatura no caixa: grava a venda do plano, baixa a fatura e gera
// a próxima. Repetir o mesmo RequestID devolve a venda já registrada.
// Cancelar ou devolver depois a venda desfaz o pagamento (undoSubscriptionPayment).
func (s *SubscriptionService) Pay(ctx context.Context, in models.PayInvoiceInput) (models.SaleDetails, error) {
	methods := []string{in.PaymentMethod}
	for _, p := range in.Payments {
		methods = append(methods, p.Method)
	}
	for _, m := range methods {
		if m == models.PaymentCredit || m == models.PaymentStoreCredit {
			return models.SaleDetails{}, invalid("A fatura da assinatura é paga no caixa: use dinheiro, Pix ou cartão.")
		}
	}
	reqID, err := cleanRequestID(in.RequestID)
	if err != nil {
		return models.SaleDetails{}, err
	}
	var saleID int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		if reqID != nil {
			id, err := s.salesRepo.FindByRequestID(ctx, tx, *reqID)
			if err == nil {
				saleID = id
				return nil
			}
			if !isNotFound(err) {
				return err
			}
		}
		now := s.Now()
		inv, subID, err := s.subs.OpenInvoice(ctx, tx, in.InvoiceID)
		if err != nil {
			return fromRepo(err, newErr(CodeStateConflict, "Esta fatura não está mais em aberto (já foi paga ou a assinatura foi cancelada)."), "")
		}
		sub, err := s.subs.GetByID(ctx, tx, subID)
		if err != nil {
			return err
		}
		if sub.PausedAt != nil {
			return newErr(CodeStateConflict, "A assinatura de %s está pausada. Retome antes de receber.", sub.CustomerName)
		}
		p, err := s.products.GetByID(ctx, tx, sub.ProductID)
		if err != nil {
			return err
		}
		amount := inv.AmountCents
		line := saleLine{productID: p.ID, qty: 1, unitPrice: &amount,
			name: fmt.Sprintf("%s (fatura de %s)", p.Name, brDate(inv.DueDate))}
		if saleID, err = createSaleTx(ctx, tx, s.Now, []saleLine{line}, in.PaymentMethod, in.Payments, &sub.CustomerID, 0, nil, reqID); err != nil {
			return err
		}
		if ok, err := s.subs.MarkPaid(ctx, tx, inv.ID, saleID, now); err != nil {
			return err
		} else if !ok {
			return newErr(CodeStateConflict, "Esta fatura já foi paga.")
		}
		days := sub.PlanDays
		if p.PlanDays != nil {
			days = *p.PlanDays // duração atual do plano
		}
		next, err := nextDue(in.NextDue, now.In(Location()), days)
		if err != nil {
			return err
		}
		if err := s.subs.InsertInvoice(ctx, tx, sub.ID, next, cmpOr(sub.PriceCents, p.PriceCents), now); err != nil {
			return err
		}
		return audit(ctx, tx, now, nil, "fatura_paga", "assinatura", &sub.ID, nil,
			map[string]any{"fatura": inv.ID, "venda": saleID, "valor": amount, "proximo_vencimento": next})
	})
	if err != nil {
		return models.SaleDetails{}, err
	}
	return s.sales.Details(ctx, saleID)
}

// PayOutside dá baixa na mensalidade em aberto sem lançar no caixa (pagamento
// retroativo de um caixa já fechado à mão). paidDate (AAAA-MM-DD, até hoje) é
// quando foi pago: a próxima vence nessa data + a duração do plano, como em Pay.
// nextDate: próximo vencimento escolhido (vazio = a conta acima).
func (s *SubscriptionService) PayOutside(ctx context.Context, invoiceID int64, paidDate, nextDate string) (models.Subscription, error) {
	var subID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermUsers) // só o administrador
		if err != nil {
			return err
		}
		paid, err := time.ParseInLocation(dateLayout, paidDate, Location())
		if err != nil {
			return invalid("Informe a data do pagamento.")
		}
		if paidDate > now.In(Location()).Format(dateLayout) {
			return invalid("A data do pagamento não pode ser no futuro.")
		}
		inv, id, err := s.subs.OpenInvoice(ctx, tx, invoiceID)
		if err != nil {
			return fromRepo(err, newErr(CodeStateConflict, "Esta fatura não está mais em aberto (já foi paga ou a assinatura foi cancelada)."), "")
		}
		subID = id
		sub, err := s.subs.GetByID(ctx, tx, subID)
		if err != nil {
			return err
		}
		if sub.PausedAt != nil {
			return newErr(CodeStateConflict, "A assinatura de %s está pausada. Retome antes de receber.", sub.CustomerName)
		}
		p, err := s.products.GetByID(ctx, tx, sub.ProductID)
		if err != nil {
			return err
		}
		if ok, err := s.subs.MarkPaidOutside(ctx, tx, inv.ID, paid.Add(12*time.Hour), actor.ID(ctx), now); err != nil {
			return err
		} else if !ok {
			return newErr(CodeStateConflict, "Esta fatura já foi paga.")
		}
		days := sub.PlanDays
		if p.PlanDays != nil {
			days = *p.PlanDays
		}
		next, err := nextDue(nextDate, paid, days)
		if err != nil {
			return err
		}
		if err := s.subs.InsertInvoice(ctx, tx, sub.ID, next, cmpOr(sub.PriceCents, p.PriceCents), now); err != nil {
			return err
		}
		return audit(ctx, tx, now, authBy, "fatura_paga_fora_caixa", "assinatura", &sub.ID, nil,
			map[string]any{"fatura": inv.ID, "valor": inv.AmountCents, "pago_em": paidDate, "proximo_vencimento": next})
	})
	if err != nil {
		return models.Subscription{}, err
	}
	return s.get(ctx, subID)
}

// nextDue: o próximo vencimento escolhido (AAAA-MM-DD) ou, vazio, from + a
// duração do plano. O escolhido precisa ser depois do pagamento.
func nextDue(chosen string, from time.Time, days int64) (string, error) {
	if chosen == "" {
		return from.AddDate(0, 0, int(days)).Format(dateLayout), nil
	}
	if _, err := time.Parse(dateLayout, chosen); err != nil {
		return "", invalid("Próximo vencimento inválido.")
	}
	if chosen <= from.Format(dateLayout) {
		return "", invalid("O próximo vencimento precisa ser depois da data do pagamento.")
	}
	return chosen, nil
}

// UndoOutside desfaz um pagamento lançado fora do caixa: a mensalidade volta a
// ficar em aberto e a seguinte, gerada no lançamento, é cancelada. Só o mais
// recente: se a seguinte já foi paga, recusa (igual a cancelar a venda).
func (s *SubscriptionService) UndoOutside(ctx context.Context, invoiceID int64) (models.Subscription, error) {
	var subID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermUsers) // só o administrador
		if err != nil {
			return err
		}
		id, status, err := s.subs.PaidOutside(ctx, tx, invoiceID)
		if err != nil {
			return fromRepo(err, newErr(CodeStateConflict, "Esta mensalidade não foi lançada fora do caixa (ou já foi desfeita)."), "")
		}
		subID = id
		if n, err := s.subs.LaterPaid(ctx, tx, subID, invoiceID); err != nil {
			return err
		} else if n > 0 {
			return newErr(CodeStateConflict, "A mensalidade seguinte já foi paga. Desfaça primeiro o pagamento mais recente.")
		}
		if err := s.subs.UndoPayment(ctx, tx, subID, invoiceID, status == "ativa"); err != nil {
			return err
		}
		if err := s.subs.ForgetOutside(ctx, tx, invoiceID); err != nil {
			return err
		}
		return audit(ctx, tx, now, authBy, "fatura_fora_caixa_desfeita", "assinatura", &subID, nil, map[string]any{"fatura": invoiceID})
	})
	if err != nil {
		return models.Subscription{}, err
	}
	return s.get(ctx, subID)
}

// ChangePlan troca o plano de uma assinatura ativa. A mensalidade em aberto
// passa a ter o valor do plano novo e vence hoje; ao pagar, a próxima vence na
// data do pagamento + a duração do plano novo (regra de Pay).
// priceCents > 0 = valor especial no plano novo (0 = preço do plano).
func (s *SubscriptionService) ChangePlan(ctx context.Context, subID, productID, priceCents int64) (models.Subscription, error) {
	price, err := subPrice(priceCents)
	if err != nil {
		return models.Subscription{}, err
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		sub, err := s.subs.GetByID(ctx, tx, subID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Assinatura %d não encontrada.", subID), "")
		}
		if sub.Status != "ativa" {
			return newErr(CodeStateConflict, "Só dá para alterar uma assinatura ativa.")
		}
		if sub.PausedAt != nil {
			return newErr(CodeStateConflict, "A assinatura está pausada. Retome antes de alterar o plano.")
		}
		if sub.ProductID == productID && cmpOr(sub.PriceCents, 0) == cmpOr(price, 0) {
			return invalid("Escolha outro plano ou outro valor.")
		}
		p, err := s.products.GetByID(ctx, tx, productID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", productID), "")
		}
		if p.PlanDays == nil || !p.Active || p.PricePending {
			return invalid("%q não é um plano ativo com preço.", p.Name)
		}
		if dup, err := s.subs.ActiveExists(ctx, tx, sub.CustomerID, productID); err != nil {
			return err
		} else if dup && sub.ProductID != productID {
			return newErr(CodeDuplicate, "%s já tem o plano %q ativo.", sub.CustomerName, p.Name)
		}
		if ok, err := s.subs.ChangePlan(ctx, tx, subID, productID, price, now.Format(dateLayout), cmpOr(price, p.PriceCents)); err != nil {
			return err
		} else if !ok {
			return newErr(CodeStateConflict, "Só dá para alterar uma assinatura ativa.")
		}
		return audit(ctx, tx, now, nil, "assinatura_alterada", "assinatura", &subID,
			map[string]any{"plano": sub.PlanName}, map[string]any{"plano": p.Name, "valor": p.PriceCents})
	})
	if err != nil {
		return models.Subscription{}, err
	}
	return s.get(ctx, subID)
}

// Pause congela a assinatura: os dias pausados não contam (ao retomar, o
// vencimento anda o mesmo tanto). Exige a permissão de pausar.
func (s *SubscriptionService) Pause(ctx context.Context, subID int64, reason string) (models.Subscription, error) {
	r := trimPtr(&reason)
	if r != nil && len([]rune(*r)) > 200 {
		return models.Subscription{}, invalid("O motivo deve ter no máximo 200 caracteres.")
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermPauseSubscription)
		if err != nil {
			return err
		}
		if ok, err := s.subs.Pause(ctx, tx, subID, r, now); err != nil {
			return err
		} else if !ok {
			return newErr(CodeStateConflict, "Só dá para pausar uma assinatura ativa que não esteja pausada.")
		}
		return audit(ctx, tx, now, authBy, "assinatura_pausada", "assinatura", &subID, nil, map[string]any{"motivo": str(r)})
	})
	if err != nil {
		return models.Subscription{}, err
	}
	return s.get(ctx, subID)
}

// Resume tira a pausa e empurra o vencimento em aberto pelos dias pausados
// (contados em dias de calendário: pausou ontem e retomou hoje = 1 dia).
func (s *SubscriptionService) Resume(ctx context.Context, subID int64) (models.Subscription, error) {
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermPauseSubscription)
		if err != nil {
			return err
		}
		sub, err := s.subs.GetByID(ctx, tx, subID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Assinatura %d não encontrada.", subID), "")
		}
		if sub.PausedAt == nil {
			return newErr(CodeStateConflict, "A assinatura não está pausada.")
		}
		days := pausedDays(*sub.PausedAt, now)
		if ok, err := s.subs.Resume(ctx, tx, subID, days); err != nil {
			return err
		} else if !ok {
			return newErr(CodeStateConflict, "A assinatura não está pausada.")
		}
		return audit(ctx, tx, now, authBy, "assinatura_retomada", "assinatura", &subID, nil, map[string]any{"dias_pausada": days})
	})
	if err != nil {
		return models.Subscription{}, err
	}
	return s.get(ctx, subID)
}

// pausedDays conta os dias de calendário (horário local) entre a pausa e agora.
func pausedDays(from, to time.Time) int {
	a, b := from.In(to.Location()), to
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, to.Location())
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, to.Location())
	return int(db.Sub(da).Hours()/24 + 0.5)
}

// Cancel encerra a assinatura; a fatura em aberto deixa de existir.
func (s *SubscriptionService) Cancel(ctx context.Context, subID int64) (models.Subscription, error) {
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		ok, err := s.subs.Cancel(ctx, tx, subID, now)
		if err != nil {
			return err
		}
		if !ok {
			return newErr(CodeStateConflict, "A assinatura já está cancelada ou não existe.")
		}
		return audit(ctx, tx, now, nil, "assinatura_cancelada", "assinatura", &subID, nil, nil)
	})
	if err != nil {
		return models.Subscription{}, err
	}
	return s.get(ctx, subID)
}

func (s *SubscriptionService) get(ctx context.Context, id int64) (models.Subscription, error) {
	v, err := s.subs.GetByID(ctx, s.DB, id)
	if err != nil {
		return v, fromRepo(err, newErr(CodeNotFound, "Assinatura %d não encontrada.", id), "")
	}
	v.Overdue = v.PausedAt == nil && v.OpenInvoice != nil && v.OpenInvoice.DueDate < s.Now().Format(dateLayout)
	return v, nil
}

// undoSubscriptionPayment: a venda que pagou uma mensalidade foi cancelada ou
// devolvida. A mensalidade volta a ficar em aberto (com o vencimento original) e
// a próxima, gerada no pagamento, é cancelada. Se a mensalidade seguinte já foi
// paga, recusa: desfaça primeiro o pagamento mais recente. Venda que não é de
// mensalidade: não faz nada.
func undoSubscriptionPayment(ctx context.Context, tx *sql.Tx, saleID int64, now time.Time) error {
	var subs repository.SubscriptionRepo
	invID, subID, subStatus, err := subs.PaidBySale(ctx, tx, saleID)
	if isNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if n, err := subs.LaterPaid(ctx, tx, subID, invID); err != nil {
		return err
	} else if n > 0 {
		return newErr(CodeStateConflict, "Esta venda pagou uma mensalidade, e a mensalidade seguinte já foi paga. Cancele primeiro o pagamento mais recente.")
	}
	if err := subs.UndoPayment(ctx, tx, subID, invID, subStatus == "ativa"); err != nil {
		return err
	}
	return audit(ctx, tx, now, nil, "mensalidade_estornada", "assinatura", &subID, nil, map[string]any{"fatura": invID, "venda": saleID})
}

// cmpOr: o valor apontado, ou def se nil.
func cmpOr(p *int64, def int64) int64 {
	if p != nil {
		return *p
	}
	return def
}

// brDate: "2026-11-10" vira "10/11/2026".
func brDate(d string) string {
	if len(d) != 10 {
		return d
	}
	return d[8:10] + "/" + d[5:7] + "/" + d[0:4]
}
