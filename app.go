// Caminho: catraca/app.go

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"pdv/internal/actor"
	"pdv/internal/db"
	"pdv/internal/device"
	"pdv/internal/license"
	"pdv/internal/models"
	"pdv/internal/pdf"
	"pdv/internal/printer"
	"pdv/internal/service"
)

// AppVersion é exibida em Configurações.
const AppVersion = "1.1.0"

// ErrorPayload é o contrato de erro entre Go e JavaScript. Todo erro devolvido
// por um método do App é um JSON {"code": "...", "message": "..."}; o frontend
// decide o fluxo pelo code e mostra a message. Ver frontend/src/api.ts.
type ErrorPayload struct {
	Code    service.Code `json:"code"`
	Message string       `json:"message"`
	// Permission vem com AUTORIZACAO_NECESSARIA: o que o gerente deve liberar.
	Permission string `json:"permission,omitempty"`
}

// App expõe os métodos chamados pelo frontend. Não contém regra de negócio:
// encaminha para os serviços e converte os erros.
type App struct {
	ctx        context.Context
	log        *slog.Logger
	db         *sql.DB
	dataDir    string
	dbPath     string
	startupErr error
	notice     string

	products  *service.ProductService
	stock     *service.StockService
	customers *service.CustomerService
	credit    *service.CreditService
	sales     *service.SaleService
	comandas  *service.ComandaService
	cash      *service.CashService
	subs      *service.SubscriptionService
	gate      *service.CatracaService
	bills     *service.BillService
	subImport *service.SubscriptionImportService
	reports   *service.ReportService
	backup    *service.BackupService
	purge     *service.PurgeService
	settings  *service.SettingsService
	receipts  *service.ReceiptService
	auth      *service.AuthService
	suppliers *service.SupplierService
	purchases *service.PurchaseService
	nfe       *service.NFeService
	promos    *service.PromotionService
	quotes    *service.QuoteService
	returns   *service.ReturnService
	analytics *service.AnalyticsService
	license   *service.LicenseService
	imports   *service.ImportService

	// backupMu impede a rotina automática e o "Copiar agora" de rodarem juntos.
	backupMu   sync.Mutex
	stopBackup chan struct{}
	// clientErrors limita quantos erros da tela vão para o log por execução.
	clientErrors atomic.Int32

	// Usuário logado nesta máquina e autorizações de gerente pendentes.
	mu     sync.Mutex
	user   *models.User
	grants *actor.Grants
}

// NewApp recebe as dependências já abertas. Se a inicialização falhou,
// startupErr é exibido pela tela e todos os métodos o devolvem.
func NewApp(log *slog.Logger, conn *sql.DB, dataDir, dbPath string, startupErr error, notice string) *App {
	a := &App{log: log, db: conn, dataDir: dataDir, dbPath: dbPath, startupErr: startupErr, notice: notice,
		grants: actor.NewGrants()}
	if conn == nil {
		return a
	}
	b := service.NewBase(conn)
	a.products = service.NewProductService(b)
	a.stock = service.NewStockService(b)
	a.customers = service.NewCustomerService(b)
	a.credit = service.NewCreditService(b)
	a.sales = service.NewSaleService(b)
	a.comandas = service.NewComandaService(b)
	a.cash = service.NewCashService(b)
	a.subs = service.NewSubscriptionService(b)
	a.gate = service.NewCatracaService(b)
	a.bills = service.NewBillService(b)
	a.subImport = service.NewSubscriptionImportService(b)
	a.reports = service.NewReportService(b)
	a.backup = service.NewBackupService(b, dataDir, dbPath)
	a.purge = service.NewPurgeService(b)
	a.settings = service.NewSettingsService(b)
	a.receipts = service.NewReceiptService(b)
	a.auth = service.NewAuthService(b)
	a.suppliers = service.NewSupplierService(b)
	a.purchases = service.NewPurchaseService(b)
	a.nfe = service.NewNFeService(b, dataDir)
	a.promos = service.NewPromotionService(b)
	a.quotes = service.NewQuoteService(b)
	a.returns = service.NewReturnService(b)
	a.analytics = service.NewAnalyticsService(b)
	a.license = service.NewLicenseService(b, &license.RegistryStore{})
	a.imports = service.NewImportService(b)
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.backup != nil {
		a.stopBackup = make(chan struct{})
		go a.autoBackupLoop()
	}
	if a.gate != nil {
		// Cada passagem vai para a tela na hora (EventsOn("acesso")).
		a.gate.OnEvent = func(e models.GateEvent) { runtime.EventsEmit(ctx, "acesso", e) }
		// Diagnóstico do giro: todo aviso do iDFace que não é rosto vai para o pdv.log.
		device.OnNotice = func(path, body string) { a.log.Info("aviso do iDFace", "caminho", path, "dados", body) }
		a.log.Info("diagnóstico do giro ativo: avisos do iDFace vão para este log")
		if err := a.gate.Start(context.Background()); err != nil {
			a.log.Error("iniciar catraca", "erro", err)
		}
	}
}

// autoBackupLoop confere ao abrir e depois de hora em hora se falta o backup do
// dia ou a cópia externa (o pendrive pode ser plugado mais tarde).
func (a *App) autoBackupLoop() {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		a.runAutoBackup(false)
		select {
		case <-tick.C:
		case <-a.stopBackup:
			return
		}
	}
}

func (a *App) runAutoBackup(force bool) (models.BackupStatus, error) {
	a.backupMu.Lock()
	defer a.backupMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	st, err := a.backup.RunAuto(ctx, force)
	if err != nil {
		a.log.Error("backup automático", "erro", err)
	} else if st.ExternalError != "" {
		a.log.Warn("cópia externa do backup", "erro", st.ExternalError)
	}
	return st, err
}

func (a *App) shutdown(ctx context.Context) {
	if a.gate != nil {
		a.gate.Stop()
	}
	if u := a.currentUser(); u != nil && a.db != nil {
		if err := a.auth.EndSession(ctx, u.ID); err != nil {
			a.log.Error("encerrar sessão ao fechar", "erro", err)
		}
	}
	if a.stopBackup != nil {
		close(a.stopBackup)
		a.backupMu.Lock() // espera um backup em andamento antes de fechar o banco
		defer a.backupMu.Unlock()
	}
	if a.db != nil {
		// Close faz o checkpoint do WAL e libera os arquivos.
		if err := a.db.Close(); err != nil {
			a.log.Error("fechar banco", "erro", err)
		}
	}
}

// fail converte um erro no contrato ErrorPayload e registra os técnicos no log.
func (a *App) fail(op string, err error) error {
	code, msg, technical := service.Public(err)
	if technical {
		user := ""
		if u := a.currentUser(); u != nil {
			user = u.Login
		}
		a.log.Error(op, "erro", err, "usuario", user)
	}
	p := ErrorPayload{Code: code, Message: msg}
	var de *service.Error
	if errors.As(err, &de) {
		p.Permission = de.Permission
	}
	b, _ := json.Marshal(p)
	return errors.New(string(b))
}

// call executa uma operação com timeout, exigindo usuário logado. O usuário
// segue no context até os serviços (permissões) e repositórios (autoria).
func call[T any](a *App, op string, fn func(ctx context.Context) (T, error)) (T, error) {
	cur := a.currentUser()
	if cur == nil && a.startupErr == nil {
		var zero T
		return zero, a.fail(op, &service.Error{Code: service.CodeLoginRequired, Message: "Entre com seu usuário para continuar."})
	}
	return callAnon(a, op, func(ctx context.Context) (T, error) {
		if cur == nil {
			return fn(ctx)
		}
		// Relido a cada operação: papel, permissões ou desativação feitos pelo
		// administrador valem na hora.
		u, err := a.auth.User(ctx, cur.ID)
		if err != nil {
			var zero T
			return zero, err
		}
		if !u.Active {
			a.setUser(nil)
			var zero T
			return zero, &service.Error{Code: service.CodeLoginRequired, Message: "Seu usuário foi desativado. Fale com o administrador."}
		}
		if u.SessionID != a.auth.Instance {
			a.setUser(nil)
			var zero T
			return zero, &service.Error{Code: service.CodeLoginRequired, Message: "Sua sessão foi encerrada: você entrou em outro lugar ou o administrador liberou seu login."}
		}
		ctx = actor.With(ctx, actor.User{ID: u.ID, Name: u.Name, Role: u.Role, Perms: u.Permissions}, a.grants)
		return fn(ctx)
	})
}

// callAnon é call sem exigir login (tela de login, primeiro acesso, nome da loja).
func callAnon[T any](a *App, op string, fn func(ctx context.Context) (T, error)) (v T, err error) {
	var zero T
	if a.startupErr != nil {
		return zero, a.fail(op, &service.Error{Code: service.CodeStorage,
			Message: "O banco de dados não pôde ser aberto. Veja as orientações em Configurações."})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Um panic numa operação não derruba o programa: vai para o log com a pilha
	// e a tela recebe o erro interno genérico.
	defer func() {
		if r := recover(); r != nil {
			a.log.Error("panic", "operacao", op, "erro", r, "pilha", string(debug.Stack()))
			err = a.fail(op, fmt.Errorf("panic: %v", r))
			v = zero
		}
	}()
	v, err = fn(ctx)
	if err != nil {
		return zero, a.fail(op, err)
	}
	return v, nil
}

// callW é o call das operações que gravam movimento: sem licença válida
// (teste encerrado, licença vencida, relógio voltado) o sistema só consulta.
func callW[T any](a *App, op string, fn func(ctx context.Context) (T, error)) (T, error) {
	return call(a, op, func(ctx context.Context) (T, error) {
		if err := a.license.RequireWrite(ctx); err != nil {
			var zero T
			return zero, err
		}
		return fn(ctx)
	})
}

func (a *App) currentUser() *models.User {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.user
}

func (a *App) setUser(u *models.User) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.user = u
	a.grants.Clear()
}

// ---------------- Login e usuários ----------------

func (a *App) GetSession() (models.Session, error) {
	return callAnon(a, "ler sessão", func(ctx context.Context) (models.Session, error) {
		cur := a.currentUser()
		if cur != nil {
			u, err := a.auth.User(ctx, cur.ID) // permissões atualizadas
			if err != nil {
				return models.Session{}, err
			}
			cur = &u
		}
		return a.auth.Session(ctx, cur)
	})
}

// SetupAdmin cria o primeiro administrador (banco sem usuários) e já entra com ele.
func (a *App) SetupAdmin(in models.UserInput) (models.Session, error) {
	return callAnon(a, "primeiro acesso", func(ctx context.Context) (models.Session, error) {
		u, err := a.auth.SetupAdmin(ctx, in)
		if err != nil {
			return models.Session{}, err
		}
		a.setUser(&u)
		return a.auth.Session(ctx, &u)
	})
}

// Login entra com o usuário; force encerra a sessão dele em outro lugar
// (depois de a tela confirmar o aviso USUARIO_JA_LOGADO).
func (a *App) Login(login, password string, force bool) (models.Session, error) {
	return callAnon(a, "entrar", func(ctx context.Context) (models.Session, error) {
		u, err := a.auth.Login(ctx, login, password, force)
		if err != nil {
			return models.Session{}, err
		}
		if prev := a.currentUser(); prev != nil && prev.ID != u.ID {
			if err := a.auth.EndSession(ctx, prev.ID); err != nil {
				return models.Session{}, err
			}
		}
		a.setUser(&u)
		a.log.Info("login", "usuario", u.Login)
		return a.auth.Session(ctx, &u)
	})
}

// Logout encerra a sessão (troca de operador ou bloqueio por inatividade).
func (a *App) Logout() error {
	_, err := call(a, "sair", func(ctx context.Context) (struct{}, error) {
		err := a.auth.Logout(ctx)
		a.setUser(nil)
		return struct{}{}, err
	})
	if err != nil && a.currentUser() == nil {
		return nil // já estava sem sessão
	}
	return err
}

// AuthorizeAction: o gerente digita login e senha para liberar a próxima
// operação da permissão para o operador logado. Devolve o nome de quem autorizou.
func (a *App) AuthorizeAction(permission, login, password string) (string, error) {
	return call(a, "autorizar operação", func(ctx context.Context) (string, error) {
		sup, err := a.auth.Authorize(ctx, permission, login, password)
		return sup.Name, err
	})
}

func (a *App) GetPermissionCatalog() (models.PermissionCatalog, error) {
	return call(a, "listar permissões", func(ctx context.Context) (models.PermissionCatalog, error) { return a.auth.Catalog(ctx) })
}

// ListSellers alimenta o filtro por operador (vazio para quem só vê as próprias vendas).
func (a *App) ListSellers() ([]models.User, error) {
	return call(a, "listar operadores", func(ctx context.Context) ([]models.User, error) { return a.auth.ListSellers(ctx) })
}

func (a *App) ListUsers() ([]models.User, error) {
	return call(a, "listar usuários", func(ctx context.Context) ([]models.User, error) { return a.auth.ListUsers(ctx) })
}

func (a *App) CreateUser(in models.UserInput) (models.User, error) {
	return call(a, "cadastrar usuário", func(ctx context.Context) (models.User, error) { return a.auth.CreateUser(ctx, in) })
}

// ReleaseUserSession: o administrador libera o login preso de um usuário.
func (a *App) ReleaseUserSession(id int64) error {
	_, err := call(a, "liberar login", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, a.auth.ReleaseSession(ctx, id)
	})
	return err
}

// onSecondInstance: abriram o programa de novo; traz para frente a janela já aberta.
func (a *App) onSecondInstance(options.SecondInstanceData) {
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
}

func (a *App) UpdateUser(in models.UserInput) (models.User, error) {
	return call(a, "alterar usuário", func(ctx context.Context) (models.User, error) {
		u, err := a.auth.UpdateUser(ctx, in)
		if err == nil {
			if cur := a.currentUser(); cur != nil && cur.ID == u.ID {
				if u.Active {
					a.mu.Lock()
					a.user = &u // nome/papel novos valem já
					a.mu.Unlock()
				} else {
					a.setUser(nil) // desativou o próprio cadastro: sai
				}
			}
		}
		return u, err
	})
}

func (a *App) ChangeOwnPassword(current, next string) error {
	_, err := call(a, "alterar senha", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, a.auth.ChangeOwnPassword(ctx, current, next)
	})
	return err
}

func (a *App) ListAudit(startDate, endDate string) ([]models.AuditEntry, error) {
	return call(a, "listar auditoria", func(ctx context.Context) ([]models.AuditEntry, error) {
		return a.auth.ListAudit(ctx, startDate, endDate)
	})
}

// ---------------- Produtos ----------------

func (a *App) ListProducts() ([]models.Product, error) {
	return call(a, "listar produtos", func(ctx context.Context) ([]models.Product, error) { return a.products.List(ctx) })
}

func (a *App) CreateProduct(in models.ProductInput) (models.Product, error) {
	return callW(a, "cadastrar produto", func(ctx context.Context) (models.Product, error) { return a.products.Create(ctx, in) })
}

func (a *App) UpdateProduct(in models.ProductInput) (models.Product, error) {
	return callW(a, "editar produto", func(ctx context.Context) (models.Product, error) { return a.products.Update(ctx, in) })
}

// SuggestProductCode: próximo SKU/código interno livre, para o formulário de cadastro.
func (a *App) SuggestProductCode() (models.ProductCode, error) {
	return call(a, "sugerir código", func(ctx context.Context) (models.ProductCode, error) { return a.products.SuggestCode(ctx) })
}

// DeleteProduct exclui o produto sem histórico; com histórico, desativa (devolve false).
func (a *App) DeleteProduct(id int64) (bool, error) {
	return callW(a, "excluir produto", func(ctx context.Context) (bool, error) { return a.products.Delete(ctx, id) })
}

func (a *App) SaveGrid(in models.GridInput) (models.ProductModel, error) {
	return callW(a, "salvar grade", func(ctx context.Context) (models.ProductModel, error) { return a.products.SaveGrid(ctx, in) })
}

func (a *App) GetProductModel(id int64) (models.ProductModel, error) {
	return call(a, "buscar grade", func(ctx context.Context) (models.ProductModel, error) { return a.products.GetModel(ctx, id) })
}

func (a *App) ApplyGridPrice(modelID, priceCents int64) (models.ProductModel, error) {
	return callW(a, "preço da grade", func(ctx context.Context) (models.ProductModel, error) {
		return a.products.ApplyGridPrice(ctx, modelID, priceCents)
	})
}

func (a *App) ListCategories() ([]models.Category, error) {
	return call(a, "listar categorias", func(ctx context.Context) ([]models.Category, error) { return a.products.ListCategories(ctx) })
}

func (a *App) SaveCategory(c models.Category) (models.Category, error) {
	return callW(a, "salvar categoria", func(ctx context.Context) (models.Category, error) { return a.products.SaveCategory(ctx, c) })
}

func (a *App) ProductsForSale(ids []int64) ([]models.Product, error) {
	return call(a, "carregar produtos", func(ctx context.Context) ([]models.Product, error) { return a.products.ForSale(ctx, ids) })
}

func (a *App) SearchProductByBarcode(code string) (models.Product, error) {
	return call(a, "buscar por código", func(ctx context.Context) (models.Product, error) { return a.products.SearchByBarcode(ctx, code) })
}

// SearchProducts busca por nome/código; categoryID 0 = todas as categorias.
func (a *App) SearchProducts(query string, onlyActive bool, categoryID int64) ([]models.Product, error) {
	return call(a, "pesquisar produtos", func(ctx context.Context) ([]models.Product, error) {
		return a.products.Search(ctx, query, onlyActive, categoryID)
	})
}

// ListSaleCatalog: produtos vendáveis para a lista da tela de vendas.
func (a *App) ListSaleCatalog() ([]models.Product, error) {
	return call(a, "lista de produtos", func(ctx context.Context) ([]models.Product, error) { return a.products.SaleCatalog(ctx) })
}

// ---------------- Fornecedores ----------------

func (a *App) ListSuppliers(term string) ([]models.Supplier, error) {
	return call(a, "listar fornecedores", func(ctx context.Context) ([]models.Supplier, error) { return a.suppliers.List(ctx, term) })
}

func (a *App) SaveSupplier(in models.Supplier) (models.Supplier, error) {
	return callW(a, "salvar fornecedor", func(ctx context.Context) (models.Supplier, error) { return a.suppliers.Save(ctx, in) })
}

func (a *App) CreatePurchase(in models.PurchaseInput) (models.PurchaseDetails, error) {
	return callW(a, "registrar compra", func(ctx context.Context) (models.PurchaseDetails, error) { return a.purchases.Create(ctx, in) })
}

// ListPurchases: com term (nota, chave ou fornecedor), procura em todas as datas.
func (a *App) ListPurchases(startDate, endDate, term string) ([]models.Purchase, error) {
	return call(a, "listar compras", func(ctx context.Context) ([]models.Purchase, error) {
		return a.purchases.List(ctx, startDate, endDate, term)
	})
}

func (a *App) CancelPurchase(id int64, reason string) (models.PurchaseDetails, error) {
	return callW(a, "estornar entrada", func(ctx context.Context) (models.PurchaseDetails, error) { return a.purchases.Cancel(ctx, id, reason) })
}

func (a *App) GetPurchase(id int64) (models.PurchaseDetails, error) {
	return call(a, "detalhes da compra", func(ctx context.Context) (models.PurchaseDetails, error) { return a.purchases.Details(ctx, id) })
}

// PickNFeFiles abre o diálogo para escolher um ou mais XMLs e devolve os caminhos.
func (a *App) PickNFeFiles() ([]string, error) {
	files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Escolher XML da NF-e",
		Filters: []runtime.FileFilter{{DisplayName: "XML da NF-e (*.xml)", Pattern: "*.xml"}},
	})
	if err != nil {
		return nil, a.fail("escolher XML", err)
	}
	return files, nil
}

// PreviewNFe lê o XML do caminho escolhido e devolve a conferência, sem gravar nada.
func (a *App) PreviewNFe(path string) (models.NFePreview, error) {
	return call(a, "conferir NF-e", func(ctx context.Context) (models.NFePreview, error) { return a.nfe.PreviewFile(ctx, path) })
}

// ConfirmNFe grava a compra da nota com as escolhas da conferência.
// priceLater cadastra os produtos novos sem preço como "a precificar".
func (a *App) ConfirmNFe(path string, decisions []models.NFeDecision, priceLater bool) (models.PurchaseDetails, error) {
	return callW(a, "importar NF-e", func(ctx context.Context) (models.PurchaseDetails, error) {
		d, err := a.nfe.ConfirmFile(ctx, path, decisions, priceLater)
		if err == nil {
			a.log.Info("NF-e importada", "compra", d.Purchase.ID)
		}
		return d, err
	})
}

// ---------------- Promoções ----------------

func (a *App) ListPromotions() ([]models.Promotion, error) {
	return call(a, "listar promoções", func(ctx context.Context) ([]models.Promotion, error) { return a.promos.List(ctx) })
}

func (a *App) SavePromotion(in models.Promotion) (models.Promotion, error) {
	return callW(a, "salvar promoção", func(ctx context.Context) (models.Promotion, error) { return a.promos.Save(ctx, in) })
}

// ---------------- Orçamentos ----------------

func (a *App) SaveQuote(in models.QuoteInput) (models.QuoteDetails, error) {
	return callW(a, "salvar orçamento", func(ctx context.Context) (models.QuoteDetails, error) { return a.quotes.Save(ctx, in) })
}

func (a *App) GetQuote(id int64) (models.QuoteDetails, error) {
	return call(a, "abrir orçamento", func(ctx context.Context) (models.QuoteDetails, error) { return a.quotes.Get(ctx, id) })
}

// ListQuotes: com term (nº ou cliente), procura em todas as datas.
func (a *App) ListQuotes(startDate, endDate, term string) ([]models.Quote, error) {
	return call(a, "listar orçamentos", func(ctx context.Context) ([]models.Quote, error) {
		return a.quotes.List(ctx, startDate, endDate, term)
	})
}

func (a *App) SetQuoteStatus(id int64, status string) (models.QuoteDetails, error) {
	return callW(a, "situação do orçamento", func(ctx context.Context) (models.QuoteDetails, error) { return a.quotes.SetStatus(ctx, id, status) })
}

func (a *App) DuplicateQuote(id int64) (models.QuoteDetails, error) {
	return callW(a, "duplicar orçamento", func(ctx context.Context) (models.QuoteDetails, error) { return a.quotes.Duplicate(ctx, id) })
}

func (a *App) CheckQuote(id int64) (models.QuoteCheck, error) {
	return call(a, "conferir orçamento", func(ctx context.Context) (models.QuoteCheck, error) { return a.quotes.Check(ctx, id) })
}

func (a *App) ConvertQuote(in models.QuoteConvertInput) (models.SaleDetails, error) {
	return callW(a, "converter orçamento", func(ctx context.Context) (models.SaleDetails, error) { return a.quotes.Convert(ctx, in) })
}

// ---------------- Clientes e fiado ----------------

func (a *App) ListCustomers(term string) ([]models.Customer, error) {
	return call(a, "listar clientes", func(ctx context.Context) ([]models.Customer, error) { return a.customers.List(ctx, term) })
}

func (a *App) GetCustomer(id int64) (models.Customer, error) {
	return call(a, "ler cliente", func(ctx context.Context) (models.Customer, error) { return a.customers.Get(ctx, id) })
}

func (a *App) CreateCustomer(in models.CustomerInput) (models.Customer, error) {
	return callW(a, "cadastrar cliente", func(ctx context.Context) (models.Customer, error) { return a.customers.Create(ctx, in) })
}

func (a *App) UpdateCustomer(in models.CustomerInput) (models.Customer, error) {
	return callW(a, "editar cliente", func(ctx context.Context) (models.Customer, error) { return a.customers.Update(ctx, in) })
}

func (a *App) GetCustomerHistory(customerID int64) (models.CustomerHistory, error) {
	return call(a, "histórico do cliente", func(ctx context.Context) (models.CustomerHistory, error) {
		return a.customers.History(ctx, customerID)
	})
}

func (a *App) PayCustomerCredit(in models.PayCreditInput) (models.CreditPayment, error) {
	return callW(a, "receber fiado", func(ctx context.Context) (models.CreditPayment, error) { return a.credit.PayCustomerCredit(ctx, in) })
}

func (a *App) ReverseCreditPayment(paymentID int64, reason string) (models.CreditPayment, error) {
	return callW(a, "estornar recebimento", func(ctx context.Context) (models.CreditPayment, error) {
		return a.credit.ReversePayment(ctx, paymentID, reason)
	})
}

// ---------------- Vendas ----------------

func (a *App) CreateSale(in models.CreateSaleInput) (models.SaleDetails, error) {
	return callW(a, "registrar venda", func(ctx context.Context) (models.SaleDetails, error) { return a.sales.CreateSale(ctx, in) })
}

func (a *App) ListSalesByDate(date string) ([]models.Sale, error) {
	return call(a, "listar vendas", func(ctx context.Context) ([]models.Sale, error) { return a.sales.ListByPeriod(ctx, date, date, 0) })
}

// ListSalesByPeriod: userID 0 = todos (só para quem pode ver as vendas de todos).
func (a *App) ListSalesByPeriod(startDate, endDate string, userID int64) ([]models.Sale, error) {
	return call(a, "listar vendas", func(ctx context.Context) ([]models.Sale, error) {
		return a.sales.ListByPeriod(ctx, startDate, endDate, userID)
	})
}

func (a *App) GetSaleDetails(saleID int64) (models.SaleDetails, error) {
	return call(a, "detalhes da venda", func(ctx context.Context) (models.SaleDetails, error) { return a.sales.Details(ctx, saleID) })
}

func (a *App) CancelSale(saleID int64, reason string) (models.SaleDetails, error) {
	return callW(a, "cancelar venda", func(ctx context.Context) (models.SaleDetails, error) {
		return a.sales.CancelSale(ctx, saleID, reason)
	})
}

// ChangeSalePaymentMethod troca a forma de pagamento da venda (uma vez por venda).
func (a *App) ChangeSalePaymentMethod(saleID int64, method, reason string) (models.SaleDetails, error) {
	return callW(a, "trocar a forma de pagamento", func(ctx context.Context) (models.SaleDetails, error) {
		return a.sales.ChangePaymentMethod(ctx, saleID, method, reason)
	})
}

// ---------------- Estoque ----------------

func (a *App) RegisterStockEntry(productID, qty int64, notes string) (models.Product, error) {
	return callW(a, "entrada de estoque", func(ctx context.Context) (models.Product, error) {
		return a.stock.RegisterEntry(ctx, productID, qty, notes)
	})
}

func (a *App) AdjustStock(productID, qtyChange int64, notes string) (models.Product, error) {
	return callW(a, "ajuste de estoque", func(ctx context.Context) (models.Product, error) {
		return a.stock.Adjust(ctx, productID, qtyChange, notes)
	})
}

func (a *App) RegisterStockReturn(productID, qty int64, notes string) (models.Product, error) {
	return callW(a, "devolução ao estoque", func(ctx context.Context) (models.Product, error) {
		return a.stock.RegisterReturn(ctx, productID, qty, notes)
	})
}

// ListStockMovements: productID 0 lista todos os produtos.
func (a *App) ListStockMovements(productID int64, startDate, endDate string) ([]models.StockMovement, error) {
	return call(a, "movimentações de estoque", func(ctx context.Context) ([]models.StockMovement, error) {
		return a.stock.ListMovements(ctx, productID, startDate, endDate)
	})
}

func (a *App) AuditStock() ([]models.StockAuditEntry, error) {
	return call(a, "auditoria de estoque", func(ctx context.Context) ([]models.StockAuditEntry, error) { return a.stock.Audit(ctx) })
}

func (a *App) ReconcileStock(productID int64, notes string) (models.StockAuditEntry, error) {
	return callW(a, "corrigir divergência", func(ctx context.Context) (models.StockAuditEntry, error) {
		return a.stock.ReconcileDivergence(ctx, productID, notes)
	})
}

// ---------------- Comandas ----------------

func (a *App) OpenComanda(tableNo string) (models.ComandaDetails, error) {
	return callW(a, "abrir comanda", func(ctx context.Context) (models.ComandaDetails, error) { return a.comandas.Open(ctx, tableNo) })
}

func (a *App) AddItemToComanda(comandaID, productID, qty int64) (models.ComandaDetails, error) {
	return callW(a, "adicionar item à comanda", func(ctx context.Context) (models.ComandaDetails, error) {
		return a.comandas.AddItem(ctx, comandaID, productID, qty)
	})
}

// SetComandaItemQty altera a quantidade; zero remove o item.
func (a *App) SetComandaItemQty(itemID, qty int64) (models.ComandaDetails, error) {
	return callW(a, "alterar item da comanda", func(ctx context.Context) (models.ComandaDetails, error) {
		return a.comandas.SetItemQty(ctx, itemID, qty)
	})
}

func (a *App) ListOpenComandas() ([]models.Comanda, error) {
	return call(a, "listar comandas", func(ctx context.Context) ([]models.Comanda, error) { return a.comandas.ListOpen(ctx) })
}

func (a *App) GetComandaDetails(comandaID int64) (models.ComandaDetails, error) {
	return call(a, "detalhes da comanda", func(ctx context.Context) (models.ComandaDetails, error) {
		return a.comandas.Details(ctx, comandaID)
	})
}

func (a *App) CloseComanda(in models.CloseComandaInput) (models.SaleDetails, error) {
	return callW(a, "fechar comanda", func(ctx context.Context) (models.SaleDetails, error) { return a.comandas.Close(ctx, in) })
}

func (a *App) DiscardComanda(comandaID int64) (models.ComandaDetails, error) {
	return callW(a, "descartar comanda", func(ctx context.Context) (models.ComandaDetails, error) {
		return a.comandas.Discard(ctx, comandaID)
	})
}

// ---------------- Caixa ----------------

func (a *App) OpenCash(initialAmountCents int64) (models.CashSummary, error) {
	return callW(a, "abrir caixa", func(ctx context.Context) (models.CashSummary, error) { return a.cash.Open(ctx, initialAmountCents) })
}

// RegisterCashMovement: kind = "suprimento" ou "sangria"; valor sempre positivo.
func (a *App) RegisterCashMovement(kind string, amountCents int64, description string) (models.CashMovement, error) {
	return callW(a, "movimentação de caixa", func(ctx context.Context) (models.CashMovement, error) {
		return a.cash.RegisterMovement(ctx, kind, amountCents, description)
	})
}

func (a *App) CloseCash(countedAmountCents int64, notes string) (models.CashSummary, error) {
	return call(a, "fechar caixa", func(ctx context.Context) (models.CashSummary, error) {
		return a.cash.Close(ctx, countedAmountCents, notes)
	})
}

// GetCurrentCash devolve null quando não há caixa aberto.
func (a *App) GetDashboard() (models.Dashboard, error) {
	return call(a, "painel", func(ctx context.Context) (models.Dashboard, error) { return a.reports.Dashboard(ctx) })
}

func (a *App) GetCurrentCash() (*models.CashSummary, error) {
	return call(a, "caixa atual", func(ctx context.Context) (*models.CashSummary, error) { return a.cash.Current(ctx) })
}

// GetCashReport: controle do caixa (entradas e saídas) do período.
// methods: formas a mostrar (vazio = todas).
func (a *App) GetCashReport(startDate, endDate string, userID int64, methods []string) (models.CashReport, error) {
	return call(a, "controle do caixa", func(ctx context.Context) (models.CashReport, error) {
		return a.reports.CashReport(ctx, startDate, endDate, userID, methods)
	})
}

// PrintCashReport imprime o controle do caixa na térmica.
func (a *App) PrintCashReport(startDate, endDate string, userID int64, methods []string) error {
	_, err := call(a, "imprimir controle do caixa", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, a.receipts.PrintCashReport(ctx, startDate, endDate, userID, methods)
	})
	return err
}

// ImportSubscriptions lê as planilhas de assinaturas (base64); a de mensalidades é opcional.
// apply=false só devolve a prévia; true grava.
func (a *App) ImportSubscriptions(clientsName, clientsData, invoicesName, invoicesData string, apply bool) (models.SubscriptionImport, error) {
	return callW(a, "importar assinaturas", func(ctx context.Context) (models.SubscriptionImport, error) {
		return a.subImport.Import(ctx, clientsName, clientsData, invoicesName, invoicesData, apply)
	})
}

// ---------------- Contas a pagar ----------------

func (a *App) BillCategories() []string { return service.BillCategories }

func (a *App) ListBills() ([]models.Bill, error) {
	return call(a, "listar contas a pagar", func(ctx context.Context) ([]models.Bill, error) { return a.bills.List(ctx) })
}

func (a *App) SaveBill(in models.BillInput) (models.Bill, error) {
	return callW(a, "salvar conta a pagar", func(ctx context.Context) (models.Bill, error) { return a.bills.Save(ctx, in) })
}

func (a *App) PayBill(in models.PayBillInput) (models.Bill, error) {
	return callW(a, "pagar conta", func(ctx context.Context) (models.Bill, error) { return a.bills.Pay(ctx, in) })
}

func (a *App) DeleteBill(id int64) error {
	_, err := callW(a, "excluir conta a pagar", func(ctx context.Context) (struct{}, error) { return struct{}{}, a.bills.Delete(ctx, id) })
	return err
}

// BillSummary: o que foi pago no mês (AAAA-MM) por categoria.
func (a *App) BillSummary(month string) ([]models.BillCategoryTotal, error) {
	return call(a, "resumo das contas pagas", func(ctx context.Context) ([]models.BillCategoryTotal, error) { return a.bills.Summary(ctx, month) })
}

// AddBillFile anexa o boleto ou o comprovante (conteúdo em base64).
func (a *App) AddBillFile(billID int64, kind, name, mime, data string) ([]models.BillFile, error) {
	return callW(a, "anexar arquivo", func(ctx context.Context) ([]models.BillFile, error) {
		return a.bills.AddFile(ctx, billID, kind, name, mime, data)
	})
}

func (a *App) ListBillFiles(billID int64) ([]models.BillFile, error) {
	return call(a, "listar arquivos", func(ctx context.Context) ([]models.BillFile, error) { return a.bills.Files(ctx, billID) })
}

func (a *App) DeleteBillFile(fileID int64) error {
	_, err := callW(a, "remover arquivo", func(ctx context.Context) (struct{}, error) { return struct{}{}, a.bills.DeleteFile(ctx, fileID) })
	return err
}

// OpenBillFile grava o arquivo numa pasta temporária e abre no programa padrão (leitor de PDF, fotos).
func (a *App) OpenBillFile(fileID int64) error {
	_, err := call(a, "abrir arquivo", func(ctx context.Context) (struct{}, error) {
		name, data, err := a.bills.FileData(ctx, fileID)
		if err != nil {
			return struct{}{}, err
		}
		dir := filepath.Join(os.TempDir(), "pdv-anexos")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return struct{}{}, err
		}
		path := filepath.Join(dir, fmt.Sprintf("%d-%s", fileID, filepath.Base(name)))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	})
	return err
}

// SaveBillFileAs salva uma cópia do arquivo onde o usuário escolher. false se cancelar.
func (a *App) SaveBillFileAs(fileID int64) (bool, error) {
	return call(a, "salvar arquivo", func(ctx context.Context) (bool, error) {
		name, data, err := a.bills.FileData(ctx, fileID)
		if err != nil {
			return false, err
		}
		dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Salvar arquivo", DefaultFilename: filepath.Base(name)})
		if err != nil || dest == "" {
			return false, err
		}
		return true, os.WriteFile(dest, data, 0o644)
	})
}

// ---------------- Assinaturas ----------------

func (a *App) ListSubscriptions() ([]models.Subscription, error) {
	return call(a, "listar assinaturas", func(ctx context.Context) ([]models.Subscription, error) { return a.subs.List(ctx) })
}

func (a *App) ListSubscriptionInvoices(subID int64) ([]models.SubscriptionInvoice, error) {
	return call(a, "listar faturas", func(ctx context.Context) ([]models.SubscriptionInvoice, error) { return a.subs.Invoices(ctx, subID) })
}

// CreateSubscription: priceCents > 0 = valor especial do aluno (0 = preço do plano).
func (a *App) CreateSubscription(customerID, productID, priceCents int64, rules *models.AccessRules) (models.Subscription, error) {
	return callW(a, "criar assinatura", func(ctx context.Context) (models.Subscription, error) {
		return a.subs.Create(ctx, customerID, productID, priceCents, rules)
	})
}

// EndSubscriptionExemption encerra a isenção; a mensalidade passa a vencer hoje.
func (a *App) EndSubscriptionExemption(subID int64) (models.Subscription, error) {
	return callW(a, "encerrar isenção", func(ctx context.Context) (models.Subscription, error) {
		return a.subs.EndExemption(ctx, subID)
	})
}

// PaySubscriptionInvoice recebe a fatura no caixa e devolve a venda gerada.
func (a *App) PaySubscriptionInvoice(in models.PayInvoiceInput) (models.SaleDetails, error) {
	return callW(a, "pagar fatura", func(ctx context.Context) (models.SaleDetails, error) { return a.subs.Pay(ctx, in) })
}

// ChangeSubscriptionPlan troca o plano; a mensalidade em aberto passa a vencer hoje com o valor novo.
func (a *App) ChangeSubscriptionPlan(subID, productID, priceCents int64) (models.Subscription, error) {
	return callW(a, "alterar assinatura", func(ctx context.Context) (models.Subscription, error) {
		return a.subs.ChangePlan(ctx, subID, productID, priceCents)
	})
}

// PauseSubscription congela a assinatura (os dias pausados não contam).
func (a *App) PauseSubscription(subID int64, reason string) (models.Subscription, error) {
	return callW(a, "pausar assinatura", func(ctx context.Context) (models.Subscription, error) { return a.subs.Pause(ctx, subID, reason) })
}

// ResumeSubscription retoma; o vencimento em aberto anda os dias pausados.
func (a *App) ResumeSubscription(subID int64) (models.Subscription, error) {
	return callW(a, "retomar assinatura", func(ctx context.Context) (models.Subscription, error) { return a.subs.Resume(ctx, subID) })
}

// SetSubscriptionRules troca a exceção de regras da assinatura (nil = segue o plano; sem mexer na mensalidade).
func (a *App) SetSubscriptionRules(subID int64, rules *models.AccessRules) (models.Subscription, error) {
	return callW(a, "alterar regras de acesso", func(ctx context.Context) (models.Subscription, error) { return a.subs.SetRules(ctx, subID, rules) })
}

// SetCustomerActive ativa ou inativa o cliente.
func (a *App) SetCustomerActive(id int64, active bool) (models.Customer, error) {
	return callW(a, "ativar cliente", func(ctx context.Context) (models.Customer, error) { return a.customers.SetActive(ctx, id, active) })
}

// DeleteCustomer exclui um cliente sem histórico (com histórico, recusa: inative).
func (a *App) DeleteCustomer(id int64) (bool, error) {
	return callW(a, "excluir cliente", func(ctx context.Context) (bool, error) { return true, a.gate.DeleteCustomer(ctx, id) })
}

// PurgeCustomerPreview mostra o que some ao apagar a pessoa com todo o histórico.
func (a *App) PurgeCustomerPreview(id int64) (models.CustomerPurge, error) {
	return call(a, "prévia de apagar pessoa", func(ctx context.Context) (models.CustomerPurge, error) { return a.gate.PurgeCustomerPreview(ctx, id) })
}

// PurgeCustomer apaga a pessoa e todo o histórico dela (vendas, mensalidades, fiado, passagens, rosto).
func (a *App) PurgeCustomer(id int64, expectSales int) (bool, error) {
	return callW(a, "apagar pessoa com histórico", func(ctx context.Context) (bool, error) { return true, a.gate.PurgeCustomer(ctx, id, expectSales) })
}

// PaySubscriptionOutside dá baixa na mensalidade sem lançar no caixa (pagamento retroativo).
// nextDue vazio = data do pagamento + duração do plano.
func (a *App) PaySubscriptionOutside(invoiceID int64, paidDate, nextDue string) (models.Subscription, error) {
	return callW(a, "receber mensalidade fora do caixa", func(ctx context.Context) (models.Subscription, error) {
		return a.subs.PayOutside(ctx, invoiceID, paidDate, nextDue)
	})
}

// UndoSubscriptionOutside desfaz um pagamento lançado fora do caixa.
func (a *App) UndoSubscriptionOutside(invoiceID int64) (models.Subscription, error) {
	return callW(a, "desfazer mensalidade fora do caixa", func(ctx context.Context) (models.Subscription, error) {
		return a.subs.UndoOutside(ctx, invoiceID)
	})
}

// ListPlanRules: regras de acesso de cada plano que tem alguma (por produto).
func (a *App) ListPlanRules() (map[int64]models.AccessRules, error) {
	return call(a, "ler regras dos planos", a.subs.PlanRules)
}

// SetPlanRules grava as regras de acesso do plano (valem para todos os assinantes).
func (a *App) SetPlanRules(productID int64, rules models.AccessRules) (models.AccessRules, error) {
	return callW(a, "gravar regras do plano", func(ctx context.Context) (models.AccessRules, error) {
		return a.subs.SetPlanRules(ctx, productID, rules)
	})
}

// ReactivateSubscription desfaz o cancelamento: a mesma assinatura volta, com a
// mensalidade em aberto de antes (quem já pagou continua liberado). Ativa o cliente.
func (a *App) ReactivateSubscription(subID int64) (models.Subscription, error) {
	return callW(a, "reativar assinatura", func(ctx context.Context) (models.Subscription, error) { return a.subs.Reactivate(ctx, subID) })
}

// DeleteSubscription exclui uma assinatura cancelada (o histórico de pagamentos fica guardado).
func (a *App) DeleteSubscription(subID int64) error {
	_, err := callW(a, "excluir assinatura", func(ctx context.Context) (struct{}, error) { return struct{}{}, a.subs.Delete(ctx, subID) })
	return err
}

// DeleteCancelledSubscriptions exclui todas as assinaturas canceladas do cliente; devolve quantas.
func (a *App) DeleteCancelledSubscriptions(customerID int64) (int, error) {
	return callW(a, "excluir assinaturas canceladas", func(ctx context.Context) (int, error) { return a.subs.DeleteCancelled(ctx, customerID) })
}

func (a *App) CancelSubscription(subID int64) (models.Subscription, error) {
	return callW(a, "cancelar assinatura", func(ctx context.Context) (models.Subscription, error) { return a.subs.Cancel(ctx, subID) })
}

// CloseOtherCash fecha o caixa aberto de outro usuário (permissão própria).
func (a *App) CloseOtherCash(sessionID, countedAmountCents int64, notes string) (models.CashSummary, error) {
	return call(a, "fechar caixa de outro usuário", func(ctx context.Context) (models.CashSummary, error) {
		return a.cash.CloseOther(ctx, sessionID, countedAmountCents, notes)
	})
}

func (a *App) ListOpenCash() ([]models.CashSession, error) {
	return call(a, "caixas abertos", func(ctx context.Context) ([]models.CashSession, error) { return a.cash.ListOpen(ctx) })
}

// GetCashDayTotals: o que entrou hoje em todos os caixas, por forma de pagamento.
func (a *App) GetCashDayTotals() ([]models.PaymentTotal, error) {
	return call(a, "total do dia nos caixas", func(ctx context.Context) ([]models.PaymentTotal, error) { return a.cash.DayTotals(ctx) })
}

func (a *App) ListCashSessions() ([]models.CashSession, error) {
	return call(a, "sessões de caixa", func(ctx context.Context) ([]models.CashSession, error) { return a.cash.ListSessions(ctx) })
}

func (a *App) GetCashSessionSummary(sessionID int64) (models.CashSummary, error) {
	return call(a, "resumo do caixa", func(ctx context.Context) (models.CashSummary, error) { return a.cash.Summary(ctx, sessionID) })
}

// ---------------- Trocas e devoluções ----------------

// GetReturnInfo traz os itens da venda com o que ainda pode ser devolvido.
func (a *App) GetReturnInfo(saleID int64) (models.ReturnInfo, error) {
	return call(a, "dados da devolução", func(ctx context.Context) (models.ReturnInfo, error) { return a.returns.Info(ctx, saleID) })
}

// ListReturns lista as devoluções entre duas datas locais (inclusive).
func (a *App) ListReturns(startDate, endDate string) ([]models.Return, error) {
	return call(a, "listar devoluções", func(ctx context.Context) ([]models.Return, error) { return a.returns.List(ctx, startDate, endDate) })
}

// CreateReturn devolve itens de uma venda (estoque volta; reembolso em vale-troca, dinheiro ou fiado).
func (a *App) CreateReturn(in models.ReturnInput) (models.Return, error) {
	return callW(a, "registrar devolução", func(ctx context.Context) (models.Return, error) { return a.returns.Create(ctx, in) })
}

// ---------------- Relatórios ----------------

func (a *App) GetDailyReport(date string) (models.SalesReport, error) {
	return call(a, "relatório diário", func(ctx context.Context) (models.SalesReport, error) { return a.reports.Daily(ctx, date) })
}

func (a *App) GetSalesReport(startDate, endDate string, userID int64) (models.SalesReport, error) {
	return call(a, "relatório de vendas", func(ctx context.Context) (models.SalesReport, error) {
		return a.reports.Sales(ctx, startDate, endDate, userID)
	})
}

// GenerateSalesReport monta o relatório e o guarda na lista de relatórios
// gerados. kind: geral, pagamento, categoria ou operador (models.Report*).
func (a *App) GenerateSalesReport(startDate, endDate string, userID int64, kind string) (models.SalesReport, error) {
	return call(a, "gerar relatório", func(ctx context.Context) (models.SalesReport, error) {
		return a.reports.Generate(ctx, startDate, endDate, userID, kind)
	})
}

// ExportSavedReport salva o relatório gerado como planilha do Excel (.xlsx)
// onde o usuário escolher. Devolve false se ele cancelar.
func (a *App) ExportSavedReport(id int64) (bool, error) {
	return call(a, "exportar relatório", func(ctx context.Context) (bool, error) {
		data, err := a.reports.SavedXLSX(ctx, id)
		if err != nil {
			return false, err
		}
		dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
			Title:           "Salvar relatório para o Excel",
			DefaultFilename: fmt.Sprintf("relatorio-%d.xlsx", id),
			Filters:         []runtime.FileFilter{{DisplayName: "Planilha do Excel (*.xlsx)", Pattern: "*.xlsx"}},
		})
		if err != nil || dest == "" {
			return false, err
		}
		if filepath.Ext(dest) == "" {
			dest += ".xlsx"
		}
		return true, os.WriteFile(dest, data, 0o644)
	})
}

func (a *App) ListSavedReports() ([]models.SavedReport, error) {
	return call(a, "relatórios gerados", func(ctx context.Context) ([]models.SavedReport, error) { return a.reports.ListSaved(ctx) })
}

// ---------------- Análises ----------------

func (a *App) GetProfit(startDate, endDate string) (models.ProfitReport, error) {
	return call(a, "lucro bruto", func(ctx context.Context) (models.ProfitReport, error) {
		return a.analytics.Profit(ctx, startDate, endDate)
	})
}

func (a *App) GetABC(startDate, endDate string) (models.ABCReport, error) {
	return call(a, "curva ABC", func(ctx context.Context) (models.ABCReport, error) { return a.analytics.ABC(ctx, startDate, endDate) })
}

func (a *App) GetIdleProducts(days int64) ([]models.IdleProduct, error) {
	return call(a, "produtos parados", func(ctx context.Context) ([]models.IdleProduct, error) { return a.analytics.Idle(ctx, days) })
}

func (a *App) GetFiadoAging() ([]models.AgingCustomer, error) {
	return call(a, "fiado em atraso", func(ctx context.Context) ([]models.AgingCustomer, error) { return a.analytics.Aging(ctx) })
}

func (a *App) GetPurchasesBySupplier(startDate, endDate string) ([]models.SupplierPurchases, error) {
	return call(a, "compras por fornecedor", func(ctx context.Context) ([]models.SupplierPurchases, error) {
		return a.analytics.Suppliers(ctx, startDate, endDate)
	})
}

func (a *App) GetStockValue() ([]models.StockValueRow, error) {
	return call(a, "estoque valorizado", func(ctx context.Context) ([]models.StockValueRow, error) { return a.analytics.StockValue(ctx) })
}

func (a *App) GetDiscountsByOperator(startDate, endDate string) ([]models.DiscountByOperator, error) {
	return call(a, "descontos por operador", func(ctx context.Context) ([]models.DiscountByOperator, error) {
		return a.analytics.Discounts(ctx, startDate, endDate)
	})
}

// ExportTable salva uma listagem da tela como planilha do Excel (.xlsx) onde o
// usuário escolher. Devolve false se ele cancelar.
func (a *App) ExportTable(in models.ExportTable) (bool, error) {
	return call(a, "exportar planilha", func(ctx context.Context) (bool, error) {
		data, err := a.analytics.ExportXLSX(in)
		if err != nil {
			return false, err
		}
		dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
			Title:           "Salvar planilha do Excel",
			DefaultFilename: exportFileName(in.Title),
			Filters:         []runtime.FileFilter{{DisplayName: "Planilha do Excel (*.xlsx)", Pattern: "*.xlsx"}},
		})
		if err != nil || dest == "" {
			return false, err
		}
		if filepath.Ext(dest) == "" {
			dest += ".xlsx"
		}
		return true, os.WriteFile(dest, data, 0o644)
	})
}

// SavePDF pergunta onde salvar e grava o PDF da folha A4 (html: página completa
// montada pela tela, com o CSS embutido). Devolve false se o usuário cancelar.
func (a *App) SavePDF(html, title string) (bool, error) {
	return call(a, "salvar PDF", func(ctx context.Context) (bool, error) {
		dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
			Title:           "Salvar em PDF",
			DefaultFilename: strings.TrimSuffix(exportFileName(title), ".xlsx") + ".pdf",
			Filters:         []runtime.FileFilter{{DisplayName: "Documento PDF (*.pdf)", Pattern: "*.pdf"}},
		})
		if err != nil || dest == "" {
			return false, err
		}
		if filepath.Ext(dest) == "" {
			dest += ".pdf"
		}
		if err := pdf.FromHTML(html, dest); err != nil {
			return false, &service.Error{Code: service.CodeInvalid, Message: "Não foi possível salvar o PDF: " + err.Error(), Err: err}
		}
		return true, nil
	})
}

// exportFileName: "Lucro bruto" -> "lucro-bruto-2026-10-18.xlsx" (sem acento nem símbolo).
func exportFileName(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case strings.ContainsRune("áàâãä", r):
			b.WriteRune('a')
		case strings.ContainsRune("éèêë", r):
			b.WriteRune('e')
		case strings.ContainsRune("íìîï", r):
			b.WriteRune('i')
		case strings.ContainsRune("óòôõö", r):
			b.WriteRune('o')
		case strings.ContainsRune("úùûü", r):
			b.WriteRune('u')
		case r == 'ç':
			b.WriteRune('c')
		default:
			if s := b.String(); s != "" && !strings.HasSuffix(s, "-") {
				b.WriteRune('-')
			}
		}
	}
	return strings.Trim(b.String(), "-") + "-" + time.Now().Format("2006-01-02") + ".xlsx"
}

// ---------------- Configurações e manutenção ----------------

func (a *App) GetSettings() (models.Settings, error) {
	return callAnon(a, "ler configurações", func(ctx context.Context) (models.Settings, error) { return a.settings.Get(ctx) })
}

func (a *App) SaveSettings(in models.Settings) (models.Settings, error) {
	return call(a, "salvar configurações", func(ctx context.Context) (models.Settings, error) { return a.settings.Save(ctx, in) })
}

// GetBackgrounds funciona sem login (a tela de entrada usa o fundo).
func (a *App) GetBackgrounds() (models.Backgrounds, error) {
	return callAnon(a, "ler imagens de fundo", func(ctx context.Context) (models.Backgrounds, error) { return a.settings.Backgrounds(ctx) })
}

func (a *App) SaveBackgrounds(in models.Backgrounds) (models.Backgrounds, error) {
	return call(a, "salvar imagens de fundo", func(ctx context.Context) (models.Backgrounds, error) { return a.settings.SaveBackgrounds(ctx, in) })
}

// GetAppInfo funciona mesmo se o banco não abriu (para diagnóstico).
// ---------------- Comprovantes e impressora ----------------

// GetReceipt monta um comprovante não fiscal (kind: venda, recebimento, caixa,
// movimento ou teste) para exibir na tela ou imprimir em A4.
func (a *App) GetReceipt(kind string, id int64) (models.Receipt, error) {
	return call(a, "montar comprovante", func(ctx context.Context) (models.Receipt, error) { return a.receipts.Build(ctx, kind, id) })
}

// PrintReceipt imprime direto na impressora térmica configurada.
func (a *App) PrintReceipt(kind string, id int64) error {
	_, err := call(a, "imprimir comprovante", func(ctx context.Context) (struct{}, error) {
		err := a.receipts.Print(ctx, kind, id)
		if err != nil {
			a.log.Warn("impressão falhou", "tipo", kind, "id", id, "erro", err)
		}
		return struct{}{}, err
	})
	return err
}

func (a *App) ListPrinters() ([]string, error) {
	list, err := printer.List()
	if err != nil {
		return nil, a.fail("listar impressoras", err)
	}
	return list, nil
}

// GetPixQR gera o QR Code Pix do valor (txid: identificação da cobrança).
func (a *App) GetPixQR(amountCents int64, txid string) (models.PixCode, error) {
	return call(a, "gerar QR Pix", func(ctx context.Context) (models.PixCode, error) { return a.settings.PixQR(ctx, amountCents, txid) })
}

// PrintPixQR imprime o QR Code Pix do valor na impressora térmica.
func (a *App) PrintPixQR(amountCents int64, txid string) error {
	_, err := call(a, "imprimir QR Pix", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, a.receipts.PrintPix(ctx, amountCents, txid)
	})
	return err
}

// LogClientError grava no log um erro da tela (JavaScript/Vue). Sem login:
// erro na tela de entrada também interessa.
func (a *App) LogClientError(message, detail string) {
	if a.clientErrors.Add(1) > 200 { // uma tela em laço não enche o disco
		return
	}
	user := ""
	if u := a.currentUser(); u != nil {
		user = u.Login
	}
	a.log.Error("erro na tela", "erro", truncate(message, 1000), "detalhe", truncate(detail, 4000), "usuario", user)
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// OpenLogsFolder abre no Explorador a pasta dos registros de erro, para o
// cliente enviar o pdv.log ao suporte.
func (a *App) OpenLogsFolder() (bool, error) {
	return callAnon(a, "abrir pasta de registros", func(ctx context.Context) (bool, error) {
		dir := filepath.Join(a.dataDir, "logs")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return false, err
		}
		return true, exec.Command("explorer", dir).Start()
	})
}

func (a *App) GetAppInfo() models.AppInfo {
	info := models.AppInfo{Version: AppVersion, DataDir: a.dataDir, DatabasePath: a.dbPath}
	if a.db != nil {
		info.SchemaVersion, _ = db.UserVersion(context.Background(), a.db)
	}
	return info
}

// GetStartupStatus informa erro de inicialização e o resultado de uma
// restauração aplicada na abertura.
func (a *App) GetStartupStatus() map[string]string {
	out := map[string]string{"notice": a.notice}
	if a.startupErr != nil {
		out["error"] = "O banco de dados não pôde ser aberto. Verifique se outro programa está usando o arquivo, se há espaço em disco e se a pasta de dados tem permissão de gravação. Se o problema persistir, restaure um backup."
	}
	return out
}

// PreviewTestPurge mostra as vendas e o caixa do dia (AAAA-MM-DD) do operador (0 = todos) que seriam apagados.
func (a *App) PreviewTestPurge(date string, userID int64) (models.TestPurge, error) {
	return call(a, "ver vendas de teste", func(ctx context.Context) (models.TestPurge, error) { return a.purge.Preview(ctx, date, userID) })
}

// ApplyTestPurge faz um backup na pasta de dados e apaga as vendas e o caixa do
// dia do operador (0 = todos). expectSales/expectSessions: o que a prévia mostrou.
func (a *App) ApplyTestPurge(date string, userID int64, expectSales, expectSessions int) (models.TestPurge, error) {
	return callW(a, "apagar vendas de teste", func(ctx context.Context) (models.TestPurge, error) {
		if _, err := a.purge.Preview(ctx, date, userID); err != nil { // só o administrador, antes do backup
			return models.TestPurge{}, err
		}
		bk, err := a.backup.CreateInDataDir(ctx)
		if err != nil {
			return models.TestPurge{}, fmt.Errorf("backup antes de apagar: %w", err)
		}
		res, err := a.purge.Apply(ctx, date, userID, expectSales, expectSessions)
		res.BackupPath = bk.Path
		return res, err
	})
}

// CreateBackup pede o destino pelo diálogo nativo. Devolve null se o usuário
// cancelar.
func (a *App) CreateBackup() (*models.BackupResult, error) {
	return call(a, "criar backup", func(ctx context.Context) (*models.BackupResult, error) {
		dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
			Title:           "Salvar backup do PDV",
			DefaultFilename: "pdv-backup-" + time.Now().Format("2006-01-02-150405") + ".db",
			Filters:         []runtime.FileFilter{{DisplayName: "Backup do PDV (*.db)", Pattern: "*.db"}},
		})
		if err != nil {
			return nil, err
		}
		if dest == "" {
			return nil, nil
		}
		if filepath.Ext(dest) == "" {
			dest += ".db"
		}
		res, err := a.backup.Create(ctx, dest)
		if err != nil {
			return nil, err
		}
		a.log.Info("backup criado", "tamanho", res.SizeBytes)
		return &res, nil
	})
}

// RestoreBackup pede o arquivo pelo diálogo nativo, valida e agenda a
// restauração para a próxima abertura. Devolve false se o usuário cancelar.
func (a *App) RestoreBackup() (bool, error) {
	return call(a, "restaurar backup", func(ctx context.Context) (bool, error) {
		src, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
			Title:   "Escolher backup para restaurar",
			Filters: []runtime.FileFilter{{DisplayName: "Backup do PDV (*.db)", Pattern: "*.db"}},
		})
		if err != nil {
			return false, err
		}
		if src == "" {
			return false, nil
		}
		if err := a.backup.RequestRestore(ctx, src); err != nil {
			return false, err
		}
		a.log.Info("restauração agendada")
		return true, nil
	})
}

// GetLicenseStatus: situação da licença (tela Licença, barra superior, Painel).
func (a *App) GetLicenseStatus() (models.LicenseStatus, error) {
	return call(a, "ler licença", a.license.Status)
}

// ActivateLicense valida e grava a licença colada (ou lida de arquivo).
func (a *App) ActivateLicense(text string) (models.LicenseStatus, error) {
	return call(a, "ativar licença", func(ctx context.Context) (models.LicenseStatus, error) {
		return a.license.Activate(ctx, text)
	})
}

// ChargeCustomer abre o WhatsApp com a mensagem de cobrança do a prazo e
// registra a data. Fica liberado no modo consulta: cobrar não grava movimento.
func (a *App) ChargeCustomer(id int64) error {
	_, err := call(a, "cobrar pelo WhatsApp", func(ctx context.Context) (struct{}, error) {
		link, err := a.customers.ChargeLink(ctx, id)
		if err == nil {
			runtime.BrowserOpenURL(a.ctx, link)
		}
		return struct{}{}, err
	})
	return err
}

// AssignInternalBarcodes gera EAN-13 interno (prefixo 2) para os produtos sem código.
func (a *App) AssignInternalBarcodes(ids []int64) ([]models.Product, error) {
	return callW(a, "gerar código interno", func(ctx context.Context) ([]models.Product, error) {
		return a.products.AssignInternalBarcodes(ctx, ids)
	})
}

// PickImportFile abre o diálogo para escolher a planilha (CSV ou XLSX) e lê o
// cabeçalho. Devolve null se o usuário cancelar.
func (a *App) PickImportFile(kind string) (*models.ImportFile, error) {
	return call(a, "abrir planilha", func(ctx context.Context) (*models.ImportFile, error) {
		path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
			Title:   "Escolher planilha",
			Filters: []runtime.FileFilter{{DisplayName: "Planilha (*.xlsx, *.csv)", Pattern: "*.xlsx;*.csv;*.txt"}},
		})
		if err != nil || path == "" {
			return nil, err
		}
		f, err := a.imports.Open(ctx, kind, path)
		return &f, err
	})
}

// PreviewImport valida a planilha linha a linha, sem gravar.
func (a *App) PreviewImport(req models.ImportRequest) (models.ImportPreview, error) {
	return call(a, "conferir planilha", func(ctx context.Context) (models.ImportPreview, error) {
		return a.imports.Preview(ctx, req)
	})
}

// ConfirmImport grava a planilha inteira numa transação (com erro, nada é gravado).
func (a *App) ConfirmImport(req models.ImportRequest) (models.ImportPreview, error) {
	return callW(a, "importar planilha", func(ctx context.Context) (models.ImportPreview, error) {
		return a.imports.Confirm(ctx, req)
	})
}

// templateCodes: linhas (códigos reservados) da planilha modelo de produtos.
const templateCodes = 100

// SaveImportTemplate salva o modelo .xlsx onde o usuário escolher. Devolve o caminho ("" se cancelar).
func (a *App) SaveImportTemplate(kind string) (string, error) {
	return call(a, "salvar modelo de planilha", func(ctx context.Context) (string, error) {
		dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
			Title: "Salvar modelo de planilha", DefaultFilename: "modelo-importacao-" + kind + ".xlsx",
			Filters: []runtime.FileFilter{{DisplayName: "Planilha (*.xlsx)", Pattern: "*.xlsx"}},
		})
		if err != nil || dest == "" {
			return "", err
		}
		if filepath.Ext(dest) == "" {
			dest += ".xlsx"
		}
		// Os códigos só são reservados depois de escolher onde salvar.
		var codes []models.ProductCode
		if sub, ok := strings.CutPrefix(kind, "assinaturas_"); ok {
			data, err := service.SubscriptionTemplate(sub)
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				return "", &service.Error{Code: service.CodeStorage, Message: "Não foi possível salvar o modelo nessa pasta."}
			}
			return dest, nil
		}
		if kind == "produtos" {
			if codes, err = a.products.ReserveCodes(ctx, templateCodes); err != nil {
				return "", err
			}
		}
		data, err := service.ImportTemplate(kind, codes)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return "", &service.Error{Code: service.CodeStorage, Message: "Não foi possível salvar o modelo nessa pasta."}
		}
		return dest, nil
	})
}

// GetBackupStatus: estado do backup automático e da cópia externa (aviso do Painel).
func (a *App) GetBackupStatus() (models.BackupStatus, error) {
	return call(a, "ler estado do backup", a.backup.Status)
}

// RunBackupNow faz agora o backup do dia e a cópia externa ("Copiar agora").
func (a *App) RunBackupNow() (models.BackupStatus, error) {
	return call(a, "backup agora", func(ctx context.Context) (models.BackupStatus, error) {
		return a.runAutoBackup(true)
	})
}

// ChooseBackupFolder abre o diálogo de pasta. Devolve "" se o usuário cancelar.
func (a *App) ChooseBackupFolder() (string, error) {
	return call(a, "escolher pasta do backup", func(ctx context.Context) (string, error) {
		return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Escolher pasta do backup"})
	})
}

func (a *App) HasPendingRestore() bool { return a.backup != nil && a.backup.HasPendingRestore() }

func (a *App) CancelPendingRestore() error {
	_, err := call(a, "cancelar restauração", func(ctx context.Context) (bool, error) {
		return true, a.backup.CancelPendingRestore()
	})
	return err
}

// QuitApp encerra o programa (usado após agendar uma restauração).
func (a *App) QuitApp() { runtime.Quit(a.ctx) }
