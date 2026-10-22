package main

import (
	"fmt"
	"time"
)

type seed struct {
	id, name, segment, city, version string
	backupHoursAgo                   float64
	size, prev                       float64
	ok                               bool
	sales7, salesAvg                 float64
	tickets, interaction             int
}

var seeds = []seed{
	{"alvorada-matriz", "Supermercado Alvorada (Matriz)", "Supermercado", "Santa Cruz do Rio Pardo", "v5.0", 7 * 24, 380, 380, true, 60000, 100000, 8, 5},
	{"padaria-estrela", "Padaria & Confeitaria Estrela (Ourinhos)", "Padaria", "Ourinhos", "v5.2", 2, 450, 450, true, 12500, 10000, 1, 35},
	{"mercadao-vila-nova", "Mercadão Vila Nova", "Supermercado", "Marília", "v5.2", 5, 820, 810, true, 98000, 100000, 2, 10},
	{"posto-rota-22", "Posto Rota 22", "Posto", "Assis", "v5.1", 20, 300, 295, true, 41000, 40000, 1, 20},
	{"restaurante-sabor-terra", "Restaurante Sabor da Terra", "Restaurante", "Bauru", "v5.2", 8, 150, 148, true, 21000, 20000, 0, 12},
	{"padaria-pao-quente", "Padaria Pão Quente", "Padaria", "Assis", "v5.1", 55, 210, 205, true, 17000, 20000, 3, 18},
	{"supermercado-bom-preco", "Supermercado Bom Preço", "Supermercado", "Bauru", "v5.2", 30, 640, 630, true, 130000, 150000, 4, 9},
	{"posto-ipiranga-centro", "Posto Centro Norte", "Posto", "Marília", "v5.0", 100, 260, 260, true, 52000, 70000, 5, 25},
	{"restaurante-cantina-nona", "Cantina da Nonna", "Restaurante", "Ourinhos", "v5.2", 4, 120, 118, true, 26000, 24000, 1, 7},
	{"mercearia-santa-rita", "Mercearia Santa Rita", "Mercearia", "Santa Cruz do Rio Pardo", "v5.1", 10, 90, 88, true, 9000, 9200, 2, 40},
	{"padaria-doce-vida", "Padaria Doce Vida", "Padaria", "Bauru", "v5.2", 3, 12, 180, true, 14000, 14500, 1, 14},
	{"posto-bandeirantes", "Auto Posto Bandeirantes", "Posto", "Assis", "v5.0", 140, 310, 300, true, 43000, 60000, 6, 33},
	{"supermercado-economico", "Supermercado Econômico", "Supermercado", "Ourinhos", "v5.2", 12, 700, 690, true, 88000, 90000, 3, 11},
	{"restaurante-churrascaria-gaucha", "Churrascaria Gaúcha", "Restaurante", "Marília", "v5.1", 26, 170, 170, true, 30000, 31000, 2, 16},
	{"mercadinho-sao-jose", "Mercadinho São José", "Mercearia", "Bauru", "v5.2", 6, 75, 74, false, 8200, 8000, 0, 8},
}

func checksum(id string, bad bool) string {
	if bad {
		return "00000000-invalid"
	}
	return fmt.Sprintf("sha256:%x", []byte(id+"-ok"))[:23]
}

// SeedIfEmpty populates the demo clients on first boot.
func SeedIfEmpty(s *Store) error {
	n, err := s.Count()
	if err != nil || n > 0 {
		return err
	}
	now := time.Now().UTC()
	for _, d := range seeds {
		c := Client{
			ID: d.id, Name: d.name, Segment: d.segment, City: d.city, Version: d.version,
			LastBackupDate:       now.Add(-time.Duration(d.backupHoursAgo * float64(time.Hour))),
			BackupSizeMB:         d.size,
			PreviousBackupSizeMB: d.prev,
			BackupIntegrityOK:    d.ok,
			BackupChecksum:       checksum(d.id, !d.ok),
			Sales7d:              d.sales7,
			SalesAvgWeekly:       d.salesAvg,
			Tickets30d:           d.tickets,
			DaysSinceInteraction: d.interaction,
			UpdatedAt:            now,
		}
		if err := s.Save(c); err != nil {
			return err
		}
	}
	return nil
}
