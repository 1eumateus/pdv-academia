// Caminho: pdv/internal/db/database.go

// Package db abre e configura o banco SQLite local e aplica as migrations.
//
// Decisões (documentadas também no README):
//   - Driver modernc.org/sqlite (Go puro, sem CGO).
//   - Os PRAGMAs são passados na DSN via _pragma=..., então o driver os aplica em
//     TODA conexão nova do pool, e não só na primeira.
//   - journal_mode=WAL: leitores não bloqueiam o escritor e o arquivo resiste
//     melhor a quedas de energia.
//   - _txlock=immediate: toda transação começa com BEGIN IMMEDIATE, reservando a
//     escrita logo no início. Isso evita o deadlock de "upgrade" de leitura para
//     escrita entre duas transações.
//   - O pool tem uma única conexão. Num PDV de uma máquina, o volume é baixo; com
//     isso, as escritas ficam serializadas no próprio Go e a aplicação não convive
//     com SQLITE_BUSY. O busy_timeout fica como proteção contra processos externos
//     (ex.: um visualizador de banco aberto pelo técnico).
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// FileName é o nome do arquivo do banco dentro do diretório de dados.
const FileName = "pdv.db"

// DataDir devolve (e cria, se preciso) o diretório os.UserConfigDir()/pdv.
// PDV_DATA_DIR substitui o local, só para desenvolvimento e testes manuais.
func DataDir() (string, error) {
	dir := os.Getenv("PDV_DATA_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("não foi possível localizar o diretório de configuração do usuário: %w", err)
		}
		dir = filepath.Join(base, "pdv")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("não foi possível criar o diretório de dados: %w", err)
	}
	return dir, nil
}

// DSN monta a string de conexão com os PRAGMAs aplicados a cada conexão.
func DSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	// Barras normais funcionam no Windows e evitam problemas com "\" na URI.
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}

// Open abre o banco no caminho informado, valida os PRAGMAs e aplica as
// migrations pendentes. Nunca apaga dados.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir o banco de dados: %w", err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)

	if err := checkPragmas(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := backupBeforeUpgrade(ctx, conn, path); err != nil {
		conn.Close()
		return nil, err
	}
	if err := Migrate(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := MigrateCatraca(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// backupBeforeUpgrade copia o banco (VACUUM INTO) para backups/ antes de
// aplicar migrations em um banco que já tem dados. Sem a cópia, não atualiza.
func backupBeforeUpgrade(ctx context.Context, conn *sql.DB, path string) error {
	v, err := UserVersion(ctx, conn)
	if err != nil {
		return err
	}
	if v == 0 || v >= LatestVersion() {
		return nil // banco novo ou já atualizado
	}
	dir := filepath.Join(filepath.Dir(path), "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("não foi possível criar a pasta de backups antes de atualizar o banco: %w", err)
	}
	dest := filepath.Join(dir, fmt.Sprintf("antes-da-atualizacao-v%d-%s.db", v, time.Now().Format("2006-01-02-150405")))
	if _, err := conn.ExecContext(ctx, `VACUUM INTO ?`, dest); err != nil {
		return fmt.Errorf("não foi possível fazer o backup antes de atualizar o banco (nada foi alterado): %w", err)
	}
	return nil
}

func checkPragmas(ctx context.Context, conn *sql.DB) error {
	var fk int
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return fmt.Errorf("falha ao ler a configuração do banco: %w", err)
	}
	if fk != 1 {
		return errors.New("o banco não ativou as chaves estrangeiras")
	}
	var mode string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("falha ao ler o modo de journal: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		return fmt.Errorf("o banco não está em modo WAL (modo atual: %s)", mode)
	}
	return nil
}

// IntegrityCheck executa PRAGMA integrity_check e PRAGMA foreign_key_check.
func IntegrityCheck(ctx context.Context, conn *sql.DB) error {
	var res string
	if err := conn.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&res); err != nil {
		return fmt.Errorf("falha na verificação de integridade: %w", err)
	}
	if res != "ok" {
		return fmt.Errorf("banco corrompido: %s", res)
	}
	rows, err := conn.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("falha na verificação de chaves estrangeiras: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("o banco possui referências inválidas entre tabelas")
	}
	return rows.Err()
}
