//go:build !windows

// Caminho: pdv/internal/printer/raw_other.go

package printer

import "errors"

// O PDV é só para Windows; aqui ficam apenas stubs para compilar e testar em outros sistemas.

func List() ([]string, error) { return []string{}, nil }

func Send(string, []byte) error { return errors.New("impressão direta disponível só no Windows") }
