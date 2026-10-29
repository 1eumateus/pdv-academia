//go:build windows

// Caminho: pdv/internal/pdf/pdf_windows.go

// Package pdf transforma uma página HTML em PDF com o Microsoft Edge em modo
// sem janela (headless), que já vem no Windows 10 e 11: nada a instalar.
package pdf

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// edgePaths: onde o Edge costuma estar instalado.
func edgePaths() []string {
	var out []string
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
		if dir := os.Getenv(env); dir != "" {
			out = append(out, filepath.Join(dir, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	}
	return out
}

// FromHTML grava em dest o PDF da página html (documento completo, com o CSS embutido).
func FromHTML(html, dest string) error {
	edge := ""
	for _, p := range edgePaths() {
		if _, err := os.Stat(p); err == nil {
			edge = p
			break
		}
	}
	if edge == "" {
		return errors.New("Microsoft Edge não encontrado; use Imprimir A4 e escolha \"Microsoft Print to PDF\"")
	}
	dir, err := os.MkdirTemp("", "pdv-pdf-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	page := filepath.Join(dir, "pagina.html")
	if err := os.WriteFile(page, []byte(html), 0o600); err != nil {
		return err
	}
	out := filepath.Join(dir, "saida.pdf")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, edge, "--headless", "--disable-gpu", "--no-pdf-header-footer",
		"--user-data-dir="+filepath.Join(dir, "perfil"), "--print-to-pdf="+out,
		(&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(page)}).String())
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	if msg, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Edge não gerou o PDF: %v %s", err, msg)
	}
	data, err := os.ReadFile(out)
	if err != nil || len(data) == 0 {
		return errors.New("o Edge não gerou o PDF")
	}
	return os.WriteFile(dest, data, 0o644)
}
