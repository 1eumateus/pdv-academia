// Caminho: pdv/internal/service/supplier_test.go

package service

import (
	"testing"

	"pdv/internal/models"
)

func TestSuppliers(t *testing.T) {
	e := newEnv(t)
	svc := NewSupplierService(NewBase(e.db))

	s, err := svc.Save(e.ctx, models.Supplier{LegalName: "Distribuidora Teste LTDA", Document: "11.222.333/0001-81",
		ZipCode: "01001-000", State: "sp", City: "São Paulo", Phone: "(11) 3333-4444"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Document != "11222333000181" || s.ZipCode != "01001000" || s.State != "SP" || !s.Active {
		t.Fatalf("normalização: %+v", s)
	}
	if _, err := svc.Save(e.ctx, models.Supplier{LegalName: "Outra", Document: "11222333000181"}); codeOf(err) != CodeDuplicate {
		t.Fatalf("CNPJ repetido: %v", err)
	}
	if _, err := svc.Save(e.ctx, models.Supplier{LegalName: "Outra", Document: "11222333000182"}); codeOf(err) != CodeInvalid {
		t.Fatalf("CNPJ inválido: %v", err)
	}
	if _, err := svc.Save(e.ctx, models.Supplier{LegalName: "Sem documento"}); err != nil {
		t.Fatalf("fornecedor sem documento: %v", err)
	}

	for _, term := range []string{"distribuidora", "11.222", "são paulo", "3333"} {
		if list, err := svc.List(e.ctx, term); err != nil || len(list) != 1 || list[0].ID != s.ID {
			t.Fatalf("busca %q: %d %v", term, len(list), err)
		}
	}

	s.Active = false
	if got, err := svc.Save(e.ctx, s); err != nil || got.Active {
		t.Fatalf("desativar: %+v %v", got, err)
	}

	op := e.user("caixa", models.RoleOperator)
	if _, err := svc.List(e.as(op), ""); codeOf(err) != CodeAuthRequired {
		t.Fatalf("operador sem a permissão compras: %v", err)
	}
}
