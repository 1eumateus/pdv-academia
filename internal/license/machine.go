// Caminho: pdv/internal/license/machine.go

package license

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
)

// Store guarda valores fora do banco (no Windows, o registro).
type Store interface {
	Get(name string) string
	Set(name, value string) error
}

// MemStore é um Store em memória (testes).
type MemStore struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *MemStore) Get(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[name]
}

func (s *MemStore) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[name] = value
	return nil
}

// MachineID é o identificador desta máquina: SHA-256 do MachineGuid, em
// 5 grupos de 4 (fácil de ditar ao suporte). Não expõe o GUID original.
func MachineID() (string, error) {
	g, err := machineGUID()
	if err != nil || strings.TrimSpace(g) == "" {
		return "", errors.New("não foi possível ler o identificador do Windows")
	}
	return FormatID(g), nil
}

// FormatID transforma um identificador bruto no formato exibido.
func FormatID(raw string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(raw))))
	h := strings.ToUpper(hex.EncodeToString(sum[:10]))
	parts := make([]string, 0, 5)
	for i := 0; i < len(h); i += 4 {
		parts = append(parts, h[i:i+4])
	}
	return strings.Join(parts, "-")
}
