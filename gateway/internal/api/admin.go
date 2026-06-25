package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/sentinel-project/sentinel/gateway/internal/config"
	"github.com/sentinel-project/sentinel/gateway/internal/stats"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// AdminHandler serves runtime configuration and stats endpoints.
type AdminHandler struct {
	cfg       *config.Config
	cfgMu     sync.RWMutex
	stats     *stats.Collector
}

func NewAdminHandler(cfg *config.Config, collector *stats.Collector) *AdminHandler {
	return &AdminHandler{cfg: cfg, stats: collector}
}

func (h *AdminHandler) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Admin-Key")
		if key == "" {
			key = r.URL.Query().Get("key")
		}
		if key != h.cfg.AdminAPIKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *AdminHandler) ListLimits(w http.ResponseWriter, _ *http.Request) {
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()

	type entry struct {
		Route string             `json:"route"`
		Rule  config.RateLimitRule `json:"rule"`
	}
	out := []entry{{Route: "default", Rule: h.cfg.DefaultLimit}}
	for route, rule := range h.cfg.RouteLimits {
		out = append(out, entry{Route: route, Rule: rule})
	}
	writeJSON(w, out)
}

func (h *AdminHandler) UpdateLimit(w http.ResponseWriter, r *http.Request) {
	route := chi.URLParam(r, "route")
	var body struct {
		Algorithm  string  `json:"algorithm"`
		Capacity   int     `json:"capacity"`
		RefillRate float64 `json:"refill_rate"`
		Window     int     `json:"window_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()

	rule := config.RateLimitRule{
		Algorithm:  body.Algorithm,
		Capacity:   body.Capacity,
		RefillRate: body.RefillRate,
		Window:     body.Window,
	}
	if rule.Algorithm == "" {
		rule.Algorithm = h.cfg.DefaultLimit.Algorithm
	}
	if rule.Window == 0 {
		rule.Window = h.cfg.DefaultLimit.Window
	}

	if route == "default" {
		h.cfg.DefaultLimit = rule
	} else {
		h.cfg.RouteLimits[route] = rule
	}

	writeJSON(w, map[string]string{"status": "updated", "route": route})
}

func (h *AdminHandler) GetStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.stats.Snapshot())
}

func (h *AdminHandler) StatsWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		if err := conn.WriteJSON(h.stats.Snapshot()); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *AdminHandler) Config() *config.Config {
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()
	return h.cfg
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
