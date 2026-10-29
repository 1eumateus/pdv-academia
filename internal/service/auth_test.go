// Caminho: pdv/internal/service/auth_test.go

package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"pdv/internal/models"
)

func codeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestSetupAndLogin(t *testing.T) {
	e := newEnv(t)
	if _, err := e.auth.SetupAdmin(context.Background(), models.UserInput{Name: "Outro", Login: "outro", Password: "1234"}); codeOf(err) != CodeStateConflict {
		t.Fatalf("segundo primeiro acesso deveria ser recusado: %v", err)
	}
	if u, err := e.auth.Login(context.Background(), " ADMIN ", "senha123", false); err != nil || u.ID != e.admin.ID {
		t.Fatalf("login com maiúsculas/espaços deveria funcionar: %v", err)
	}
	if _, err := e.auth.Login(context.Background(), "admin", "errada", false); codeOf(err) != CodeInvalid {
		t.Fatalf("senha errada deveria falhar: %v", err)
	}

	op := e.user("joao", models.RoleOperator)
	op.Active = false
	if _, err := e.auth.UpdateUser(e.ctx, models.UserInput{ID: op.ID, Name: op.Name, Login: op.Login, Role: op.Role}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.auth.Login(context.Background(), "joao", "1234", false); err == nil {
		t.Fatal("usuário desativado não pode entrar")
	}
}

func TestLoginLockout(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	b := NewBase(e.db)
	b.Now = func() time.Time { return now }
	auth := NewAuthService(b)
	for i := 0; i < maxLoginFailures; i++ {
		auth.Login(context.Background(), "admin", "x", false)
	}
	if _, err := auth.Login(context.Background(), "admin", "senha123", false); err == nil {
		t.Fatal("após várias falhas, deveria bloquear mesmo com a senha certa")
	}
	now = now.Add(loginLockout + time.Second)
	// O admin do newEnv está logado em outra instância: entra forçando.
	if _, err := auth.Login(context.Background(), "admin", "senha123", true); err != nil {
		t.Fatalf("depois do bloqueio deveria entrar: %v", err)
	}
}

func TestOperatorNeedsManagerToCancel(t *testing.T) {
	e := newEnv(t).withCash()
	op := e.user("caixa", models.RoleOperator)
	mgr := e.user("gerente", models.RoleManager)
	p := e.product("Pão", 100, 10)
	opCtx := e.as(op)

	sale, err := e.sales.CreateSale(opCtx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 2)}, PaymentMethod: models.PaymentCash})
	if err != nil {
		t.Fatal(err)
	}
	if sale.Sale.UserName == nil || *sale.Sale.UserName != "caixa" {
		t.Fatalf("venda deveria registrar o operador: %+v", sale.Sale.UserName)
	}

	_, err = e.sales.CancelSale(opCtx, sale.Sale.ID, "erro")
	var de *Error
	if !errors.As(err, &de) || de.Code != CodeAuthRequired || de.Permission != PermCancelSale {
		t.Fatalf("operador deveria precisar de autorização: %v", err)
	}
	if _, err := e.auth.Authorize(opCtx, PermCancelSale, "gerente", "errada"); err == nil {
		t.Fatal("senha errada do gerente não pode autorizar")
	}
	e.user("outro", models.RoleOperator)
	if _, err := e.auth.Authorize(opCtx, PermCancelSale, "outro", "1234"); codeOf(err) != CodeForbidden {
		t.Fatalf("operador não pode autorizar: %v", err)
	}
	if _, err := e.auth.Authorize(opCtx, PermCancelSale, "gerente", "1234"); err != nil {
		t.Fatal(err)
	}
	got, err := e.sales.CancelSale(opCtx, sale.Sale.ID, "erro de digitação")
	if err != nil {
		t.Fatal(err)
	}
	if got.Sale.CancelledByName == nil || *got.Sale.CancelledByName != "caixa" {
		t.Fatalf("cancelamento deveria registrar o operador: %v", got.Sale.CancelledByName)
	}
	var authBy int64
	if err := e.db.QueryRow(`SELECT authorized_by_id FROM audit_log WHERE action = 'venda_cancelada'`).Scan(&authBy); err != nil || authBy != mgr.ID {
		t.Fatalf("auditoria deveria apontar o gerente (%d): %d %v", mgr.ID, authBy, err)
	}

	// A autorização vale uma vez só.
	sale2, _ := e.sales.CreateSale(opCtx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, PaymentMethod: models.PaymentCash})
	if _, err := e.sales.CancelSale(opCtx, sale2.Sale.ID, "x"); codeOf(err) != CodeAuthRequired {
		t.Fatalf("autorização não pode ser reaproveitada: %v", err)
	}
	// O gerente cancela sem pedir autorização.
	if _, err := e.sales.CancelSale(e.as(mgr), sale2.Sale.ID, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestGrantExpires(t *testing.T) {
	e := newEnv(t).withCash()
	op := e.user("caixa", models.RoleOperator)
	e.user("gerente", models.RoleManager)
	opCtx := e.as(op)
	if _, err := e.auth.Authorize(opCtx, PermWithdrawal, "gerente", "1234"); err != nil {
		t.Fatal(err)
	}
	b := NewBase(e.db)
	b.Now = func() time.Time { return time.Now().Add(3 * time.Minute) }
	if _, err := NewCashService(b).RegisterMovement(opCtx, models.CashWithdrawal, 100, "troco"); codeOf(err) != CodeAuthRequired {
		t.Fatalf("autorização vencida não pode valer: %v", err)
	}
}

func TestOperatorLimits(t *testing.T) {
	e := newEnv(t).withCash()
	op := e.user("caixa", models.RoleOperator)
	opCtx := e.as(op)
	p := e.product("Camisa", 10000, 10)

	sell := func(discount int64) error {
		_, err := e.sales.CreateSale(opCtx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)},
			PaymentMethod: models.PaymentCash, DiscountCents: discount})
		return err
	}
	if err := sell(1000); err != nil { // 10%: no limite padrão
		t.Fatalf("desconto dentro do limite: %v", err)
	}
	before := e.count("sales")
	if err := sell(1001); codeOf(err) != CodeAuthRequired {
		t.Fatalf("desconto acima do limite deveria pedir autorização: %v", err)
	}
	if e.count("sales") != before || e.stockOf(p.ID) != 9 {
		t.Fatal("venda recusada não pode gravar nada")
	}

	if _, err := e.cash.RegisterMovement(opCtx, models.CashSupply, 500, "troco"); err != nil {
		t.Fatalf("suprimento é livre: %v", err)
	}
	if _, err := e.cash.RegisterMovement(opCtx, models.CashWithdrawal, 500, "banco"); codeOf(err) != CodeAuthRequired {
		t.Fatalf("sangria exige autorização: %v", err)
	}
	if _, err := e.stock.Adjust(opCtx, p.ID, -1, "quebra"); codeOf(err) != CodeAuthRequired {
		t.Fatalf("ajuste de estoque exige autorização: %v", err)
	}
	if _, err := e.stock.RegisterEntry(opCtx, p.ID, 5, ""); err != nil {
		t.Fatalf("entrada de mercadoria é livre: %v", err)
	}

	in := models.ProductInput{ID: p.ID, Name: "Camisa azul", PriceCents: p.PriceCents, Active: true}
	if _, err := e.products.Update(opCtx, in); err != nil {
		t.Fatalf("mudar só o nome é livre: %v", err)
	}
	in.PriceCents = 9000
	if _, err := e.products.Update(opCtx, in); codeOf(err) != CodeAuthRequired {
		t.Fatalf("mudar preço exige autorização: %v", err)
	}
	in.PriceCents, in.Active = p.PriceCents, false
	if _, err := e.products.Update(opCtx, in); codeOf(err) != CodeAuthRequired {
		t.Fatalf("desativar produto exige permissão: %v", err)
	}
	if _, err := NewSettingsService(NewBase(e.db)).Save(opCtx, storeSettings()); codeOf(err) != CodeAuthRequired {
		t.Fatalf("configurações exigem autorização: %v", err)
	}
	if _, err := e.customers.Create(opCtx, models.CustomerInput{Name: "Sem limite", Active: true}); err != nil {
		t.Fatalf("cliente sem limite é livre: %v", err)
	}
	if _, err := e.customers.Create(opCtx, models.CustomerInput{Name: "Com limite", CreditLimitCents: 5000, Active: true}); codeOf(err) != CodeAuthRequired {
		t.Fatalf("limite de fiado exige gerente: %v", err)
	}
	c := e.customer("Maria", 1000)
	if _, err := e.customers.Update(opCtx, models.CustomerInput{ID: c.ID, Name: "Maria Silva", CreditLimitCents: 1000, Active: true}); err != nil {
		t.Fatalf("editar sem mudar o limite é livre: %v", err)
	}
	if _, err := e.customers.Update(opCtx, models.CustomerInput{ID: c.ID, Name: "Maria Silva", CreditLimitCents: 99000, Active: true}); codeOf(err) != CodeAuthRequired {
		t.Fatalf("aumentar limite exige gerente: %v", err)
	}
	if _, err := e.auth.ListUsers(opCtx); codeOf(err) != CodeForbidden {
		t.Fatalf("usuários são só do administrador: %v", err)
	}
	if _, err := e.sales.CreateSale(context.Background(), models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)},
		PaymentMethod: models.PaymentCash, DiscountCents: 5000}); codeOf(err) != CodeLoginRequired {
		t.Fatalf("sem usuário deveria pedir login: %v", err)
	}

	r, err := e.reports.Sales(e.ctx, today(), today(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.ByOperator) != 1 || r.ByOperator[0].UserName != "caixa" || r.ByOperator[0].Count != 1 {
		t.Fatalf("relatório por operador errado: %+v", r.ByOperator)
	}
}

func TestUsersAdminRules(t *testing.T) {
	e := newEnv(t)
	if _, err := e.auth.UpdateUser(e.ctx, models.UserInput{ID: e.admin.ID, Name: "Admin", Login: "admin",
		Role: models.RoleManager, Active: true}); codeOf(err) != CodeStateConflict {
		t.Fatalf("não pode rebaixar o único administrador: %v", err)
	}
	if _, err := e.auth.CreateUser(e.ctx, models.UserInput{Name: "X", Login: "Admin", Password: "1234",
		Role: models.RoleOperator, Active: true}); codeOf(err) != CodeDuplicate {
		t.Fatalf("login repetido (sem diferenciar maiúsculas): %v", err)
	}
	if _, err := e.auth.CreateUser(e.ctx, models.UserInput{Name: "X", Login: "x", Password: "12", Role: models.RoleOperator}); codeOf(err) != CodeInvalid {
		t.Fatalf("senha curta: %v", err)
	}
	if err := e.auth.ChangeOwnPassword(e.ctx, "errada", "novasenha"); err == nil {
		t.Fatal("trocar senha exige a atual")
	}
	if err := e.auth.ChangeOwnPassword(e.ctx, "senha123", "novasenha"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.auth.Login(context.Background(), "admin", "novasenha", false); err != nil {
		t.Fatal(err)
	}
	list, err := e.auth.ListAudit(e.ctx, today(), today())
	if err != nil || len(list) == 0 {
		t.Fatalf("auditoria vazia: %v", err)
	}
}

func TestPasswordHash(t *testing.T) {
	h, err := hashPassword("segredo")
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := hashPassword("segredo")
	if h == h2 {
		t.Fatal("o sal deveria tornar os hashes diferentes")
	}
	if !checkPassword(h, "segredo") || checkPassword(h, "Segredo") || checkPassword("lixo", "segredo") {
		t.Fatal("verificação de senha errada")
	}
}

func today() string { return time.Now().Format("2006-01-02") }

func TestPerUserPermissions(t *testing.T) {
	e := newEnv(t).withCash()
	op := e.user("caixa", models.RoleOperator)
	mgr := e.user("gerente", models.RoleManager)
	if !slices.Equal(op.Permissions, DefaultPerms(models.RoleOperator)) || !slices.Equal(mgr.Permissions, DefaultPerms(models.RoleManager)) || slices.Contains(mgr.Permissions, PermAllSales) {
		t.Fatalf("usuário novo deveria vir com o padrão do papel: %v / %v", op.Permissions, mgr.Permissions)
	}

	// O administrador libera o cancelamento e tira a entrada de estoque deste operador.
	upd, err := e.auth.UpdateUser(e.ctx, models.UserInput{ID: op.ID, Name: op.Name, Login: op.Login, Role: op.Role, Active: true,
		Permissions: []string{PermCancelSale, PermProductCreate}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(upd.Permissions, []string{PermCancelSale, PermProductCreate}) {
		t.Fatalf("permissões gravadas erradas: %v", upd.Permissions)
	}
	opCtx := e.as(op)
	p := e.product("Pão", 100, 10)
	sale, err := e.sales.CreateSale(opCtx, models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, PaymentMethod: models.PaymentCash})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sales.CancelSale(opCtx, sale.Sale.ID, "erro"); err != nil {
		t.Fatalf("operador com permissão cancela sozinho: %v", err)
	}
	if _, err := e.stock.RegisterEntry(opCtx, p.ID, 5, ""); codeOf(err) != CodeAuthRequired {
		t.Fatalf("sem a permissão, entrada pede gerente: %v", err)
	}

	// Gerente sem uma permissão não pode autorizá-la.
	if _, err := e.auth.UpdateUser(e.ctx, models.UserInput{ID: mgr.ID, Name: mgr.Name, Login: mgr.Login, Role: mgr.Role, Active: true,
		Permissions: []string{PermWithdrawal}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.auth.Authorize(opCtx, PermStockEntry, "gerente", "1234"); codeOf(err) != CodeForbidden {
		t.Fatalf("gerente sem a permissão não autoriza: %v", err)
	}
	if _, err := e.auth.Authorize(opCtx, PermWithdrawal, "gerente", "1234"); err != nil {
		t.Fatal(err)
	}

	if _, err := e.auth.UpdateUser(e.ctx, models.UserInput{ID: op.ID, Name: op.Name, Login: op.Login, Role: op.Role, Active: true,
		Permissions: []string{PermUsers}}); codeOf(err) != CodeInvalid {
		t.Fatalf("usuários/auditoria não podem ser dados a quem não é administrador: %v", err)
	}
}

func TestSalesScope(t *testing.T) {
	e := newEnv(t).withCash()
	ana := e.user("ana", models.RoleOperator)
	bia := e.user("bia", models.RoleOperator)
	mgr := e.user("gerente", models.RoleManager)
	p := e.product("Pão", 100, 100)
	sellAs := func(u models.User, qty int64) {
		if _, err := e.sales.CreateSale(e.as(u), models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, qty)}, PaymentMethod: models.PaymentCash}); err != nil {
			t.Fatal(err)
		}
	}
	sellAs(ana, 1)
	sellAs(bia, 2)
	sellAs(bia, 3)

	// Operadora vê só as próprias, mesmo pedindo outra pessoa ou "todos".
	for _, asked := range []int64{0, bia.ID} {
		list, _ := e.sales.ListByPeriod(e.as(ana), today(), today(), asked)
		r, _ := e.reports.Sales(e.as(ana), today(), today(), asked)
		if len(list) != 1 || r.SaleCount != 1 || r.TotalCents != 100 {
			t.Fatalf("ana pediu %d e viu %d vendas / relatório %d", asked, len(list), r.SaleCount)
		}
	}
	// Gerente sem a permissão também vê só as próprias (nenhuma).
	if r, _ := e.reports.Sales(e.as(mgr), today(), today(), 0); r.SaleCount != 0 {
		t.Fatalf("gerente sem permissão viu %d vendas", r.SaleCount)
	}
	// Administrador vê todas e filtra por operador.
	if r, _ := e.reports.Sales(e.ctx, today(), today(), 0); r.SaleCount != 3 {
		t.Fatalf("admin deveria ver 3: %d", r.SaleCount)
	}
	if list, _ := e.sales.ListByPeriod(e.ctx, today(), today(), bia.ID); len(list) != 2 {
		t.Fatalf("filtro por operador: %d", len(list))
	}
	if sellers, _ := e.auth.ListSellers(e.as(ana)); len(sellers) != 0 {
		t.Fatal("operadora não deveria receber a lista de operadores")
	}
}

func TestOneLoginPerUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	other := NewAuthService(NewBase(e.db)) // outro PC (ou o programa aberto de novo)
	if _, err := other.Login(ctx, "admin", "senha123", false); codeOf(err) != CodeAlreadyLoggedIn {
		t.Fatalf("logado em outra instância: esperava USUARIO_JA_LOGADO, veio %v", err)
	}
	u, err := other.Login(ctx, "admin", "senha123", true)
	if err != nil || u.SessionID != other.Instance {
		t.Fatalf("forçar deveria tomar a sessão: %v", err)
	}
	// A primeira instância não sai com a sessão que já não é dela.
	if err := e.auth.EndSession(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := other.User(ctx, u.ID); got.SessionID != other.Instance {
		t.Fatal("EndSession de outra instância não pode liberar a sessão atual")
	}
	if err := other.EndSession(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.auth.Login(ctx, "admin", "senha123", false); err != nil {
		t.Fatalf("depois de sair, deveria entrar sem aviso: %v", err)
	}
}
