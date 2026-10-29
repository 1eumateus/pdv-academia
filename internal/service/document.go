// Caminho: pdv/internal/service/document.go

package service

import "strings"

// NormalizeDocument remove pontuação e espaços e passa letras para maiúsculas
// (o CNPJ alfanumérico, emitido desde julho de 2026, tem letras).
func NormalizeDocument(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func allSame(d string) bool {
	return strings.Count(d, d[:1]) == len(d)
}

// checkDigit calcula um dígito verificador com os pesos informados (módulo 11).
// Cada caractere vale seu código ASCII menos 48: '0'..'9' = 0..9 e 'A'..'Z' =
// 17..42, regra da Receita para o CNPJ alfanumérico.
func checkDigit(d string, weights []int) byte {
	sum := 0
	for i, w := range weights {
		sum += (int(d[i]) - '0') * w
	}
	r := sum % 11
	if r < 2 {
		return '0'
	}
	return byte('0' + 11 - r)
}

// ValidCPF confere os dois dígitos verificadores de um CPF (só dígitos).
func ValidCPF(d string) bool {
	if len(d) != 11 || onlyDigits(d) != d || allSame(d) {
		return false
	}
	return checkDigit(d, []int{10, 9, 8, 7, 6, 5, 4, 3, 2}) == d[9] &&
		checkDigit(d, []int{11, 10, 9, 8, 7, 6, 5, 4, 3, 2}) == d[10]
}

// ValidCNPJ confere um CNPJ numérico ou alfanumérico já normalizado: 12
// caracteres [0-9A-Z] seguidos de 2 dígitos verificadores.
func ValidCNPJ(d string) bool {
	if len(d) != 14 || allSame(d) || onlyDigits(d[12:]) != d[12:] || NormalizeDocument(d) != d {
		return false
	}
	return checkDigit(d, []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) == d[12] &&
		checkDigit(d, []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) == d[13]
}

var brazilianStates = map[string]bool{
	"AC": true, "AL": true, "AP": true, "AM": true, "BA": true, "CE": true, "DF": true, "ES": true, "GO": true,
	"MA": true, "MT": true, "MS": true, "MG": true, "PA": true, "PB": true, "PR": true, "PE": true, "PI": true,
	"RJ": true, "RN": true, "RS": true, "RO": true, "RR": true, "SC": true, "SP": true, "SE": true, "TO": true,
}
