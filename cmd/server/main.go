package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"sort"
	"time"

	"bvx-pulse/internal/health"
)

//go:embed web
var webFS embed.FS

var versionRe = regexp.MustCompile(`^v\d{1,2}\.\d{1,2}$`)

type ClientView struct {
	Client
	health.Result
	OpenTicket *Ticket  `json:"open_ticket"`
	Profile    *Profile `json:"profile,omitempty"`
}

type Summary struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Warning  int `json:"warning"`
	Critical int `json:"critical"`
}

type Server struct {
	store *Store
	hub   *Hub
}

func metrics(c Client) health.Metrics {
	return health.Metrics{
		Version: c.Version, LastBackupDate: c.LastBackupDate, BackupSizeMB: c.BackupSizeMB,
		PreviousBackupSizeMB: c.PreviousBackupSizeMB, BackupIntegrityOK: c.BackupIntegrityOK,
		Sales7d: c.Sales7d, SalesAvgWeekly: c.SalesAvgWeekly, Tickets30d: c.Tickets30d,
		DaysSinceInteraction: c.DaysSinceInteraction,
	}
}

func view(c Client, tk map[string]Ticket, now time.Time) ClientView {
	v := ClientView{Client: c, Result: health.Evaluate(metrics(c), now)}
	if t, ok := tk[c.ID]; ok {
		v.OpenTicket = &t
		p := MockProfile(c)
		v.Profile = &p
	}
	return v
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.List()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	tk, _ := s.store.OpenTickets()
	now := time.Now()
	views := make([]ClientView, 0, len(list))
	var sum Summary
	for _, c := range list {
		v := view(c, tk, now)
		views = append(views, v)
		sum.Total++
		switch v.Status {
		case health.StatusLow:
			sum.Healthy++
		case health.StatusMedium:
			sum.Warning++
		default:
			sum.Critical++
		}
	}
	// priority queue: lowest score first
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Score != views[j].Score {
			return views[i].Score < views[j].Score
		}
		return views[i].Name < views[j].Name
	})
	writeJSON(w, 200, map[string]any{"summary": sum, "clients": views, "server_time": now.UTC()})
}

func (s *Server) handleClient(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "client not found")
		return
	}
	tk, _ := s.store.OpenTickets()
	writeJSON(w, 200, view(c, tk, time.Now()))
}

type telemetry struct {
	ClientID             string     `json:"client_id"`
	Version              *string    `json:"version"`
	LastBackupDate       *time.Time `json:"last_backup_date"`
	BackupSizeMB         *float64   `json:"backup_size_mb"`
	PreviousBackupSizeMB *float64   `json:"previous_backup_size_mb"`
	BackupIntegrityOK    *bool      `json:"backup_integrity_ok"`
	BackupChecksum       *string    `json:"backup_checksum"`
	Sales7d              *float64   `json:"sales_last_7d"`
	SalesAvgWeekly       *float64   `json:"sales_avg_weekly"`
	Tickets30d           *int       `json:"tickets_last_30d"`
	DaysSinceInteraction *int       `json:"days_since_last_interaction"`
}

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	var t telemetry
	if err := dec.Decode(&t); err != nil {
		writeErr(w, 400, "invalid JSON: "+err.Error())
		return
	}
	c, err := s.store.Get(t.ClientID)
	if err != nil {
		writeErr(w, 404, "unknown client_id")
		return
	}
	now := time.Now()
	old := health.Evaluate(metrics(c), now)

	if t.Version != nil {
		if !versionRe.MatchString(*t.Version) {
			writeErr(w, 400, "version must look like v5.2")
			return
		}
		c.Version = *t.Version
	}
	if t.LastBackupDate != nil {
		c.LastBackupDate = t.LastBackupDate.UTC()
	}
	if t.BackupSizeMB != nil {
		c.BackupSizeMB = *t.BackupSizeMB
	}
	if t.PreviousBackupSizeMB != nil {
		c.PreviousBackupSizeMB = *t.PreviousBackupSizeMB
	}
	if t.BackupIntegrityOK != nil {
		c.BackupIntegrityOK = *t.BackupIntegrityOK
	}
	if t.BackupChecksum != nil {
		if len(*t.BackupChecksum) > 128 {
			writeErr(w, 400, "backup_checksum too long")
			return
		}
		c.BackupChecksum = *t.BackupChecksum
	}
	if t.Sales7d != nil {
		c.Sales7d = *t.Sales7d
	}
	if t.SalesAvgWeekly != nil {
		c.SalesAvgWeekly = *t.SalesAvgWeekly
	}
	if t.Tickets30d != nil {
		c.Tickets30d = *t.Tickets30d
	}
	if t.DaysSinceInteraction != nil {
		c.DaysSinceInteraction = *t.DaysSinceInteraction
	}
	if c.BackupSizeMB < 0 || c.PreviousBackupSizeMB < 0 || c.Sales7d < 0 || c.SalesAvgWeekly < 0 ||
		c.Tickets30d < 0 || c.DaysSinceInteraction < 0 {
		writeErr(w, 400, "negative values are not allowed")
		return
	}
	c.UpdatedAt = now.UTC()
	if err := s.store.Save(c); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	nw := health.Evaluate(metrics(c), now)
	critical := nw.Status == health.StatusHigh && old.Status != health.StatusHigh

	msg := fmt.Sprintf("%s: %d → %d (%s)", c.Name, old.Score, nw.Score, nw.StatusLabel)
	if critical {
		msg = fmt.Sprintf("ALERTA CRÍTICO — %s rebaixado para ALTO RISCO (%d → %d)", c.Name, old.Score, nw.Score)
	}
	ev := s.hub.Publish(Event{
		Time: now.UTC(), ClientID: c.ID, ClientName: c.Name, OldScore: old.Score, NewScore: nw.Score,
		OldStatus: old.Status, NewStatus: nw.Status, Critical: critical, Message: msg,
	})
	log.Printf("telemetry %s score %d -> %d (%s) critical=%v", c.ID, old.Score, nw.Score, nw.Status, critical)

	writeJSON(w, 200, map[string]any{
		"client_id": c.ID, "client_name": c.Name,
		"old_score": old.Score, "new_score": nw.Score,
		"old_status": old.Status, "status": nw.Status, "status_label": nw.StatusLabel,
		"critical_alert": critical, "recommended_actions": nw.Actions, "event_id": ev.ID,
	})
}

func (s *Server) handleTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.store.Get(id)
	if err != nil {
		writeErr(w, 404, "client not found")
		return
	}
	res := health.Evaluate(metrics(c), time.Now())
	note := "Ticket preventivo (score " + fmt.Sprint(res.Score) + "): " + res.Actions[0]
	t, created, err := s.store.OpenTicket(id, note)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	code := 200
	if created {
		code = 201
	}
	writeJSON(w, code, map[string]any{"ticket": t, "created": created, "profile": MockProfile(c)})
}

func (s *Server) handleCancelTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.Get(id); err != nil {
		writeErr(w, 404, "client not found")
		return
	}
	ok, err := s.store.CancelTicket(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !ok {
		writeErr(w, 404, "no open ticket")
		return
	}
	writeJSON(w, 200, map[string]any{"cancelled": true})
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "bvx-pulse.db", "SQLite database file")
	flag.Parse()

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := SeedIfEmpty(store); err != nil {
		log.Fatal(err)
	}
	s := &Server{store: store, hub: NewHub()}

	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(sub))
	mux.HandleFunc("GET /api/v1/clients", s.handleClients)
	mux.HandleFunc("GET /api/v1/clients/{id}", s.handleClient)
	mux.HandleFunc("POST /api/v1/clients/{id}/ticket", s.handleTicket)
	mux.HandleFunc("DELETE /api/v1/clients/{id}/ticket", s.handleCancelTicket)
	mux.HandleFunc("POST /api/v1/telemetry", s.handleTelemetry)
	mux.HandleFunc("GET /api/v1/events", s.hub.ServeSSE)
	mux.HandleFunc("GET /api/v1/events/recent", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.hub.Recent())
	})

	log.Printf("BVX Pulse - Cliente 360 em http://localhost%s", *addr)
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
