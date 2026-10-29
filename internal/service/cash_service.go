// Caminho: pdv/internal/service/cash_service.go

package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
	"pdv/internal/repository"
)

// Regra do caixa: vendas (de qualquer forma de pagamento), fechamento de
// comanda, recebimentos de fiado e estornos exigem uma sessão de caixa aberta.
// Vendas em dinheiro, pix e cartão geram movimentação na sessão; vendas a prazo
// não, porque nenhum valor entrou. O saldo esperado considera só dinheiro.
type CashService struct {
	Base
	cash  repository.CashRepo
	sales repository.SaleRepo
}

func NewCashService(b Base) *CashService { return &CashService{Base: b} }

// cashOwner: no modo "caixa por usuário" o caixa é de quem está logado; no
// modo geral devolve nil (o caixa único da loja).
func cashOwner(ctx context.Context, q repository.DBTX) (*int64, error) {
	v, _, err := repository.SettingsRepo{}.Get(ctx, q, "cash_per_user")
	if err != nil || v != "1" {
		return nil, err
	}
	u, ok := actor.From(ctx)
	if !ok {
		return nil, newErr(CodeLoginRequired, "Entre com seu usuário para continuar.")
	}
	return &u.ID, nil
}

// requireOpenCash devolve o caixa aberto de quem opera (ou o geral) ou ErrCashNotOpen.
func requireOpenCash(ctx context.Context, q repository.DBTX) (models.CashSession, error) {
	owner, err := cashOwner(ctx, q)
	if err != nil {
		return models.CashSession{}, err
	}
	s, err := repository.CashRepo{}.GetOpen(ctx, q, owner)
	if isNotFound(err) {
		if owner != nil {
			return s, newErr(CodeCashNotOpen, "Você não tem caixa aberto. Abra o seu caixa antes de registrar a operação.")
		}
		return s, newErr(CodeCashNotOpen, "Não há caixa aberto. Abra o caixa antes de registrar a operação.")
	}
	return s, err
}

func (s *CashService) Open(ctx context.Context, openingCents int64) (models.CashSummary, error) {
	if openingCents < 0 || openingCents > MaxPriceCents {
		return models.CashSummary{}, invalid("Saldo inicial inválido.")
	}
	var id int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		owner, err := cashOwner(ctx, tx)
		if err != nil {
			return err
		}
		id, err = s.cash.Open(ctx, tx, openingCents, owner, s.Now())
		if errors.Is(err, repository.ErrUnique) {
			if owner != nil {
				return newErr(CodeCashAlreadyOpen, "Você já tem um caixa aberto.")
			}
			return newErr(CodeCashAlreadyOpen, "Já existe um caixa aberto.")
		}
		if err != nil {
			return err
		}
		return audit(ctx, tx, s.Now(), nil, "caixa_aberto", "caixa", &id, nil, map[string]any{"saldo_inicial": openingCents})
	})
	if err != nil {
		return models.CashSummary{}, err
	}
	return s.Summary(ctx, id)
}

// RegisterMovement lança suprimento (entrada) ou sangria (saída) em dinheiro.
func (s *CashService) RegisterMovement(ctx context.Context, kind string, amountCents int64, description string) (models.CashMovement, error) {
	if kind != models.CashSupply && kind != models.CashWithdrawal {
		return models.CashMovement{}, invalid("Tipo de movimentação inválido: use suprimento ou sangria.")
	}
	if err := validateAmount(amountCents, "O valor"); err != nil {
		return models.CashMovement{}, err
	}
	desc := trimPtr(&description)
	if desc == nil {
		return models.CashMovement{}, invalid("Informe a descrição da movimentação.")
	}
	var out models.CashMovement
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		session, err := requireOpenCash(ctx, tx)
		if err != nil {
			return err
		}
		signed := amountCents
		if kind == models.CashWithdrawal {
			expected, err := s.cash.ExpectedCash(ctx, tx, session.ID)
			if err != nil {
				return err
			}
			if amountCents > expected {
				return newErr(CodeInvalid, "A sangria (%s) é maior que o dinheiro esperado no caixa (%s).",
					FormatBRL(amountCents), FormatBRL(expected))
			}
			signed = -amountCents
		}
		var authBy *int64
		if kind == models.CashWithdrawal {
			if authBy, err = requirePerm(ctx, s.Now(), PermWithdrawal); err != nil {
				return err
			}
		}
		id, err := s.cash.InsertMovement(ctx, tx, repository.NewCashMovement{
			SessionID: session.ID, Type: kind, PaymentMethod: models.PaymentCash, AmountCents: signed, Description: desc,
		}, s.Now())
		if err != nil {
			return err
		}
		if err := audit(ctx, tx, s.Now(), authBy, kind, "movimento_caixa", &id, nil,
			map[string]any{"valor": signed, "descricao": *desc}); err != nil {
			return err
		}
		out, err = s.cash.GetMovement(ctx, tx, id)
		return err
	})
	return out, err
}

// Close fecha a sessão com o valor contado e grava a diferença
// (contado - esperado).
func (s *CashService) Close(ctx context.Context, countedCents int64, notes string) (models.CashSummary, error) {
	if countedCents < 0 || countedCents > MaxPriceCents {
		return models.CashSummary{}, invalid("Valor contado inválido.")
	}
	var id int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		session, err := requireOpenCash(ctx, tx)
		if err != nil {
			return err
		}
		id = session.ID
		return s.closeSession(ctx, tx, session.ID, countedCents, notes, nil)
	})
	if err != nil {
		return models.CashSummary{}, err
	}
	return s.Summary(ctx, id)
}

// CloseOther fecha o caixa aberto de outro usuário (ex.: ele foi embora sem
// fechar). Exige a permissão "fechar caixa de outro usuário".
func (s *CashService) CloseOther(ctx context.Context, sessionID, countedCents int64, notes string) (models.CashSummary, error) {
	if countedCents < 0 || countedCents > MaxPriceCents {
		return models.CashSummary{}, invalid("Valor contado inválido.")
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		session, err := s.cash.GetByID(ctx, tx, sessionID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Sessão de caixa %d não encontrada.", sessionID), "")
		}
		if session.Status != models.CashOpen {
			return newErr(CodeStateConflict, "O caixa já foi fechado.")
		}
		var authBy *int64
		if u, _ := actor.From(ctx); session.OwnerUserID == nil || *session.OwnerUserID != u.ID {
			if authBy, err = requirePerm(ctx, s.Now(), PermCloseOtherCash); err != nil {
				return err
			}
		}
		return s.closeSession(ctx, tx, sessionID, countedCents, notes, authBy)
	})
	if err != nil {
		return models.CashSummary{}, err
	}
	return s.Summary(ctx, sessionID)
}

func (s *CashService) closeSession(ctx context.Context, tx *sql.Tx, id, countedCents int64, notes string, authBy *int64) error {
	expected, err := s.cash.ExpectedCash(ctx, tx, id)
	if err != nil {
		return err
	}
	ok, err := s.cash.Close(ctx, tx, id, expected, countedCents, trimPtr(&notes), s.Now())
	if err != nil {
		return err
	}
	if !ok {
		return newErr(CodeStateConflict, "O caixa já foi fechado.")
	}
	return audit(ctx, tx, s.Now(), authBy, "caixa_fechado", "caixa", &id, nil,
		map[string]any{"esperado": expected, "contado": countedCents, "diferenca": countedCents - expected})
}

// Current devolve o resumo da sessão aberta ou nil se o caixa estiver fechado.
func (s *CashService) Current(ctx context.Context) (*models.CashSummary, error) {
	owner, err := cashOwner(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	session, err := s.cash.GetOpen(ctx, s.DB, owner)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sum, err := s.Summary(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	return &sum, nil
}

func (s *CashService) Summary(ctx context.Context, sessionID int64) (models.CashSummary, error) {
	var out models.CashSummary
	session, err := s.cash.GetByID(ctx, s.DB, sessionID)
	if err != nil {
		return out, fromRepo(err, newErr(CodeNotFound, "Sessão de caixa %d não encontrada.", sessionID), "")
	}
	out.Session = session
	if out.Movements, err = s.cash.Movements(ctx, s.DB, sessionID); err != nil {
		return out, err
	}
	if out.Sales, err = s.sales.ListBySession(ctx, s.DB, sessionID); err != nil {
		return out, err
	}
	if out.ByPaymentMethod, err = s.cash.TotalsByMethod(ctx, s.DB, sessionID); err != nil {
		return out, err
	}
	out.ExpectedCashCents, err = s.cash.ExpectedCash(ctx, s.DB, sessionID)
	return out, err
}

func (s *CashService) ListSessions(ctx context.Context) ([]models.CashSession, error) {
	return s.cash.ListSessions(ctx, s.DB, 100)
}

// ListOpen lista todos os caixas abertos (no modo por usuário, um por pessoa).
func (s *CashService) ListOpen(ctx context.Context) ([]models.CashSession, error) {
	return s.cash.ListOpen(ctx, s.DB)
}

// DayTotals soma por forma de pagamento o que entrou hoje em todos os caixas.
// Mostra dinheiro de todos, então exige ver as vendas de todos.
func (s *CashService) DayTotals(ctx context.Context) ([]models.PaymentTotal, error) {
	if _, err := requirePerm(ctx, s.Now(), PermAllSales); err != nil {
		return nil, err
	}
	now := s.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return s.cash.TotalsBetween(ctx, s.DB, day, day.AddDate(0, 0, 1))
}
