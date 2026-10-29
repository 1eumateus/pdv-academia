// Caminho: pdv/internal/repository/repository.go

// Package repository executa SQL parametrizado. Não contém regras de negócio
// nem abre transações: quem decide a transação é o serviço, que passa o
// executor (DB ou Tx) para cada chamada.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// DBTX é satisfeito por *sql.DB e *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var (
	_ DBTX = (*sql.DB)(nil)
	_ DBTX = (*sql.Tx)(nil)
)

// Categorias de erro de banco, para os serviços decidirem sem olhar texto.
var (
	ErrNotFound   = errors.New("registro não encontrado")
	ErrUnique     = errors.New("valor duplicado")
	ErrForeignKey = errors.New("referência inválida")
	ErrCheck      = errors.New("valor fora das regras do banco")
	ErrTrigger    = errors.New("operação recusada pelo banco")
	ErrBusy       = errors.New("banco ocupado")
	ErrStorage    = errors.New("falha de armazenamento")
)

// wrap adiciona contexto e a categoria do erro. sql.ErrNoRows vira ErrNotFound.
func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", op, ErrNotFound)
	}
	if cat := classify(err); cat != nil {
		return fmt.Errorf("%s: %w: %w", op, cat, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// Classify acrescenta a categoria (ErrBusy, ErrStorage...) a um erro do driver,
// para erros que não passaram por um repositório (BEGIN/COMMIT).
func Classify(err error) error {
	if cat := classify(err); cat != nil {
		return fmt.Errorf("%w: %w", cat, err)
	}
	return err
}

func classify(err error) error {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return nil
	}
	code := se.Code()
	switch code {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return ErrUnique
	case sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
		return ErrForeignKey
	case sqlite3.SQLITE_CONSTRAINT_CHECK:
		return ErrCheck
	case sqlite3.SQLITE_CONSTRAINT_TRIGGER:
		return ErrTrigger
	}
	switch code & 0xff {
	case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED:
		return ErrBusy
	case sqlite3.SQLITE_FULL, sqlite3.SQLITE_IOERR, sqlite3.SQLITE_READONLY,
		sqlite3.SQLITE_CANTOPEN, sqlite3.SQLITE_CORRUPT, sqlite3.SQLITE_NOTADB:
		return ErrStorage
	case sqlite3.SQLITE_CONSTRAINT:
		return ErrCheck
	}
	return nil
}

// TimeLayout tem largura fixa, então comparar texto equivale a comparar tempo.
const TimeLayout = "2006-01-02T15:04:05.000Z"

// FormatTime grava sempre em UTC.
func FormatTime(t time.Time) string { return t.UTC().Format(TimeLayout) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(TimeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("data inválida no banco (%q): %w", s, err)
	}
	return t, nil
}

func parseNullTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func nullStr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func nullInt(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// lastID devolve o id inserido.
func lastID(op string, res sql.Result) (int64, error) {
	id, err := res.LastInsertId()
	if err != nil {
		return 0, wrap(op, err)
	}
	return id, nil
}

// affected devolve quantas linhas o comando alterou.
func affected(op string, res sql.Result) (int64, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return 0, wrap(op, err)
	}
	return n, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
