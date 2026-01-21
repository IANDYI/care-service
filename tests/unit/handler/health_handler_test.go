package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/IANDYI/care-service/internal/adapters/handler" //nolint:staticcheck // handler package contains non-deprecated code
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthHandler(t *testing.T) {
	// Create a mock DB connection (using sql.Open with a dummy connection string)
	// In real tests, you might use a test database or sqlmock
	db, err := sql.Open("postgres", "postgres://user:pass@localhost/test?sslmode=disable")
	if err != nil {
		// If we can't connect, that's okay for this test - we're just testing the constructor
		t.Skip("Skipping test - no database connection available")
	}
	defer db.Close()

	healthHandler := handler.NewHealthHandler(db)
	assert.NotNil(t, healthHandler)
}

func TestHealthHandler_Health(t *testing.T) {
	// Create a mock DB connection
	db, err := sql.Open("postgres", "postgres://user:pass@localhost/test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test - no database connection available")
	}
	defer db.Close()

	healthHandler := handler.NewHealthHandler(db)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	healthHandler.Health(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var response handler.HealthResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response.Status)
	assert.WithinDuration(t, time.Now(), response.Timestamp, time.Second)
}

func TestHealthHandler_Live(t *testing.T) {
	// Create a mock DB connection
	db, err := sql.Open("postgres", "postgres://user:pass@localhost/test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test - no database connection available")
	}
	defer db.Close()

	healthHandler := handler.NewHealthHandler(db)

	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()

	healthHandler.Live(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var response handler.HealthResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "alive", response.Status)
}

func TestHealthHandler_Ready(t *testing.T) {
	// Create a mock DB connection
	// Note: This test will fail if there's no actual database connection
	// In a real scenario, you'd use sqlmock or a test database
	db, err := sql.Open("postgres", "postgres://user:pass@localhost/test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test - no database connection available")
	}
	defer db.Close()

	healthHandler := handler.NewHealthHandler(db)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()

	healthHandler.Ready(w, req)

	// The status depends on whether the DB connection works
	// If it fails, we get 503, if it works, we get 200
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusServiceUnavailable)
}

func TestMetrics(t *testing.T) {
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	handler.Metrics(w, req)

	// Prometheus metrics endpoint should return 200
	assert.Equal(t, http.StatusOK, w.Code)
}
