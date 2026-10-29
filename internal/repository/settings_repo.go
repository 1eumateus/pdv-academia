// Caminho: pdv/internal/repository/settings_repo.go

package repository

import (
	"context"
	"database/sql"
	"errors"
)

type SettingsRepo struct{}

// All devolve todas as chaves gravadas.
func (SettingsRepo) All(ctx context.Context, q DBTX) (map[string]string, error) {
	const op = "ler configurações"
	rows, err := q.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, wrap(op, err)
		}
		out[k] = v
	}
	return out, wrap(op, rows.Err())
}

// Get lê uma chave; ok = false se nunca foi gravada.
func (SettingsRepo) Get(ctx context.Context, q DBTX, key string) (value string, ok bool, err error) {
	err = q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, wrap("ler configuração", err)
}

func (SettingsRepo) Set(ctx context.Context, q DBTX, key, value string) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return wrap("gravar configuração", err)
}
