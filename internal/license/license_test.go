// Caminho: pdv/internal/license/license_test.go

package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"regexp"
	"testing"
)

func TestSignVerifyAndMachineID(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	text, err := Sign(priv, Data{Store: "Loja Ação", MachineID: "AB12", Expires: "2027-01-31"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := Verify(pub, text)
	if err != nil || d.Store != "Loja Ação" || d.ExpiresAt().Format("2006-01-02 15:04") != "2027-01-31 23:59" {
		t.Fatalf("ida e volta: %+v %v", d, err)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := Verify(other, text); err == nil {
		t.Fatal("aceitou assinatura de outra chave")
	}
	if _, err := Sign(priv, Data{Expires: "31/01/2027"}); err == nil {
		t.Fatal("aceitou validade fora do formato")
	}
	_ = PublicKey() // a chave embutida precisa ser válida

	id, err := MachineID()
	if err != nil || !regexp.MustCompile(`^[0-9A-F]{4}(-[0-9A-F]{4}){4}$`).MatchString(id) {
		t.Fatalf("ID da máquina: %q %v", id, err)
	}
	if again, _ := MachineID(); again != id {
		t.Fatal("ID da máquina mudou entre leituras")
	}
}
