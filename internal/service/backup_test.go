// Caminho: pdv/internal/service/backup_test.go

package service

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pdv/internal/db"
)

func TestBackupCreateValidateRestore(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Arroz", 2500, 10)
	e.sell("dinheiro", nil, item(p.ID, 3))

	dest := filepath.Join(t.TempDir(), "copia.db")
	res, err := e.backup.Create(e.ctx, dest)
	if err != nil {
		t.Fatal(err)
	}
	if res.SizeBytes == 0 {
		t.Fatal("backup vazio")
	}
	if v, err := ValidateBackupFile(e.ctx, dest); err != nil || v != db.LatestVersion() {
		t.Fatalf("validação: v=%d err=%v", v, err)
	}

	// Depois do backup, o banco muda: a restauração deve voltar ao estado salvo.
	e.sell("dinheiro", nil, item(p.ID, 2))

	if err := e.backup.RequestRestore(e.ctx, dest); err != nil {
		t.Fatal(err)
	}
	if !e.backup.HasPendingRestore() {
		t.Fatal("restauração não ficou pendente")
	}
	// O banco em uso não foi tocado.
	if e.stockOf(p.ID) != 5 {
		t.Fatal("banco ativo alterado antes do reinício")
	}
	e.db.Close() // simula o encerramento do programa

	out, err := ApplyPendingRestore(e.ctx, e.dataDir, time.Now())
	if err != nil || !out.Applied {
		t.Fatalf("aplicação: %+v %v", out, err)
	}
	conn, err := db.Open(e.ctx, e.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.IntegrityCheck(e.ctx, conn); err != nil {
		t.Fatal(err)
	}
	var stock, sales int64
	conn.QueryRow(`SELECT stock_qty FROM products WHERE id = ?`, p.ID).Scan(&stock)
	conn.QueryRow(`SELECT COUNT(*) FROM sales`).Scan(&sales)
	if stock != 7 || sales != 1 {
		t.Fatalf("dados restaurados incorretos: estoque=%d vendas=%d", stock, sales)
	}
	// O banco anterior foi guardado.
	if _, err := os.Stat(filepath.Join(out.SavedTo, db.FileName)); err != nil {
		t.Fatal("banco anterior não foi preservado")
	}
}

func TestBackupRejectsInvalidFiles(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()

	garbage := filepath.Join(dir, "lixo.db")
	os.WriteFile(garbage, make([]byte, 4096), 0o600)
	if _, err := ValidateBackupFile(e.ctx, garbage); !errors.Is(err, ErrInvalid) {
		t.Fatalf("arquivo inválido aceito: %v", err)
	}
	if err := e.backup.RequestRestore(e.ctx, garbage); !errors.Is(err, ErrInvalid) {
		t.Fatalf("restauração de arquivo inválido aceita: %v", err)
	}
	if e.backup.HasPendingRestore() {
		t.Fatal("arquivo inválido ficou pendente")
	}

	// Banco SQLite válido, mas que não é do PDV.
	other := filepath.Join(dir, "outro.db")
	foreign, err := sql.Open("sqlite", "file:"+filepath.ToSlash(other))
	if err != nil {
		t.Fatal(err)
	}
	foreign.Exec(`CREATE TABLE qualquer (id INTEGER PRIMARY KEY, txt TEXT)`)
	foreign.Exec(`INSERT INTO qualquer (txt) VALUES (zeroblob(2000))`)
	foreign.Close()
	if _, err := ValidateBackupFile(e.ctx, other); !errors.Is(err, ErrInvalid) {
		t.Fatalf("banco de outra origem aceito: %v", err)
	}

	for _, bad := range []string{"", "relativo.db", filepath.Join(dir, "x.txt"), e.dbPath} {
		if _, err := e.backup.Create(e.ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("destino %q aceito: %v", bad, err)
		}
	}
}

func TestApplyPendingRestoreRejectsCorruptPending(t *testing.T) {
	e := newEnv(t)
	e.product("Original", 100, 1)
	e.db.Close()
	os.WriteFile(filepath.Join(e.dataDir, db.FileName+".restore"), []byte("corrompido"), 0o600)
	out, err := ApplyPendingRestore(e.ctx, e.dataDir, time.Now())
	if err != nil || out.Applied {
		t.Fatalf("restauração corrompida aplicada: %+v %v", out, err)
	}
	conn, err := db.Open(e.ctx, e.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var n int
	conn.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&n)
	if n != 1 {
		t.Fatal("banco original perdido")
	}
}
