// Caminho: pdv/internal/service/license_test.go

package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"pdv/internal/license"
	"pdv/internal/models"
	"pdv/internal/repository"
)

func TestLicenseTrialClockAndActivation(t *testing.T) {
	e := newEnv(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	reg := &license.MemStore{}
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.Local)
	ls := NewLicenseService(e.backup.Base, reg)
	ls.Pub, ls.Machine = pub, func() (string, error) { return "AAAA-BBBB", nil }
	ls.Now = func() time.Time { return now }
	status := func() models.LicenseStatus {
		s, err := ls.Status(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	// 1ª execução: teste de 15 dias.
	if st := status(); st.State != "teste" || !st.CanWrite || st.DaysLeft != 15 {
		t.Fatalf("início do teste: %+v", st)
	}
	// Apagar a data do banco não reinicia o teste (o registro guarda a 1ª execução).
	now = now.AddDate(0, 0, 10)
	(repository.SettingsRepo{}).Set(e.ctx, e.db, keyFirstRun, "")
	if st := status(); st.State != "teste" || st.DaysLeft != 5 {
		t.Fatalf("teste reiniciado ao apagar o banco: %+v", st)
	}
	// Relógio voltado: somente consulta.
	now = now.AddDate(0, 0, -3)
	if st := status(); st.State != "relogio" || st.CanWrite {
		t.Fatalf("relógio voltado: %+v", st)
	}
	if err := ls.RequireWrite(e.ctx); err == nil {
		t.Fatal("gravou com o relógio voltado")
	}
	// Fim do teste: somente consulta, nada apagado.
	now = now.AddDate(0, 0, 10)
	if st := status(); st.State != "teste_encerrado" || st.CanWrite {
		t.Fatalf("fim do teste: %+v", st)
	}

	// Licença de outra máquina, adulterada e vencida são recusadas.
	sign := func(d license.Data) string {
		s, err := license.Sign(priv, d)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if _, err := ls.Activate(e.ctx, sign(license.Data{Store: "X", MachineID: "OUTRA"})); err == nil || !strings.Contains(err.Error(), "outro computador") {
		t.Fatalf("outra máquina: %v", err)
	}
	good := sign(license.Data{Store: "Loja", MachineID: "AAAA-BBBB", Expires: now.AddDate(0, 0, 5).Format("2006-01-02"), Plan: "anual"})
	if _, err := ls.Activate(e.ctx, strings.Replace(good, "PDV1.e", "PDV1.f", 1)); err == nil {
		t.Fatal("aceitou licença adulterada")
	}
	if _, err := ls.Activate(e.ctx, sign(license.Data{Store: "Loja", MachineID: "AAAA-BBBB", Expires: "2020-01-01"})); err == nil {
		t.Fatal("aceitou licença vencida")
	}

	// Licença válida (colada com quebras de linha): grava e avisa a 5 dias do fim.
	s, err := ls.Activate(e.ctx, good[:20]+"\n"+good[20:])
	if err != nil || s.State != "ativa" || !s.CanWrite || s.DaysLeft != 6 || s.Warning == "" {
		t.Fatalf("ativação: %+v %v", s, err)
	}
	if err := ls.RequireWrite(e.ctx); err != nil {
		t.Fatal(err)
	}
	// Venceu: somente consulta.
	now = now.AddDate(0, 0, 7)
	if st := status(); st.State != "vencida" || st.CanWrite {
		t.Fatalf("vencida: %+v", st)
	}
	// Vitalícia, ativada por um operador (qualquer usuário pode ativar).
	op := e.as(e.user("operador", models.RoleOperator))
	if s, err = ls.Activate(op, sign(license.Data{Store: "Loja", MachineID: "AAAA-BBBB"})); err != nil || s.State != "ativa" || s.Warning != "" {
		t.Fatalf("vitalícia: %+v %v", s, err)
	}
}
