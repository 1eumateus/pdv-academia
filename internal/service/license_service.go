// Caminho: pdv/internal/service/license_service.go

package service

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"fmt"
	"math"
	"sync"
	"time"

	"pdv/internal/license"
	"pdv/internal/models"
	"pdv/internal/repository"
)

const (
	trialDays      = 15
	licenseWarnDay = 7
	clockTolerance = time.Hour // ajuste de horário de verão, sincronização etc.
	statusCacheTTL = time.Minute

	keyLicense  = "license_key"
	keyFirstRun = "license_first_run"
	keyLastSeen = "license_last_seen"
	regFirstRun = "PrimeiraExecucao"
	regLastSeen = "UltimaData"
)

// LicenseService decide se o sistema pode gravar. As datas do teste e a maior
// data já vista ficam no banco E fora dele (registro do Windows); vale sempre
// a mais antiga (1ª execução) e a mais nova (última data), para apagar o banco
// ou restaurar um backup antigo não reiniciar o teste nem esconder relógio voltado.
type LicenseService struct {
	Base
	Store   license.Store
	Pub     ed25519.PublicKey
	Machine func() (string, error)

	mu       sync.Mutex
	cached   *models.LicenseStatus
	cachedAt time.Time
}

func NewLicenseService(b Base, store license.Store) *LicenseService {
	return &LicenseService{Base: b, Store: store, Pub: license.PublicKey(), Machine: license.MachineID}
}

func parseRFC(v string) *time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil
	}
	return &t
}

func pick(a, b *time.Time, earliest bool) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case a.Before(*b) == earliest:
		return a
	}
	return b
}

func daysUntil(now, end time.Time) int { return int(math.Ceil(end.Sub(now).Hours() / 24)) }

// Status calcula a situação agora (e atualiza a última data vista).
func (s *LicenseService) Status(ctx context.Context) (models.LicenseStatus, error) {
	st, err := s.compute(ctx)
	if err != nil {
		return st, err
	}
	s.mu.Lock()
	s.cached, s.cachedAt = &st, s.Now()
	s.mu.Unlock()
	return st, nil
}

func (s *LicenseService) compute(ctx context.Context) (models.LicenseStatus, error) {
	repo := repository.SettingsRepo{}
	m, err := repo.All(ctx, s.DB)
	if err != nil {
		return models.LicenseStatus{}, err
	}
	now := s.Now()
	// Falha ao gravar no registro não bloqueia o uso: o banco ainda guarda as datas.
	save := func(dbKey, regKey string, t time.Time) error {
		v := t.Format(time.RFC3339)
		_ = s.Store.Set(regKey, v)
		return repo.Set(ctx, s.DB, dbKey, v)
	}

	first := pick(parseRFC(m[keyFirstRun]), parseRFC(s.Store.Get(regFirstRun)), true)
	if first == nil {
		first = &now
	}
	if m[keyFirstRun] != first.Format(time.RFC3339) || s.Store.Get(regFirstRun) != first.Format(time.RFC3339) {
		if err := save(keyFirstRun, regFirstRun, *first); err != nil {
			return models.LicenseStatus{}, err
		}
	}

	machine, err := s.Machine()
	if err != nil {
		machine = "indisponível"
	}
	st := models.LicenseStatus{MachineID: machine}

	last := pick(parseRFC(m[keyLastSeen]), parseRFC(s.Store.Get(regLastSeen)), false)
	if last != nil && now.Before(last.Add(-clockTolerance)) {
		st.State = "relogio"
		st.Message = fmt.Sprintf("A data do computador (%s) está antes da última data em que o sistema foi usado (%s). "+
			"Corrija a data e a hora do Windows; o sistema volta ao normal na hora.", now.Format("02/01/2006 15:04"), last.In(now.Location()).Format("02/01/2006 15:04"))
		st.Warning = "Data do computador incorreta: sistema em modo somente consulta."
		return st, nil
	}
	if last == nil || now.After(*last) {
		if err := save(keyLastSeen, regLastSeen, now); err != nil {
			return models.LicenseStatus{}, err
		}
	}

	if text := m[keyLicense]; text != "" {
		d, err := s.check(text, machine)
		if err == nil {
			st.Store, st.Document, st.Plan, st.Expires = d.Store, d.Document, d.Plan, d.Expires
			exp := d.ExpiresAt()
			switch {
			case exp == nil:
				st.State, st.CanWrite, st.Message = "ativa", true, "Licença vitalícia."
			case now.After(*exp):
				st.State = "vencida"
				st.Message = "A licença venceu em " + exp.Format("02/01/2006") + ". Os dados continuam disponíveis para consulta e backup."
				st.Warning = "Licença vencida: sistema em modo somente consulta. Renove em Administração → Licença."
			default:
				st.State, st.CanWrite, st.DaysLeft = "ativa", true, daysUntil(now, *exp)
				st.Message = "Licença válida até " + exp.Format("02/01/2006") + "."
				if st.DaysLeft <= licenseWarnDay {
					st.Warning = fmt.Sprintf("A licença vence em %d dia(s), em %s. Fale com o suporte para renovar.", st.DaysLeft, exp.Format("02/01/2006"))
				}
			}
			return st, nil
		}
		st.LicenseError = err.Error()
	}

	end := first.AddDate(0, 0, trialDays)
	st.Expires = end.In(now.Location()).Format("2006-01-02")
	if now.Before(end) {
		st.State, st.CanWrite, st.DaysLeft = "teste", true, daysUntil(now, end)
		st.Message = fmt.Sprintf("Período de teste: faltam %d dia(s) (até %s).", st.DaysLeft, end.In(now.Location()).Format("02/01/2006"))
		if st.DaysLeft <= licenseWarnDay {
			st.Warning = st.Message + " Ative uma licença em Administração → Licença."
		}
		return st, nil
	}
	st.State = "teste_encerrado"
	st.Message = "O período de teste terminou. Os dados continuam disponíveis para consulta e backup."
	st.Warning = "Período de teste encerrado: sistema em modo somente consulta. Ative uma licença em Administração → Licença."
	return st, nil
}

// check confere assinatura e máquina.
func (s *LicenseService) check(text, machine string) (license.Data, error) {
	d, err := license.Verify(s.Pub, text)
	if err != nil {
		return d, invalid("Licença inválida: confira se o texto foi copiado inteiro.")
	}
	if d.MachineID != machine {
		return d, invalid("Esta licença é de outro computador (ID %s). O ID deste computador é %s.", d.MachineID, machine)
	}
	return d, nil
}

// Activate valida e grava a licença. Uma licença já vencida é recusada.
// Qualquer usuário logado pode ativar: só passa licença assinada para este
// computador, e a auditoria registra quem ativou.
func (s *LicenseService) Activate(ctx context.Context, text string) (models.LicenseStatus, error) {
	machine, err := s.Machine()
	if err != nil {
		return models.LicenseStatus{}, invalid("Não foi possível ler o identificador deste computador.")
	}
	d, err := s.check(text, machine)
	if err != nil {
		return models.LicenseStatus{}, err
	}
	if exp := d.ExpiresAt(); exp != nil && s.Now().After(*exp) {
		return models.LicenseStatus{}, invalid("Esta licença venceu em %s.", exp.Format("02/01/2006"))
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		if err := (repository.SettingsRepo{}).Set(ctx, tx, keyLicense, text); err != nil {
			return err
		}
		return audit(ctx, tx, s.Now(), nil, "licenca_ativada", "licenca", nil, nil,
			map[string]any{"loja": d.Store, "documento": d.Document, "validade": d.Expires, "plano": d.Plan})
	})
	if err != nil {
		return models.LicenseStatus{}, err
	}
	return s.Status(ctx)
}

// RequireWrite recusa operações que gravam movimento quando não há licença
// válida. Usa o último status por até 1 minuto (é chamado a cada operação).
func (s *LicenseService) RequireWrite(ctx context.Context) error {
	s.mu.Lock()
	st := s.cached
	fresh := st != nil && s.Now().Sub(s.cachedAt) < statusCacheTTL && s.Now().After(s.cachedAt)
	s.mu.Unlock()
	if !fresh {
		cur, err := s.Status(ctx)
		if err != nil {
			return err
		}
		st = &cur
	}
	if !st.CanWrite {
		return newErr(CodeReadOnly, "Sistema em modo somente consulta: %s", st.Message)
	}
	return nil
}
