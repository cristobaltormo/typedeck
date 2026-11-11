package engine

import (
	"time"
)

type Event struct {
	ID        int      `json:"id"`
	TS        float64  `json:"ts"`
	Kind      string   `json:"kind"`
	Key       string   `json:"key,omitempty"`
	Gesture   string   `json:"gesture,omitempty"`
	Layer     *int     `json:"layer,omitempty"`
	LayerName string   `json:"layer_name,omitempty"`
	Name      string   `json:"name,omitempty"`
	Auto      bool     `json:"auto,omitempty"`
	Label     string   `json:"label,omitempty"`
	Type      string   `json:"type,omitempty"`
	OK        *bool    `json:"ok,omitempty"`
	MS        int64    `json:"ms,omitempty"`
	Output    string   `json:"output,omitempty"`
	Names     []string `json:"names,omitempty"`
	Connected *bool    `json:"connected,omitempty"`
	Port      string   `json:"port,omitempty"`
}

func (e *Engine) emitLocked(ev Event) {
	ev.ID = e.nextID
	ev.TS = float64(time.Now().UnixNano()) / 1e9
	e.nextID++
	e.events = append(e.events, ev)
	if len(e.events) > 400 {
		e.events = e.events[len(e.events)-400:]
	}
	close(e.sig)
	e.sig = make(chan struct{})
}

func (e *Engine) emit(ev Event) {
	e.mu.Lock()
	e.emitLocked(ev)
	e.mu.Unlock()
}

func (e *Engine) EventsSince(id int, wait time.Duration, cancel <-chan struct{}) ([]Event, int) {
	deadline := time.After(wait)
	for {
		e.mu.Lock()
		var out []Event
		for _, ev := range e.events {
			if ev.ID > id {
				out = append(out, ev)
			}
		}
		last := e.nextID - 1
		sig := e.sig
		e.mu.Unlock()
		if len(out) > 0 || wait <= 0 {
			if out == nil {
				out = []Event{}
			}
			return out, last
		}
		select {
		case <-sig:
		case <-deadline:
			return []Event{}, last
		case <-cancel:
			return []Event{}, last
		}
	}
}
