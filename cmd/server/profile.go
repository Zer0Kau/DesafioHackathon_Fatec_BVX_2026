package main

import (
	"fmt"
	"hash/fnv"
)

// Profile is mocked CRM data shown to the support agent once a ticket is open.
type Profile struct {
	Owner         string   `json:"owner"`
	Phone         string   `json:"phone"`
	Email         string   `json:"email"`
	CNPJ          string   `json:"cnpj"`
	Address       string   `json:"address"`
	Plan          string   `json:"plan"`
	PDVCount      int      `json:"pdv_count"`
	MonthlyFee    float64  `json:"monthly_fee"`
	ClientSince   string   `json:"client_since"`
	Account       string   `json:"account_manager"`
	Technician    string   `json:"technician"`
	RemoteAccess  string   `json:"remote_access"`
	LastContacts  []string `json:"last_contacts"`
	PreferredTime string   `json:"preferred_contact_time"`
}

var (
	owners   = []string{"Carlos Henrique Almeida", "Marlene Souza", "João Batista Ferreira", "Patrícia Nogueira", "Roberto Takahashi", "Ana Paula Moreira", "Edson Camargo", "Luciana Barros"}
	plans    = []string{"BVX Retail Essencial", "BVX Retail Pro", "BVX Retail Enterprise"}
	managers = []string{"Fernanda Lima (CS)", "Rafael Costa (CS)", "Juliana Prado (CS)"}
	techs    = []string{"Diego Martins", "Thiago Ramos", "Camila Duarte"}
	streets  = []string{"Av. Rui Barbosa", "Rua XV de Novembro", "Av. Brasil", "Rua Siqueira Campos", "Av. Dom Pedro II"}
	contacts = []string{
		"Chamado: impressora fiscal não responde após atualização",
		"Chamado: lentidão na abertura do caixa",
		"Ligação CS: cliente pediu treinamento para novos operadores",
		"Chamado: divergência na emissão de NFC-e",
		"Visita técnica: troca de leitor de código de barras",
		"E-mail: dúvida sobre relatório de fechamento",
		"Chamado: erro ao sincronizar estoque entre filiais",
	}
	months = []string{"03/2019", "08/2020", "01/2021", "06/2021", "11/2022", "02/2023", "09/2023"}
)

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// MockProfile is deterministic per client so the demo is repeatable.
func MockProfile(c Client) Profile {
	h := hash(c.ID)
	pick := func(list []string, salt uint32) string { return list[(h+salt*7919)%uint32(len(list))] }
	plan := (h / 3) % uint32(len(plans))
	owner := pick(owners, 1)
	n := 1 + int(h%4)
	last := make([]string, 0, 3)
	for i := uint32(0); i < 3; i++ {
		last = append(last, fmt.Sprintf("%02d/%02d · %s", 1+(h+i*5)%28, 9+i%2, pick(contacts, i+2)))
	}
	return Profile{
		Owner:         owner,
		Phone:         fmt.Sprintf("(14) 9%04d-%04d", 9000+h%999, (h/7)%10000),
		Email:         fmt.Sprintf("contato@%s.com.br", c.ID),
		CNPJ:          fmt.Sprintf("%02d.%03d.%03d/0001-%02d", 10+h%80, h%1000, (h/11)%1000, h%100),
		Address:       fmt.Sprintf("%s, %d - %s/SP", pick(streets, 3), 50+h%1900, c.City),
		Plan:          plans[plan],
		PDVCount:      n * (1 + int(plan)*2),
		MonthlyFee:    float64(450+int(plan)*600) * float64(n),
		ClientSince:   pick(months, 4),
		Account:       pick(managers, 5),
		Technician:    pick(techs, 6),
		RemoteAccess:  fmt.Sprintf("AnyDesk 7%02d %03d %03d", h%100, (h/13)%1000, (h/17)%1000),
		LastContacts:  last,
		PreferredTime: []string{"08h–11h", "13h–16h", "Manhã, exceto segundas"}[h%3],
	}
}
