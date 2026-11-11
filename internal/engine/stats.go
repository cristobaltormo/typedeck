package engine

import (
	"encoding/json"
	"os"
	"time"
)

type Stats struct {
	Since  string         `json:"since"`
	Total  int            `json:"total"`
	Keys   map[string]int `json:"keys"`
	Layers map[string]int `json:"layers"`
	Types  map[string]int `json:"types"`
	Days   map[string]int `json:"days"`
}

func newStats() Stats {
	return Stats{Since: time.Now().Format("2006-01-02"), Keys: map[string]int{}, Layers: map[string]int{}, Types: map[string]int{}, Days: map[string]int{}}
}

func (e *Engine) loadStats() Stats {
	s := newStats()
	if b, err := os.ReadFile(e.paths.Stats()); err == nil {
		var in Stats
		if json.Unmarshal(b, &in) == nil && in.Keys != nil {
			s = in
			for _, m := range []*map[string]int{&s.Layers, &s.Types, &s.Days} {
				if *m == nil {
					*m = map[string]int{}
				}
			}
		}
	}
	return s
}

func (e *Engine) count(key, layer, atype string) {
	e.mu.Lock()
	s := &e.stats
	s.Total++
	s.Keys[key]++
	s.Layers[layer]++
	s.Types[atype]++
	day := time.Now().Format("2006-01-02")
	s.Days[day]++
	if len(s.Days) > 60 {
		oldest := ""
		for d := range s.Days {
			if oldest == "" || d < oldest {
				oldest = d
			}
		}
		delete(s.Days, oldest)
	}
	if e.statsTimer == nil {
		e.statsTimer = time.AfterFunc(30*time.Second, e.SaveStats)
	}
	e.mu.Unlock()
}

func (e *Engine) SaveStats() {
	e.mu.Lock()
	e.statsTimer = nil
	data, _ := json.Marshal(e.stats)
	e.mu.Unlock()
	_ = os.MkdirAll(e.paths.Dir, 0o755)
	tmp := e.paths.Stats() + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, e.paths.Stats())
	}
}

func (e *Engine) Stats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := e.stats
	cp.Keys, cp.Layers, cp.Types, cp.Days = cloneMap(e.stats.Keys), cloneMap(e.stats.Layers), cloneMap(e.stats.Types), cloneMap(e.stats.Days)
	return cp
}

func cloneMap(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (e *Engine) ResetStats() {
	e.mu.Lock()
	e.stats = newStats()
	e.mu.Unlock()
	e.SaveStats()
}
