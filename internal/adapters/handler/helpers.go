package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"
)

// generateRequestID generates a unique request ID for tracing
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// logStructured logs structured JSON with request metadata
// Includes: request_id, user_id, role, endpoint, status_code, duration
func logStructured(requestID, userID string, isAdmin bool, method, endpoint string, statusCode int, duration time.Duration) {
	role := "PARENT"
	if isAdmin {
		role = "ADMIN"
	}

	logEntry := map[string]interface{}{
		"request_id":  requestID,
		"user_id":     userID,
		"role":        role,
		"method":      method,
		"endpoint":    endpoint,
		"status_code": statusCode,
		"duration_ms": duration.Milliseconds(),
	}

	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("[%s] Failed to marshal log entry: %v", requestID, err)
		return
	}

	log.Printf("%s", string(jsonBytes))
}

