// Caminho: pdv/internal/service/bill_service.go

package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
	"pdv/internal/repository"
)

// BillService: contas a pagar. Cadastra (com boleto anexado, opcional), paga
// (com comprovante; em dinheiro do caixa vira sangria no caixa aberto) e, se a
// conta repete todo mês, gera a do mês seguinte ao pagar. Tudo exige a
// permissão "contas a pagar".
type BillService struct {
	Base
	bills repository.BillRepo
	cash  repository.CashRepo
}

func NewBillService(b Base) *BillService { return &BillService{Base: b} }

// BillCategories: categorias aceitas (a ordem é a da tela).
var BillCategories = []string{"Aluguel", "Energia", "Água", "Internet e telefone", "Fornecedor", "Salários", "Impostos", "Manutenção", "Outros"}

var billMethods = []string{"pix", "dinheiro", "transferencia", "boleto", "debito", "credito"}

const maxBillFile = 5 << 20 // 5 MB por arquivo (fica no banco e entra no backup)

func (s *BillService) List(ctx context.Context) ([]models.Bill, error) {
	if _, err := requirePerm(ctx, s.Now(), PermBills); err != nil {
		return nil, err
	}
	return s.bills.List(ctx, s.DB)
}

func validateBill(in *models.BillInput) error {
	in.Description, in.Payee = strings.TrimSpace(in.Description), strings.TrimSpace(in.Payee)
	in.Barcode, in.Notes = strings.TrimSpace(in.Barcode), strings.TrimSpace(in.Notes)
	switch {
	case in.Description == "" || len([]rune(in.Description)) > 200:
		return invalid("Informe a descrição da conta (até 200 caracteres).")
	case !slices.Contains(BillCategories, in.Category):
		return invalid("Escolha a categoria da conta.")
	case in.AmountCents <= 0 || in.AmountCents > MaxPriceCents:
		return invalid("Valor da conta inválido.")
	case len([]rune(in.Payee)) > 200 || len([]rune(in.Barcode)) > 100 || len([]rune(in.Notes)) > 500:
		return invalid("Favorecido, código do boleto ou observação muito longos.")
	}
	if _, err := time.Parse(time.DateOnly, in.DueDate); err != nil {
		return invalid("Data de vencimento inválida.")
	}
	return nil
}

// Save cadastra (ID 0) ou altera uma conta ainda aberta.
func (s *BillService) Save(ctx context.Context, in models.BillInput) (models.Bill, error) {
	if err := validateBill(&in); err != nil {
		return models.Bill{}, err
	}
	id := in.ID
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermBills)
		if err != nil {
			return err
		}
		if id == 0 {
			if id, err = s.bills.Insert(ctx, tx, in, actor.ID(ctx), now); err != nil {
				return err
			}
		} else if ok, err := s.bills.Update(ctx, tx, in); err != nil {
			return err
		} else if !ok {
			return newErr(CodeStateConflict, "Só dá para alterar uma conta que ainda não foi paga.")
		}
		return audit(ctx, tx, now, authBy, "conta_salva", "conta_pagar", &id, nil,
			map[string]any{"descricao": in.Description, "valor": in.AmountCents, "vencimento": in.DueDate})
	})
	if err != nil {
		return models.Bill{}, err
	}
	return s.get(ctx, id)
}

// Pay baixa a conta. fromCash (só em dinheiro): lança a saída como sangria no
// caixa aberto, para o caixa bater. Conta que repete todo mês gera a próxima.
func (s *BillService) Pay(ctx context.Context, in models.PayBillInput) (models.Bill, error) {
	if _, err := time.Parse(time.DateOnly, in.PaidDate); err != nil {
		return models.Bill{}, invalid("Data do pagamento inválida.")
	}
	if in.PaidCents <= 0 || in.PaidCents > MaxPriceCents {
		return models.Bill{}, invalid("Valor pago inválido.")
	}
	if !slices.Contains(billMethods, in.Method) {
		return models.Bill{}, invalid("Escolha a forma de pagamento.")
	}
	if in.FromCash && in.Method != "dinheiro" {
		return models.Bill{}, invalid("Só o pagamento em dinheiro sai do caixa.")
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermBills)
		if err != nil {
			return err
		}
		b, err := s.bills.GetByID(ctx, tx, in.BillID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Conta %d não encontrada.", in.BillID), "")
		}
		if ok, err := s.bills.MarkPaid(ctx, tx, b.ID, in.PaidDate, in.PaidCents, in.Method, in.FromCash); err != nil {
			return err
		} else if !ok {
			return newErr(CodeStateConflict, "Esta conta já foi paga.")
		}
		if in.FromCash {
			session, err := requireOpenCash(ctx, tx)
			if err != nil {
				return err
			}
			expected, err := s.cash.ExpectedCash(ctx, tx, session.ID)
			if err != nil {
				return err
			}
			if in.PaidCents > expected {
				return newErr(CodeInvalid, "O pagamento (%s) é maior que o dinheiro esperado no caixa (%s).", FormatBRL(in.PaidCents), FormatBRL(expected))
			}
			desc := "Conta paga: " + b.Description
			if _, err := s.cash.InsertMovement(ctx, tx, repository.NewCashMovement{
				SessionID: session.ID, Type: models.CashWithdrawal, PaymentMethod: models.PaymentCash, AmountCents: -in.PaidCents,
				Description: &desc, ReferenceType: ptr("bill"), ReferenceID: &b.ID,
			}, now); err != nil {
				return err
			}
		}
		if b.Recurring {
			due, _ := time.Parse(time.DateOnly, b.DueDate)
			next := models.BillInput{Description: b.Description, Category: b.Category, Payee: b.Payee, AmountCents: b.AmountCents,
				DueDate: addMonthClamped(due).Format(time.DateOnly), Notes: b.Notes, Recurring: true}
			if _, err := s.bills.Insert(ctx, tx, next, actor.ID(ctx), now); err != nil {
				return err
			}
		}
		return audit(ctx, tx, now, authBy, "conta_paga", "conta_pagar", &b.ID, nil,
			map[string]any{"valor_pago": in.PaidCents, "forma": in.Method, "do_caixa": in.FromCash, "data": in.PaidDate})
	})
	if err != nil {
		return models.Bill{}, err
	}
	return s.get(ctx, in.BillID)
}

// addMonthClamped: mesmo dia no mês seguinte; 31/01 vira 28 (ou 29)/02.
func addMonthClamped(d time.Time) time.Time {
	first := time.Date(d.Year(), d.Month()+1, 1, 0, 0, 0, 0, d.Location())
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(d.Day(), last), 0, 0, 0, 0, d.Location())
}

// Delete tira da lista uma conta aberta lançada por engano.
func (s *BillService) Delete(ctx context.Context, id int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermBills)
		if err != nil {
			return err
		}
		if ok, err := s.bills.Cancel(ctx, tx, id); err != nil {
			return err
		} else if !ok {
			return newErr(CodeStateConflict, "Só dá para excluir uma conta que ainda não foi paga.")
		}
		return audit(ctx, tx, now, authBy, "conta_excluida", "conta_pagar", &id, nil, nil)
	})
}

// Summary: o que foi pago no mês (AAAA-MM), por categoria.
func (s *BillService) Summary(ctx context.Context, month string) ([]models.BillCategoryTotal, error) {
	if _, err := requirePerm(ctx, s.Now(), PermBills); err != nil {
		return nil, err
	}
	m, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, invalid("Mês inválido.")
	}
	return s.bills.PaidByCategory(ctx, s.DB, m.Format(time.DateOnly), m.AddDate(0, 1, -1).Format(time.DateOnly))
}

// ---------- arquivos ----------

var billMimes = map[string]bool{"application/pdf": true, "image/jpeg": true, "image/png": true, "image/webp": true}

// AddFile anexa o boleto ou o comprovante (PDF ou imagem, até 5 MB, em base64).
func (s *BillService) AddFile(ctx context.Context, billID int64, kind, name, mime, b64 string) ([]models.BillFile, error) {
	if kind != "boleto" && kind != "comprovante" {
		return nil, invalid("Tipo de arquivo inválido.")
	}
	if !billMimes[mime] {
		return nil, invalid("Envie um PDF ou uma imagem (JPG ou PNG).")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(data) == 0 {
		return nil, invalid("Não foi possível ler o arquivo.")
	}
	if len(data) > maxBillFile {
		return nil, invalid("O arquivo tem %.1f MB; o limite é 5 MB.", float64(len(data))/(1<<20))
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 200 {
		name = fmt.Sprintf("%s-%d", kind, billID)
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if _, err := requirePerm(ctx, now, PermBills); err != nil {
			return err
		}
		if _, err := s.bills.GetByID(ctx, tx, billID); err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Conta %d não encontrada.", billID), "")
		}
		_, err := s.bills.InsertFile(ctx, tx, billID, kind, name, mime, data, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.bills.Files(ctx, s.DB, billID)
}

func (s *BillService) Files(ctx context.Context, billID int64) ([]models.BillFile, error) {
	if _, err := requirePerm(ctx, s.Now(), PermBills); err != nil {
		return nil, err
	}
	return s.bills.Files(ctx, s.DB, billID)
}

// FileData devolve nome e conteúdo (para abrir ou salvar).
func (s *BillService) FileData(ctx context.Context, fileID int64) (string, []byte, error) {
	if _, err := requirePerm(ctx, s.Now(), PermBills); err != nil {
		return "", nil, err
	}
	name, data, err := s.bills.FileData(ctx, s.DB, fileID)
	return name, data, fromRepo(err, newErr(CodeNotFound, "Arquivo %d não encontrado.", fileID), "")
}

func (s *BillService) DeleteFile(ctx context.Context, fileID int64) error {
	if _, err := requirePerm(ctx, s.Now(), PermBills); err != nil {
		return err
	}
	return s.bills.DeleteFile(ctx, s.DB, fileID)
}

func (s *BillService) get(ctx context.Context, id int64) (models.Bill, error) {
	b, err := s.bills.GetByID(ctx, s.DB, id)
	return b, fromRepo(err, newErr(CodeNotFound, "Conta %d não encontrada.", id), "")
}
