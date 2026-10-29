//go:build windows

package pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFromHTML(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "teste.pdf")
	err := FromHTML(`<!doctype html><html><head><meta charset="utf-8"></head><body><h1>Relatório de ação</h1></body></html>`, dest)
	if err != nil {
		t.Skipf("Edge indisponível neste PC: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("não é um PDF: %v %q", err, data[:min(len(data), 20)])
	}
}
