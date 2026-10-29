// Caminho: pdv/internal/service/supplier_service.go

package service

import (
	"context"
	"database/sql"

	"pdv/internal/models"
	"pdv/internal/repository"
)

type SupplierService struct {
	Base
	suppliers repository.SupplierRepo
}

func NewSupplierService(b Base) *SupplierService { return &SupplierService{Base: b} }

// validateSupplier usa as mesmas regras do cadastro de cliente para documento,
// e-mail, telefone e endereço.
func validateSupplier(in models.Supplier) (models.Supplier, error) {
	out := in
	var err error
	if out.LegalName, err = cleanName(in.LegalName, "razão social"); err != nil {
		return out, err
	}
	opt := func(v string) *string { return &v }
	f, err := validateCustomer(models.CustomerInput{
		Name: out.LegalName, Phone: opt(in.Phone), Document: opt(in.Document), Email: opt(in.Email),
		ZipCode: opt(in.ZipCode), Street: opt(in.Street), Number: opt(in.Number), Complement: opt(in.Complement),
		District: opt(in.District), City: opt(in.City), State: opt(in.State), Notes: opt(in.Notes),
	})
	if err != nil {
		return out, err
	}
	out.Phone, out.Document, out.Email, out.ZipCode = str(f.Phone), str(f.Document), str(f.Email), str(f.ZipCode)
	out.Street, out.Number, out.Complement = str(f.Street), str(f.Number), str(f.Complement)
	out.District, out.City, out.State, out.Notes = str(f.District), str(f.City), str(f.State), str(f.Notes)
	for _, c := range []struct {
		dst   *string
		label string
		max   int
	}{
		{&out.TradeName, "nome fantasia", MaxNameLen},
		{&out.StateRegistration, "inscrição estadual", maxStateRegistryLen},
		{&out.Contact, "contato", MaxNameLen},
	} {
		v, err := optText(c.dst, c.label, c.max)
		if err != nil {
			return out, err
		}
		*c.dst = str(v)
	}
	return out, nil
}

const duplicateSupplierMsg = "Já existe um fornecedor com este CPF/CNPJ."

func (s *SupplierService) List(ctx context.Context, term string) ([]models.Supplier, error) {
	if _, err := requirePerm(ctx, s.Now(), PermPurchases); err != nil {
		return nil, err
	}
	return s.suppliers.List(ctx, s.DB, trimOrEmpty(term))
}

// Save cadastra (ID 0) ou altera um fornecedor.
func (s *SupplierService) Save(ctx context.Context, in models.Supplier) (models.Supplier, error) {
	v, err := validateSupplier(in)
	if err != nil {
		return models.Supplier{}, err
	}
	if v.ID == 0 {
		v.Active = true
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if _, err := requirePerm(ctx, now, PermPurchases); err != nil {
			return err
		}
		action := "fornecedor_alterado"
		if v.ID == 0 {
			action = "fornecedor_cadastrado"
			if v.ID, err = s.suppliers.Insert(ctx, tx, v, now); err != nil {
				return fromRepo(err, nil, duplicateSupplierMsg)
			}
		} else if err := s.suppliers.Update(ctx, tx, v, now); err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Fornecedor %d não encontrado.", v.ID), duplicateSupplierMsg)
		}
		return audit(ctx, tx, now, nil, action, "fornecedor", &v.ID, nil,
			map[string]any{"razao_social": v.LegalName, "documento": v.Document, "ativo": v.Active})
	})
	if err != nil {
		return models.Supplier{}, err
	}
	return s.suppliers.GetByID(ctx, s.DB, v.ID)
}
