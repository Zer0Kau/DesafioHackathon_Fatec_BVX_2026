// Package health implements the Health Score engine and the action recommender.
package health

import (
	"fmt"
	"math"
	"time"
)

const CurrentVersion = "v5.2"

const (
	StatusLow    = "BAIXO_RISCO"
	StatusMedium = "ATENCAO"
	StatusHigh   = "ALTO_RISCO"
)

const (
	ActionBackup = "URGENTE [Técnico]: Falha grave/atraso de Backup. Acessar servidor do PDV remotamente, checar espaço em disco e serviço de dump."
	ActionChurn  = "CRÍTICO [Comercial/CS]: Sinal forte de Churn (queda brusca de uso). Ligar para o proprietário imediatamente para pesquisa de satisfação."
	ActionTicket = "AÇÃO [Suporte N2]: Alto volume de chamados em versão defasada. Agendar atualização para a v5.2 para corrigir possíveis bugs conhecidos."
	ActionOK     = "Cliente saudável. Manter monitoramento preventivo padrão."
	ActionWatch  = "ATENÇÃO [CS]: Score abaixo do ideal sem falha crítica isolada. Revisar os sinais detalhados e agendar contato preventivo."
)

// Metrics are the raw signals of one client.
type Metrics struct {
	Version              string
	LastBackupDate       time.Time
	BackupSizeMB         float64
	PreviousBackupSizeMB float64
	BackupIntegrityOK    bool
	Sales7d              float64
	SalesAvgWeekly       float64
	Tickets30d           int
	DaysSinceInteraction int
}

type Pillar struct {
	Key    string  `json:"key"`
	Name   string  `json:"name"`
	Weight int     `json:"weight"`
	Score  int     `json:"score"`
	Points float64 `json:"points"`
	Detail string  `json:"detail"`
}

type Result struct {
	Score          int      `json:"health_score"`
	Status         string   `json:"status"`
	StatusLabel    string   `json:"status_label"`
	Pillars        []Pillar `json:"pillars"`
	BackupAgeHours float64  `json:"backup_age_hours"`
	BackupAgeDays  int      `json:"backup_age_days"`
	SizeAnomaly    bool     `json:"backup_size_anomaly"`
	UsageChangePct float64  `json:"usage_change_pct"` // negative = drop
	Actions        []string `json:"recommended_actions"`
}

func StatusFor(score int) (string, string) {
	switch {
	case score >= 75:
		return StatusLow, "BAIXO RISCO"
	case score >= 50:
		return StatusMedium, "ATENÇÃO"
	default:
		return StatusHigh, "ALTO RISCO"
	}
}

func versionScore(v string) int {
	switch v {
	case CurrentVersion:
		return 100
	case "v5.1":
		return 60
	default:
		return 10
	}
}

// Evaluate computes the 0-100 score, pillar breakdown and recommended actions.
func Evaluate(m Metrics, now time.Time) Result {
	r := Result{}

	// Backup (35%)
	hours := now.Sub(m.LastBackupDate).Hours()
	if hours < 0 {
		hours = 0
	}
	days := int(hours / 24)
	r.BackupAgeHours, r.BackupAgeDays = hours, days
	r.SizeAnomaly = m.PreviousBackupSizeMB > 0 && m.BackupSizeMB < 0.5*m.PreviousBackupSizeMB

	var bScore int
	var bDetail string
	switch {
	case !m.BackupIntegrityOK:
		bScore, bDetail = 0, "Backup com integridade inválida (checksum divergente)"
	case r.SizeAnomaly:
		bScore, bDetail = 0, fmt.Sprintf("Anomalia de tamanho: %.0fMB vs %.0fMB anterior (backup truncado)", m.BackupSizeMB, m.PreviousBackupSizeMB)
	case days <= 0:
		bScore, bDetail = 100, "Backup nas últimas 24h, íntegro"
	case days == 1:
		bScore, bDetail = 75, "Backup há 1 dia"
	case days <= 3:
		bScore, bDetail = 50, fmt.Sprintf("Backup atrasado há %d dias", days)
	case days <= 6:
		bScore, bDetail = 20, fmt.Sprintf("Backup atrasado há %d dias", days)
	default:
		bScore, bDetail = 0, fmt.Sprintf("Backup atrasado há %d dias", days)
	}

	// Sales / churn (30%)
	drop := 0.0
	if m.SalesAvgWeekly > 0 {
		drop = math.Round((m.SalesAvgWeekly-m.Sales7d)/m.SalesAvgWeekly*10000) / 100
	}
	r.UsageChangePct = -drop
	var sScore int
	switch {
	case drop >= 40:
		sScore = 0
	case drop >= 30:
		sScore = 20
	case drop >= 15:
		sScore = 60
	default:
		sScore = 100
	}
	sDetail := fmt.Sprintf("Vendas 7d: %+.0f%% vs média semanal", r.UsageChangePct)

	// Relationship (20%)
	var tScore int
	switch {
	case m.Tickets30d <= 2:
		tScore = 100
	case m.Tickets30d <= 5:
		tScore = 60
	default:
		tScore = 10
	}
	tDetail := fmt.Sprintf("%d chamados em 30d, última interação há %d dias", m.Tickets30d, m.DaysSinceInteraction)

	// Version (15%)
	vScore := versionScore(m.Version)

	r.Pillars = []Pillar{
		{"backup", "Backup", 35, bScore, float64(bScore) * 0.35, bDetail},
		{"sales", "Comportamento / Vendas", 30, sScore, float64(sScore) * 0.30, sDetail},
		{"tickets", "Relacionamento / Chamados", 20, tScore, float64(tScore) * 0.20, tDetail},
		{"version", "Versionamento", 15, vScore, float64(vScore) * 0.15, "Versão " + m.Version + " (oficial " + CurrentVersion + ")"},
	}
	total := 0.0
	for _, p := range r.Pillars {
		total += p.Points
	}
	r.Score = int(math.Round(total))
	r.Status, r.StatusLabel = StatusFor(r.Score)

	// Recommendations
	if days >= 3 || !m.BackupIntegrityOK || r.SizeAnomaly {
		r.Actions = append(r.Actions, ActionBackup)
	}
	if drop >= 30 {
		r.Actions = append(r.Actions, ActionChurn)
	}
	if m.Tickets30d >= 5 && m.Version != CurrentVersion {
		r.Actions = append(r.Actions, ActionTicket)
	}
	if len(r.Actions) == 0 {
		if r.Status == StatusLow {
			r.Actions = append(r.Actions, ActionOK)
		} else {
			r.Actions = append(r.Actions, ActionWatch)
		}
	}
	return r
}
