// Caminho: pdv/internal/db/migrations.go

package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Migration é um passo incremental do esquema. O número da versão é a posição
// na lista (1, 2, 3...) e fica gravado em PRAGMA user_version.
type Migration struct {
	Name       string
	Statements []string
	// Rebuild: a migration recria tabelas (o SQLite não altera CHECK). Segue o
	// procedimento oficial: chaves estrangeiras desligadas só durante a migration,
	// foreign_key_check antes de confirmar e religadas no fim.
	Rebuild bool
}

// Migrations é a lista oficial, em ordem. Nunca altere uma migration já
// distribuída: crie uma nova no final.
func Migrations() []Migration {
	return []Migration{
		{Name: "tabelas principais", Statements: migration1},
		{Name: "recebimentos de crédito", Statements: migration2},
		{Name: "caixa e configurações", Statements: migration3},
		{Name: "documento, contato e endereço do cliente", Statements: migration4},
		{Name: "usuários, permissões e auditoria", Statements: migration5},
		{Name: "permissões por usuário", Statements: migration6},
		{Name: "categorias de produto", Statements: migration7},
		{Name: "grade de tamanho e cor", Statements: migration8},
		{Name: "custo e estoque mínimo", Statements: migration9},
		{Name: "fornecedores", Statements: migration10},
		{Name: "compras (entrada de mercadoria)", Statements: migration11},
		{Name: "vínculos de produto do fornecedor", Statements: migration12},
		{Name: "estorno de entrada de mercadoria", Statements: migration13},
		{Name: "produto a precificar", Statements: migration14},
		{Name: "vários pagamentos na mesma venda", Statements: migration15, Rebuild: true},
		{Name: "desconto por item", Statements: migration16},
		{Name: "promoções", Statements: migration17},
		{Name: "orçamentos", Statements: migration18},
		{Name: "orçamento volta a aberto se a venda for cancelada", Statements: migration19},
		{Name: "relatórios gerados", Statements: migration20},
		{Name: "tipo do relatório gerado", Statements: migration21},
		{Name: "trocas e devoluções com vale-troca", Statements: migration22},
		{Name: "devolução por Pix e estorno no cartão", Statements: migration23, Rebuild: true},
		{Name: "custo do item na venda", Statements: migration24},
		{Name: "recebimento por nota do fiado", Statements: migration25},
		{Name: "venda para entrega", Statements: migration26},
		{Name: "SKU do produto e última cobrança do cliente", Statements: migration27},
		{Name: "assistente de primeira configuração", Statements: migration28},
		{Name: "um login por usuário", Statements: migration29},
		{Name: "produto que não controla estoque", Statements: migration30},
		{Name: "caixa por usuário", Statements: migration31},
		{Name: "assinaturas (planos)", Statements: migration32},
		{Name: "descrição do produto (serviços e planos)", Statements: migration33},
		{Name: "data de nascimento do cliente", Statements: migration34},
		{Name: "pausa de assinatura", Statements: migration35},
		{Name: "contas a pagar", Statements: migration36},
		{Name: "valor especial da assinatura e faturas importadas", Statements: migration37, Rebuild: true},
		// Versão só marcada: um número 38 antigo já circulou sem esta tabela. O esquema
		// da troca vem de ensurePaymentChanges, que olha a tabela e não o número.
		{Name: "troca única da forma de pagamento da venda"},
		{Name: "excluir assinatura cancelada", Statements: migration39},
	}
}

// LatestVersion é a versão de esquema que este executável conhece.
func LatestVersion() int { return len(Migrations()) }

// Migrate aplica as migrations pendentes.
func Migrate(ctx context.Context, conn *sql.DB) error {
	if err := migrate(ctx, conn, Migrations()); err != nil {
		return err
	}
	if err := ensurePaymentChanges(ctx, conn); err != nil {
		return err
	}
	if err := ensureSubscriptionExempt(ctx, conn); err != nil {
		return err
	}
	return ensureCustomerGender(ctx, conn)
}

// ensureCustomerGender cria o gênero do cliente, se faltar (nulo = não informado).
// Olha a coluna e não o número da versão, como a isenção.
func ensureCustomerGender(ctx context.Context, conn *sql.DB) error {
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('customers') WHERE name = 'gender'`).Scan(&n); err != nil {
		return fmt.Errorf("gênero do cliente: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := conn.ExecContext(ctx, `ALTER TABLE customers ADD COLUMN gender TEXT`); err != nil {
		return fmt.Errorf("gênero do cliente: %w", err)
	}
	return nil
}

// ensureSubscriptionExempt cria os campos da isenção da assinatura, se faltarem.
// exempt_until: nulo = sem isenção; vazio = permanente; data = isenta até o dia.
func ensureSubscriptionExempt(ctx context.Context, conn *sql.DB) error {
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('subscriptions') WHERE name = 'exempt_until'`).Scan(&n); err != nil {
		return fmt.Errorf("isenção da assinatura: %w", err)
	}
	if n > 0 {
		return nil
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("isenção da assinatura: %w", err)
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`ALTER TABLE subscriptions ADD COLUMN exempt_until TEXT`,
		`ALTER TABLE subscriptions ADD COLUMN exempt_reason TEXT`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("isenção da assinatura: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("isenção da assinatura: %w", err)
	}
	return nil
}

// ensurePaymentChanges cria a tabela e as travas da troca de forma de pagamento
// se faltarem, em qualquer versão do banco. Tudo numa transação.
func ensurePaymentChanges(ctx context.Context, conn *sql.DB) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("troca da forma de pagamento: %w", err)
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'sale_payment_changes'`).Scan(&n); err != nil {
		return fmt.Errorf("troca da forma de pagamento: %w", err)
	}
	if n > 0 {
		return nil
	}
	for i, stmt := range paymentChangeSchema {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("troca da forma de pagamento, comando %d: %w", i+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("troca da forma de pagamento: %w", err)
	}
	return nil
}

func migrate(ctx context.Context, conn *sql.DB, list []Migration) error {
	current, err := UserVersion(ctx, conn)
	if err != nil {
		return err
	}
	if current > len(list) {
		return fmt.Errorf("o banco está na versão %d, mais nova que a suportada por este programa (%d); atualize o PDV antes de abrir este banco", current, len(list))
	}
	for v := current + 1; v <= len(list); v++ {
		if err := applyMigration(ctx, conn, v, list[v-1]); err != nil {
			return err
		}
	}
	return nil
}

// Cada migration roda numa transação: DDL e user_version no SQLite são
// transacionais, então uma falha desfaz tudo e a versão não avança.
func applyMigration(ctx context.Context, conn *sql.DB, version int, m Migration) error {
	// Conexão fixa: PRAGMA foreign_keys vale por conexão e não muda dentro de transação.
	c, err := conn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration %d (%s): falha ao obter conexão: %w", version, m.Name, err)
	}
	defer c.Close()
	if m.Rebuild {
		if _, err := c.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
			return fmt.Errorf("migration %d (%s): %w", version, m.Name, err)
		}
		defer c.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
	}
	tx, err := c.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migration %d (%s): falha ao iniciar transação: %w", version, m.Name, err)
	}
	defer tx.Rollback()
	for i, stmt := range m.Statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migration %d (%s), comando %d: %w", version, m.Name, i+1, err)
		}
	}
	if m.Rebuild {
		rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
		if err != nil {
			return fmt.Errorf("migration %d (%s): %w", version, m.Name, err)
		}
		bad := rows.Next()
		rows.Close()
		if bad {
			return fmt.Errorf("migration %d (%s): a reconstrução deixaria referências quebradas; nada foi alterado", version, m.Name)
		}
	}
	// PRAGMA não aceita parâmetro; version é um inteiro controlado pelo código.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return fmt.Errorf("migration %d (%s): falha ao gravar a versão: %w", version, m.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %d (%s): falha ao confirmar: %w", version, m.Name, err)
	}
	return nil
}

// UserVersion lê PRAGMA user_version.
func UserVersion(ctx context.Context, conn *sql.DB) (int, error) {
	var v int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("falha ao ler a versão do banco: %w", err)
	}
	return v, nil
}

// Datas são TEXT em UTC no formato 2006-01-02T15:04:05.000Z (largura fixa, então
// a comparação de texto equivale à comparação de tempo). Quem grava é o Go.

var migration1 = []string{
	`CREATE TABLE products (
		id            INTEGER PRIMARY KEY,
		name          TEXT    NOT NULL CHECK (length(trim(name)) > 0),
		barcode       TEXT    UNIQUE CHECK (barcode IS NULL OR length(trim(barcode)) > 0),
		price_cents   INTEGER NOT NULL CHECK (price_cents >= 0),
		stock_qty     INTEGER NOT NULL DEFAULT 0 CHECK (stock_qty >= 0),
		active        INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
		created_at    TEXT    NOT NULL,
		updated_at    TEXT    NOT NULL
	)`,
	`CREATE INDEX ix_products_name ON products (name COLLATE NOCASE)`,

	`CREATE TABLE customers (
		id                   INTEGER PRIMARY KEY,
		name                 TEXT    NOT NULL CHECK (length(trim(name)) > 0),
		phone                TEXT,
		credit_limit_cents   INTEGER NOT NULL DEFAULT 0 CHECK (credit_limit_cents >= 0),
		credit_balance_cents INTEGER NOT NULL DEFAULT 0 CHECK (credit_balance_cents >= 0),
		active               INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
		created_at           TEXT    NOT NULL,
		updated_at           TEXT    NOT NULL
	)`,
	`CREATE INDEX ix_customers_name ON customers (name COLLATE NOCASE)`,

	`CREATE TABLE comandas (
		id         INTEGER PRIMARY KEY,
		table_no   TEXT NOT NULL CHECK (length(trim(table_no)) > 0),
		-- 'descartada' só é permitido para comanda sem itens (aberta por engano).
		status     TEXT NOT NULL DEFAULT 'aberta' CHECK (status IN ('aberta', 'fechada', 'descartada')),
		opened_at  TEXT NOT NULL,
		closed_at  TEXT,
		CHECK ((status <> 'aberta') = (closed_at IS NOT NULL))
	)`,
	// Uma mesa/código só pode ter uma comanda aberta por vez.
	`CREATE UNIQUE INDEX ux_comandas_open_table ON comandas (table_no) WHERE status = 'aberta'`,

	`CREATE TABLE comanda_items (
		id                    INTEGER PRIMARY KEY,
		comanda_id            INTEGER NOT NULL REFERENCES comandas (id),
		product_id            INTEGER NOT NULL REFERENCES products (id),
		product_name_snapshot TEXT    NOT NULL,
		qty                   INTEGER NOT NULL CHECK (qty > 0),
		unit_price_cents      INTEGER NOT NULL CHECK (unit_price_cents >= 0),
		created_at            TEXT    NOT NULL
	)`,
	`CREATE INDEX ix_comanda_items_comanda ON comanda_items (comanda_id)`,
	// Comanda fechada é histórico: seus itens ficam congelados.
	`CREATE TRIGGER tr_comanda_items_insert_closed BEFORE INSERT ON comanda_items
	 WHEN (SELECT status FROM comandas WHERE id = NEW.comanda_id) <> 'aberta'
	 BEGIN SELECT RAISE(ABORT, 'comanda fechada'); END`,
	`CREATE TRIGGER tr_comanda_items_update_closed BEFORE UPDATE ON comanda_items
	 WHEN (SELECT status FROM comandas WHERE id = OLD.comanda_id) <> 'aberta'
	 BEGIN SELECT RAISE(ABORT, 'comanda fechada'); END`,
	`CREATE TRIGGER tr_comanda_items_delete_closed BEFORE DELETE ON comanda_items
	 WHEN (SELECT status FROM comandas WHERE id = OLD.comanda_id) <> 'aberta'
	 BEGIN SELECT RAISE(ABORT, 'comanda fechada'); END`,
	`CREATE TRIGGER tr_comandas_closed_immutable BEFORE UPDATE ON comandas
	 WHEN OLD.status <> 'aberta'
	 BEGIN SELECT RAISE(ABORT, 'comanda fechada'); END`,
	`CREATE TRIGGER tr_comandas_no_delete BEFORE DELETE ON comandas
	 BEGIN SELECT RAISE(ABORT, 'comandas não podem ser apagadas'); END`,

	`CREATE TABLE sales (
		id                  INTEGER PRIMARY KEY,
		customer_id         INTEGER REFERENCES customers (id),
		comanda_id          INTEGER UNIQUE REFERENCES comandas (id),
		subtotal_cents      INTEGER NOT NULL CHECK (subtotal_cents >= 0),
		discount_cents      INTEGER NOT NULL DEFAULT 0 CHECK (discount_cents >= 0 AND discount_cents <= subtotal_cents),
		total_cents         INTEGER NOT NULL CHECK (total_cents = subtotal_cents - discount_cents),
		payment_method      TEXT    NOT NULL CHECK (payment_method IN ('dinheiro', 'pix', 'cartao', 'fiado')),
		status              TEXT    NOT NULL CHECK (status IN ('concluida', 'cancelada')),
		created_at          TEXT    NOT NULL,
		cancelled_at        TEXT,
		cancellation_reason TEXT,
		-- Chave de idempotência enviada pelo frontend: o mesmo pedido repetido
		-- (clique duplo, reenvio) não gera uma segunda venda.
		request_id          TEXT    UNIQUE,
		CHECK (payment_method <> 'fiado' OR customer_id IS NOT NULL),
		CHECK ((status = 'cancelada') = (cancelled_at IS NOT NULL))
	)`,
	`CREATE INDEX ix_sales_created ON sales (created_at)`,
	`CREATE INDEX ix_sales_customer ON sales (customer_id)`,

	`CREATE TABLE sale_items (
		id                    INTEGER PRIMARY KEY,
		sale_id               INTEGER NOT NULL REFERENCES sales (id),
		product_id            INTEGER NOT NULL REFERENCES products (id),
		product_name_snapshot TEXT    NOT NULL,
		qty                   INTEGER NOT NULL CHECK (qty > 0),
		unit_price_cents      INTEGER NOT NULL CHECK (unit_price_cents >= 0),
		subtotal_cents        INTEGER NOT NULL CHECK (subtotal_cents = qty * unit_price_cents)
	)`,
	`CREATE INDEX ix_sale_items_sale ON sale_items (sale_id)`,
	`CREATE INDEX ix_sale_items_product ON sale_items (product_id)`,

	`CREATE TABLE stock_movements (
		id             INTEGER PRIMARY KEY,
		product_id     INTEGER NOT NULL REFERENCES products (id),
		qty_change     INTEGER NOT NULL CHECK (qty_change <> 0),
		reason         TEXT    NOT NULL CHECK (reason IN ('entrada', 'venda', 'ajuste', 'devolucao', 'cancelamento')),
		reference_type TEXT,
		reference_id   INTEGER,
		notes          TEXT,
		created_at     TEXT    NOT NULL
	)`,
	`CREATE INDEX ix_stock_movements_product ON stock_movements (product_id, created_at)`,

	// Histórico imutável: o banco recusa edição e exclusão silenciosas.
	`CREATE TRIGGER tr_stock_movements_no_update BEFORE UPDATE ON stock_movements
	 BEGIN SELECT RAISE(ABORT, 'movimentações de estoque não podem ser alteradas'); END`,
	`CREATE TRIGGER tr_stock_movements_no_delete BEFORE DELETE ON stock_movements
	 BEGIN SELECT RAISE(ABORT, 'movimentações de estoque não podem ser apagadas'); END`,
	`CREATE TRIGGER tr_sale_items_no_update BEFORE UPDATE ON sale_items
	 BEGIN SELECT RAISE(ABORT, 'itens de venda não podem ser alterados'); END`,
	`CREATE TRIGGER tr_sale_items_no_delete BEFORE DELETE ON sale_items
	 BEGIN SELECT RAISE(ABORT, 'itens de venda não podem ser apagados'); END`,
	`CREATE TRIGGER tr_sales_no_delete BEFORE DELETE ON sales
	 BEGIN SELECT RAISE(ABORT, 'vendas não podem ser apagadas; use o cancelamento'); END`,
	`CREATE TRIGGER tr_sales_values_immutable
	 BEFORE UPDATE OF customer_id, comanda_id, subtotal_cents, discount_cents, total_cents, payment_method, created_at ON sales
	 BEGIN SELECT RAISE(ABORT, 'os valores de uma venda não podem ser alterados'); END`,
	`CREATE TRIGGER tr_sales_cancel_once BEFORE UPDATE OF status, cancelled_at, cancellation_reason ON sales
	 WHEN OLD.status = 'cancelada'
	 BEGIN SELECT RAISE(ABORT, 'venda já cancelada'); END`,
}

var migration2 = []string{
	// kind = 'estorno' corrige um recebimento sem apagá-lo; o estorno aponta para
	// o recebimento original e cada recebimento só pode ser estornado uma vez.
	`CREATE TABLE credit_payments (
		id                 INTEGER PRIMARY KEY,
		customer_id        INTEGER NOT NULL REFERENCES customers (id),
		amount_cents       INTEGER NOT NULL CHECK (amount_cents > 0),
		payment_method     TEXT    NOT NULL CHECK (payment_method IN ('dinheiro', 'pix', 'cartao')),
		kind               TEXT    NOT NULL DEFAULT 'recebimento' CHECK (kind IN ('recebimento', 'estorno')),
		reverses_payment_id INTEGER UNIQUE REFERENCES credit_payments (id),
		created_at         TEXT    NOT NULL,
		notes              TEXT,
		request_id         TEXT    UNIQUE,
		CHECK ((kind = 'estorno') = (reverses_payment_id IS NOT NULL))
	)`,
	`CREATE INDEX ix_credit_payments_customer ON credit_payments (customer_id, created_at)`,
	`CREATE INDEX ix_credit_payments_created ON credit_payments (created_at)`,
	`CREATE TRIGGER tr_credit_payments_no_update BEFORE UPDATE ON credit_payments
	 BEGIN SELECT RAISE(ABORT, 'recebimentos não podem ser alterados; registre um estorno'); END`,
	`CREATE TRIGGER tr_credit_payments_no_delete BEFORE DELETE ON credit_payments
	 BEGIN SELECT RAISE(ABORT, 'recebimentos não podem ser apagados; registre um estorno'); END`,
}

var migration3 = []string{
	`CREATE TABLE cash_sessions (
		id               INTEGER PRIMARY KEY,
		status           TEXT    NOT NULL CHECK (status IN ('aberta', 'fechada')),
		opening_cents    INTEGER NOT NULL CHECK (opening_cents >= 0),
		expected_cents   INTEGER,
		counted_cents    INTEGER CHECK (counted_cents IS NULL OR counted_cents >= 0),
		difference_cents INTEGER,
		notes            TEXT,
		opened_at        TEXT    NOT NULL,
		closed_at        TEXT,
		CHECK ((status = 'fechada') = (closed_at IS NOT NULL)),
		CHECK (status = 'aberta' OR (expected_cents IS NOT NULL AND counted_cents IS NOT NULL AND difference_cents IS NOT NULL))
	)`,
	// Só pode existir uma sessão de caixa aberta.
	`CREATE UNIQUE INDEX ux_cash_sessions_one_open ON cash_sessions (status) WHERE status = 'aberta'`,
	`CREATE TRIGGER tr_cash_sessions_closed_immutable BEFORE UPDATE ON cash_sessions
	 WHEN OLD.status = 'fechada'
	 BEGIN SELECT RAISE(ABORT, 'sessão de caixa fechada não pode ser alterada'); END`,
	`CREATE TRIGGER tr_cash_sessions_no_delete BEFORE DELETE ON cash_sessions
	 BEGIN SELECT RAISE(ABORT, 'sessões de caixa não podem ser apagadas'); END`,

	// amount_cents tem sinal: positivo entra no caixa, negativo sai.
	`CREATE TABLE cash_movements (
		id             INTEGER PRIMARY KEY,
		session_id     INTEGER NOT NULL REFERENCES cash_sessions (id),
		type           TEXT    NOT NULL CHECK (type IN ('suprimento', 'sangria', 'venda', 'recebimento_fiado', 'estorno_venda', 'estorno_recebimento')),
		payment_method TEXT    NOT NULL CHECK (payment_method IN ('dinheiro', 'pix', 'cartao')),
		amount_cents   INTEGER NOT NULL CHECK (amount_cents <> 0),
		description    TEXT,
		reference_type TEXT,
		reference_id   INTEGER,
		created_at     TEXT    NOT NULL,
		CHECK (type NOT IN ('suprimento', 'venda', 'recebimento_fiado') OR amount_cents > 0),
		CHECK (type NOT IN ('sangria', 'estorno_venda', 'estorno_recebimento') OR amount_cents < 0)
	)`,
	`CREATE INDEX ix_cash_movements_session ON cash_movements (session_id)`,
	`CREATE TRIGGER tr_cash_movements_no_update BEFORE UPDATE ON cash_movements
	 BEGIN SELECT RAISE(ABORT, 'movimentações de caixa não podem ser alteradas'); END`,
	`CREATE TRIGGER tr_cash_movements_no_delete BEFORE DELETE ON cash_movements
	 BEGIN SELECT RAISE(ABORT, 'movimentações de caixa não podem ser apagadas'); END`,

	`ALTER TABLE sales ADD COLUMN cash_session_id INTEGER REFERENCES cash_sessions (id)`,
	`CREATE INDEX ix_sales_cash_session ON sales (cash_session_id)`,

	`CREATE TABLE settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
}

var migration4 = []string{
	`ALTER TABLE customers ADD COLUMN document TEXT CHECK (document IS NULL OR length(document) IN (11, 14))`,
	`ALTER TABLE customers ADD COLUMN email TEXT`,
	`ALTER TABLE customers ADD COLUMN zip_code TEXT CHECK (zip_code IS NULL OR length(zip_code) = 8)`,
	`ALTER TABLE customers ADD COLUMN street TEXT`,
	`ALTER TABLE customers ADD COLUMN number TEXT`,
	`ALTER TABLE customers ADD COLUMN complement TEXT`,
	`ALTER TABLE customers ADD COLUMN district TEXT`,
	`ALTER TABLE customers ADD COLUMN city TEXT`,
	`ALTER TABLE customers ADD COLUMN state TEXT CHECK (state IS NULL OR length(state) = 2)`,
	`ALTER TABLE customers ADD COLUMN reference TEXT`,
	`ALTER TABLE customers ADD COLUMN notes TEXT`,
	// Documento é opcional, mas não pode se repetir quando informado.
	`CREATE UNIQUE INDEX ux_customers_document ON customers (document) WHERE document IS NOT NULL`,
}

var migration5 = []string{
	`CREATE TABLE users (
		id            INTEGER PRIMARY KEY,
		name          TEXT    NOT NULL CHECK (length(trim(name)) > 0),
		login         TEXT    NOT NULL UNIQUE COLLATE NOCASE CHECK (length(trim(login)) > 0),
		password_hash TEXT    NOT NULL,
		role          TEXT    NOT NULL CHECK (role IN ('admin', 'gerente', 'operador')),
		active        INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
		created_at    TEXT    NOT NULL,
		updated_at    TEXT    NOT NULL
	)`,
	`CREATE TRIGGER tr_users_no_delete BEFORE DELETE ON users
	 BEGIN SELECT RAISE(ABORT, 'usuários não podem ser apagados; desative o cadastro'); END`,

	// Autor de cada registro. NULL nos registros anteriores a esta versão.
	`ALTER TABLE sales ADD COLUMN user_id INTEGER REFERENCES users (id)`,
	`ALTER TABLE sales ADD COLUMN cancelled_by_user_id INTEGER REFERENCES users (id)`,
	`CREATE INDEX ix_sales_user ON sales (user_id)`,
	`ALTER TABLE stock_movements ADD COLUMN user_id INTEGER REFERENCES users (id)`,
	`ALTER TABLE cash_movements ADD COLUMN user_id INTEGER REFERENCES users (id)`,
	`ALTER TABLE credit_payments ADD COLUMN user_id INTEGER REFERENCES users (id)`,
	`ALTER TABLE cash_sessions ADD COLUMN opened_by_user_id INTEGER REFERENCES users (id)`,
	`ALTER TABLE cash_sessions ADD COLUMN closed_by_user_id INTEGER REFERENCES users (id)`,

	// Auditoria: quem fez, quem autorizou, o quê e os valores antes/depois (JSON).
	`CREATE TABLE audit_log (
		id                 INTEGER PRIMARY KEY,
		user_id            INTEGER REFERENCES users (id),
		authorized_by_id   INTEGER REFERENCES users (id),
		action             TEXT    NOT NULL,
		entity             TEXT,
		entity_id          INTEGER,
		before_json        TEXT,
		after_json         TEXT,
		created_at         TEXT    NOT NULL
	)`,
	`CREATE INDEX ix_audit_log_created ON audit_log (created_at)`,
	`CREATE TRIGGER tr_audit_log_no_update BEFORE UPDATE ON audit_log
	 BEGIN SELECT RAISE(ABORT, 'a auditoria não pode ser alterada'); END`,
	`CREATE TRIGGER tr_audit_log_no_delete BEFORE DELETE ON audit_log
	 BEGIN SELECT RAISE(ABORT, 'a auditoria não pode ser apagada'); END`,
}

var migration6 = []string{
	// Lista JSON de permissões escolhidas pelo administrador; NULL = padrão do papel.
	`ALTER TABLE users ADD COLUMN permissions TEXT`,
}

var migration7 = []string{
	`CREATE TABLE categories (
		id         INTEGER PRIMARY KEY,
		name       TEXT    NOT NULL UNIQUE COLLATE NOCASE CHECK (length(trim(name)) > 0),
		active     INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
		created_at TEXT    NOT NULL,
		updated_at TEXT    NOT NULL
	)`,
	`CREATE TRIGGER tr_categories_no_delete BEFORE DELETE ON categories
	 BEGIN SELECT RAISE(ABORT, 'categorias não podem ser apagadas; desative'); END`,
	`ALTER TABLE products ADD COLUMN category_id INTEGER REFERENCES categories (id)`,
	`CREATE INDEX ix_products_category ON products (category_id)`,
}

// Grade: cada variação é um produto comum ligado a um modelo, então venda,
// comanda, estoque e auditoria continuam funcionando por produto.
var migration8 = []string{
	`CREATE TABLE product_models (
		id               INTEGER PRIMARY KEY,
		name             TEXT    NOT NULL UNIQUE COLLATE NOCASE CHECK (length(trim(name)) > 0),
		category_id      INTEGER REFERENCES categories (id),
		base_price_cents INTEGER NOT NULL CHECK (base_price_cents >= 0),
		created_at       TEXT    NOT NULL,
		updated_at       TEXT    NOT NULL
	)`,
	`CREATE TRIGGER tr_product_models_no_delete BEFORE DELETE ON product_models
	 BEGIN SELECT RAISE(ABORT, 'modelos não podem ser apagados'); END`,
	`ALTER TABLE products ADD COLUMN model_id INTEGER REFERENCES product_models (id)`,
	`ALTER TABLE products ADD COLUMN size TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE products ADD COLUMN color TEXT NOT NULL DEFAULT ''`,
	// Cada combinação tamanho × cor aparece uma vez por modelo.
	`CREATE UNIQUE INDEX ux_products_variation ON products (model_id, size COLLATE NOCASE, color COLLATE NOCASE)
	 WHERE model_id IS NOT NULL`,
}

var migration9 = []string{
	`ALTER TABLE products ADD COLUMN cost_cents INTEGER NOT NULL DEFAULT 0 CHECK (cost_cents >= 0)`,
	`ALTER TABLE products ADD COLUMN min_stock INTEGER NOT NULL DEFAULT 0 CHECK (min_stock >= 0)`,
}

var migration10 = []string{
	`CREATE TABLE suppliers (
		id                 INTEGER PRIMARY KEY,
		legal_name         TEXT    NOT NULL CHECK (length(trim(legal_name)) > 0),
		trade_name         TEXT    NOT NULL DEFAULT '',
		document           TEXT    CHECK (document IS NULL OR length(document) IN (11, 14)),
		state_registration TEXT    NOT NULL DEFAULT '',
		contact            TEXT    NOT NULL DEFAULT '',
		phone              TEXT    NOT NULL DEFAULT '',
		email              TEXT    NOT NULL DEFAULT '',
		zip_code           TEXT    NOT NULL DEFAULT '',
		street             TEXT    NOT NULL DEFAULT '',
		number             TEXT    NOT NULL DEFAULT '',
		complement         TEXT    NOT NULL DEFAULT '',
		district           TEXT    NOT NULL DEFAULT '',
		city               TEXT    NOT NULL DEFAULT '',
		state              TEXT    NOT NULL DEFAULT '',
		notes              TEXT    NOT NULL DEFAULT '',
		active             INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
		created_at         TEXT    NOT NULL,
		updated_at         TEXT    NOT NULL
	)`,
	`CREATE UNIQUE INDEX ux_suppliers_document ON suppliers (document) WHERE document IS NOT NULL`,
	`CREATE TRIGGER tr_suppliers_no_delete BEFORE DELETE ON suppliers
	 BEGIN SELECT RAISE(ABORT, 'fornecedores não podem ser apagados; desative'); END`,
}

// Compras são histórico: não se alteram nem se apagam (correção = ajuste de estoque).
var migration11 = []string{
	`CREATE TABLE purchases (
		id              INTEGER PRIMARY KEY,
		supplier_id     INTEGER NOT NULL REFERENCES suppliers (id),
		invoice_number  TEXT    NOT NULL DEFAULT '',
		issue_date      TEXT    NOT NULL,
		access_key      TEXT    UNIQUE CHECK (access_key IS NULL OR length(access_key) = 44),
		products_cents  INTEGER NOT NULL CHECK (products_cents >= 0),
		freight_cents   INTEGER NOT NULL DEFAULT 0 CHECK (freight_cents >= 0),
		taxes_cents     INTEGER NOT NULL DEFAULT 0 CHECK (taxes_cents >= 0),
		discount_cents  INTEGER NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
		total_cents     INTEGER NOT NULL CHECK (total_cents >= 0),
		notes           TEXT    NOT NULL DEFAULT '',
		user_id         INTEGER REFERENCES users (id),
		created_at      TEXT    NOT NULL
	)`,
	`CREATE INDEX ix_purchases_created ON purchases (created_at)`,
	`CREATE TABLE purchase_items (
		id                INTEGER PRIMARY KEY,
		purchase_id       INTEGER NOT NULL REFERENCES purchases (id),
		product_id        INTEGER NOT NULL REFERENCES products (id),
		product_name      TEXT    NOT NULL,
		qty               INTEGER NOT NULL CHECK (qty > 0),
		unit_cost_cents   INTEGER NOT NULL CHECK (unit_cost_cents >= 0),
		total_cents       INTEGER NOT NULL CHECK (total_cents >= 0),
		cost_before_cents INTEGER NOT NULL,
		cost_after_cents  INTEGER NOT NULL
	)`,
	`CREATE INDEX ix_purchase_items_purchase ON purchase_items (purchase_id)`,
	`CREATE TRIGGER tr_purchases_no_update BEFORE UPDATE ON purchases
	 BEGIN SELECT RAISE(ABORT, 'compras não podem ser alteradas'); END`,
	`CREATE TRIGGER tr_purchases_no_delete BEFORE DELETE ON purchases
	 BEGIN SELECT RAISE(ABORT, 'compras não podem ser apagadas'); END`,
	`CREATE TRIGGER tr_purchase_items_no_update BEFORE UPDATE ON purchase_items
	 BEGIN SELECT RAISE(ABORT, 'itens de compra não podem ser alterados'); END`,
	`CREATE TRIGGER tr_purchase_items_no_delete BEFORE DELETE ON purchase_items
	 BEGIN SELECT RAISE(ABORT, 'itens de compra não podem ser apagados'); END`,
}

// Lembra qual produto do sistema corresponde ao código do fornecedor (cProd) e
// quantas unidades vêm em cada unidade da nota (ex.: CX com 12 → fator 12).
var migration12 = []string{
	`CREATE TABLE supplier_products (
		supplier_id   INTEGER NOT NULL REFERENCES suppliers (id),
		supplier_code TEXT    NOT NULL CHECK (length(supplier_code) > 0),
		product_id    INTEGER NOT NULL REFERENCES products (id),
		factor        INTEGER NOT NULL DEFAULT 1 CHECK (factor >= 1),
		updated_at    TEXT    NOT NULL,
		PRIMARY KEY (supplier_id, supplier_code)
	)`,
}

// Estorno de compra: a única alteração permitida numa compra é marcá-la como
// estornada (uma vez). A chave da NF-e sai de access_key (liberando importar a
// mesma nota de novo) e fica guardada em cancelled_access_key.
var migration13 = []string{
	`ALTER TABLE purchases ADD COLUMN cancelled_at TEXT`,
	`ALTER TABLE purchases ADD COLUMN cancelled_by_user_id INTEGER REFERENCES users (id)`,
	`ALTER TABLE purchases ADD COLUMN cancellation_reason TEXT`,
	`ALTER TABLE purchases ADD COLUMN cancelled_access_key TEXT`,
	`DROP TRIGGER tr_purchases_no_update`,
	`CREATE TRIGGER tr_purchases_no_update
	 BEFORE UPDATE OF supplier_id, invoice_number, issue_date, products_cents, freight_cents, taxes_cents,
	   discount_cents, total_cents, notes, user_id, created_at ON purchases
	 BEGIN SELECT RAISE(ABORT, 'compras não podem ser alteradas; use o estorno'); END`,
	`CREATE TRIGGER tr_purchases_cancel_once BEFORE UPDATE ON purchases
	 WHEN OLD.cancelled_at IS NOT NULL OR NEW.cancelled_at IS NULL
	 BEGIN SELECT RAISE(ABORT, 'a compra só pode ser estornada uma vez'); END`,
}

// "A precificar": produto que entrou (pela nota) sem preço de venda definido.
// Fica fora da venda até alguém definir o preço.
var migration14 = []string{
	`ALTER TABLE products ADD COLUMN price_pending INTEGER NOT NULL DEFAULT 0 CHECK (price_pending IN (0, 1))`,
}

// migration15: vários pagamentos por venda. Recria sales, cash_movements e
// credit_payments com as formas novas (débito, crédito, vale-troca e "misto" no
// resumo da venda; "cartao" segue válido para registros antigos), copiando todos
// os dados e recriando índices e triggers. Cada venda antiga ganha uma linha em
// sale_payments com a forma e o total que já tinha.
var migration15 = []string{
	`CREATE TABLE sales_new (
		id                   INTEGER PRIMARY KEY,
		customer_id          INTEGER REFERENCES customers (id),
		comanda_id           INTEGER UNIQUE REFERENCES comandas (id),
		subtotal_cents       INTEGER NOT NULL CHECK (subtotal_cents >= 0),
		discount_cents       INTEGER NOT NULL DEFAULT 0 CHECK (discount_cents >= 0 AND discount_cents <= subtotal_cents),
		total_cents          INTEGER NOT NULL CHECK (total_cents = subtotal_cents - discount_cents),
		payment_method       TEXT    NOT NULL CHECK (payment_method IN ('dinheiro', 'pix', 'cartao', 'debito', 'credito', 'fiado', 'vale_troca', 'misto')),
		status               TEXT    NOT NULL CHECK (status IN ('concluida', 'cancelada')),
		created_at           TEXT    NOT NULL,
		cancelled_at         TEXT,
		cancellation_reason  TEXT,
		request_id           TEXT    UNIQUE,
		cash_session_id      INTEGER REFERENCES cash_sessions (id),
		user_id              INTEGER REFERENCES users (id),
		cancelled_by_user_id INTEGER REFERENCES users (id),
		CHECK (payment_method <> 'fiado' OR customer_id IS NOT NULL),
		CHECK ((status = 'cancelada') = (cancelled_at IS NOT NULL))
	)`,
	`INSERT INTO sales_new (id, customer_id, comanda_id, subtotal_cents, discount_cents, total_cents, payment_method, status,
		created_at, cancelled_at, cancellation_reason, request_id, cash_session_id, user_id, cancelled_by_user_id)
	 SELECT id, customer_id, comanda_id, subtotal_cents, discount_cents, total_cents, payment_method, status,
		created_at, cancelled_at, cancellation_reason, request_id, cash_session_id, user_id, cancelled_by_user_id FROM sales`,
	`DROP TABLE sales`,
	`ALTER TABLE sales_new RENAME TO sales`,
	`CREATE INDEX ix_sales_created ON sales (created_at)`,
	`CREATE INDEX ix_sales_customer ON sales (customer_id)`,
	`CREATE INDEX ix_sales_cash_session ON sales (cash_session_id)`,
	`CREATE INDEX ix_sales_user ON sales (user_id)`,
	`CREATE TRIGGER tr_sales_no_delete BEFORE DELETE ON sales
	 BEGIN SELECT RAISE(ABORT, 'vendas não podem ser apagadas; use o cancelamento'); END`,
	`CREATE TRIGGER tr_sales_values_immutable
	 BEFORE UPDATE OF customer_id, comanda_id, subtotal_cents, discount_cents, total_cents, payment_method, created_at ON sales
	 BEGIN SELECT RAISE(ABORT, 'os valores de uma venda não podem ser alterados'); END`,
	`CREATE TRIGGER tr_sales_cancel_once BEFORE UPDATE OF status, cancelled_at, cancellation_reason ON sales
	 WHEN OLD.status = 'cancelada'
	 BEGIN SELECT RAISE(ABORT, 'venda já cancelada'); END`,

	// Partes do pagamento. received_cents só para dinheiro (troco = recebido − valor).
	`CREATE TABLE sale_payments (
		id             INTEGER PRIMARY KEY,
		sale_id        INTEGER NOT NULL REFERENCES sales (id),
		method         TEXT    NOT NULL CHECK (method IN ('dinheiro', 'pix', 'cartao', 'debito', 'credito', 'fiado', 'vale_troca')),
		amount_cents   INTEGER NOT NULL CHECK (amount_cents > 0),
		received_cents INTEGER NOT NULL DEFAULT 0 CHECK (received_cents >= 0),
		card_brand     TEXT    NOT NULL DEFAULT '',
		installments   INTEGER NOT NULL DEFAULT 0 CHECK (installments BETWEEN 0 AND 24),
		CHECK (method = 'dinheiro' OR received_cents = 0),
		CHECK (method <> 'dinheiro' OR received_cents = 0 OR received_cents >= amount_cents)
	)`,
	`CREATE INDEX ix_sale_payments_sale ON sale_payments (sale_id)`,
	`CREATE TRIGGER tr_sale_payments_no_update BEFORE UPDATE ON sale_payments
	 BEGIN SELECT RAISE(ABORT, 'pagamentos de venda não podem ser alterados'); END`,
	`CREATE TRIGGER tr_sale_payments_no_delete BEFORE DELETE ON sale_payments
	 BEGIN SELECT RAISE(ABORT, 'pagamentos de venda não podem ser apagados'); END`,
	`INSERT INTO sale_payments (sale_id, method, amount_cents) SELECT id, payment_method, total_cents FROM sales WHERE total_cents > 0`,

	`CREATE TABLE cash_movements_new (
		id             INTEGER PRIMARY KEY,
		session_id     INTEGER NOT NULL REFERENCES cash_sessions (id),
		type           TEXT    NOT NULL CHECK (type IN ('suprimento', 'sangria', 'venda', 'recebimento_fiado', 'estorno_venda', 'estorno_recebimento', 'devolucao')),
		payment_method TEXT    NOT NULL CHECK (payment_method IN ('dinheiro', 'pix', 'cartao', 'debito', 'credito')),
		amount_cents   INTEGER NOT NULL CHECK (amount_cents <> 0),
		description    TEXT,
		reference_type TEXT,
		reference_id   INTEGER,
		created_at     TEXT    NOT NULL,
		user_id        INTEGER REFERENCES users (id),
		CHECK (type NOT IN ('suprimento', 'venda', 'recebimento_fiado') OR amount_cents > 0),
		CHECK (type NOT IN ('sangria', 'estorno_venda', 'estorno_recebimento', 'devolucao') OR amount_cents < 0)
	)`,
	`INSERT INTO cash_movements_new (id, session_id, type, payment_method, amount_cents, description, reference_type, reference_id, created_at, user_id)
	 SELECT id, session_id, type, payment_method, amount_cents, description, reference_type, reference_id, created_at, user_id FROM cash_movements`,
	`DROP TABLE cash_movements`,
	`ALTER TABLE cash_movements_new RENAME TO cash_movements`,
	`CREATE INDEX ix_cash_movements_session ON cash_movements (session_id)`,
	`CREATE TRIGGER tr_cash_movements_no_update BEFORE UPDATE ON cash_movements
	 BEGIN SELECT RAISE(ABORT, 'movimentações de caixa não podem ser alteradas'); END`,
	`CREATE TRIGGER tr_cash_movements_no_delete BEFORE DELETE ON cash_movements
	 BEGIN SELECT RAISE(ABORT, 'movimentações de caixa não podem ser apagadas'); END`,

	`CREATE TABLE credit_payments_new (
		id                  INTEGER PRIMARY KEY,
		customer_id         INTEGER NOT NULL REFERENCES customers (id),
		amount_cents        INTEGER NOT NULL CHECK (amount_cents > 0),
		payment_method      TEXT    NOT NULL CHECK (payment_method IN ('dinheiro', 'pix', 'cartao', 'debito', 'credito')),
		kind                TEXT    NOT NULL DEFAULT 'recebimento' CHECK (kind IN ('recebimento', 'estorno')),
		reverses_payment_id INTEGER UNIQUE REFERENCES credit_payments (id),
		created_at          TEXT    NOT NULL,
		notes               TEXT,
		request_id          TEXT    UNIQUE,
		user_id             INTEGER REFERENCES users (id),
		CHECK ((kind = 'estorno') = (reverses_payment_id IS NOT NULL))
	)`,
	`INSERT INTO credit_payments_new (id, customer_id, amount_cents, payment_method, kind, reverses_payment_id, created_at, notes, request_id, user_id)
	 SELECT id, customer_id, amount_cents, payment_method, kind, reverses_payment_id, created_at, notes, request_id, user_id FROM credit_payments`,
	`DROP TABLE credit_payments`,
	`ALTER TABLE credit_payments_new RENAME TO credit_payments`,
	`CREATE INDEX ix_credit_payments_customer ON credit_payments (customer_id, created_at)`,
	`CREATE INDEX ix_credit_payments_created ON credit_payments (created_at)`,
	`CREATE TRIGGER tr_credit_payments_no_update BEFORE UPDATE ON credit_payments
	 BEGIN SELECT RAISE(ABORT, 'recebimentos não podem ser alterados; registre um estorno'); END`,
	`CREATE TRIGGER tr_credit_payments_no_delete BEFORE DELETE ON credit_payments
	 BEGIN SELECT RAISE(ABORT, 'recebimentos não podem ser apagados; registre um estorno'); END`,
}

// Desconto em cada item da venda (valor do item = subtotal − desconto). O
// desconto total gravado na venda soma os descontos dos itens e o da venda.
var migration16 = []string{
	`ALTER TABLE sale_items ADD COLUMN discount_cents INTEGER NOT NULL DEFAULT 0
	 CHECK (discount_cents >= 0 AND discount_cents <= subtotal_cents)`,
}

// Promoções por produto ou categoria, com período (datas locais, inclusive).
// preco: preço fixo; percentual: % de desconto; leve_pague: leve buy_qty, pague pay_qty.
var migration17 = []string{
	`CREATE TABLE promotions (
		id          INTEGER PRIMARY KEY,
		name        TEXT    NOT NULL CHECK (length(trim(name)) > 0),
		kind        TEXT    NOT NULL CHECK (kind IN ('preco', 'percentual', 'leve_pague')),
		product_id  INTEGER REFERENCES products (id),
		category_id INTEGER REFERENCES categories (id),
		price_cents INTEGER NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
		percent     INTEGER NOT NULL DEFAULT 0 CHECK (percent BETWEEN 0 AND 99),
		buy_qty     INTEGER NOT NULL DEFAULT 0 CHECK (buy_qty >= 0),
		pay_qty     INTEGER NOT NULL DEFAULT 0 CHECK (pay_qty >= 0),
		start_date  TEXT    NOT NULL,
		end_date    TEXT    NOT NULL,
		active      INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
		created_at  TEXT    NOT NULL,
		updated_at  TEXT    NOT NULL,
		CHECK ((product_id IS NULL) <> (category_id IS NULL)),
		CHECK (start_date <= end_date),
		CHECK (kind <> 'percentual' OR percent > 0),
		CHECK (kind <> 'leve_pague' OR (pay_qty >= 1 AND buy_qty > pay_qty))
	)`,
	`CREATE INDEX ix_promotions_product ON promotions (product_id)`,
	`CREATE INDEX ix_promotions_category ON promotions (category_id)`,
	`CREATE TRIGGER tr_promotions_no_delete BEFORE DELETE ON promotions
	 BEGIN SELECT RAISE(ABORT, 'promoções não podem ser apagadas; desative'); END`,
	// Promoção aplicada no item: preço promocional já vai no preço unitário;
	// "leve X pague Y" vai em promo_discount_cents (não conta no limite do operador).
	`ALTER TABLE sale_items ADD COLUMN promotion_id INTEGER REFERENCES promotions (id)`,
	`ALTER TABLE sale_items ADD COLUMN promo_discount_cents INTEGER NOT NULL DEFAULT 0 CHECK (promo_discount_cents >= 0)`,
	`ALTER TABLE sale_items ADD COLUMN promo_label TEXT NOT NULL DEFAULT ''`,
}

// Orçamentos: não mexem no estoque. "expirado" é calculado pela validade.
var migration18 = []string{
	`CREATE TABLE quotes (
		id             INTEGER PRIMARY KEY,
		customer_id    INTEGER REFERENCES customers (id),
		status         TEXT    NOT NULL CHECK (status IN ('aberto', 'aprovado', 'convertido', 'cancelado')),
		valid_until    TEXT    NOT NULL,
		notes          TEXT    NOT NULL DEFAULT '',
		subtotal_cents INTEGER NOT NULL CHECK (subtotal_cents >= 0),
		discount_cents INTEGER NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
		total_cents    INTEGER NOT NULL CHECK (total_cents >= 0),
		sale_id        INTEGER REFERENCES sales (id),
		user_id        INTEGER REFERENCES users (id),
		created_at     TEXT    NOT NULL,
		updated_at     TEXT    NOT NULL,
		CHECK ((status = 'convertido') = (sale_id IS NOT NULL))
	)`,
	`CREATE INDEX ix_quotes_created ON quotes (created_at)`,
	`CREATE TABLE quote_items (
		id                   INTEGER PRIMARY KEY,
		quote_id             INTEGER NOT NULL REFERENCES quotes (id),
		product_id           INTEGER NOT NULL REFERENCES products (id),
		product_name         TEXT    NOT NULL,
		qty                  INTEGER NOT NULL CHECK (qty > 0),
		unit_price_cents     INTEGER NOT NULL CHECK (unit_price_cents >= 0),
		discount_cents       INTEGER NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
		promo_discount_cents INTEGER NOT NULL DEFAULT 0 CHECK (promo_discount_cents >= 0),
		promo_label          TEXT    NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX ix_quote_items_quote ON quote_items (quote_id)`,
	`CREATE TRIGGER tr_quotes_no_delete BEFORE DELETE ON quotes
	 BEGIN SELECT RAISE(ABORT, 'orçamentos não podem ser apagados; cancele'); END`,
	`CREATE TRIGGER tr_quotes_final BEFORE UPDATE ON quotes WHEN OLD.status IN ('convertido', 'cancelado')
	 BEGIN SELECT RAISE(ABORT, 'orçamento convertido ou cancelado não pode ser alterado'); END`,
}

// O orçamento convertido só pode mudar para voltar a "aberto" (sem venda),
// quando a venda feita com ele é cancelada. Cancelado continua definitivo.
var migration19 = []string{
	`DROP TRIGGER tr_quotes_final`,
	`CREATE TRIGGER tr_quotes_final BEFORE UPDATE ON quotes
	 WHEN OLD.status = 'cancelado' OR (OLD.status = 'convertido' AND NOT (NEW.status = 'aberto' AND NEW.sale_id IS NULL))
	 BEGIN SELECT RAISE(ABORT, 'orçamento convertido ou cancelado não pode ser alterado'); END`,
}

// Relatórios gerados: cópia fixa (JSON) do relatório no momento em que foi
// gerado, para consultar e reimprimir depois sem os números mudarem.
var migration20 = []string{
	`CREATE TABLE saved_reports (
		id             INTEGER PRIMARY KEY,
		start_date     TEXT    NOT NULL,
		end_date       TEXT    NOT NULL,
		filter_user_id INTEGER REFERENCES users (id),
		sale_count     INTEGER NOT NULL,
		total_cents    INTEGER NOT NULL,
		data           TEXT    NOT NULL,
		user_id        INTEGER REFERENCES users (id),
		created_at     TEXT    NOT NULL
	)`,
	`CREATE INDEX ix_saved_reports_created ON saved_reports (created_at)`,
}

// Tipo do relatório gerado: geral ou só uma seção (forma de pagamento, categoria, operador).
var migration21 = []string{
	`ALTER TABLE saved_reports ADD COLUMN kind TEXT NOT NULL DEFAULT 'geral' CHECK (kind IN ('geral', 'pagamento', 'categoria', 'operador'))`,
}

// Trocas e devoluções: a devolução fica ligada à venda e a cada item vendido
// (para não devolver mais do que foi vendido). O reembolso sai em vale-troca
// (saldo do cliente), dinheiro (movimento de caixa) ou abatendo o fiado.
var migration22 = []string{
	`ALTER TABLE customers ADD COLUMN store_credit_cents INTEGER NOT NULL DEFAULT 0 CHECK (store_credit_cents >= 0)`,
	`CREATE TABLE returns (
		id              INTEGER PRIMARY KEY,
		sale_id         INTEGER NOT NULL REFERENCES sales (id),
		customer_id     INTEGER REFERENCES customers (id),
		refund_method   TEXT    NOT NULL CHECK (refund_method IN ('vale_troca', 'dinheiro', 'fiado')),
		total_cents     INTEGER NOT NULL CHECK (total_cents >= 0),
		reason          TEXT    NOT NULL,
		cash_session_id INTEGER REFERENCES cash_sessions (id),
		user_id         INTEGER REFERENCES users (id),
		created_at      TEXT    NOT NULL,
		CHECK (refund_method = 'dinheiro' OR customer_id IS NOT NULL)
	)`,
	`CREATE INDEX ix_returns_sale ON returns (sale_id)`,
	`CREATE INDEX ix_returns_created ON returns (created_at)`,
	`CREATE TABLE return_items (
		id           INTEGER PRIMARY KEY,
		return_id    INTEGER NOT NULL REFERENCES returns (id),
		sale_item_id INTEGER NOT NULL REFERENCES sale_items (id),
		product_id   INTEGER NOT NULL REFERENCES products (id),
		product_name TEXT    NOT NULL,
		qty          INTEGER NOT NULL CHECK (qty > 0),
		amount_cents INTEGER NOT NULL CHECK (amount_cents >= 0)
	)`,
	`CREATE INDEX ix_return_items_return ON return_items (return_id)`,
	`CREATE INDEX ix_return_items_sale_item ON return_items (sale_item_id)`,
	`CREATE TRIGGER tr_returns_no_delete BEFORE DELETE ON returns
	 BEGIN SELECT RAISE(ABORT, 'devoluções não podem ser apagadas'); END`,
	`CREATE TRIGGER tr_returns_no_update BEFORE UPDATE ON returns
	 BEGIN SELECT RAISE(ABORT, 'devoluções não podem ser alteradas'); END`,
	`CREATE TRIGGER tr_return_items_no_change BEFORE DELETE ON return_items
	 BEGIN SELECT RAISE(ABORT, 'itens de devolução não podem ser apagados'); END`,
}

// Devolução também por Pix e estorno no cartão. O SQLite não altera CHECK:
// recria returns com os mesmos dados, índices e travas.
var migration23 = []string{
	`CREATE TABLE returns_new (
		id              INTEGER PRIMARY KEY,
		sale_id         INTEGER NOT NULL REFERENCES sales (id),
		customer_id     INTEGER REFERENCES customers (id),
		refund_method   TEXT    NOT NULL CHECK (refund_method IN ('vale_troca', 'dinheiro', 'fiado', 'pix', 'cartao')),
		total_cents     INTEGER NOT NULL CHECK (total_cents >= 0),
		reason          TEXT    NOT NULL,
		cash_session_id INTEGER REFERENCES cash_sessions (id),
		user_id         INTEGER REFERENCES users (id),
		created_at      TEXT    NOT NULL,
		CHECK (refund_method IN ('dinheiro', 'pix', 'cartao') OR customer_id IS NOT NULL)
	)`,
	`INSERT INTO returns_new (id, sale_id, customer_id, refund_method, total_cents, reason, cash_session_id, user_id, created_at)
	 SELECT id, sale_id, customer_id, refund_method, total_cents, reason, cash_session_id, user_id, created_at FROM returns`,
	`DROP TABLE returns`,
	`ALTER TABLE returns_new RENAME TO returns`,
	`CREATE INDEX ix_returns_sale ON returns (sale_id)`,
	`CREATE INDEX ix_returns_created ON returns (created_at)`,
	`CREATE TRIGGER tr_returns_no_delete BEFORE DELETE ON returns
	 BEGIN SELECT RAISE(ABORT, 'devoluções não podem ser apagadas'); END`,
	`CREATE TRIGGER tr_returns_no_update BEFORE UPDATE ON returns
	 BEGIN SELECT RAISE(ABORT, 'devoluções não podem ser alteradas'); END`,
}

// Custo do produto no momento da venda (lucro bruto). Vendas antigas ficam NULL
// e os relatórios usam o custo atual, avisando que é estimado.
var migration24 = []string{
	`ALTER TABLE sale_items ADD COLUMN unit_cost_cents INTEGER CHECK (unit_cost_cents IS NULL OR unit_cost_cents >= 0)`,
}

// Recebimento por nota: para quais compras no fiado (vendas) foi cada
// recebimento. Recebimentos sem nota escolhida quitam as mais antigas.
var migration25 = []string{
	`CREATE TABLE credit_allocations (
		id           INTEGER PRIMARY KEY,
		payment_id   INTEGER NOT NULL REFERENCES credit_payments (id),
		sale_id      INTEGER NOT NULL REFERENCES sales (id),
		amount_cents INTEGER NOT NULL CHECK (amount_cents > 0)
	)`,
	`CREATE INDEX ix_credit_allocations_payment ON credit_allocations (payment_id)`,
	`CREATE INDEX ix_credit_allocations_sale ON credit_allocations (sale_id)`,
	`CREATE TRIGGER tr_credit_allocations_no_update BEFORE UPDATE ON credit_allocations
	 BEGIN SELECT RAISE(ABORT, 'pagamentos de nota não podem ser alterados'); END`,
	`CREATE TRIGGER tr_credit_allocations_no_delete BEFORE DELETE ON credit_allocations
	 BEGIN SELECT RAISE(ABORT, 'pagamentos de nota não podem ser apagados'); END`,
}

// Venda para entrega: a venda é paga normalmente; aqui fica para onde levar
// (endereço do cliente ou outro digitado na venda). Só impressão, sem status.
var migration26 = []string{
	`CREATE TABLE sale_deliveries (
		sale_id INTEGER PRIMARY KEY REFERENCES sales (id),
		address TEXT    NOT NULL CHECK (length(trim(address)) > 0),
		phone   TEXT    NOT NULL DEFAULT '',
		notes   TEXT    NOT NULL DEFAULT ''
	)`,
	`CREATE TRIGGER tr_sale_deliveries_no_update BEFORE UPDATE ON sale_deliveries
	 BEGIN SELECT RAISE(ABORT, 'a entrega da venda não pode ser alterada'); END`,
	`CREATE TRIGGER tr_sale_deliveries_no_delete BEFORE DELETE ON sale_deliveries
	 BEGIN SELECT RAISE(ABORT, 'a entrega da venda não pode ser apagada'); END`,
}

// SKU: código interno opcional (único) para busca e importação de planilhas.
// Última cobrança: quando o fiado do cliente foi cobrado pelo WhatsApp.
var migration27 = []string{
	`ALTER TABLE products ADD COLUMN sku TEXT CHECK (sku IS NULL OR length(trim(sku)) > 0)`,
	`CREATE UNIQUE INDEX ux_products_sku ON products (sku)`,
	`ALTER TABLE customers ADD COLUMN last_charge_at TEXT`,
}

// Assistente de primeira configuração: quem já usa o sistema (há usuários)
// não precisa vê-lo; um banco novo começa sem a marca e abre o assistente.
var migration28 = []string{
	`INSERT OR IGNORE INTO settings (key, value) SELECT 'setup_done', '1' WHERE EXISTS (SELECT 1 FROM users)`,
}

// Sessão ativa do usuário: id da instância do programa onde ele está logado
// (NULL = ninguém). Impede o mesmo usuário logado em dois lugares.
var migration29 = []string{
	`ALTER TABLE users ADD COLUMN session_id TEXT`,
}

// Produto que não controla estoque (serviço, plano de academia): vende sem
// saldo e não gera movimentação. Os existentes continuam controlando.
var migration30 = []string{
	`ALTER TABLE products ADD COLUMN track_stock INTEGER NOT NULL DEFAULT 1 CHECK (track_stock IN (0, 1))`,
}

// Caixa por usuário: owner_user_id é o dono da sessão (NULL = caixa geral, como
// todas as sessões antigas). Um caixa geral aberto, ou um aberto por usuário.
var migration31 = []string{
	`ALTER TABLE cash_sessions ADD COLUMN owner_user_id INTEGER REFERENCES users (id)`,
	`DROP INDEX ux_cash_sessions_one_open`,
	`CREATE UNIQUE INDEX ux_cash_sessions_one_open ON cash_sessions (status) WHERE status = 'aberta' AND owner_user_id IS NULL`,
	`CREATE UNIQUE INDEX ux_cash_sessions_one_open_per_user ON cash_sessions (owner_user_id) WHERE status = 'aberta' AND owner_user_id IS NOT NULL`,
}

// Assinaturas (planos de academia): o plano é um produto com duração em dias.
// Cada assinatura tem no máximo uma fatura aberta; pagar a fatura gera uma
// venda do plano e a próxima fatura (data do pagamento + duração).
var migration32 = []string{
	`ALTER TABLE products ADD COLUMN plan_days INTEGER CHECK (plan_days IS NULL OR plan_days BETWEEN 1 AND 3650)`,
	`CREATE TABLE subscriptions (
		id           INTEGER PRIMARY KEY,
		customer_id  INTEGER NOT NULL REFERENCES customers (id),
		product_id   INTEGER NOT NULL REFERENCES products (id),
		status       TEXT    NOT NULL CHECK (status IN ('ativa', 'cancelada')),
		created_at   TEXT    NOT NULL,
		cancelled_at TEXT,
		CHECK ((status = 'cancelada') = (cancelled_at IS NOT NULL))
	)`,
	`CREATE INDEX ix_subscriptions_customer ON subscriptions (customer_id)`,
	`CREATE TABLE subscription_invoices (
		id              INTEGER PRIMARY KEY,
		subscription_id INTEGER NOT NULL REFERENCES subscriptions (id),
		due_date        TEXT    NOT NULL, -- AAAA-MM-DD
		amount_cents    INTEGER NOT NULL CHECK (amount_cents >= 0),
		status          TEXT    NOT NULL CHECK (status IN ('aberta', 'paga', 'cancelada')),
		sale_id         INTEGER REFERENCES sales (id),
		paid_at         TEXT,
		created_at      TEXT    NOT NULL,
		CHECK ((status = 'paga') = (sale_id IS NOT NULL AND paid_at IS NOT NULL))
	)`,
	`CREATE UNIQUE INDEX ux_subscription_invoices_one_open ON subscription_invoices (subscription_id) WHERE status = 'aberta'`,
	`CREATE TRIGGER tr_subscription_invoices_no_delete BEFORE DELETE ON subscription_invoices
	 BEGIN SELECT RAISE(ABORT, 'faturas não podem ser apagadas'); END`,
}

// Descrição livre do produto (ex.: "3x por semana, musculação" num plano).
var migration33 = []string{
	`ALTER TABLE products ADD COLUMN description TEXT`,
}

// Data de nascimento do cliente (AAAA-MM-DD, opcional): aniversariantes.
var migration34 = []string{
	`ALTER TABLE customers ADD COLUMN birth_date TEXT`,
}

// Pausa de assinatura: enquanto paused_at está preenchido os dias não contam;
// ao retomar, o vencimento da fatura aberta anda os dias pausados.
// Contas a pagar (boletos, aluguel, energia…) com o boleto e o comprovante
// anexados no próprio banco (entram no backup).
var migration36 = []string{
	`CREATE TABLE bills (
		id           INTEGER PRIMARY KEY,
		description  TEXT    NOT NULL CHECK (length(trim(description)) > 0),
		category     TEXT    NOT NULL,
		payee        TEXT    NOT NULL DEFAULT '',
		amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
		due_date     TEXT    NOT NULL, -- AAAA-MM-DD
		barcode      TEXT    NOT NULL DEFAULT '',
		notes        TEXT    NOT NULL DEFAULT '',
		recurring    INTEGER NOT NULL DEFAULT 0 CHECK (recurring IN (0, 1)),
		status       TEXT    NOT NULL CHECK (status IN ('aberta', 'paga', 'cancelada')),
		paid_at      TEXT, -- AAAA-MM-DD
		paid_cents   INTEGER CHECK (paid_cents IS NULL OR paid_cents > 0),
		pay_method   TEXT,
		from_cash    INTEGER NOT NULL DEFAULT 0 CHECK (from_cash IN (0, 1)),
		created_at   TEXT    NOT NULL,
		user_id      INTEGER REFERENCES users (id),
		CHECK ((status = 'paga') = (paid_at IS NOT NULL AND paid_cents IS NOT NULL))
	)`,
	`CREATE INDEX ix_bills_status_due ON bills (status, due_date)`,
	`CREATE TABLE bill_files (
		id         INTEGER PRIMARY KEY,
		bill_id    INTEGER NOT NULL REFERENCES bills (id),
		kind       TEXT    NOT NULL CHECK (kind IN ('boleto', 'comprovante')),
		name       TEXT    NOT NULL,
		mime       TEXT    NOT NULL,
		data       BLOB    NOT NULL,
		created_at TEXT    NOT NULL
	)`,
	`CREATE INDEX ix_bill_files_bill ON bill_files (bill_id)`,
}

// Assinatura cancelada excluída: some das telas (deleted_at), mas as mensalidades
// pagas e as vendas continuam guardadas.
var migration39 = []string{
	`ALTER TABLE subscriptions ADD COLUMN deleted_at TEXT`,
}

// Valor especial por aluno (price_cents: nulo = preço do plano) e faturas que
// vieram de outro sistema: paga sem venda (imported) e "não paga" antiga.
var migration37 = []string{
	`ALTER TABLE subscriptions ADD COLUMN price_cents INTEGER CHECK (price_cents IS NULL OR price_cents > 0)`,
	`CREATE TABLE subscription_invoices_new (
		id              INTEGER PRIMARY KEY,
		subscription_id INTEGER NOT NULL REFERENCES subscriptions (id),
		due_date        TEXT    NOT NULL, -- AAAA-MM-DD
		amount_cents    INTEGER NOT NULL CHECK (amount_cents >= 0),
		status          TEXT    NOT NULL CHECK (status IN ('aberta', 'paga', 'cancelada', 'nao_paga')),
		sale_id         INTEGER REFERENCES sales (id),
		paid_at         TEXT,
		created_at      TEXT    NOT NULL,
		imported        INTEGER NOT NULL DEFAULT 0 CHECK (imported IN (0, 1)),
		CHECK ((status = 'paga') = (paid_at IS NOT NULL)),
		CHECK (status <> 'paga' OR imported = 1 OR sale_id IS NOT NULL)
	)`,
	`INSERT INTO subscription_invoices_new (id, subscription_id, due_date, amount_cents, status, sale_id, paid_at, created_at)
	 SELECT id, subscription_id, due_date, amount_cents, status, sale_id, paid_at, created_at FROM subscription_invoices`,
	`DROP TABLE subscription_invoices`,
	`ALTER TABLE subscription_invoices_new RENAME TO subscription_invoices`,
	`CREATE UNIQUE INDEX ux_subscription_invoices_one_open ON subscription_invoices (subscription_id) WHERE status = 'aberta'`,
	`CREATE INDEX ix_subscription_invoices_sub ON subscription_invoices (subscription_id)`,
	`CREATE TRIGGER tr_subscription_invoices_no_delete BEFORE DELETE ON subscription_invoices
	 BEGIN SELECT RAISE(ABORT, 'faturas não podem ser apagadas'); END`,
}

// Troca da forma de pagamento: no máximo uma por venda (sale_id único). As travas
// da venda, do pagamento e do caixa só deixam passar exatamente a troca registrada
// aqui; depois dela a forma atual já não é a "de origem" e nada mais muda. O
// lançamento só muda com o caixa aberto; a regra de "caixa de hoje" fica no serviço,
// que conhece o fuso da loja.
var paymentChangeSchema = []string{
	`CREATE TABLE sale_payment_changes (
		id               INTEGER PRIMARY KEY,
		sale_id          INTEGER NOT NULL UNIQUE REFERENCES sales (id),
		from_method      TEXT    NOT NULL,
		to_method        TEXT    NOT NULL CHECK (to_method IN ('dinheiro', 'pix', 'debito', 'credito')),
		reason           TEXT    NOT NULL CHECK (length(trim(reason)) > 0),
		user_id          INTEGER REFERENCES users (id),
		authorized_by_id INTEGER REFERENCES users (id),
		created_at       TEXT    NOT NULL,
		CHECK (from_method <> to_method)
	)`,
	`CREATE TRIGGER tr_sale_payment_changes_no_update BEFORE UPDATE ON sale_payment_changes
	 BEGIN SELECT RAISE(ABORT, 'a troca da forma de pagamento não pode ser alterada'); END`,
	`CREATE TRIGGER tr_sale_payment_changes_no_delete BEFORE DELETE ON sale_payment_changes
	 BEGIN SELECT RAISE(ABORT, 'a troca da forma de pagamento não pode ser apagada'); END`,
	`DROP TRIGGER tr_sales_values_immutable`,
	`CREATE TRIGGER tr_sales_values_immutable
	 BEFORE UPDATE OF customer_id, comanda_id, subtotal_cents, discount_cents, total_cents, payment_method, created_at ON sales
	 WHEN NEW.customer_id IS NOT OLD.customer_id OR NEW.comanda_id IS NOT OLD.comanda_id
	   OR NEW.subtotal_cents IS NOT OLD.subtotal_cents OR NEW.discount_cents IS NOT OLD.discount_cents
	   OR NEW.total_cents IS NOT OLD.total_cents OR NEW.created_at IS NOT OLD.created_at
	   OR NOT EXISTS (SELECT 1 FROM sale_payment_changes c
	     WHERE c.sale_id = OLD.id AND c.from_method = OLD.payment_method AND c.to_method = NEW.payment_method)
	 BEGIN SELECT RAISE(ABORT, 'os valores de uma venda não podem ser alterados'); END`,
	`DROP TRIGGER tr_sale_payments_no_update`,
	`CREATE TRIGGER tr_sale_payments_no_update BEFORE UPDATE ON sale_payments
	 WHEN NEW.id IS NOT OLD.id OR NEW.sale_id IS NOT OLD.sale_id OR NEW.amount_cents IS NOT OLD.amount_cents
	   OR NOT EXISTS (SELECT 1 FROM sale_payment_changes c
	     WHERE c.sale_id = OLD.sale_id AND c.from_method = OLD.method AND c.to_method = NEW.method)
	 BEGIN SELECT RAISE(ABORT, 'pagamentos de venda não podem ser alterados'); END`,
	`DROP TRIGGER tr_cash_movements_no_update`,
	`CREATE TRIGGER tr_cash_movements_no_update BEFORE UPDATE ON cash_movements
	 WHEN NEW.id IS NOT OLD.id OR NEW.session_id IS NOT OLD.session_id OR NEW.type IS NOT OLD.type
	   OR NEW.amount_cents IS NOT OLD.amount_cents OR NEW.description IS NOT OLD.description
	   OR NEW.reference_type IS NOT OLD.reference_type OR NEW.reference_id IS NOT OLD.reference_id
	   OR NEW.created_at IS NOT OLD.created_at OR NEW.user_id IS NOT OLD.user_id
	   OR OLD.type <> 'venda' OR OLD.reference_type IS NOT 'sale'
	   OR (SELECT status FROM cash_sessions WHERE id = OLD.session_id) IS NOT 'aberta'
	   OR NOT EXISTS (SELECT 1 FROM sale_payment_changes c
	     WHERE c.sale_id = OLD.reference_id AND c.from_method = OLD.payment_method AND c.to_method = NEW.payment_method)
	 BEGIN SELECT RAISE(ABORT, 'movimentações de caixa não podem ser alteradas'); END`,
}

var migration35 = []string{
	`ALTER TABLE subscriptions ADD COLUMN paused_at TEXT`,
	`ALTER TABLE subscriptions ADD COLUMN pause_reason TEXT`,
}
