// Caminho: catraca/internal/models/purge.go

package models

import "time"

// TestPurge: prévia (ou resultado) de apagar as vendas de teste de um operador
// (UserID 0 = todos) num dia, junto com o caixa que ele abriu nesse dia.
type TestPurge struct {
	Date       string             `json:"date"` // AAAA-MM-DD
	UserID     int64              `json:"userId"`
	Sales      []TestPurgeSale    `json:"sales"`
	Sessions   []TestPurgeSession `json:"sessions"`
	TotalCents int64              `json:"totalCents"` // vendas concluídas
	// TouchedSessions: caixas que ficam, mas perdem os lançamentos dessas vendas.
	TouchedSessions []int64 `json:"touchedSessions"`
	// Blockers: o que impede apagar (devolução ou fiado já recebido dessas vendas).
	Blockers []string `json:"blockers"`
	// BackupPath: backup feito antes de apagar (só no resultado).
	BackupPath string `json:"backupPath"`
	Applied    bool   `json:"applied"`
}

type TestPurgeSale struct {
	ID            int64     `json:"id"`
	At            time.Time `json:"at"`
	TotalCents    int64     `json:"totalCents"`
	PaymentMethod string    `json:"paymentMethod"`
	Status        string    `json:"status"`
	CustomerName  string    `json:"customerName"`
}

type TestPurgeSession struct {
	ID        int64     `json:"id"`
	OpenedAt  time.Time `json:"openedAt"`
	Status    string    `json:"status"`
	Movements int64     `json:"movements"`
}
