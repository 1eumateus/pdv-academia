// Caminho: catraca/internal/device/serial_windows.go

package device

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Pulse liga o sinal DTR ou RTS da porta (ex.: "COM3") por d e desliga: é o
// que aciona a placa de relé da catraca eletromecânica.
func Pulse(port, signal string, d time.Duration) error {
	name, err := windows.UTF16PtrFromString(`\\.\` + strings.ToUpper(strings.TrimSpace(port)))
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return fmt.Errorf("não foi possível abrir a porta %s (em uso ou inexistente)", port)
	}
	defer windows.CloseHandle(h)
	on, off := uint32(windows.SETDTR), uint32(windows.CLRDTR)
	if strings.EqualFold(signal, "RTS") {
		on, off = windows.SETRTS, windows.CLRRTS
	}
	if err := windows.EscapeCommFunction(h, on); err != nil {
		return fmt.Errorf("acionar %s na porta %s: %w", signal, port, err)
	}
	time.Sleep(d)
	return windows.EscapeCommFunction(h, off)
}

// SerialPorts lista as portas COM existentes (registro do Windows).
func SerialPorts() []string {
	out := []string{}
	for i := 1; i <= 32; i++ {
		port := fmt.Sprintf("COM%d", i)
		name, _ := windows.UTF16PtrFromString(`\\.\` + port)
		h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
		if err == nil {
			windows.CloseHandle(h)
			out = append(out, port)
		} else if err == windows.ERROR_ACCESS_DENIED {
			out = append(out, port) // existe, mas está em uso
		}
	}
	return out
}
