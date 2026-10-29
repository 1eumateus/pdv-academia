// Caminho: pdv/app_test.go

package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// Um panic dentro de uma operação vira erro interno e fica no log com a pilha,
// em vez de fechar o programa.
func TestCallRecoversPanic(t *testing.T) {
	var buf bytes.Buffer
	a := &App{log: slog.New(slog.NewTextHandler(&buf, nil))}
	_, err := callAnon(a, "teste", func(ctx context.Context) (int, error) {
		var m map[string]int
		m["x"] = 1 // panic: escrita em mapa nulo
		return 0, nil
	})
	if err == nil || !strings.Contains(err.Error(), "ERRO_INTERNO") {
		t.Fatalf("esperava erro interno, veio %v", err)
	}
	if log := buf.String(); !strings.Contains(log, "panic") || !strings.Contains(log, "app_test.go") {
		t.Fatalf("log sem o panic e a pilha: %s", log)
	}
}
