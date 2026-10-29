// Caminho: pdv/internal/printer/raw_windows.go

package printer

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// Spooler do Windows (winspool.drv) via syscall, sem CGO.
var (
	winspool         = syscall.NewLazyDLL("winspool.drv")
	procOpenPrinter  = winspool.NewProc("OpenPrinterW")
	procClosePrinter = winspool.NewProc("ClosePrinter")
	procStartDoc     = winspool.NewProc("StartDocPrinterW")
	procEndDoc       = winspool.NewProc("EndDocPrinter")
	procStartPage    = winspool.NewProc("StartPagePrinter")
	procEndPage      = winspool.NewProc("EndPagePrinter")
	procWritePrinter = winspool.NewProc("WritePrinter")
	procEnumPrinters = winspool.NewProc("EnumPrintersW")
)

type docInfo1 struct {
	docName, outputFile, datatype *uint16
}

// printerInfo4 é PRINTER_INFO_4W.
type printerInfo4 struct {
	name, server *uint16
	attributes   uint32
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	var s []uint16
	for ptr := unsafe.Pointer(p); *(*uint16)(ptr) != 0; ptr = unsafe.Add(ptr, 2) {
		s = append(s, *(*uint16)(ptr))
	}
	return syscall.UTF16ToString(s)
}

// List devolve as impressoras locais e de rede conectadas.
func List() ([]string, error) {
	const flags = 0x2 | 0x4 // PRINTER_ENUM_LOCAL | PRINTER_ENUM_CONNECTIONS
	var needed, count uint32
	procEnumPrinters.Call(flags, 0, 4, 0, 0, uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)))
	if needed == 0 {
		return []string{}, nil
	}
	buf := make([]byte, needed)
	r, _, err := procEnumPrinters.Call(flags, 0, 4, uintptr(unsafe.Pointer(&buf[0])), uintptr(needed),
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)))
	if r == 0 {
		return nil, fmt.Errorf("listar impressoras: %w", err)
	}
	infos := unsafe.Slice((*printerInfo4)(unsafe.Pointer(&buf[0])), count)
	out := make([]string, 0, count)
	for _, in := range infos {
		out = append(out, utf16PtrToString(in.name))
	}
	return out, nil
}

// Send manda os bytes sem conversão (datatype RAW) para a impressora.
func Send(printerName string, data []byte) error {
	if len(data) == 0 {
		return errors.New("nada para imprimir")
	}
	name, err := syscall.UTF16PtrFromString(printerName)
	if err != nil {
		return err
	}
	var h syscall.Handle
	if r, _, err := procOpenPrinter.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&h)), 0); r == 0 {
		return fmt.Errorf("impressora não encontrada (%v)", err)
	}
	defer procClosePrinter.Call(uintptr(h))

	doc, _ := syscall.UTF16PtrFromString("Comprovante PDV")
	raw, _ := syscall.UTF16PtrFromString("RAW")
	di := docInfo1{docName: doc, datatype: raw}
	if r, _, err := procStartDoc.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&di))); r == 0 {
		return fmt.Errorf("iniciar impressão (%v)", err)
	}
	defer procEndDoc.Call(uintptr(h))
	if r, _, err := procStartPage.Call(uintptr(h)); r == 0 {
		return fmt.Errorf("iniciar página (%v)", err)
	}
	defer procEndPage.Call(uintptr(h))

	var written uint32
	r, _, err := procWritePrinter.Call(uintptr(h), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)))
	if r == 0 {
		return fmt.Errorf("enviar dados (%v)", err)
	}
	if int(written) != len(data) {
		return fmt.Errorf("envio incompleto (%d de %d bytes)", written, len(data))
	}
	return nil
}
