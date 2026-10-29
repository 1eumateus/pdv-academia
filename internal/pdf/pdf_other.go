//go:build !windows

// Caminho: pdv/internal/pdf/pdf_other.go

package pdf

import "errors"

// O PDV é só para Windows; aqui fica só o stub para compilar em outros sistemas.
func FromHTML(string, string) error { return errors.New("salvar PDF disponível só no Windows") }
