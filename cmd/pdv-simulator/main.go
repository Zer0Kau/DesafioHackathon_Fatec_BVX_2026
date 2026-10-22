// pdv-simulator: CLI that mimics a PDV sending telemetry heartbeats to the central server.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

const target = "padaria-estrela"

var (
	baseURL string
	httpc   = &http.Client{Timeout: 8 * time.Second}
)

const (
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	bold   = "\033[1m"
	reset  = "\033[0m"
)

type payload map[string]any

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func healthy() payload {
	return payload{
		"client_id": target, "version": "v5.2", "last_backup_date": ts(time.Now()),
		"backup_size_mb": 450, "previous_backup_size_mb": 450, "backup_integrity_ok": true,
		"backup_checksum": "sha256:9f2c1a7be4d05a31", "sales_last_7d": 12500, "sales_avg_weekly": 10000,
		"tickets_last_30d": 1, "days_since_last_interaction": 35,
	}
}

func corruptedBackup() payload {
	p := healthy()
	p["backup_size_mb"] = 2
	p["previous_backup_size_mb"] = 450
	p["backup_integrity_ok"] = false
	p["backup_checksum"] = "00000000-invalid"
	return p
}

func crisis() payload {
	p := healthy()
	p["version"] = "v5.1"
	p["last_backup_date"] = ts(time.Now().Add(-7 * 24 * time.Hour))
	p["sales_last_7d"] = 6000
	p["tickets_last_30d"] = 7
	return p
}

func send(p payload) (map[string]any, error) {
	body, _ := json.MarshalIndent(p, "", "  ")
	fmt.Printf("\n%s→ POST %s/api/v1/telemetry%s\n%s\n", cyan, baseURL, reset, body)
	resp, err := httpc.Post(baseURL+"/api/v1/telemetry", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("%sFalha de conexão: %v%s\n", red, err, reset)
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != 200 {
		fmt.Printf("%s← HTTP %d: %s%s\n", red, resp.StatusCode, strings.TrimSpace(string(raw)), reset)
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	color := green
	switch out["status"] {
	case "ATENCAO":
		color = yellow
	case "ALTO_RISCO":
		color = red
	}
	fmt.Printf("%s← HTTP 200%s\n  old_score: %v\n  new_score: %s%v%s\n  status: %s%v%s\n  critical_alert: %v\n",
		cyan, reset, out["old_score"], bold, out["new_score"], reset, color, out["status"], reset, out["critical_alert"])
	if acts, ok := out["recommended_actions"].([]any); ok {
		for _, a := range acts {
			fmt.Printf("  • %v\n", a)
		}
	}
	if out["critical_alert"] == true {
		fmt.Printf("%s%s🚨 ALERTA CRÍTICO DISPARADO NO SERVIDOR%s\n", red, bold, reset)
	}
	return out, nil
}

func fetchIDs() ([]string, error) {
	resp, err := httpc.Get(baseURL + "/api/v1/clients")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var d struct {
		Clients []struct {
			ID string `json:"id"`
		} `json:"clients"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&d); err != nil {
		return nil, err
	}
	ids := make([]string, len(d.Clients))
	for i, c := range d.Clients {
		ids[i] = c.ID
	}
	return ids, nil
}

func randomPayload(id string) payload {
	avg := float64(8000 + rand.Intn(90000))
	ratio := 0.55 + rand.Float64()*0.6 // 55%..115% of average
	size := float64(100 + rand.Intn(700))
	prev := size
	ok := rand.Intn(10) > 0
	switch rand.Intn(8) {
	case 0:
		size = prev * 0.05 // truncated backup
	case 1:
		ok = false
	}
	ver := []string{"v5.2", "v5.2", "v5.1", "v5.0"}[rand.Intn(4)]
	age := time.Duration(rand.Intn(10*24)) * time.Hour
	if rand.Intn(2) == 0 {
		age = time.Duration(rand.Intn(20)) * time.Hour
	}
	chk := fmt.Sprintf("sha256:%08x", rand.Uint32())
	if !ok {
		chk = "00000000-invalid"
	}
	return payload{
		"client_id": id, "version": ver, "last_backup_date": ts(time.Now().Add(-age)),
		"backup_size_mb": size, "previous_backup_size_mb": prev, "backup_integrity_ok": ok,
		"backup_checksum": chk, "sales_last_7d": avg * ratio, "sales_avg_weekly": avg,
		"tickets_last_30d": rand.Intn(10), "days_since_last_interaction": rand.Intn(45),
	}
}

func chaos(stdin *bufio.Reader) {
	ids, err := fetchIDs()
	if err != nil || len(ids) == 0 {
		fmt.Printf("%sNão foi possível listar os PDVs: %v%s\n", red, err, reset)
		return
	}
	fmt.Printf("%sModo Caos ativo (%d PDVs). Pressione ENTER para parar.%s\n", yellow, len(ids), reset)
	stop := make(chan struct{})
	go func() { stdin.ReadString('\n'); close(stop) }()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		for _, i := range rand.Perm(len(ids))[:min(3, len(ids))] {
			send(randomPayload(ids[i]))
		}
		select {
		case <-stop:
			fmt.Println("Modo Caos encerrado.")
			return
		case <-tick.C:
		}
	}
}

func main() {
	flag.StringVar(&baseURL, "server", "http://localhost:8080", "URL do servidor central")
	flag.Parse()
	baseURL = strings.TrimRight(baseURL, "/")
	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf(`
%s=== BVX Pulse · Simulador de PDV ===%s  (servidor: %s)
 [1] Enviar Heartbeat Saudável (Padaria Estrela - Tudo OK)
 [2] Simular Falha Silenciosa de Backup (backup corrompido de 2MB, checksum inválido)
 [3] Simular Crise Completa / Risco de Churn (v5.1, backup há 7 dias, -40%% vendas, 7 chamados)
 [4] Modo Caos Contínuo (telemetria aleatória a cada 5s)
 [0] Sair
> `, bold, reset, baseURL)
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return
		}
		switch strings.TrimSpace(line) {
		case "1":
			send(healthy())
		case "2":
			send(corruptedBackup())
		case "3":
			send(crisis())
		case "4":
			chaos(in)
		case "0", "q":
			return
		default:
			fmt.Println("Opção inválida.")
		}
	}
}
