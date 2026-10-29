// Caminho: pdv/internal/actor/actor.go

// Package actor leva no context quem está operando o sistema e as
// autorizações de gerente pendentes. O App coloca o usuário logado em cada
// chamada; serviços e repositórios leem daqui para checar permissão e gravar
// o autor de cada registro.
package actor

import (
	"context"
	"sync"
	"time"
)

type User struct {
	ID    int64
	Name  string
	Role  string
	Perms []string // permissões efetivas (o administrador tem todas)
}

type ctxKey struct{}

type value struct {
	user   User
	grants *Grants
}

// With devolve um context com o usuário e o cofre de autorizações.
func With(ctx context.Context, u User, g *Grants) context.Context {
	return context.WithValue(ctx, ctxKey{}, value{user: u, grants: g})
}

// From devolve o usuário do context, se houver.
func From(ctx context.Context) (User, bool) {
	v, ok := ctx.Value(ctxKey{}).(value)
	return v.user, ok && v.user.ID != 0
}

// ID devolve o id do usuário para gravar no banco (nil se não houver).
func ID(ctx context.Context) any {
	if u, ok := From(ctx); ok {
		return u.ID
	}
	return nil
}

// Grants guarda autorizações dadas por um gerente para a PRÓXIMA operação de
// uma permissão. Cada autorização vale uma vez e expira em pouco tempo.
type Grants struct {
	mu      sync.Mutex
	pending map[string]grant
}

type grant struct {
	by      User
	expires time.Time
}

const GrantTTL = 2 * time.Minute

func NewGrants() *Grants { return &Grants{pending: map[string]grant{}} }

func (g *Grants) Add(perm string, by User, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pending[perm] = grant{by: by, expires: now.Add(GrantTTL)}
}

// Take consome a autorização da permissão, se ainda for válida.
func (g *Grants) Take(perm string, now time.Time) (User, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	gr, ok := g.pending[perm]
	delete(g.pending, perm)
	if !ok || now.After(gr.expires) {
		return User{}, false
	}
	return gr.by, true
}

// Clear descarta tudo (troca de operador).
func (g *Grants) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pending = map[string]grant{}
}

// GrantsFrom devolve o cofre de autorizações do context.
func GrantsFrom(ctx context.Context) *Grants {
	v, _ := ctx.Value(ctxKey{}).(value)
	return v.grants
}
