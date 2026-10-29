// Caminho: pdv/internal/db/db_test.go

package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func tableExists(t *testing.T, conn *sql.DB, name string) bool {
	t.Helper()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestOpenEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "pdv.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	v, err := UserVersion(ctx, conn)
	if err != nil || v != LatestVersion() {
		t.Fatalf("versão = %d, err = %v; esperado %d", v, err, LatestVersion())
	}
	for _, tbl := range []string{"products", "customers", "sales", "sale_items", "stock_movements",
		"comandas", "comanda_items", "credit_payments", "cash_sessions", "cash_movements", "settings"} {
		if !tableExists(t, conn, tbl) {
			t.Errorf("tabela %s não criada", tbl)
		}
	}
	if err := IntegrityCheck(ctx, conn); err != nil {
		t.Fatal(err)
	}
}

func TestPragmasOnEveryConnection(t *testing.T) {
	ctx := context.Background()
	conn, err := sql.Open("sqlite", DSN(filepath.Join(t.TempDir(), "pdv.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(3)
	// Segura três conexões distintas ao mesmo tempo e confere cada uma.
	var conns []*sql.Conn
	for i := 0; i < 3; i++ {
		c, err := conn.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for i, c := range conns {
		var fk, timeout int
		var mode string
		c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk)
		c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout)
		c.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode)
		if fk != 1 || timeout != 5000 || !strings.EqualFold(mode, "wal") {
			t.Errorf("conexão %d: foreign_keys=%d busy_timeout=%d journal_mode=%s", i, fk, timeout, mode)
		}
		c.Close()
	}
}

func TestUpgradeFromPreviousVersionPreservesData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pdv.db")
	conn := openRaw(t, path)
	// Banco "antigo": só a migration 1.
	if err := migrate(ctx, conn, Migrations()[:1]); err != nil {
		t.Fatal(err)
	}
	if v, _ := UserVersion(ctx, conn); v != 1 {
		t.Fatalf("versão inicial = %d", v)
	}
	if tableExists(t, conn, "credit_payments") {
		t.Fatal("credit_payments não deveria existir na versão 1")
	}
	ts := "2026-01-01T10:00:00.000Z"
	mustExec(t, conn, `INSERT INTO products (name, price_cents, stock_qty, created_at, updated_at) VALUES ('Arroz', 2500, 0, ?, ?)`, ts, ts)
	mustExec(t, conn, `INSERT INTO customers (name, credit_limit_cents, credit_balance_cents, created_at, updated_at) VALUES ('Ana', 10000, 3000, ?, ?)`, ts, ts)
	mustExec(t, conn, `INSERT INTO sales (customer_id, subtotal_cents, discount_cents, total_cents, payment_method, status, created_at) VALUES (1, 3000, 0, 3000, 'fiado', 'concluida', ?)`, ts)

	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if v, _ := UserVersion(ctx, conn); v != LatestVersion() {
		t.Fatalf("versão final = %d", v)
	}
	var name string
	var balance int64
	var session sql.NullInt64
	conn.QueryRow(`SELECT name FROM products WHERE id = 1`).Scan(&name)
	conn.QueryRow(`SELECT credit_balance_cents FROM customers WHERE id = 1`).Scan(&balance)
	conn.QueryRow(`SELECT cash_session_id FROM sales WHERE id = 1`).Scan(&session)
	if name != "Arroz" || balance != 3000 || session.Valid {
		t.Fatalf("dados não preservados: name=%q balance=%d session=%v", name, balance, session)
	}
	if !tableExists(t, conn, "credit_payments") || !tableExists(t, conn, "cash_sessions") {
		t.Fatal("tabelas novas não criadas")
	}
	// Migration 4: colunas novas do cliente existem e ficam nulas nos registros antigos.
	var doc, street sql.NullString
	if err := conn.QueryRow(`SELECT document, street FROM customers WHERE id = 1`).Scan(&doc, &street); err != nil || doc.Valid || street.Valid {
		t.Fatalf("colunas novas do cliente: err=%v doc=%v street=%v", err, doc, street)
	}
	// Rodar de novo não faz nada.
	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	ctx := context.Background()
	conn := openRaw(t, filepath.Join(t.TempDir(), "pdv.db"))
	list := append(Migrations()[:1:1], Migration{
		Name: "quebrada",
		Statements: []string{
			`CREATE TABLE tabela_nova (id INTEGER PRIMARY KEY)`,
			`ALTER TABLE products ADD COLUMN extra TEXT`,
			`ISTO NAO E SQL`,
		},
	})
	err := migrate(ctx, conn, list)
	if err == nil || !strings.Contains(err.Error(), "migration 2 (quebrada)") {
		t.Fatalf("esperado erro contextualizado, veio %v", err)
	}
	if v, _ := UserVersion(ctx, conn); v != 1 {
		t.Fatalf("versão deveria continuar 1, está %d", v)
	}
	if tableExists(t, conn, "tabela_nova") {
		t.Fatal("tabela da migration com falha não foi desfeita")
	}
	var cols int
	conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('products') WHERE name = 'extra'`).Scan(&cols)
	if cols != 0 {
		t.Fatal("coluna da migration com falha não foi desfeita")
	}
}

func TestRefusesFutureVersion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pdv.db")
	conn := openRaw(t, path)
	mustExec(t, conn, `PRAGMA user_version = 99`)
	conn.Close()
	if _, err := Open(ctx, path); err == nil || !strings.Contains(err.Error(), "mais nova") {
		t.Fatalf("esperado recusa de versão futura, veio %v", err)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "pdv.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec(`INSERT INTO sale_items (sale_id, product_id, product_name_snapshot, qty, unit_price_cents, subtotal_cents) VALUES (999, 999, 'x', 1, 1, 1)`)
	if err == nil {
		t.Fatal("chave estrangeira inválida foi aceita")
	}
}

func TestHistoryIsImmutable(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "pdv.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ts := "2026-01-01T10:00:00.000Z"
	mustExec(t, conn, `INSERT INTO products (name, price_cents, stock_qty, created_at, updated_at) VALUES ('Café', 1000, 5, ?, ?)`, ts, ts)
	mustExec(t, conn, `INSERT INTO stock_movements (product_id, qty_change, reason, created_at) VALUES (1, 5, 'entrada', ?)`, ts)
	if _, err := conn.Exec(`DELETE FROM stock_movements`); err == nil {
		t.Fatal("exclusão de movimentação foi aceita")
	}
	if _, err := conn.Exec(`UPDATE stock_movements SET qty_change = 50`); err == nil {
		t.Fatal("edição de movimentação foi aceita")
	}
}

func mustExec(t *testing.T, conn *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// A migration 15 recria sales, cash_movements e credit_payments: nada pode se
// perder, as referências continuam válidas e as travas (triggers) continuam lá.
func TestRebuildPaymentsMigrationPreservesData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pdv.db")
	conn := openRaw(t, path)
	if err := migrate(ctx, conn, Migrations()[:14]); err != nil {
		t.Fatal(err)
	}
	ts := "2026-01-01T10:00:00.000Z"
	mustExec(t, conn, `INSERT INTO users (name, login, password_hash, role, created_at, updated_at) VALUES ('Ana', 'ana', 'x', 'admin', ?, ?)`, ts, ts)
	mustExec(t, conn, `INSERT INTO products (name, price_cents, stock_qty, created_at, updated_at) VALUES ('Pão', 100, 10, ?, ?)`, ts, ts)
	mustExec(t, conn, `INSERT INTO customers (name, credit_limit_cents, credit_balance_cents, created_at, updated_at) VALUES ('Bia', 10000, 500, ?, ?)`, ts, ts)
	mustExec(t, conn, `INSERT INTO cash_sessions (status, opening_cents, opened_at) VALUES ('aberta', 1000, ?)`, ts)
	mustExec(t, conn, `INSERT INTO sales (customer_id, subtotal_cents, discount_cents, total_cents, payment_method, status, created_at, cash_session_id, user_id)
		VALUES (NULL, 300, 0, 300, 'cartao', 'concluida', ?, 1, 1), (1, 500, 0, 500, 'fiado', 'concluida', ?, 1, 1), (NULL, 0, 0, 0, 'dinheiro', 'concluida', ?, 1, 1)`, ts, ts, ts)
	mustExec(t, conn, `INSERT INTO sale_items (sale_id, product_id, product_name_snapshot, qty, unit_price_cents, subtotal_cents) VALUES (1, 1, 'Pão', 3, 100, 300)`)
	mustExec(t, conn, `INSERT INTO cash_movements (session_id, type, payment_method, amount_cents, created_at, user_id) VALUES (1, 'venda', 'cartao', 300, ?, 1)`, ts)
	mustExec(t, conn, `INSERT INTO credit_payments (customer_id, amount_cents, payment_method, kind, created_at, user_id) VALUES (1, 200, 'pix', 'recebimento', ?, 1)`, ts)
	mustExec(t, conn, `INSERT INTO credit_payments (customer_id, amount_cents, payment_method, kind, reverses_payment_id, created_at) VALUES (1, 200, 'pix', 'estorno', 1, ?)`, ts)

	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	count := func(q string) (n int) {
		if err := conn.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	if count(`SELECT COUNT(*) FROM sales`) != 3 || count(`SELECT COUNT(*) FROM sale_items`) != 1 ||
		count(`SELECT COUNT(*) FROM cash_movements`) != 1 || count(`SELECT COUNT(*) FROM credit_payments`) != 2 {
		t.Fatal("registros perdidos na reconstrução")
	}
	// Cada venda com valor ganhou seu pagamento; a de valor zero, nenhum.
	if count(`SELECT COUNT(*) FROM sale_payments`) != 2 ||
		count(`SELECT COUNT(*) FROM sale_payments WHERE sale_id = 2 AND method = 'fiado' AND amount_cents = 500`) != 1 {
		t.Fatal("pagamentos das vendas antigas não foram criados")
	}
	// Referências e travas continuam valendo.
	rows, _ := conn.Query(`PRAGMA foreign_key_check`)
	if rows.Next() {
		t.Fatal("referência quebrada depois da migração")
	}
	rows.Close()
	if _, err := conn.Exec(`UPDATE sales SET total_cents = 1 WHERE id = 1`); err == nil {
		t.Fatal("trava de valores da venda sumiu")
	}
	if _, err := conn.Exec(`DELETE FROM cash_movements`); err == nil {
		t.Fatal("trava do caixa sumiu")
	}
	if _, err := conn.Exec(`INSERT INTO sales (subtotal_cents, total_cents, payment_method, status, created_at) VALUES (100, 100, 'misto', 'concluida', ?)`, ts); err != nil {
		t.Fatalf("forma 'misto' deveria ser aceita: %v", err)
	}
	var fk int
	conn.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Fatal("chaves estrangeiras deveriam voltar ligadas")
	}
}

// A migration 23 recria returns para aceitar Pix e cartão: devoluções e itens
// antigos continuam, ligados, e as travas voltam.
func TestRebuildReturnsMigrationPreservesData(t *testing.T) {
	ctx := context.Background()
	conn := openRaw(t, filepath.Join(t.TempDir(), "pdv.db"))
	if err := migrate(ctx, conn, Migrations()[:22]); err != nil {
		t.Fatal(err)
	}
	ts := "2026-01-01T10:00:00.000Z"
	mustExec(t, conn, `INSERT INTO products (name, price_cents, stock_qty, created_at, updated_at) VALUES ('Pão', 100, 10, ?, ?)`, ts, ts)
	mustExec(t, conn, `INSERT INTO sales (subtotal_cents, discount_cents, total_cents, payment_method, status, created_at) VALUES (300, 0, 300, 'debito', 'concluida', ?)`, ts)
	mustExec(t, conn, `INSERT INTO sale_items (sale_id, product_id, product_name_snapshot, qty, unit_price_cents, subtotal_cents) VALUES (1, 1, 'Pão', 3, 100, 300)`)
	mustExec(t, conn, `INSERT INTO returns (sale_id, refund_method, total_cents, reason, created_at) VALUES (1, 'dinheiro', 100, 'velho', ?)`, ts)
	mustExec(t, conn, `INSERT INTO return_items (return_id, sale_item_id, product_id, product_name, qty, amount_cents) VALUES (1, 1, 1, 'Pão', 1, 100)`)

	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	var n int
	conn.QueryRow(`SELECT COUNT(*) FROM returns r JOIN return_items i ON i.return_id = r.id WHERE r.reason = 'velho'`).Scan(&n)
	if n != 1 {
		t.Fatal("devolução antiga perdida na reconstrução")
	}
	rows, _ := conn.Query(`PRAGMA foreign_key_check`)
	if rows.Next() {
		t.Fatal("referência quebrada depois da migração")
	}
	rows.Close()
	mustExec(t, conn, `INSERT INTO returns (sale_id, refund_method, total_cents, reason, created_at) VALUES (1, 'cartao', 100, 'novo', ?)`, ts)
	if _, err := conn.Exec(`DELETE FROM returns`); err == nil {
		t.Fatal("trava contra apagar devolução sumiu")
	}
}

func TestBackupBeforeUpgrade(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "pdv.db")
	conn := openRaw(t, path)
	if err := migrate(ctx, conn, Migrations()[:3]); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	matches, _ := filepath.Glob(filepath.Join(dir, "backups", "antes-da-atualizacao-v3-*.db"))
	if len(matches) != 1 {
		t.Fatalf("deveria haver um backup antes da atualização: %v", matches)
	}
}
