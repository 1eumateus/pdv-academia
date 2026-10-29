// Caminho: pdv/internal/service/backup_auto.go

package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// Backup automático: um arquivo por dia (auto-AAAA-MM-DD.db) em <dados>/backups,
// guardando só os últimos N. Se houver pasta externa (pendrive, pasta do Google
// Drive/OneDrive), o backup do dia é copiado para lá também; se ela não estiver
// acessível, a falha fica registrada e a cópia é tentada de novo na próxima rodada.
const (
	autoPrefix          = "auto-"
	keyLastAuto         = "backup_last_auto"
	keyLastExternal     = "backup_last_external"
	keyExternalError    = "backup_external_error"
	keyDirError         = "backup_dir_error"
	externalCopyTimeout = 10 * time.Minute
)

func (s *BackupService) settingsGet(ctx context.Context) (models.Settings, map[string]string, error) {
	st, err := (&SettingsService{Base: s.Base}).Get(ctx)
	if err != nil {
		return st, nil, err
	}
	m, err := repository.SettingsRepo{}.All(ctx, s.DB)
	return st, m, err
}

func parseTime(v string) *time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil
	}
	return &t
}

func sameDay(t *time.Time, now time.Time) bool {
	return t != nil && t.In(now.Location()).Format("2006-01-02") == now.Format("2006-01-02")
}

// RunAuto faz o backup do dia (se ainda não feito) e a cópia externa (se ainda
// não copiada hoje). force refaz os dois agora ("Copiar agora").
func (s *BackupService) RunAuto(ctx context.Context, force bool) (models.BackupStatus, error) {
	st, m, err := s.settingsGet(ctx)
	if err != nil {
		return models.BackupStatus{}, err
	}
	now := s.Now()
	dir, dirErr := s.autoDir(st)
	today := filepath.Join(dir, autoPrefix+now.Format("2006-01-02")+".db")
	set := func(k, v string) error { return repository.SettingsRepo{}.Set(ctx, s.DB, k, v) }

	if (st.BackupAuto || force) && (force || !sameDay(parseTime(m[keyLastAuto]), now)) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return models.BackupStatus{}, fmt.Errorf("criar pasta de backups: %w", err)
		}
		if _, err := s.Create(ctx, today); err != nil {
			return models.BackupStatus{}, err
		}
		if err := pruneAuto(dir, int(st.BackupKeep)); err != nil {
			return models.BackupStatus{}, err
		}
		if err := set(keyLastAuto, now.Format(time.RFC3339)); err != nil {
			return models.BackupStatus{}, err
		}
		if err := set(keyDirError, dirErr); err != nil {
			return models.BackupStatus{}, err
		}
	}

	if st.BackupExternalDir != "" && (force || !sameDay(parseTime(m[keyLastExternal]), now)) {
		src := latestAuto(dir)
		var copyErr error
		if src == "" {
			copyErr = fmt.Errorf("ainda não há backup automático para copiar")
		} else {
			copyErr = copyExternal(ctx, src, st.BackupExternalDir, int(st.BackupKeep))
		}
		if copyErr != nil {
			if err := set(keyExternalError, copyErr.Error()); err != nil {
				return models.BackupStatus{}, err
			}
		} else {
			if err := set(keyLastExternal, now.Format(time.RFC3339)); err != nil {
				return models.BackupStatus{}, err
			}
			if err := set(keyExternalError, ""); err != nil {
				return models.BackupStatus{}, err
			}
		}
	}
	return s.Status(ctx)
}

// autoDir devolve a pasta do backup automático. Se a pasta escolhida não estiver
// acessível, usa a padrão (melhor um backup no lugar errado que nenhum) e explica.
func (s *BackupService) autoDir(st models.Settings) (dir, problem string) {
	def := filepath.Join(s.DataDir, backupsDirName)
	if st.BackupDir == "" {
		return def, ""
	}
	if fi, err := os.Stat(st.BackupDir); err != nil || !fi.IsDir() {
		return def, fmt.Sprintf("a pasta %s não está acessível; o backup foi salvo em %s", st.BackupDir, def)
	}
	return st.BackupDir, ""
}

// Status lê o estado salvo e monta o aviso do Painel.
func (s *BackupService) Status(ctx context.Context) (models.BackupStatus, error) {
	st, m, err := s.settingsGet(ctx)
	if err != nil {
		return models.BackupStatus{}, err
	}
	out := models.BackupStatus{AutoEnabled: st.BackupAuto, LastAuto: parseTime(m[keyLastAuto]), AutoDirError: m[keyDirError],
		ExternalDir: st.BackupExternalDir, LastExternal: parseTime(m[keyLastExternal]), ExternalError: m[keyExternalError]}
	out.AutoDir = st.BackupDir
	if out.AutoDir == "" {
		out.AutoDir = filepath.Join(s.DataDir, backupsDirName)
	}
	now := s.Now()
	switch {
	case out.AutoDirError != "":
		out.Warning = "Backup automático: " + out.AutoDirError + "."
	case out.ExternalDir == "":
		out.Warning = "Nenhuma cópia do backup fora deste computador. Escolha um pendrive ou uma pasta do Google Drive/OneDrive em Configurações."
	case out.ExternalError != "":
		out.Warning = "A cópia externa do backup falhou: " + out.ExternalError
	case out.LastExternal == nil:
		out.Warning = "A cópia externa do backup ainda não foi feita."
	case now.Sub(*out.LastExternal) > time.Duration(st.BackupWarnDays)*24*time.Hour:
		out.Warning = fmt.Sprintf("A última cópia externa do backup foi há %d dias.", int(now.Sub(*out.LastExternal).Hours()/24))
	}
	return out, nil
}

// autoFiles lista os backups automáticos da pasta, do mais novo ao mais antigo
// (o nome tem a data, então a ordem do nome é a ordem do tempo).
func autoFiles(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), autoPrefix) && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names
}

func latestAuto(dir string) string {
	if names := autoFiles(dir); len(names) > 0 {
		return filepath.Join(dir, names[0])
	}
	return ""
}

func pruneAuto(dir string, keep int) error {
	names := autoFiles(dir)
	if len(names) <= keep {
		return nil
	}
	for _, n := range names[keep:] {
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			return fmt.Errorf("apagar backup antigo %s: %w", n, err)
		}
	}
	return nil
}

// copyExternal copia para a pasta externa (via .tmp, validando a cópia) e
// aplica a mesma retenção lá. Erros saem em português: vão para a tela.
func copyExternal(ctx context.Context, src, destDir string, keep int) error {
	if fi, err := os.Stat(destDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("a pasta %s não está acessível (pendrive desconectado ou pasta removida?)", destDir)
	}
	dest := filepath.Join(destDir, filepath.Base(src))
	tmp := dest + ".tmp"
	if err := copyFile(src, tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("não foi possível gravar em %s (sem espaço ou sem permissão?)", destDir)
	}
	ctx, cancel := context.WithTimeout(ctx, externalCopyTimeout)
	defer cancel()
	if _, err := ValidateBackupFile(ctx, tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("a cópia gravada em %s ficou com defeito", destDir)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("não foi possível gravar em %s", destDir)
	}
	if err := pruneAuto(destDir, keep); err != nil {
		return fmt.Errorf("não foi possível apagar cópias antigas em %s", destDir)
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
