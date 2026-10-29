// Caminho: catraca/internal/models/catraca.go

package models

import "time"

// Quem abre o giro quando o acesso é liberado.
const (
	GateRelayIDFace = "idface" // o próprio leitor (SecBox ou relé interno)
	GateRelaySerial = "serial" // placa de relé na porta serial (catraca eletromecânica)
	GateRelayNone   = "nenhum" // só registra
)

// GateConfig: configuração da catraca (tabela settings, chaves catraca_*).
type GateConfig struct {
	Relay string `json:"relay"`
	// ServerPort: porta em que o programa atende o modo online do iDFace.
	ServerPort int64 `json:"serverPort"`
	// ToleranceDays: dias depois do vencimento da mensalidade em que o aluno ainda passa.
	ToleranceDays  int64  `json:"toleranceDays"`
	DeviceHost     string `json:"deviceHost"` // ip ou ip:porta do iDFace
	DeviceLogin    string `json:"deviceLogin"`
	DevicePassword string `json:"devicePassword"`
	// SecBoxID: módulo de acionamento externo do iDFace (0 = relé interno).
	SecBoxID     int64  `json:"secBoxId"`
	SerialPort   string `json:"serialPort"`
	SerialSignal string `json:"serialSignal"` // DTR ou RTS
	PulseMs      int64  `json:"pulseMs"`
}

// GateStatus alimenta o topo da tela da catraca.
type GateStatus struct {
	Config        GateConfig `json:"config"`
	ServerRunning bool       `json:"serverRunning"`
	ServerError   string     `json:"serverError"`
	// DeviceLastSeen: última chamada do iDFace (nil = ainda não chamou).
	DeviceLastSeen *time.Time `json:"deviceLastSeen"`
	LocalIPs       []string   `json:"localIps"`
	SerialPorts    []string   `json:"serialPorts"`
	AllowedToday   int64      `json:"allowedToday"`
	DeniedToday    int64      `json:"deniedToday"`
}

// AccessRules: quando o aluno pode entrar (zero = sem restrição). Ficam no
// plano; a assinatura só tem as suas quando é exceção.
type AccessRules struct {
	// Weekdays: dias permitidos, 1 = segunda … 7 = domingo (vazio = todos).
	Weekdays    []int64 `json:"weekdays"`
	WeeklyLimit int64   `json:"weeklyLimit"` // dias com entrada por semana (seg a dom)
	DailyLimit  int64   `json:"dailyLimit"`  // passagens no mesmo dia
}

// GatePlan: uma assinatura ativa do aluno, do jeito que a catraca precisa.
type GatePlan struct {
	Name  string      `json:"name"`
	Rules AccessRules `json:"rules"`
	// DueDate: vencimento da mensalidade em aberto (AAAA-MM-DD; "" = nenhuma).
	DueDate string `json:"dueDate"`
	Paused  bool   `json:"paused"`
	// EverPaid: já teve alguma mensalidade paga (a primeira em aberto não libera).
	EverPaid bool `json:"everPaid"`
}

// GateMember: cliente do PDV visto pela catraca (aluno).
type GateMember struct {
	CustomerID int64   `json:"customerId"`
	Name       string  `json:"name"`
	Phone      *string `json:"phone"`
	Active     bool    `json:"active"`
	// DeviceUserID: id no leitor facial (nil = rosto não cadastrado).
	DeviceUserID *int64 `json:"deviceUserId"`
	Blocked      bool   `json:"blocked"`
	// FreeUntil: cortesia até (AAAA-MM-DD; "" = sem cortesia).
	FreeUntil string     `json:"freeUntil"`
	Notes     string     `json:"notes"`
	Plans     []GatePlan `json:"plans"`
	// Staff: funcionário (passa sempre); Role: a função dele.
	Staff bool   `json:"staff"`
	Role  string `json:"role"`
	// Uso da semana (para as regras do plano): dias com entrada desde segunda
	// e passagens liberadas hoje.
	DaysThisWeek int64 `json:"daysThisWeek"`
	EntriesToday int64 `json:"entriesToday"`
	// Allowed e Reason: o que a catraca faria agora.
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	// LastAccess: última passagem liberada (nil = nunca passou).
	LastAccess *time.Time `json:"lastAccess"`
}

// GateMemberInput: o que o balcão altera na situação do aluno na catraca.
type GateMemberInput struct {
	CustomerID int64  `json:"customerId"`
	Blocked    bool   `json:"blocked"`
	FreeUntil  string `json:"freeUntil"`
	Notes      string `json:"notes"`
}

// GateStaffInput: cadastro de funcionário. CustomerID 0 = pessoa nova (Name
// obrigatório); senão, o cliente existente vira funcionário.
type GateStaffInput struct {
	CustomerID int64  `json:"customerId"`
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	Role       string `json:"role"`
}

// GateEvent: uma passagem pela catraca.
type GateEvent struct {
	ID         int64     `json:"id"`
	At         time.Time `json:"at"`
	CustomerID *int64    `json:"customerId"`
	Name       string    `json:"name"`
	Allowed    bool      `json:"allowed"`
	Reason     string    `json:"reason"`
	Source     string    `json:"source"` // leitor, balcao, manual
	UserName   string    `json:"userName"`
}

// GateCheck: resposta da verificação feita no balcão.
type GateCheck struct {
	Allowed  bool   `json:"allowed"`
	Reason   string `json:"reason"`
	Released bool   `json:"released"`
}

// GateImport: resultado de trazer as pessoas já cadastradas no iDFace.
type GateImport struct {
	DeviceUsers int `json:"deviceUsers"`
	Linked      int `json:"linked"`  // ligadas a um cliente existente
	Created     int `json:"created"` // viraram cliente novo
	Already     int `json:"already"`
}
