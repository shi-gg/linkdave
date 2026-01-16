package server

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

type HealthResponse struct {
	Status       string `json:"status"`
	Version      string `json:"version"`
	GoVersion    string `json:"go_version"`
	Uptime       int64  `json:"uptime_ms"`
	NumGoroutine int    `json:"num_goroutines"`
	MemoryMB     uint64 `json:"memory_mb"`
}

type HealthHandler struct {
	server    *Server
	version   string
	startTime time.Time
}

func NewHealthHandler(server *Server, version string) *HealthHandler {
	return &HealthHandler{
		server:    server,
		version:   version,
		startTime: time.Now(),
	}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	response := HealthResponse{
		Status:       "ok",
		Version:      h.version,
		GoVersion:    runtime.Version(),
		Uptime:       time.Since(h.startTime).Milliseconds(),
		NumGoroutine: runtime.NumGoroutine(),
		MemoryMB:     memStats.Alloc / 1024 / 1024,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

type StatsHandler struct {
	server *Server
}

func NewStatsHandler(server *Server) *StatsHandler {
	return &StatsHandler{server: server}
}

func (h *StatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	stats := h.server.GetStats()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(stats)
}
