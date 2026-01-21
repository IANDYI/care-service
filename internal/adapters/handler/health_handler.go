package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HealthHandler handles health check endpoints
// OpenShift compatible: /health, /health/ready, /health/live
type HealthHandler struct {
	db *sql.DB
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{
		db: db,
	}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// Health handles GET /health - general health check
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Log error but don't fail health check
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// Ready handles GET /health/ready - readiness probe
// Checks database connectivity
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := h.db.PingContext(ctx)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		response := HealthResponse{
			Status:    "not ready",
			Timestamp: time.Now(),
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			// Log error but don't fail health check
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	response := HealthResponse{
		Status:    "ready",
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Log error but don't fail health check
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// Live handles GET /health/live - liveness probe
func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "alive",
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Log error but don't fail health check
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// Metrics handles GET /metrics - Prometheus metrics endpoint
func Metrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

// RegisterMetrics registers Prometheus metrics
func RegisterMetrics() {
	// HTTP request duration histogram
	httpRequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint", "status"},
	)

	// HTTP request counter
	httpRequestTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(httpRequestTotal)
}

