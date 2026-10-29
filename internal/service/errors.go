// Caminho: pdv/internal/service/errors.go

package service

import (
	"errors"
	"fmt"
	"strings"

	"pdv/internal/repository"
)

// Code é a categoria estável de um erro. O frontend decide o fluxo pelo código,
// nunca pelo texto traduzido.
type Code string

const (
	CodeInsufficientStock   Code = "ESTOQUE_INSUFICIENTE"
	CodeCreditLimitExceeded Code = "LIMITE_CREDITO_EXCEDIDO"
	CodeCustomerNotFound    Code = "CLIENTE_INEXISTENTE"
	CodeCustomerInactive    Code = "CLIENTE_INATIVO"
	CodeProductNotFound     Code = "PRODUTO_INEXISTENTE"
	CodeProductInactive     Code = "PRODUTO_INATIVO"
	CodeComandaNotFound     Code = "COMANDA_INEXISTENTE"
	CodeComandaClosed       Code = "COMANDA_FECHADA"
	CodeSaleNotFound        Code = "VENDA_INEXISTENTE"
	CodeSaleCancelled       Code = "VENDA_JA_CANCELADA"
	CodeCashNotOpen         Code = "CAIXA_NAO_ABERTO"
	CodeCashAlreadyOpen     Code = "CAIXA_JA_ABERTO"
	CodeInvalid             Code = "OPERACAO_INVALIDA"
	CodeStateConflict       Code = "CONFLITO_DE_ESTADO"
	CodeDuplicate           Code = "RECURSO_DUPLICADO"
	CodeNotFound            Code = "NAO_ENCONTRADO"
	CodeBusy                Code = "BANCO_OCUPADO"
	CodeStorage             Code = "ERRO_ARMAZENAMENTO"
	CodeInternal            Code = "ERRO_INTERNO"
	CodeLoginRequired       Code = "LOGIN_NECESSARIO"
	CodeAlreadyLoggedIn     Code = "USUARIO_JA_LOGADO"      // o mesmo usuário está logado em outro lugar
	CodeAuthRequired        Code = "AUTORIZACAO_NECESSARIA" // um gerente pode autorizar
	CodeForbidden           Code = "SEM_PERMISSAO"          // só o papel certo pode fazer
	CodeReadOnly            Code = "SOMENTE_CONSULTA"       // licença vencida, teste encerrado ou relógio voltado
)

// Error é um erro de domínio: tem código estável e mensagem em português
// pronta para o usuário.
type Error struct {
	Code    Code
	Message string
	Err     error
	// Permission acompanha CodeAuthRequired: qual permissão o gerente deve autorizar.
	Permission string
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

// Is permite errors.Is(err, service.ErrInsufficientStock) comparando só o código.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// Sentinelas para errors.Is.
var (
	ErrInsufficientStock   = &Error{Code: CodeInsufficientStock}
	ErrCreditLimitExceeded = &Error{Code: CodeCreditLimitExceeded}
	ErrCustomerNotFound    = &Error{Code: CodeCustomerNotFound}
	ErrCustomerInactive    = &Error{Code: CodeCustomerInactive}
	ErrProductNotFound     = &Error{Code: CodeProductNotFound}
	ErrProductInactive     = &Error{Code: CodeProductInactive}
	ErrComandaNotFound     = &Error{Code: CodeComandaNotFound}
	ErrComandaClosed       = &Error{Code: CodeComandaClosed}
	ErrSaleNotFound        = &Error{Code: CodeSaleNotFound}
	ErrSaleCancelled       = &Error{Code: CodeSaleCancelled}
	ErrCashNotOpen         = &Error{Code: CodeCashNotOpen}
	ErrCashAlreadyOpen     = &Error{Code: CodeCashAlreadyOpen}
	ErrInvalid             = &Error{Code: CodeInvalid}
	ErrStateConflict       = &Error{Code: CodeStateConflict}
	ErrDuplicate           = &Error{Code: CodeDuplicate}
)

func newErr(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func invalid(format string, args ...any) *Error { return newErr(CodeInvalid, format, args...) }

// InsufficientStockError carrega os números para quem quiser exibi-los.
type InsufficientStockError struct {
	ProductID   int64
	ProductName string
	Available   int64
	Requested   int64
}

func (e *InsufficientStockError) Error() string {
	return fmt.Sprintf("Estoque insuficiente para o produto %q. Disponível: %d; solicitado: %d.", e.ProductName, e.Available, e.Requested)
}

func insufficientStock(id int64, name string, available, requested int64) *Error {
	inner := &InsufficientStockError{ProductID: id, ProductName: name, Available: available, Requested: requested}
	return &Error{Code: CodeInsufficientStock, Message: inner.Error(), Err: inner}
}

// Public converte qualquer erro no par (código, mensagem) que pode ir para a
// tela. Erros técnicos viram mensagens genéricas, sem detalhes do banco. O
// segundo retorno indica se o erro é técnico (e deve ser registrado em log).
func Public(err error) (code Code, message string, technical bool) {
	var de *Error
	if errors.As(err, &de) {
		return de.Code, de.Message, false
	}
	switch {
	case errors.Is(err, repository.ErrBusy):
		return CodeBusy, "O banco de dados está ocupado. Aguarde alguns segundos e tente novamente.", true
	case errors.Is(err, repository.ErrStorage):
		return CodeStorage, "Falha ao gravar no disco. Verifique o espaço livre e as permissões e tente novamente.", true
	}
	return CodeInternal, "Ocorreu um erro inesperado. A operação não foi concluída.", true
}

// fromRepo traduz erros de repositório comuns em erros de domínio.
func fromRepo(err error, notFound *Error, duplicateMsg string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrNotFound) && notFound != nil:
		return notFound
	case errors.Is(err, repository.ErrUnique) && duplicateMsg != "":
		return newErr(CodeDuplicate, "%s", duplicateMsg)
	}
	return err
}

func notFoundErr(code Code, what string, id int64) *Error {
	return newErr(code, "%s %d não encontrado(a).", what, id)
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}
