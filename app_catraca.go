// Caminho: catraca/app_catraca.go

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/notify"
)

// ---------------- Catraca (leitor facial iDFace) ----------------

func (a *App) GateStatus() (models.GateStatus, error) {
	return call(a, "ler situação da catraca", a.gate.Status)
}

// GateConfig devolve a configuração para editar (com a senha do iDFace).
func (a *App) GateConfig() (models.GateConfig, error) {
	return call(a, "ler configuração da catraca", a.gate.Config)
}

func (a *App) SaveGateConfig(c models.GateConfig) (models.GateConfig, error) {
	return call(a, "salvar configuração da catraca", func(ctx context.Context) (models.GateConfig, error) { return a.gate.SaveConfig(ctx, c) })
}

func (a *App) TestGateDevice() (int, error) {
	return call(a, "testar o iDFace", a.gate.TestDevice)
}

func (a *App) ListGateEvents(limit int) ([]models.GateEvent, error) {
	return call(a, "listar passagens", func(ctx context.Context) ([]models.GateEvent, error) { return a.gate.Events(ctx, limit) })
}

// GateEventReport: relatório de entradas na catraca (tela Catraca › Entradas).
func (a *App) GateEventReport(start, end, term, status string) ([]models.GateEvent, error) {
	return call(a, "relatório de entradas", func(ctx context.Context) ([]models.GateEvent, error) {
		return a.gate.EventReport(ctx, start, end, term, status)
	})
}

// GateCheckAndRelease verifica o aluno no balcão e, se pode, abre a catraca.
func (a *App) GateCheckAndRelease(customerID int64) (models.GateCheck, error) {
	return call(a, "liberar catraca", func(ctx context.Context) (models.GateCheck, error) { return a.gate.CheckAndRelease(ctx, customerID) })
}

// GateManualRelease abre a catraca sem aluno; o motivo fica registrado.
func (a *App) GateManualRelease(reason string) (bool, error) {
	return call(a, "liberar catraca", func(ctx context.Context) (bool, error) { return true, a.gate.ManualRelease(ctx, reason) })
}

func (a *App) ListGateMembers(term string) ([]models.GateMember, error) {
	return call(a, "listar alunos", func(ctx context.Context) ([]models.GateMember, error) { return a.gate.Members(ctx, term) })
}

func (a *App) SaveGateMember(in models.GateMemberInput) (models.GateMember, error) {
	return call(a, "salvar aluno na catraca", func(ctx context.Context) (models.GateMember, error) { return a.gate.SaveMember(ctx, in) })
}

// EnrollFace espera o aluno olhar para o leitor: prazo maior que o normal.
func (a *App) EnrollFace(customerID int64) (models.GateMember, error) {
	return call(a, "cadastrar rosto", func(ctx context.Context) (models.GateMember, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		return a.gate.EnrollFace(ctx, customerID)
	})
}

func (a *App) RemoveFace(customerID int64) (models.GateMember, error) {
	return call(a, "remover rosto", func(ctx context.Context) (models.GateMember, error) { return a.gate.RemoveFace(ctx, customerID) })
}

// MemberPhoto: foto do aluno no iDFace (data URL; "" = sem foto).
func (a *App) MemberPhoto(customerID int64) (string, error) {
	return call(a, "ler foto", func(ctx context.Context) (string, error) { return a.gate.Photo(ctx, customerID) })
}

// RemoveGateMember tira o aluno da academia (rosto, assinaturas ativas e cadastro ativo).
func (a *App) RemoveGateMember(customerID int64) (bool, error) {
	return callW(a, "apagar aluno", func(ctx context.Context) (bool, error) { return true, a.gate.RemoveMember(ctx, customerID) })
}

func (a *App) ListGateStaff() ([]models.GateMember, error) {
	return call(a, "listar funcionários", a.gate.Staff)
}

func (a *App) SaveGateStaff(in models.GateStaffInput) (models.GateMember, error) {
	return callW(a, "salvar funcionário", func(ctx context.Context) (models.GateMember, error) { return a.gate.SaveStaff(ctx, in) })
}

func (a *App) RemoveGateStaff(customerID int64) (models.GateMember, error) {
	return callW(a, "desligar funcionário", func(ctx context.Context) (models.GateMember, error) { return a.gate.RemoveStaff(ctx, customerID) })
}

func (a *App) ImportFromIDFace() (models.GateImport, error) {
	return call(a, "importar do iDFace", func(ctx context.Context) (models.GateImport, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		return a.gate.ImportFromDevice(ctx)
	})
}

// NotifyAccess mostra a passagem na notificação do Windows. A tela chama só
// quando a janela do programa não está na frente (senão já aparece o aviso dela).
func (a *App) NotifyAccess(e models.GateEvent) (bool, error) {
	return call(a, "notificar passagem", func(ctx context.Context) (bool, error) {
		title := e.Name + " · NEGADO"
		if e.Allowed {
			title = e.Name + " · LIBERADO"
		}
		icon := ""
		if e.CustomerID != nil {
			icon = a.photoFile(ctx, *e.CustomerID)
		}
		return true, notify.Show(title, e.Reason, icon)
	})
}

// photoFile grava a foto do iDFace numa pasta temporária (a notificação do
// Windows só aceita arquivo). Sem foto ou com erro: "".
func (a *App) photoFile(ctx context.Context, customerID int64) string {
	url, err := a.gate.Photo(ctx, customerID)
	head, data, ok := strings.Cut(url, ";base64,")
	if err != nil || !ok {
		return ""
	}
	img, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return ""
	}
	ext := ".jpg"
	if strings.HasSuffix(head, "png") {
		ext = ".png"
	}
	dir := filepath.Join(os.TempDir(), "pdv-academia-fotos")
	path := filepath.Join(dir, fmt.Sprint(customerID)+ext)
	if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(path, img, 0o644) != nil {
		return ""
	}
	return path
}
