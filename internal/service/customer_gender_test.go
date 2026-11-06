// Caminho: catraca/internal/service/customer_gender_test.go

package service

import (
	"testing"

	"pdv/internal/models"
)

func TestCustomerGender(t *testing.T) {
	e := newEnv(t)
	bad := "x"
	if _, err := e.customers.Create(e.ctx, models.CustomerInput{Name: "Bia", Active: true, Gender: &bad}); codeOf(err) != CodeInvalid {
		t.Fatalf("gênero inválido: %v", err)
	}
	g := " Feminino "
	c, err := e.customers.Create(e.ctx, models.CustomerInput{Name: "Ana", Active: true, Gender: &g})
	if err != nil || c.Gender == nil || *c.Gender != "feminino" {
		t.Fatalf("gênero no cadastro: %v %v", err, c.Gender)
	}
	// nil mantém; "" apaga.
	in := models.CustomerInput{ID: c.ID, Name: "Ana", Active: true}
	if c, _ = e.customers.Update(e.ctx, in); c.Gender == nil || *c.Gender != "feminino" {
		t.Fatalf("sem mexer, o gênero deveria ficar: %v", c.Gender)
	}
	empty := ""
	in.Gender = &empty
	if c, _ = e.customers.Update(e.ctx, in); c.Gender != nil {
		t.Fatalf("vazio deveria apagar: %v", *c.Gender)
	}
}
