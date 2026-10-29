// Caminho: pdv/internal/service/settings_service.go

package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"

	"pdv/internal/models"
	"pdv/internal/pix"
	"pdv/internal/repository"
)

const (
	maxLogoLen            = 400_000 // data URL; ~300 KB de imagem
	maxFooterLen          = 300
	maxBackgroundLen      = 2_000_000 // data URL; a tela reduz a imagem antes de enviar
	defaultQuoteDays      = 7
	defaultLowStock       = 5
	maxLowStock           = 100000
	defaultStoreName      = "Meu Estabelecimento"
	maxQuoteDays          = 365
	maxPrinterNameLen     = 260
	maxStateRegistryLen   = 20
	defaultMaxDiscountPct = 10
	maxLockMinutes        = 240
	defaultBackupKeep     = 7
	maxBackupKeep         = 90
	defaultBackupWarnDays = 3
	maxBackupWarnDays     = 60
	maxChargeMessageLen   = 1000
)

// Mensagens padrão do WhatsApp (assinaturas e aniversário) até a loja escrever as suas.
const (
	DefaultMsgBirthday = "Feliz aniversário, {nome}! 🎉 Toda a equipe da {loja} deseja um dia incrível. Conte com a gente!"
	DefaultMsgDue      = "Olá, {nome}! Aqui é da {loja}. Passando para lembrar que a mensalidade do plano {plano} ({valor}) vence em {vencimento}. Obrigado!"
	DefaultMsgOverdue  = "Olá, {nome}! Aqui é da {loja}. A mensalidade do plano {plano} ({valor}) venceu em {vencimento}. Podemos ajudar com o pagamento?"
	DefaultMsgMissing  = "Oi, {nome}! Sentimos sua falta aqui na {loja}. Que tal voltar a treinar? Estamos te esperando!"
	DefaultMsgWinback  = "Oi, {nome}! Aqui é da {loja}. Temos novidades e adoraríamos ter você de volta. Vamos conversar?"

	defaultDueDays     = 3
	defaultMissingDays = 15
)

// DefaultChargeMessage: cobrança do a prazo pelo WhatsApp até a loja escrever a sua.
const DefaultChargeMessage = "Olá, {nome}! Aqui é da {loja}. Consta em aberto {saldo} nas suas compras a prazo. Podemos combinar o pagamento? Obrigado!"

// intSetting lê um inteiro da tabela settings, com padrão se ausente ou inválido.
func intSetting(m map[string]string, key string, def int64) int64 {
	if v, err := strconv.ParseInt(m[key], 10, 64); err == nil {
		return v
	}
	return def
}

type SettingsService struct {
	Base
	settings repository.SettingsRepo
}

func NewSettingsService(b Base) *SettingsService { return &SettingsService{Base: b} }

// textFields liga cada campo de texto à sua chave na tabela settings.
func textFields(s *models.Settings) map[string]*string {
	return map[string]*string{
		"store_name": &s.StoreName, "legal_name": &s.LegalName, "document": &s.Document,
		"state_registration": &s.StateRegistration, "phone": &s.Phone, "whatsapp": &s.Whatsapp,
		"email": &s.Email, "zip_code": &s.ZipCode, "street": &s.Street, "number": &s.Number,
		"complement": &s.Complement, "district": &s.District, "city": &s.City, "state": &s.State,
		"logo": &s.Logo, "receipt_footer": &s.ReceiptFooter, "paper_width": &s.PaperWidth,
		"printer_name": &s.PrinterName, "pix_key": &s.PixKey, "pix_name": &s.PixName, "pix_city": &s.PixCity,
		"backup_external_dir": &s.BackupExternalDir, "backup_dir": &s.BackupDir, "charge_message": &s.ChargeMessage,
		"msg_birthday": &s.MsgBirthday, "msg_due": &s.MsgDue, "msg_overdue": &s.MsgOverdue, "msg_missing": &s.MsgMissing,
		"msg_winback": &s.MsgWinback,
	}
}

func boolFields(s *models.Settings) map[string]*bool {
	return map[string]*bool{"auto_receipt": &s.AutoReceipt, "cut_paper": &s.CutPaper, "open_drawer": &s.OpenDrawer,
		"nfe_cost_freight": &s.NFeCostFreight, "nfe_cost_ipi": &s.NFeCostIPI, "nfe_cost_st": &s.NFeCostST,
		"nfe_cost_other": &s.NFeCostOther, "nfe_cost_discount": &s.NFeCostDiscount, "pix_on_receipt": &s.PixOnReceipt,
		"backup_auto": &s.BackupAuto, "setup_done": &s.SetupDone, "cash_per_user": &s.CashPerUser}
}

func (s *SettingsService) Get(ctx context.Context) (models.Settings, error) {
	m, err := s.settings.All(ctx, s.DB)
	if err != nil {
		return models.Settings{}, err
	}
	// Padrões para quem nunca salvou: corta o papel e mantém o rodapé vazio.
	out := models.Settings{CutPaper: true, PaperWidth: "80", BackupAuto: true,
		NFeCostFreight: true, NFeCostIPI: true, NFeCostST: true, NFeCostOther: true, NFeCostDiscount: true}
	for k, p := range textFields(&out) {
		if v, ok := m[k]; ok {
			*p = v
		}
	}
	for k, p := range boolFields(&out) {
		if v, ok := m[k]; ok {
			*p = v == "1"
		}
	}
	out.QuoteValidityDays = intSetting(m, "quote_validity_days", defaultQuoteDays)
	out.LowStockDefault = intSetting(m, "low_stock_default", defaultLowStock)
	out.LockMinutes = intSetting(m, "lock_minutes", 0)
	out.OperatorMaxDiscountPct = intSetting(m, "operator_max_discount_pct", defaultMaxDiscountPct)
	if strings.TrimSpace(out.ChargeMessage) == "" {
		out.ChargeMessage = DefaultChargeMessage
	}
	for p, def := range map[*string]string{&out.MsgBirthday: DefaultMsgBirthday, &out.MsgDue: DefaultMsgDue,
		&out.MsgOverdue: DefaultMsgOverdue, &out.MsgMissing: DefaultMsgMissing, &out.MsgWinback: DefaultMsgWinback} {
		if strings.TrimSpace(*p) == "" {
			*p = def
		}
	}
	out.DueDays = intSetting(m, "due_days", defaultDueDays)
	out.MissingDays = intSetting(m, "missing_days", defaultMissingDays)
	out.BackupKeep = intSetting(m, "backup_keep", defaultBackupKeep)
	out.BackupWarnDays = intSetting(m, "backup_warn_days", defaultBackupWarnDays)
	if out.StoreName == "" {
		out.StoreName = defaultStoreName
	}
	return out, nil
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// validateSettings normaliza os dados da loja. Endereço, documento, e-mail e
// telefone seguem exatamente as regras do cadastro de clientes.
func validateSettings(in models.Settings) (models.Settings, error) {
	out := in
	var err error
	if out.StoreName, err = cleanName(in.StoreName, "nome do estabelecimento"); err != nil {
		return out, err
	}
	opt := func(v string) *string { return &v }
	f, err := validateCustomer(models.CustomerInput{
		Name: out.StoreName, Phone: opt(in.Phone), Document: opt(in.Document), Email: opt(in.Email),
		ZipCode: opt(in.ZipCode), Street: opt(in.Street), Number: opt(in.Number), Complement: opt(in.Complement),
		District: opt(in.District), City: opt(in.City), State: opt(in.State),
	})
	if err != nil {
		return out, err
	}
	out.Phone, out.Document, out.Email, out.ZipCode = str(f.Phone), str(f.Document), str(f.Email), str(f.ZipCode)
	out.Street, out.Number, out.Complement = str(f.Street), str(f.Number), str(f.Complement)
	out.District, out.City, out.State = str(f.District), str(f.City), str(f.State)

	checks := []struct {
		dst   *string
		label string
		max   int
	}{
		{&out.LegalName, "razão social", MaxNameLen},
		{&out.StateRegistration, "inscrição estadual", maxStateRegistryLen},
		{&out.Whatsapp, "WhatsApp", maxPhoneLen},
		{&out.ReceiptFooter, "mensagem do rodapé", maxFooterLen},
		{&out.PrinterName, "nome da impressora", maxPrinterNameLen},
	}
	for _, c := range checks {
		v, err := optText(c.dst, c.label, c.max)
		if err != nil {
			return out, err
		}
		*c.dst = str(v)
	}

	if strings.TrimSpace(out.PixKey) != "" {
		k, err := pix.NormalizeKey(out.PixKey, func(d string) bool { return ValidCPF(d) || ValidCNPJ(d) })
		if err != nil {
			return out, invalid("%s.", err.Error())
		}
		out.PixKey = k
		out.PixName, out.PixCity = pix.ASCII(out.PixName), pix.ASCII(out.PixCity)
		if out.PixName == "" {
			out.PixName = pix.ASCII(out.StoreName)
		}
		if out.PixCity == "" {
			out.PixCity = pix.ASCII(out.City)
		}
		if out.PixName == "" || out.PixCity == "" {
			return out, invalid("Informe o nome do recebedor e a cidade do Pix.")
		}
		if len(out.PixName) > 25 || len(out.PixCity) > 15 {
			return out, invalid("No Pix, o nome do recebedor vai até 25 letras e a cidade até 15 (sem acento).")
		}
	} else {
		out.PixKey, out.PixName, out.PixCity = "", "", ""
	}

	switch out.PaperWidth {
	case "58", "80", "a4":
	default:
		return out, invalid("Tamanho de papel inválido: use 58 mm, 80 mm ou A4.")
	}
	if out.QuoteValidityDays < 1 || out.QuoteValidityDays > maxQuoteDays {
		return out, invalid("A validade do orçamento deve ser de 1 a %d dias.", maxQuoteDays)
	}
	if out.LowStockDefault < 0 || out.LowStockDefault > maxLowStock {
		return out, invalid("O estoque baixo padrão deve ser de 0 (desligado) a %d unidades.", maxLowStock)
	}
	if out.LockMinutes < 0 || out.LockMinutes > maxLockMinutes {
		return out, invalid("O bloqueio por inatividade deve ser de 0 (desligado) a %d minutos.", maxLockMinutes)
	}
	if out.OperatorMaxDiscountPct < 0 || out.OperatorMaxDiscountPct > 100 {
		return out, invalid("O desconto máximo do operador deve ser de 0%% a 100%%.")
	}
	// 0 = não informado (quem salva sem conhecer estes campos): fica o padrão.
	if out.BackupKeep == 0 {
		out.BackupKeep = defaultBackupKeep
	}
	if out.BackupWarnDays == 0 {
		out.BackupWarnDays = defaultBackupWarnDays
	}
	if out.BackupKeep < 1 || out.BackupKeep > maxBackupKeep {
		return out, invalid("Guarde de 1 a %d backups automáticos.", maxBackupKeep)
	}
	if out.BackupWarnDays < 1 || out.BackupWarnDays > maxBackupWarnDays {
		return out, invalid("O aviso de cópia externa deve ser de 1 a %d dias.", maxBackupWarnDays)
	}
	out.ChargeMessage = strings.TrimSpace(out.ChargeMessage)
	if len([]rune(out.ChargeMessage)) > maxChargeMessageLen {
		return out, invalid("A mensagem de cobrança deve ter no máximo %d caracteres.", maxChargeMessageLen)
	}
	for _, p := range []*string{&out.MsgBirthday, &out.MsgDue, &out.MsgOverdue, &out.MsgMissing, &out.MsgWinback} {
		if *p = strings.TrimSpace(*p); len([]rune(*p)) > maxChargeMessageLen {
			return out, invalid("Cada mensagem do WhatsApp deve ter no máximo %d caracteres.", maxChargeMessageLen)
		}
	}
	if out.DueDays == 0 {
		out.DueDays = defaultDueDays
	}
	if out.MissingDays == 0 {
		out.MissingDays = defaultMissingDays
	}
	if out.DueDays < 1 || out.DueDays > 60 || out.MissingDays < 1 || out.MissingDays > 365 {
		return out, invalid("\"A vencer\" deve ser de 1 a 60 dias e \"inativo\" de 1 a 365 dias.")
	}
	out.BackupDir = strings.TrimSpace(out.BackupDir)
	if out.BackupDir != "" && !filepath.IsAbs(out.BackupDir) {
		return out, invalid("Escolha a pasta do backup pelo botão \"Escolher pasta\".")
	}
	out.BackupExternalDir = strings.TrimSpace(out.BackupExternalDir)
	if out.BackupExternalDir != "" && !filepath.IsAbs(out.BackupExternalDir) {
		return out, invalid("Escolha a pasta da cópia externa pelo botão \"Escolher pasta\".")
	}
	out.Logo = strings.TrimSpace(out.Logo)
	if out.Logo != "" {
		if len(out.Logo) > maxLogoLen {
			return out, invalid("A logo é grande demais. Use uma imagem de até 300 KB.")
		}
		if !strings.HasPrefix(out.Logo, "data:image/png;base64,") && !strings.HasPrefix(out.Logo, "data:image/jpeg;base64,") {
			return out, invalid("A logo deve ser uma imagem PNG ou JPEG.")
		}
	}
	return out, nil
}

// PixQR gera o "copia e cola" e o QR do valor com a chave da loja. txid
// identifica a cobrança (letras e números, até 25).
func (s *SettingsService) PixQR(ctx context.Context, amountCents int64, txid string) (models.PixCode, error) {
	st, err := s.Get(ctx)
	if err != nil {
		return models.PixCode{}, err
	}
	if st.PixKey == "" {
		return models.PixCode{}, invalid("Configure a chave Pix da loja em Configurações.")
	}
	payload, err := pix.Payload(st.PixKey, st.PixName, st.PixCity, amountCents, txid)
	if err != nil {
		return models.PixCode{}, invalid("%s.", err.Error())
	}
	img, err := pix.QRDataURL(payload, 320)
	if err != nil {
		return models.PixCode{}, err
	}
	return models.PixCode{Payload: payload, Image: img}, nil
}

func (s *SettingsService) Save(ctx context.Context, in models.Settings) (models.Settings, error) {
	clean, err := validateSettings(in)
	if err != nil {
		return models.Settings{}, err
	}
	before, err := s.Get(ctx)
	if err != nil {
		return models.Settings{}, err
	}
	authBy, err := requirePerm(ctx, s.Now(), PermSettings)
	if err != nil {
		return models.Settings{}, err
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		// Trocar o modo do caixa com caixa aberto deixaria esse caixa sem dono certo.
		if clean.CashPerUser != before.CashPerUser {
			n, err := repository.CashRepo{}.CountOpen(ctx, tx)
			if err != nil {
				return err
			}
			if n > 0 {
				return newErr(CodeStateConflict, "Feche todos os caixas antes de mudar entre caixa geral e caixa por usuário.")
			}
		}
		// A logo é grande: na auditoria só registra se mudou.
		b, a := before, clean
		if b.Logo != a.Logo {
			b.Logo, a.Logo = "(anterior)", "(nova)"
		} else {
			b.Logo, a.Logo = "", ""
		}
		if err := audit(ctx, tx, s.Now(), authBy, "configuracoes_alteradas", "configuracoes", nil, b, a); err != nil {
			return err
		}
		for k, p := range textFields(&clean) {
			if err := s.settings.Set(ctx, tx, k, *p); err != nil {
				return err
			}
		}
		for k, p := range boolFields(&clean) {
			v := "0"
			if *p {
				v = "1"
			}
			if err := s.settings.Set(ctx, tx, k, v); err != nil {
				return err
			}
		}
		ints := map[string]int64{"quote_validity_days": clean.QuoteValidityDays, "lock_minutes": clean.LockMinutes,
			"operator_max_discount_pct": clean.OperatorMaxDiscountPct, "low_stock_default": clean.LowStockDefault,
			"backup_keep": clean.BackupKeep, "backup_warn_days": clean.BackupWarnDays,
			"due_days": clean.DueDays, "missing_days": clean.MissingDays}
		for k, v := range ints {
			if err := s.settings.Set(ctx, tx, k, strconv.FormatInt(v, 10)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return models.Settings{}, err
	}
	return s.Get(ctx)
}

// Backgrounds lê as imagens de fundo (login e sistema). Sem login: a tela de
// entrada também usa.
func (s *SettingsService) Backgrounds(ctx context.Context) (models.Backgrounds, error) {
	m, err := s.settings.All(ctx, s.DB)
	return models.Backgrounds{Login: m["bg_login"], App: m["bg_app"]}, err
}

// SaveBackgrounds grava as duas imagens de fundo (vazio remove).
func (s *SettingsService) SaveBackgrounds(ctx context.Context, in models.Backgrounds) (models.Backgrounds, error) {
	for _, v := range []string{in.Login, in.App} {
		if v == "" {
			continue
		}
		if !strings.HasPrefix(v, "data:image/png;base64,") && !strings.HasPrefix(v, "data:image/jpeg;base64,") {
			return models.Backgrounds{}, invalid("A imagem de fundo deve ser PNG ou JPEG.")
		}
		if len(v) > maxBackgroundLen {
			return models.Backgrounds{}, invalid("A imagem de fundo é grande demais.")
		}
	}
	before, err := s.Backgrounds(ctx)
	if err != nil {
		return models.Backgrounds{}, err
	}
	authBy, err := requirePerm(ctx, s.Now(), PermSettings)
	if err != nil {
		return models.Backgrounds{}, err
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		// As imagens são grandes: a auditoria registra só o que mudou.
		changed := map[string]bool{"login": before.Login != in.Login, "sistema": before.App != in.App}
		if err := audit(ctx, tx, s.Now(), authBy, "fundo_alterado", "configuracoes", nil, nil, changed); err != nil {
			return err
		}
		if err := s.settings.Set(ctx, tx, "bg_login", in.Login); err != nil {
			return err
		}
		return s.settings.Set(ctx, tx, "bg_app", in.App)
	})
	if err != nil {
		return models.Backgrounds{}, err
	}
	return in, nil
}
