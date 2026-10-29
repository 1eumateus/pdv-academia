// Caminho: pdv/internal/service/service.go

// Package service implementa as regras de negócio. Cada operação que altera
// mais de uma tabela roda numa única transação, e todos os repositórios dessa
// operação recebem o mesmo *sql.Tx.
package service

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// Limites de entrada. Mantêm qualquer soma bem longe do estouro de int64.
const (
	MaxQty         int64 = 100_000
	MaxPriceCents  int64 = 100_000_000_00 // R$ 100 milhões
	MaxItemsInSale       = 500
	MaxNameLen           = 200
)

// Clock permite fixar a hora nos testes.
type Clock func() time.Time

// Base reúne as dependências comuns aos serviços.
type Base struct {
	DB  *sql.DB
	Now Clock
}

func NewBase(db *sql.DB) Base { return Base{DB: db, Now: time.Now} }

// withTx executa fn numa transação (BEGIN IMMEDIATE, ver db.DSN) e faz rollback
// em qualquer erro ou panic. Nunca repete a operação automaticamente: um retry
// cego poderia duplicar uma venda.
func (b Base) withTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := b.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciar transação: %w", repository.Classify(err))
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
		if err != nil {
			tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("confirmar transação: %w", repository.Classify(err))
	}
	return nil
}

// --- validações comuns ---

func validPaymentMethod(m string) bool {
	switch m {
	case models.PaymentCash, models.PaymentPix, models.PaymentCard, models.PaymentDebit, models.PaymentCreditCard, models.PaymentCredit,
		models.PaymentStoreCredit:
		return true
	}
	return false
}

// validCashMethod: formas de pagamento que movimentam o caixa (todas menos fiado).
func validCashMethod(m string) bool {
	switch m {
	case models.PaymentCash, models.PaymentPix, models.PaymentCard, models.PaymentDebit, models.PaymentCreditCard:
		return true
	}
	return false
}

func validateQty(q int64) error {
	if q <= 0 {
		return invalid("A quantidade deve ser um número inteiro maior que zero.")
	}
	if q > MaxQty {
		return invalid("A quantidade máxima por operação é %d.", MaxQty)
	}
	return nil
}

func validatePrice(p int64) error {
	if p < 0 {
		return invalid("O preço não pode ser negativo.")
	}
	if p > MaxPriceCents {
		return invalid("O preço informado excede o limite suportado.")
	}
	return nil
}

func validateAmount(a int64, what string) error {
	if a <= 0 {
		return invalid("%s deve ser maior que zero.", what)
	}
	if a > MaxPriceCents {
		return invalid("%s excede o limite suportado.", what)
	}
	return nil
}

func cleanName(s, what string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", invalid("Informe o %s.", what)
	}
	if len([]rune(s)) > MaxNameLen {
		return "", invalid("O %s deve ter no máximo %d caracteres.", what, MaxNameLen)
	}
	return s, nil
}

// mulCents multiplica com verificação de estouro.
func mulCents(qty, unit int64) (int64, error) {
	if qty != 0 && unit > math.MaxInt64/qty {
		return 0, invalid("O valor total excede o limite suportado.")
	}
	return qty * unit, nil
}

func addCents(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, invalid("O valor total excede o limite suportado.")
	}
	return a + b, nil
}

func ptr[T any](v T) *T { return &v }

// cleanRequestID valida a chave de idempotência (opcional).
func cleanRequestID(id string) (*string, error) {
	p := trimPtr(&id)
	if p != nil && len(*p) > 64 {
		return nil, invalid("Identificador de pedido inválido.")
	}
	return p, nil
}

// FormatBRL formata centavos como "R$ 1.234,56" para mensagens de erro.
func FormatBRL(cents int64) string {
	sign := ""
	u := uint64(cents)
	if cents < 0 {
		sign = "-"
		u = uint64(-(cents + 1)) + 1 // evita estouro em MinInt64
	}
	reais := fmt.Sprintf("%d", u/100)
	var b strings.Builder
	for i, r := range reais {
		if i > 0 && (len(reais)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("%sR$ %s,%02d", sign, b.String(), u%100)
}

// --- datas ---

// Location é o fuso usado para interpretar "dia" nos relatórios. Usa o fuso
// configurado no Windows.
func Location() *time.Location { return time.Local }

// DayRange converte "AAAA-MM-DD" (data local) em [início, fim) absolutos.
func DayRange(date string) (time.Time, time.Time, error) {
	return PeriodRange(date, date)
}

// PeriodRange converte duas datas locais inclusivas em [início, fim+1 dia).
func PeriodRange(startDate, endDate string) (time.Time, time.Time, error) {
	loc := Location()
	s, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(startDate), loc)
	if err != nil {
		return time.Time{}, time.Time{}, invalid("Data inicial inválida. Use o formato AAAA-MM-DD.")
	}
	e, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(endDate), loc)
	if err != nil {
		return time.Time{}, time.Time{}, invalid("Data final inválida. Use o formato AAAA-MM-DD.")
	}
	if e.Before(s) {
		return time.Time{}, time.Time{}, invalid("A data final não pode ser anterior à data inicial.")
	}
	if e.Sub(s) > 366*24*time.Hour {
		return time.Time{}, time.Time{}, invalid("O período máximo é de um ano.")
	}
	// AddDate respeita mudanças de horário de verão.
	return s, e.AddDate(0, 0, 1), nil
}
