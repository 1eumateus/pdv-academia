// Caminho: pdv/internal/service/backup_auto_test.go

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pdv/internal/repository"
)

func TestAutoBackupRetentionAndExternalCopy(t *testing.T) {
	e := newEnv(t)
	set := func(k, v string) {
		if err := (repository.SettingsRepo{}).Set(e.ctx, e.db, k, v); err != nil {
			t.Fatal(err)
		}
	}
	set("backup_keep", "3")
	day := time.Date(2026, 1, 1, 10, 0, 0, 0, time.Local)
	e.backup.Now = func() time.Time { return day }

	// 5 dias seguidos: ficam só os 3 mais novos.
	for i := 0; i < 5; i++ {
		if _, err := e.backup.RunAuto(e.ctx, false); err != nil {
			t.Fatal(err)
		}
		day = day.AddDate(0, 0, 1)
	}
	dir := filepath.Join(e.dataDir, backupsDirName)
	got := autoFiles(dir)
	want := []string{"auto-2026-01-05.db", "auto-2026-01-04.db", "auto-2026-01-03.db"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("retenção: %v", got)
	}

	// Mesmo dia de novo: não refaz.
	day = day.AddDate(0, 0, -1)
	os.Remove(filepath.Join(dir, "auto-2026-01-05.db"))
	if _, err := e.backup.RunAuto(e.ctx, false); err != nil {
		t.Fatal(err)
	}
	if latestAuto(dir) != filepath.Join(dir, "auto-2026-01-04.db") {
		t.Fatal("refez o backup no mesmo dia")
	}

	// Sem pasta externa: avisa para configurar.
	st, err := e.backup.Status(e.ctx)
	if err != nil || !strings.Contains(st.Warning, "Escolha um pendrive") {
		t.Fatalf("aviso sem destino: %q %v", st.Warning, err)
	}

	// Pasta externa inexistente (pendrive fora): registra a falha e avisa.
	ext := filepath.Join(t.TempDir(), "pendrive")
	set("backup_external_dir", ext)
	st, err = e.backup.RunAuto(e.ctx, false)
	if err != nil || st.ExternalError == "" || st.LastExternal != nil || !strings.Contains(st.Warning, "não está acessível") {
		t.Fatalf("destino ausente: %+v %v", st, err)
	}

	// Pendrive plugado: copia, valida e limpa o erro.
	os.Mkdir(ext, 0o700)
	st, err = e.backup.RunAuto(e.ctx, true)
	if err != nil || st.ExternalError != "" || st.LastExternal == nil || st.Warning != "" {
		t.Fatalf("cópia externa: %+v %v", st, err)
	}
	if _, err := ValidateBackupFile(e.ctx, filepath.Join(ext, "auto-2026-01-05.db")); err != nil {
		t.Fatal("cópia externa inválida:", err)
	}

	// Pasta do backup escolhida: o backup do dia vai para lá.
	chosen := t.TempDir()
	set("backup_dir", chosen)
	day = day.AddDate(0, 0, 1)
	if st, err = e.backup.RunAuto(e.ctx, false); err != nil || st.AutoDir != chosen || st.AutoDirError != "" {
		t.Fatalf("pasta escolhida: %+v %v", st, err)
	}
	if latestAuto(chosen) != filepath.Join(chosen, "auto-2026-01-06.db") {
		t.Fatal("backup não foi para a pasta escolhida")
	}
	// Pasta escolhida sumiu: salva na padrão e avisa.
	set("backup_dir", filepath.Join(chosen, "nao-existe"))
	if st, err = e.backup.RunAuto(e.ctx, true); err != nil || !strings.Contains(st.Warning, "não está acessível") {
		t.Fatalf("pasta escolhida ausente: %+v %v", st, err)
	}
	if latestAuto(dir) != filepath.Join(dir, "auto-2026-01-06.db") {
		t.Fatal("não usou a pasta padrão")
	}
	set("backup_dir", "")
	day = day.AddDate(0, 0, -1)
	if _, err = e.backup.RunAuto(e.ctx, true); err != nil {
		t.Fatal(err)
	}

	// Muitos dias sem cópia externa: avisa.
	day = day.AddDate(0, 0, 10)
	if st, _ = e.backup.Status(e.ctx); !strings.Contains(st.Warning, "há 10 dias") {
		t.Fatalf("aviso de atraso: %q", st.Warning)
	}
}
