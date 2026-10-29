// Caminho: pdv/internal/service/helpers_test.go

package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"pdv/internal/actor"
	"pdv/internal/db"
	"pdv/internal/models"
)

// Hash rápido nos testes; o formato guarda as iterações, então nada muda na verificação.
func init() { pbkdf2Iterations = 1000 }

type env struct {
	t         *testing.T
	ctx       context.Context
	db        *sql.DB
	dataDir   string
	dbPath    string
	products  *ProductService
	stock     *StockService
	customers *CustomerService
	credit    *CreditService
	sales     *SaleService
	comandas  *ComandaService
	cash      *CashService
	reports   *ReportService
	backup    *BackupService
	auth      *AuthService
	grants    *actor.Grants
	admin     models.User
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, db.FileName)
	conn, err := db.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	b := NewBase(conn)
	e := &env{
		t: t, ctx: ctx, db: conn, dataDir: dir, dbPath: path,
		products: NewProductService(b), stock: NewStockService(b), customers: NewCustomerService(b),
		credit: NewCreditService(b), sales: NewSaleService(b), comandas: NewComandaService(b),
		cash: NewCashService(b), reports: NewReportService(b), backup: NewBackupService(b, dir, path),
		auth: NewAuthService(b), grants: actor.NewGrants(),
	}
	// Por padrão os testes operam como administrador (todas as permissões).
	admin, err := e.auth.SetupAdmin(ctx, models.UserInput{Name: "Admin", Login: "admin", Password: "senha123"})
	if err != nil {
		t.Fatal(err)
	}
	e.admin = admin
	e.ctx = e.as(admin)
	return e
}

// as devolve um context operando como o usuário informado.
// as devolve um context operando como o usuário, com as permissões gravadas (como faz o App).
func (e *env) as(u models.User) context.Context {
	e.t.Helper()
	fresh, err := e.auth.User(context.Background(), u.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return actor.With(context.Background(), actor.User{ID: fresh.ID, Name: fresh.Name, Role: fresh.Role, Perms: fresh.Permissions}, e.grants)
}

// user cadastra um usuário com o papel informado (senha "1234").
func (e *env) user(login, role string) models.User {
	e.t.Helper()
	u, err := e.auth.CreateUser(e.ctx, models.UserInput{Name: login, Login: login, Password: "1234", Role: role, Active: true})
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

// withCash abre o caixa com R$ 100,00.
func (e *env) withCash() *env {
	e.t.Helper()
	if _, err := e.cash.Open(e.ctx, 10000); err != nil {
		e.t.Fatal(err)
	}
	return e
}

func (e *env) product(name string, price, stock int64) models.Product {
	e.t.Helper()
	p, err := e.products.Create(e.ctx, models.ProductInput{Name: name, PriceCents: price, Active: true, InitialStock: stock})
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) customer(name string, limit int64) models.Customer {
	e.t.Helper()
	c, err := e.customers.Create(e.ctx, models.CustomerInput{Name: name, CreditLimitCents: limit, Active: true})
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

func (e *env) sell(method string, customerID *int64, items ...models.SaleItemInput) (models.SaleDetails, error) {
	return e.sales.CreateSale(e.ctx, models.CreateSaleInput{Items: items, PaymentMethod: method, CustomerID: customerID})
}

func item(productID, qty int64) models.SaleItemInput {
	return models.SaleItemInput{ProductID: productID, Qty: qty}
}

func (e *env) count(table string) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) stockOf(id int64) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.QueryRow(`SELECT stock_qty FROM products WHERE id = ?`, id).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) balanceOf(id int64) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.QueryRow(`SELECT credit_balance_cents FROM customers WHERE id = ?`, id).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// failOn instala um trigger que aborta a operação na tabela, simulando uma
// falha no meio de uma transação.
func (e *env) failOn(event, table string) {
	e.t.Helper()
	name := "test_fail_" + event + "_" + table
	if _, err := e.db.Exec(`CREATE TRIGGER ` + name + ` BEFORE ` + event + ` ON ` + table +
		` BEGIN SELECT RAISE(ABORT, 'falha simulada'); END`); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { e.db.Exec(`DROP TRIGGER IF EXISTS ` + name) })
}

// snapshot captura contagens e saldos para comparar antes/depois de um rollback.
type snapshot map[string]int64

func (e *env) snap(productIDs, customerIDs []int64) snapshot {
	s := snapshot{}
	for _, t := range []string{"sales", "sale_items", "stock_movements", "cash_movements", "credit_payments"} {
		s[t] = e.count(t)
	}
	for _, id := range productIDs {
		s["stock_"+itoa(id)] = e.stockOf(id)
	}
	for _, id := range customerIDs {
		s["balance_"+itoa(id)] = e.balanceOf(id)
	}
	return s
}

func (e *env) assertUnchanged(before snapshot, productIDs, customerIDs []int64) {
	e.t.Helper()
	after := e.snap(productIDs, customerIDs)
	for k, v := range before {
		if after[k] != v {
			e.t.Errorf("%s mudou após rollback: antes %d, depois %d", k, v, after[k])
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func wantCode(t *testing.T, err error, code Code) {
	t.Helper()
	var de *Error
	if !errors.As(err, &de) || de.Code != code {
		t.Fatalf("esperado erro %s, veio %v", code, err)
	}
}
