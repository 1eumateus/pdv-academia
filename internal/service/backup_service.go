// Caminho: pdv/internal/service/backup_service.go

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pdv/internal/db"
	"pdv/internal/models"
)

// Estratégia de backup (SQLite em WAL):
//   - Criação: VACUUM INTO gera, a partir da conexão ativa, uma cópia íntegra e
//     consistente que já inclui o que está no arquivo -wal. Copiar só o pdv.db
//     perderia transações ainda não transferidas do WAL.
//   - A cópia é gravada num arquivo temporário, validada e só então renomeada
//     para o destino.
//   - Restauração: o arquivo escolhido é validado e copiado (também por VACUUM
//     INTO) para pdv.db.restore no diretório de dados. O banco ativo NÃO é
//     tocado com o programa aberto. Na próxima inicialização, antes de abrir o
//     banco, ApplyPendingRestore move o banco atual (com -wal e -shm) para
//     backups/antes-da-restauracao-<data>, coloca a cópia no lugar, abre,
//     aplica migrations e verifica a integridade. Se algo falhar, os arquivos
//     originais voltam para o lugar.
type BackupService struct {
	Base
	DataDir string
	DBPath  string
}

func NewBackupService(b Base, dataDir, dbPath string) *BackupService {
	return &BackupService{Base: b, DataDir: dataDir, DBPath: dbPath}
}

const (
	pendingRestoreName = db.FileName + ".restore"
	backupsDirName     = "backups"
)

func (s *BackupService) PendingRestorePath() string {
	return filepath.Join(s.DataDir, pendingRestoreName)
}

// validateUserPath recusa caminhos relativos, extensões inesperadas e o próprio
// banco ativo.
func (s *BackupService) validateUserPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", invalid("Nenhum arquivo foi selecionado.")
	}
	if !filepath.IsAbs(path) {
		return "", invalid("Selecione o arquivo pelo diálogo do sistema.")
	}
	path = filepath.Clean(path)
	if strings.ToLower(filepath.Ext(path)) != ".db" {
		return "", invalid("O arquivo de backup deve ter a extensão .db.")
	}
	active, _ := filepath.Abs(s.DBPath)
	if strings.EqualFold(path, active) || strings.EqualFold(path, s.PendingRestorePath()) {
		return "", invalid("Escolha um arquivo diferente do banco de dados em uso.")
	}
	return path, nil
}

// Create grava um backup consistente no destino informado.
func (s *BackupService) Create(ctx context.Context, destination string) (models.BackupResult, error) {
	dest, err := s.validateUserPath(destination)
	if err != nil {
		return models.BackupResult{}, err
	}
	if err := s.vacuumInto(ctx, s.DB, dest); err != nil {
		return models.BackupResult{}, err
	}
	info, err := os.Stat(dest)
	if err != nil {
		return models.BackupResult{}, fmt.Errorf("ler backup gravado: %w", err)
	}
	return models.BackupResult{Path: dest, SizeBytes: info.Size(), CreatedAt: s.Now()}, nil
}

// CreateInDataDir grava um backup em <dados>/backups com nome datado.
func (s *BackupService) CreateInDataDir(ctx context.Context) (models.BackupResult, error) {
	dir := filepath.Join(s.DataDir, backupsDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return models.BackupResult{}, fmt.Errorf("criar pasta de backups: %w", err)
	}
	name := "pdv-backup-" + s.Now().Format("2006-01-02-150405") + ".db"
	return s.Create(ctx, filepath.Join(dir, name))
}

// vacuumInto grava em arquivo temporário ao lado do destino, valida e renomeia.
func (s *BackupService) vacuumInto(ctx context.Context, src *sql.DB, dest string) error {
	tmp := dest + ".tmp"
	_ = os.Remove(tmp) // VACUUM INTO exige que o arquivo não exista
	if _, err := src.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("gerar cópia do banco: %w", err)
	}
	if _, err := ValidateBackupFile(ctx, tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("gravar backup no destino: %w", err)
	}
	return nil
}

// ValidateBackupFile abre o arquivo somente leitura e confere integridade,
// chaves estrangeiras, versão de esquema e tabelas obrigatórias. Devolve a
// versão do esquema do arquivo.
func ValidateBackupFile(ctx context.Context, path string) (int, error) {
	bad := func(format string, args ...any) (int, error) {
		return 0, newErr(CodeInvalid, "Arquivo de backup inválido: "+format, args...)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return bad("o arquivo não foi encontrado.")
	}
	if info.Size() < 512 {
		return bad("o arquivo é pequeno demais para ser um banco do PDV.")
	}
	header := make([]byte, 16)
	f, err := os.Open(path)
	if err != nil {
		return bad("não foi possível ler o arquivo.")
	}
	_, err = f.Read(header)
	f.Close()
	if err != nil || string(header) != "SQLite format 3\x00" {
		return bad("o arquivo não é um banco SQLite.")
	}

	conn, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return bad("não foi possível abrir o arquivo.")
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)

	version, err := db.UserVersion(ctx, conn)
	if err != nil {
		return bad("não foi possível ler a versão.")
	}
	if version < 1 {
		return bad("o arquivo não contém dados do PDV.")
	}
	if version > db.LatestVersion() {
		return bad("o backup é de uma versão mais nova do PDV (%d). Atualize o programa antes de restaurar.", version)
	}
	if err := db.IntegrityCheck(ctx, conn); err != nil {
		return bad("a verificação de integridade falhou.")
	}
	for _, t := range []string{"products", "customers", "sales", "sale_items", "stock_movements", "comandas"} {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, t).Scan(&n); err != nil || n != 1 {
			return bad("a tabela %s não foi encontrada.", t)
		}
	}
	return version, nil
}

// RequestRestore valida o arquivo e o prepara para ser aplicado na próxima
// inicialização. O banco em uso não é alterado.
func (s *BackupService) RequestRestore(ctx context.Context, source string) error {
	authBy, err := requirePerm(ctx, s.Now(), PermRestore)
	if err != nil {
		return err
	}
	src, err := s.validateUserPath(source)
	if err != nil {
		return err
	}
	if _, err := ValidateBackupFile(ctx, src); err != nil {
		return err
	}
	conn, err := sql.Open("sqlite", "file:"+filepath.ToSlash(src)+"?mode=ro")
	if err != nil {
		return fmt.Errorf("abrir backup: %w", err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	if err := s.vacuumInto(ctx, conn, s.PendingRestorePath()); err != nil {
		return err
	}
	// Fica na auditoria do banco atual, que é guardado na pasta backups ao restaurar.
	return audit(ctx, s.DB, s.Now(), authBy, "restauracao_agendada", "backup", nil, nil, map[string]any{"arquivo": filepath.Base(src)})
}

// CancelPendingRestore desiste de uma restauração agendada.
func (s *BackupService) CancelPendingRestore() error {
	err := os.Remove(s.PendingRestorePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *BackupService) HasPendingRestore() bool {
	_, err := os.Stat(s.PendingRestorePath())
	return err == nil
}

// RestoreOutcome informa o que aconteceu na inicialização.
type RestoreOutcome struct {
	Applied bool
	Message string
	SavedTo string
}

// ApplyPendingRestore roda na inicialização, antes de abrir o banco. Sem
// arquivo pendente, não faz nada.
func ApplyPendingRestore(ctx context.Context, dataDir string, now time.Time) (RestoreOutcome, error) {
	pending := filepath.Join(dataDir, pendingRestoreName)
	if _, err := os.Stat(pending); errors.Is(err, os.ErrNotExist) {
		return RestoreOutcome{}, nil
	}
	if _, err := ValidateBackupFile(ctx, pending); err != nil {
		rejected := pending + ".rejeitado-" + now.Format("20060102-150405")
		os.Rename(pending, rejected)
		return RestoreOutcome{Message: "A restauração agendada foi cancelada porque o arquivo não passou na validação. O banco atual foi mantido."}, nil
	}

	active := filepath.Join(dataDir, db.FileName)
	saveDir := filepath.Join(dataDir, backupsDirName, "antes-da-restauracao-"+now.Format("2006-01-02-150405"))
	if err := os.MkdirAll(saveDir, 0o700); err != nil {
		return RestoreOutcome{}, fmt.Errorf("criar pasta de segurança: %w", err)
	}
	moved := []string{}
	rollback := func() {
		os.Remove(active)
		for _, name := range moved {
			os.Rename(filepath.Join(saveDir, name), filepath.Join(dataDir, name))
		}
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		name := db.FileName + suffix
		if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
			continue
		}
		if err := os.Rename(filepath.Join(dataDir, name), filepath.Join(saveDir, name)); err != nil {
			rollback()
			return RestoreOutcome{}, fmt.Errorf("mover banco atual: %w", err)
		}
		moved = append(moved, name)
	}
	if err := os.Rename(pending, active); err != nil {
		rollback()
		return RestoreOutcome{}, fmt.Errorf("colocar backup no lugar: %w", err)
	}
	conn, err := db.Open(ctx, active)
	if err == nil {
		err = db.IntegrityCheck(ctx, conn)
		conn.Close()
	}
	if err != nil {
		// Devolve o backup como pendente rejeitado e restaura o banco anterior.
		os.Rename(active, pending+".rejeitado-"+now.Format("20060102-150405"))
		for _, suffix := range []string{"-wal", "-shm"} {
			os.Remove(active + suffix)
		}
		rollback()
		return RestoreOutcome{Message: "A restauração falhou na verificação final e o banco anterior foi mantido."}, nil
	}
	return RestoreOutcome{Applied: true, SavedTo: saveDir,
		Message: "Backup restaurado com sucesso. O banco anterior foi guardado em " + saveDir + "."}, nil
}
