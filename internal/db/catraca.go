// Caminho: catraca/internal/db/catraca.go

package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Tabelas da catraca (só na versão da academia). Têm numeração própria,
// gravada em catraca_versao, para não disputar o PRAGMA user_version com as
// migrations do PDV: uma migration nova do PDV entra aqui sem renumerar nada.
// Nunca altere uma já distribuída: acrescente no final.
func catracaMigrations() [][]string {
	return [][]string{
		{
			// Situação do aluno na catraca. device_user_id: id dele no iDFace
			// (nulo = sem rosto cadastrado). free_until: cortesia, libera até esta
			// data (AAAA-MM-DD) mesmo sem plano em dia.
			`CREATE TABLE catraca_alunos (
				customer_id    INTEGER PRIMARY KEY REFERENCES customers (id),
				device_user_id INTEGER UNIQUE,
				blocked        INTEGER NOT NULL DEFAULT 0 CHECK (blocked IN (0, 1)),
				free_until     TEXT,
				notes          TEXT    NOT NULL DEFAULT '',
				updated_at     TEXT    NOT NULL
			)`,
			// Cada passagem. source: leitor (iDFace), balcao (aluno verificado
			// pelo operador) ou manual (liberação sem aluno, com motivo).
			`CREATE TABLE catraca_passagens (
				id          INTEGER PRIMARY KEY,
				at          TEXT    NOT NULL,
				customer_id INTEGER REFERENCES customers (id),
				name        TEXT    NOT NULL DEFAULT '',
				allowed     INTEGER NOT NULL CHECK (allowed IN (0, 1)),
				reason      TEXT    NOT NULL,
				source      TEXT    NOT NULL CHECK (source IN ('leitor', 'balcao', 'manual')),
				user_id     INTEGER REFERENCES users (id)
			)`,
			`CREATE INDEX ix_catraca_passagens_customer ON catraca_passagens (customer_id, at)`,
			`CREATE INDEX ix_catraca_passagens_at ON catraca_passagens (at)`,
		},
		{
			// Funcionário: passa sempre, sem plano. funcao: o cargo (NULL = aluno).
			`ALTER TABLE catraca_alunos ADD COLUMN funcao TEXT`,
		},
		{
			// Regras de acesso da assinatura (sem linha = sem restrição). dias: dias
			// permitidos, "1,3,5" (1 = segunda … 7 = domingo; vazio = todos);
			// vezes_semana: dias diferentes com entrada na semana (seg a dom);
			// entradas_dia: passagens no mesmo dia. 0 = sem limite.
			`CREATE TABLE catraca_regras (
				subscription_id INTEGER PRIMARY KEY REFERENCES subscriptions (id),
				dias            TEXT    NOT NULL DEFAULT '',
				vezes_semana    INTEGER NOT NULL DEFAULT 0 CHECK (vezes_semana BETWEEN 0 AND 7),
				entradas_dia    INTEGER NOT NULL DEFAULT 0 CHECK (entradas_dia BETWEEN 0 AND 20)
			)`,
		},
		{
			// Regras de acesso do plano (produto com duração), mesmas colunas de
			// catraca_regras. Valem para todos os assinantes; a linha em
			// catraca_regras vira a exceção daquela assinatura.
			`CREATE TABLE catraca_plano_regras (
				product_id   INTEGER PRIMARY KEY REFERENCES products (id),
				dias         TEXT    NOT NULL DEFAULT '',
				vezes_semana INTEGER NOT NULL DEFAULT 0 CHECK (vezes_semana BETWEEN 0 AND 7),
				entradas_dia INTEGER NOT NULL DEFAULT 0 CHECK (entradas_dia BETWEEN 0 AND 20)
			)`,
		},
		{
			// Mensalidade paga fora do caixa (retroativa, caixa já fechado à mão):
			// fica paga sem venda; esta linha diz quem lançou e permite desfazer.
			`CREATE TABLE fatura_fora_caixa (
				invoice_id INTEGER PRIMARY KEY REFERENCES subscription_invoices (id),
				user_id    INTEGER REFERENCES users (id),
				created_at TEXT    NOT NULL
			)`,
		},
	}
}

// MigrateCatraca cria ou atualiza as tabelas da catraca.
func MigrateCatraca(ctx context.Context, conn *sql.DB) error {
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS catraca_versao (versao INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("catraca: criar controle de versão: %w", err)
	}
	var current int
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(versao), 0) FROM catraca_versao`).Scan(&current); err != nil {
		return fmt.Errorf("catraca: ler versão: %w", err)
	}
	list := catracaMigrations()
	if current > len(list) {
		return fmt.Errorf("as tabelas da catraca estão na versão %d, mais nova que a suportada por este programa (%d); atualize o programa", current, len(list))
	}
	for v := current + 1; v <= len(list); v++ {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for i, stmt := range list[v-1] {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("catraca: migration %d, comando %d: %w", v, i+1, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM catraca_versao`); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO catraca_versao (versao) VALUES (?)`, v); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
