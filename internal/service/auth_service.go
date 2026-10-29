// Caminho: pdv/internal/service/auth_service.go

package service

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
	"pdv/internal/repository"
)

// Permissões verificadas nos serviços.
const (
	PermCancelSale        = "cancelar_venda"
	PermReturns           = "trocas_devolucoes"
	PermDiscount          = "desconto_acima_limite"
	PermWithdrawal        = "sangria"
	PermPauseSubscription = "pausar_assinatura"
	PermBills             = "contas_a_pagar"
	PermCloseOtherCash    = "fechar_caixa_outro"
	PermStockAdjust       = "ajuste_estoque"
	PermStockEntry        = "entrada_estoque"
	PermStockReturn       = "devolucao_estoque"
	PermProductCreate     = "cadastrar_produto"
	PermPrice             = "alterar_preco"
	PermReverseCredit     = "estornar_fiado"
	PermCreditLimit       = "alterar_limite_fiado"
	PermSettings          = "alterar_configuracoes"
	PermRestore           = "restaurar_backup"
	PermCost              = "ver_custo"
	PermProductActive     = "ativar_produto"
	PermPurchases         = "compras"
	PermAllSales          = "ver_vendas_de_todos"
	PermUsers             = "gerenciar_usuarios"
	PermAudit             = "ver_auditoria"
)

// configurablePerms é a lista (na ordem da tela) que o administrador libera
// por papel. Usuários e auditoria ficam fora: são sempre só do administrador,
// para ninguém conseguir tirar o próprio acesso de administrador.
var configurablePerms = []models.PermissionItem{
	{Key: PermCancelSale, Label: "cancelar venda"},
	{Key: PermReturns, Label: "fazer trocas e devoluções de vendas"},
	{Key: PermDiscount, Label: "dar desconto acima do limite"},
	{Key: PermWithdrawal, Label: "fazer sangria"},
	{Key: PermBills, Label: "contas a pagar: cadastrar, pagar e ver comprovantes"},
	{Key: PermPauseSubscription, Label: "pausar e retomar assinatura (os dias pausados não contam)"},
	{Key: PermGate, Label: "catraca: configurar, bloquear aluno, dar cortesia e mexer no cadastro do leitor facial"},
	{Key: PermCloseOtherCash, Label: "fechar o caixa de outro usuário (modo caixa por usuário)"},
	{Key: PermStockEntry, Label: "dar entrada no estoque"},
	{Key: PermStockReturn, Label: "registrar devolução ao estoque"},
	{Key: PermStockAdjust, Label: "ajustar estoque"},
	{Key: PermProductCreate, Label: "cadastrar produto"},
	{Key: PermProductActive, Label: "ativar e desativar produto"},
	{Key: PermCost, Label: "ver e alterar custo e margem"},
	{Key: PermPurchases, Label: "cadastrar fornecedores, registrar compras e importar XML"},
	{Key: PermAllSales, Label: "ver vendas e relatórios de todos os usuários (sem ela, só as próprias)"},
	{Key: PermPrice, Label: "alterar preço de produto"},
	{Key: PermReverseCredit, Label: "estornar recebimento a prazo"},
	{Key: PermCreditLimit, Label: "definir o limite de compra a prazo do cliente"},
	{Key: PermSettings, Label: "alterar configurações"},
	{Key: PermRestore, Label: "restaurar backup"},
}

func permLabel(perm string) string {
	switch perm {
	case PermUsers:
		return "gerenciar usuários e permissões"
	case PermAudit:
		return "consultar a auditoria"
	}
	for _, p := range configurablePerms {
		if p.Key == perm {
			return p.Label
		}
	}
	return perm
}

func isConfigurable(perm string) bool {
	for _, p := range configurablePerms {
		if p.Key == perm {
			return true
		}
	}
	return false
}

// DefaultPerms são as permissões que um usuário novo recebe pelo papel: o
// gerente pode tudo e o operador só cadastra produto e dá entrada/devolução.
func DefaultPerms(role string) []string {
	switch role {
	case models.RoleManager:
		// O gerente pode tudo, menos ver as vendas dos outros (só o administrador, salvo se marcado).
		out := make([]string, 0, len(configurablePerms))
		for _, p := range configurablePerms {
			// Ver as vendas de todos e pausar assinatura: só se o administrador marcar.
			if p.Key != PermAllSales && p.Key != PermPauseSubscription {
				out = append(out, p.Key)
			}
		}
		return out
	case models.RoleOperator:
		return []string{PermProductCreate, PermStockEntry, PermStockReturn}
	}
	return []string{}
}

// EffectivePerms: o administrador tem todas; os demais têm as gravadas no
// cadastro ou, se nunca foram escolhidas (nil), as padrão do papel.
func EffectivePerms(u models.User) []string {
	if u.Role == models.RoleAdmin {
		out := []string{PermUsers, PermAudit}
		for _, p := range configurablePerms {
			out = append(out, p.Key)
		}
		return out
	}
	src := u.Permissions
	if src == nil {
		src = DefaultPerms(u.Role)
	}
	out := []string{}
	for _, k := range src {
		if isConfigurable(k) && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// requirePerm confere se o usuário do context pode fazer a operação. Se não
// puder, consome uma autorização de gerente dada para ela (actor.Grants).
// Devolve o id de quem autorizou (nil se o próprio usuário tinha permissão).
func requirePerm(ctx context.Context, now time.Time, perm string) (*int64, error) {
	u, ok := actor.From(ctx)
	if !ok {
		return nil, newErr(CodeLoginRequired, "Entre com seu usuário para continuar.")
	}
	if u.Role == models.RoleAdmin || slices.Contains(u.Perms, perm) {
		return nil, nil
	}
	if !isConfigurable(perm) {
		return nil, newErr(CodeForbidden, "Somente o administrador pode %s.", permLabel(perm))
	}
	if g := actor.GrantsFrom(ctx); g != nil {
		if by, ok := g.Take(perm, now); ok {
			return &by.ID, nil
		}
	}
	return nil, &Error{Code: CodeAuthRequired, Permission: perm,
		Message: fmt.Sprintf("Para %s é preciso a autorização de um gerente.", permLabel(perm))}
}

// audit grava uma linha na auditoria; before/after viram JSON (nil = vazio).
func audit(ctx context.Context, q repository.DBTX, now time.Time, authorizedBy *int64, action, entity string, entityID *int64, before, after any) error {
	enc := func(v any) (*string, error) {
		if v == nil {
			return nil, nil
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		s := string(b)
		return &s, nil
	}
	b, err := enc(before)
	if err != nil {
		return err
	}
	a, err := enc(after)
	if err != nil {
		return err
	}
	return repository.AuditRepo{}.Insert(ctx, q, authorizedBy, action, entity, entityID, b, a, now)
}

// ---------- senhas ----------

// pbkdf2Iterations segue a recomendação OWASP para PBKDF2-SHA256. Os testes
// reduzem o valor; o número fica gravado no hash, então mudar não invalida senhas.
var pbkdf2Iterations = 600_000

const minPasswordLen = 4

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func checkPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err != nil || err1 != nil || err2 != nil || iter < 1 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

// ---------- serviço ----------

const (
	maxLoginFailures = 5
	loginLockout     = time.Minute
)

type AuthService struct {
	Base
	users repository.UserRepo

	mu       sync.Mutex
	failures map[string]loginFailure
	// Instance identifica este programa aberto: é o que fica em users.session_id.
	Instance string
}

type loginFailure struct {
	count int
	until time.Time
}

func NewAuthService(b Base) *AuthService {
	return &AuthService{Base: b, failures: map[string]loginFailure{}, Instance: rand.Text()}
}

// Session descreve o estado de login para a tela.
func (s *AuthService) Session(ctx context.Context, current *models.User) (models.Session, error) {
	n, err := s.users.Count(ctx, s.DB)
	if err != nil {
		return models.Session{}, err
	}
	out := models.Session{User: current, NeedsSetup: n == 0, Permissions: []string{}}
	if current != nil {
		out.Permissions = EffectivePerms(*current)
	}
	return out, nil
}

type userFields struct {
	name, login, role string
	active            bool
	perms             []string // nil = padrão do papel
}

func validateUser(in models.UserInput) (userFields, error) {
	var f userFields
	var err error
	if f.name, err = cleanName(in.Name, "nome do usuário"); err != nil {
		return f, err
	}
	f.login = strings.ToLower(strings.TrimSpace(in.Login))
	if f.login == "" || len(f.login) > 50 || strings.ContainsAny(f.login, " \t") {
		return f, invalid("Informe um login sem espaços, com até 50 caracteres.")
	}
	switch in.Role {
	case models.RoleAdmin, models.RoleManager, models.RoleOperator:
	default:
		return f, invalid("Papel inválido: use administrador, gerente ou operador.")
	}
	f.role, f.active = in.Role, in.Active
	if in.Permissions != nil && in.Role != models.RoleAdmin {
		f.perms = []string{}
		for _, k := range in.Permissions {
			if !isConfigurable(k) {
				return f, invalid("Permissão desconhecida: %s.", k)
			}
			if !slices.Contains(f.perms, k) {
				f.perms = append(f.perms, k)
			}
		}
	}
	return f, nil
}

// User busca o cadastro com as permissões efetivas preenchidas.
func (s *AuthService) User(ctx context.Context, id int64) (models.User, error) {
	u, err := s.users.GetByID(ctx, s.DB, id)
	u.Permissions = EffectivePerms(u)
	return u, err
}

// Catalog lista as permissões e o padrão de cada papel para a tela de usuários.
func (s *AuthService) Catalog(ctx context.Context) (models.PermissionCatalog, error) {
	if _, err := requirePerm(ctx, s.Now(), PermUsers); err != nil {
		return models.PermissionCatalog{}, err
	}
	return models.PermissionCatalog{Items: configurablePerms, Defaults: map[string][]string{
		models.RoleManager: DefaultPerms(models.RoleManager), models.RoleOperator: DefaultPerms(models.RoleOperator),
	}}, nil
}

func validatePassword(pw string) error {
	if len([]rune(pw)) < minPasswordLen || len(pw) > 200 {
		return invalid("A senha deve ter pelo menos %d caracteres.", minPasswordLen)
	}
	return nil
}

// SetupAdmin cria o primeiro administrador. Só funciona com o banco sem usuários.
func (s *AuthService) SetupAdmin(ctx context.Context, in models.UserInput) (models.User, error) {
	in.Role, in.Active = models.RoleAdmin, true
	f, err := validateUser(in)
	if err != nil {
		return models.User{}, err
	}
	if err := validatePassword(in.Password); err != nil {
		return models.User{}, err
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return models.User{}, err
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		n, err := s.users.Count(ctx, tx)
		if err != nil {
			return err
		}
		if n > 0 {
			return newErr(CodeStateConflict, "O administrador já foi cadastrado. Entre com seu usuário.")
		}
		now := s.Now()
		if id, err = s.users.Insert(ctx, tx, f.name, f.login, hash, f.role, true, nil, now); err != nil {
			return err
		}
		ctx := actor.With(ctx, actor.User{ID: id, Name: f.name, Role: f.role}, nil)
		if err := s.users.SetSession(ctx, tx, id, s.Instance); err != nil {
			return err
		}
		return audit(ctx, tx, now, nil, "primeiro_acesso", "usuario", &id, nil, map[string]any{"login": f.login})
	})
	if err != nil {
		return models.User{}, err
	}
	return s.User(ctx, id)
}

// verify confere login e senha, com bloqueio temporário após várias falhas.
func (s *AuthService) verify(ctx context.Context, login, password string) (models.User, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	now := s.Now()
	s.mu.Lock()
	f := s.failures[login]
	s.mu.Unlock()
	if now.Before(f.until) {
		return models.User{}, invalid("Muitas tentativas erradas. Aguarde %d segundos e tente de novo.", int(f.until.Sub(now).Seconds())+1)
	}
	u, hash, err := s.users.GetByLogin(ctx, s.DB, login)
	ok := err == nil && checkPassword(hash, password) && u.Active
	if err != nil && !isNotFound(err) {
		return models.User{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ok {
		f.count++
		if f.count >= maxLoginFailures {
			f = loginFailure{until: now.Add(loginLockout)}
		}
		s.failures[login] = f
		return models.User{}, invalid("Usuário ou senha incorretos (ou usuário desativado).")
	}
	delete(s.failures, login)
	return u, nil
}

// Login entra com o usuário. Se ele já estiver logado em outro lugar (ou o
// programa fechou sem sair), recusa com CodeAlreadyLoggedIn; force (depois de
// o usuário confirmar) encerra a outra sessão.
func (s *AuthService) Login(ctx context.Context, login, password string, force bool) (models.User, error) {
	u, err := s.verify(ctx, login, password)
	now := s.Now()
	if err != nil {
		if e, ok := err.(*Error); ok && e.Code == CodeInvalid {
			_ = audit(ctx, s.DB, now, nil, "login_falhou", "", nil, nil, map[string]any{"login": strings.TrimSpace(login)})
		}
		return models.User{}, err
	}
	if u.LoggedIn && u.SessionID != s.Instance && !force {
		return models.User{}, newErr(CodeAlreadyLoggedIn, "%s já está logado em outro lugar (ou o sistema foi fechado sem sair).", u.Name)
	}
	if err := s.users.SetSession(ctx, s.DB, u.ID, s.Instance); err != nil {
		return models.User{}, err
	}
	u.SessionID, u.LoggedIn = s.Instance, true
	ctx = actor.With(ctx, actor.User{ID: u.ID, Name: u.Name, Role: u.Role}, nil)
	return u, audit(ctx, s.DB, now, nil, "login", "usuario", &u.ID, nil, nil)
}

func (s *AuthService) Logout(ctx context.Context) error {
	if u, ok := actor.From(ctx); ok {
		if err := s.EndSession(ctx, u.ID); err != nil {
			return err
		}
		return audit(ctx, s.DB, s.Now(), nil, "logout", "usuario", &u.ID, nil, nil)
	}
	return nil
}

// EndSession libera o login do usuário, se a sessão for desta instância.
func (s *AuthService) EndSession(ctx context.Context, userID int64) error {
	return s.users.ClearSession(ctx, s.DB, userID, s.Instance)
}

// ReleaseSession: o administrador libera o login preso de um usuário (o
// programa fechou sem sair). Uma sessão ainda aberta é encerrada na próxima ação.
func (s *AuthService) ReleaseSession(ctx context.Context, userID int64) error {
	if _, err := requirePerm(ctx, s.Now(), PermUsers); err != nil {
		return err
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.users.GetByID(ctx, tx, userID); err != nil {
			return fromRepo(err, notFoundErr(CodeNotFound, "Usuário", userID), "")
		}
		if err := s.users.ClearSession(ctx, tx, userID, ""); err != nil {
			return err
		}
		return audit(ctx, tx, s.Now(), nil, "liberar_login", "usuario", &userID, nil, nil)
	})
}

// Authorize: um gerente digita login e senha para liberar UMA operação de perm
// para o operador logado.
func (s *AuthService) Authorize(ctx context.Context, perm, login, password string) (models.User, error) {
	if !isConfigurable(perm) {
		return models.User{}, invalid("Permissão desconhecida.")
	}
	if _, ok := actor.From(ctx); !ok {
		return models.User{}, newErr(CodeLoginRequired, "Entre com seu usuário para continuar.")
	}
	g := actor.GrantsFrom(ctx)
	if g == nil {
		return models.User{}, newErr(CodeInternal, "Autorização indisponível.")
	}
	sup, err := s.verify(ctx, login, password)
	if err != nil {
		return models.User{}, err
	}
	if !slices.Contains(EffectivePerms(sup), perm) {
		return models.User{}, newErr(CodeForbidden, "%s não tem permissão para %s.", sup.Name, permLabel(perm))
	}
	now := s.Now()
	g.Add(perm, actor.User{ID: sup.ID, Name: sup.Name, Role: sup.Role}, now)
	return sup, audit(ctx, s.DB, now, &sup.ID, "autorizacao", "", nil, nil, map[string]any{"permissao": perm})
}

// ---------- cadastro de usuários (somente administrador) ----------

func (s *AuthService) ListUsers(ctx context.Context) ([]models.User, error) {
	if _, err := requirePerm(ctx, s.Now(), PermUsers); err != nil {
		return nil, err
	}
	list, err := s.users.List(ctx, s.DB)
	for i := range list {
		list[i].Permissions = EffectivePerms(list[i])
	}
	return list, err
}

const duplicateLoginMsg = "Já existe um usuário com este login."

func (s *AuthService) CreateUser(ctx context.Context, in models.UserInput) (models.User, error) {
	if _, err := requirePerm(ctx, s.Now(), PermUsers); err != nil {
		return models.User{}, err
	}
	f, err := validateUser(in)
	if err != nil {
		return models.User{}, err
	}
	if err := validatePassword(in.Password); err != nil {
		return models.User{}, err
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return models.User{}, err
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if id, err = s.users.Insert(ctx, tx, f.name, f.login, hash, f.role, f.active, f.perms, now); err != nil {
			return fromRepo(err, nil, duplicateLoginMsg)
		}
		created := models.User{Role: f.role, Permissions: f.perms}
		return audit(ctx, tx, now, nil, "usuario_criado", "usuario", &id, nil,
			map[string]any{"nome": f.name, "login": f.login, "papel": f.role, "ativo": f.active,
				"permissoes": strings.Join(EffectivePerms(created), ", ")})
	})
	if err != nil {
		return models.User{}, err
	}
	return s.User(ctx, id)
}

// UpdateUser altera o cadastro e, se Password vier preenchido, redefine a senha.
// Nunca deixa o sistema sem um administrador ativo.
func (s *AuthService) UpdateUser(ctx context.Context, in models.UserInput) (models.User, error) {
	if _, err := requirePerm(ctx, s.Now(), PermUsers); err != nil {
		return models.User{}, err
	}
	f, err := validateUser(in)
	if err != nil {
		return models.User{}, err
	}
	var hash string
	if in.Password != "" {
		if err := validatePassword(in.Password); err != nil {
			return models.User{}, err
		}
		if hash, err = hashPassword(in.Password); err != nil {
			return models.User{}, err
		}
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		before, err := s.users.GetByID(ctx, tx, in.ID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Usuário %d não encontrado.", in.ID), "")
		}
		if before.Role == models.RoleAdmin && before.Active && (f.role != models.RoleAdmin || !f.active) {
			n, err := s.users.CountActiveAdmins(ctx, tx, in.ID)
			if err != nil {
				return err
			}
			if n == 0 {
				return newErr(CodeStateConflict, "Este é o único administrador ativo. Cadastre outro administrador antes de alterá-lo.")
			}
		}
		now := s.Now()
		if err := s.users.Update(ctx, tx, in.ID, f.name, f.login, f.role, f.active, f.perms, now); err != nil {
			return fromRepo(err, nil, duplicateLoginMsg)
		}
		after := map[string]any{"nome": f.name, "login": f.login, "papel": f.role, "ativo": f.active,
			"permissoes": strings.Join(EffectivePerms(models.User{Role: f.role, Permissions: f.perms}), ", ")}
		if hash != "" {
			if err := s.users.SetPassword(ctx, tx, in.ID, hash, now); err != nil {
				return err
			}
			after["senha"] = "redefinida"
		}
		return audit(ctx, tx, now, nil, "usuario_alterado", "usuario", &in.ID,
			map[string]any{"nome": before.Name, "login": before.Login, "papel": before.Role, "ativo": before.Active,
				"permissoes": strings.Join(EffectivePerms(before), ", ")}, after)
	})
	if err != nil {
		return models.User{}, err
	}
	return s.User(ctx, in.ID)
}

// ChangeOwnPassword troca a senha do usuário logado, conferindo a atual.
func (s *AuthService) ChangeOwnPassword(ctx context.Context, current, next string) error {
	u, ok := actor.From(ctx)
	if !ok {
		return newErr(CodeLoginRequired, "Entre com seu usuário para continuar.")
	}
	hash, err := s.users.PasswordHash(ctx, s.DB, u.ID)
	if err != nil {
		return err
	}
	if !checkPassword(hash, current) {
		return invalid("A senha atual está incorreta.")
	}
	if err := validatePassword(next); err != nil {
		return err
	}
	newHash, err := hashPassword(next)
	if err != nil {
		return err
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if err := s.users.SetPassword(ctx, tx, u.ID, newHash, now); err != nil {
			return err
		}
		return audit(ctx, tx, now, nil, "senha_alterada", "usuario", &u.ID, nil, nil)
	})
}

// ListAudit devolve a auditoria entre duas datas locais (inclusive).
func (s *AuthService) ListAudit(ctx context.Context, startDate, endDate string) ([]models.AuditEntry, error) {
	if _, err := requirePerm(ctx, s.Now(), PermAudit); err != nil {
		return nil, err
	}
	start, end, err := PeriodRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	return repository.AuditRepo{}.List(ctx, s.DB, start, end, 2000)
}

// salesScope devolve de quem são as vendas que o usuário pode consultar: o
// operador pedido (0 = todos) para quem tem ver_vendas_de_todos, ou só ele
// mesmo. Sem usuário no context, nenhum (-1).
func salesScope(ctx context.Context, requested int64) int64 {
	u, ok := actor.From(ctx)
	if !ok {
		return -1
	}
	if u.Role == models.RoleAdmin || slices.Contains(u.Perms, PermAllSales) {
		return requested
	}
	return u.ID
}

// ListSellers lista nome e id dos usuários para o filtro por operador.
func (s *AuthService) ListSellers(ctx context.Context) ([]models.User, error) {
	if salesScope(ctx, 0) != 0 {
		return []models.User{}, nil
	}
	list, err := s.users.List(ctx, s.DB)
	out := make([]models.User, 0, len(list))
	for _, u := range list {
		out = append(out, models.User{ID: u.ID, Name: u.Name, Active: u.Active})
	}
	return out, err
}
