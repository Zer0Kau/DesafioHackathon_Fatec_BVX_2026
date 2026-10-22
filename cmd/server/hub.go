package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Event struct {
	ID         int64     `json:"id"`
	Time       time.Time `json:"time"`
	ClientID   string    `json:"client_id"`
	ClientName string    `json:"client_name"`
	OldScore   int       `json:"old_score"`
	NewScore   int       `json:"new_score"`
	OldStatus  string    `json:"old_status"`
	NewStatus  string    `json:"new_status"`
	Critical   bool      `json:"critical"`
	Message    string    `json:"message"`
}

// Hub keeps a ring of recent events and fans them out to SSE subscribers.
type Hub struct {
	mu     sync.Mutex
	nextID int64
	recent []Event
	subs   map[chan Event]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

func (h *Hub) Publish(e Event) Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	e.ID = h.nextID
	h.recent = append(h.recent, e)
	if len(h.recent) > 50 {
		h.recent = h.recent[len(h.recent)-50:]
	}
	for ch := range h.subs {
		select {
		case ch <- e:
		default: // slow consumer: drop, the UI also polls
		}
	}
	return e
}

func (h *Hub) Recent() []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Event{}, h.recent...)
}

func (h *Hub) subscribe() chan Event {
	ch := make(chan Event, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan Event) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := h.subscribe()
	defer h.unsubscribe(ch)
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "id: %d\nevent: telemetry\ndata: %s\n\n", e.ID, b)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}
