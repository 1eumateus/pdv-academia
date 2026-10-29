// Caminho: pdv/internal/applog/applog.go

// Package applog grava o log técnico local em <dados>/logs/pdv.log.
package applog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
)

// maxSize: ao abrir, um log maior que isso vira pdv.log.1 (só uma geração).
const maxSize = 5 << 20

// Open cria o logger. O chamador fecha o io.Closer ao encerrar.
func Open(dataDir string) (*slog.Logger, io.Closer, error) {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("criar pasta de logs: %w", err)
	}
	path := filepath.Join(dir, "pdv.log")
	if info, err := os.Stat(path); err == nil && info.Size() > maxSize {
		os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("abrir arquivo de log: %w", err)
	}
	// Erro fatal do Go (que fecha o programa) também vai para este arquivo.
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	return slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})), f, nil
}
