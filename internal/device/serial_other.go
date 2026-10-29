//go:build !windows

// Caminho: catraca/internal/device/serial_other.go

package device

import (
	"errors"
	"time"
)

// O programa é só para Windows; aqui ficam apenas stubs para compilar e testar em outros sistemas.

func Pulse(port, signal string, d time.Duration) error {
	return errors.New("relé serial disponível apenas no Windows")
}

func SerialPorts() []string { return []string{} }
