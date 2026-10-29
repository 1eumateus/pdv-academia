// Caminho: pdv/internal/license/machine_other.go

//go:build !windows

package license

import "os"

// Fora do Windows (só desenvolvimento): usa o nome da máquina.
func machineGUID() (string, error) { return os.Hostname() }

// RegistryStore fora do Windows não persiste (só desenvolvimento).
type RegistryStore = MemStore
