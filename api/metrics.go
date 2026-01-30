package api

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/Scrin/siikabot/db"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// MetricsResponse represents the parsed metrics for the web UI
type MetricsResponse struct {
	Memory   MemoryMetrics   `json:"memory"`
	Runtime  RuntimeMetrics  `json:"runtime"`
	Database DatabaseMetrics `json:"database"`
	Bot      BotMetrics      `json:"bot"`
}

// MemoryMetrics contains memory usage information
type MemoryMetrics struct {
	ResidentMB float64 `json:"resident_mb"`
}

// RuntimeMetrics contains Go runtime information
type RuntimeMetrics struct {
	Goroutines int `json:"goroutines"`
}

// DatabaseMetrics contains database pool information
type DatabaseMetrics struct {
	ActiveConns int32 `json:"active_conns"`
	MaxConns    int32 `json:"max_conns"`
	IdleConns   int32 `json:"idle_conns"`
}

// BotMetrics contains bot activity information
type BotMetrics struct {
	EventsHandled int64 `json:"events_handled"`
}

// MetricsHandler returns parsed metrics as JSON for the web UI
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	response := MetricsResponse{
		Memory:   getMemoryMetrics(ctx),
		Runtime:  getRuntimeMetrics(),
		Database: getDatabaseMetrics(),
		Bot:      getBotMetrics(ctx),
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to encode metrics response")
	}
}

func getMemoryMetrics(ctx context.Context) MemoryMetrics {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to gather prometheus metrics")
		return MemoryMetrics{}
	}

	for _, mf := range mfs {
		if mf.GetName() == "process_resident_memory_bytes" {
			for _, m := range mf.GetMetric() {
				bytes := m.GetGauge().GetValue()
				return MemoryMetrics{
					ResidentMB: bytes / (1024 * 1024),
				}
			}
		}
	}
	return MemoryMetrics{}
}

func getRuntimeMetrics() RuntimeMetrics {
	return RuntimeMetrics{
		Goroutines: runtime.NumGoroutine(),
	}
}

func getDatabaseMetrics() DatabaseMetrics {
	stats := db.GetPoolStats()
	if stats == nil {
		return DatabaseMetrics{}
	}
	return DatabaseMetrics{
		ActiveConns: stats.TotalConns(),
		MaxConns:    stats.MaxConns(),
		IdleConns:   stats.IdleConns(),
	}
}

func getBotMetrics(ctx context.Context) BotMetrics {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to gather prometheus metrics for bot")
		return BotMetrics{}
	}

	var total int64
	for _, mf := range mfs {
		if mf.GetName() == "siikabot_events_handled_count" {
			for _, m := range mf.GetMetric() {
				total += int64(m.GetCounter().GetValue())
			}
		}
	}
	return BotMetrics{EventsHandled: total}
}
