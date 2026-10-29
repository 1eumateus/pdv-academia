// Caminho: pdv/internal/license/machine_windows.go

//go:build windows

package license

import (
	"golang.org/x/sys/windows/registry"
)

// machineGUID lê o MachineGuid do Windows (criado na instalação do sistema).
func machineGUID() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("MachineGuid")
	return v, err
}

const regPath = `Software\PDV`

// RegistryStore guarda as datas da licença em HKCU\Software\PDV: fora do banco,
// para apagar ou trocar o banco não reiniciar o período de teste.
type RegistryStore struct{}

func (RegistryStore) Get(name string) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue(name)
	return v
}

func (RegistryStore) Set(name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, regPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}
