package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/IANDYI/care-service/internal/adapters/middleware"
	"github.com/IANDYI/care-service/internal/core/ports"
	"github.com/google/uuid"
)

// MeasurementHandler handles HTTP requests for measurement operations
type MeasurementHandler struct {
	measurementService ports.MeasurementService
}

// NewMeasurementHandler creates a new measurement handler
func NewMeasurementHandler(measurementService ports.MeasurementService) *MeasurementHandler {
	return &MeasurementHandler{
		measurementService: measurementService,
	}
}

// CreateMeasurementRequest represents the request body for creating a measurement
// This matches the ports.CreateMeasurementRequest structure
type CreateMeasurementRequest struct {
	Type        string    `json:"type"`          // feeding, weight, temperature, diaper
	Value       float64   `json:"value"`         // Numeric value (weight in grams, temperature in Celsius)
	Note        string    `json:"note"`         // Optional contextual metadata
	Timestamp   time.Time `json:"timestamp"`    // When the measurement was taken
	
	// Feeding-specific fields
	FeedingType     string   `json:"feeding_type,omitempty"`     // "bottle" or "breast"
	VolumeML        *int     `json:"volume_ml,omitempty"`        // ml for bottle feeding
	Position        string   `json:"position,omitempty"`         // Position for breast feeding
	Side            string   `json:"side,omitempty"`             // "left", "right", or "both"
	LeftDuration    *int     `json:"left_duration,omitempty"`    // Duration in seconds for left side
	RightDuration   *int     `json:"right_duration,omitempty"`  // Duration in seconds for right side
	Duration        *int     `json:"duration,omitempty"`         // Total duration in seconds (for single side)
	
	// Temperature-specific fields
	ValueCelsius    *float64 `json:"value_celsius,omitempty"`   // Temperature in Celsius
	
	// Diaper-specific fields
	DiaperStatus    string   `json:"diaper_status,omitempty"`   // "dry", "wet", "dirty", or "both"
}

// CreateMeasurement handles POST /babies/{baby_id}/measurements
// PARENT: owned only (ADMIN cannot create measurements)
// Response time < 2s
func (h *MeasurementHandler) CreateMeasurement(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	requestID := generateRequestID()

	// Extract user info from context
	userIDStr, ok := middleware.GetUserID(r.Context())
	if !ok {
		log.Printf("[%s] Failed to get user ID from context", requestID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("[%s] Invalid user ID: %v", requestID, err)
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	isAdmin := middleware.IsAdmin(r.Context())
	role, roleOk := middleware.GetRole(r.Context())
	if !roleOk {
		log.Printf("[%s] WARNING: CreateMeasurement - role not found in context for user_id=%s", requestID, userIDStr)
		http.Error(w, "internal server error: missing role", http.StatusInternalServerError)
		return
	}
	log.Printf("[%s] CreateMeasurement - user_id=%s, role=%s (len=%d), isAdmin=%v", requestID, userIDStr, role, len(role), isAdmin)

	// Extract baby_id from URL path
	babyIDStr := r.PathValue("baby_id")
	babyID, err := uuid.Parse(babyIDStr)
	if err != nil {
		log.Printf("[%s] Invalid baby ID: %v", requestID, err)
		http.Error(w, "invalid baby ID", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req CreateMeasurementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[%s] Failed to decode request: %v", requestID, err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Set timestamp if not provided (default to now)
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}

	// Create measurement with full details (supports feeding, temperature, and diaper types)
	measurement, err := h.measurementService.CreateMeasurementWithDetails(
		r.Context(),
		babyID,
		ports.CreateMeasurementRequest{
			Type:          req.Type,
			Value:         req.Value,
			Note:          req.Note,
			Timestamp:     req.Timestamp,
			FeedingType:   req.FeedingType,
			VolumeML:      req.VolumeML,
			Position:      req.Position,
			Side:          req.Side,
			LeftDuration:  req.LeftDuration,
			RightDuration: req.RightDuration,
			Duration:      req.Duration,
			ValueCelsius:  req.ValueCelsius,
			DiaperStatus:  req.DiaperStatus,
		},
		userID,
		isAdmin,
	)
	if err != nil {
		roleStr, _ := middleware.GetRole(r.Context())
		log.Printf("[%s] Failed to create measurement: user_id=%s, role=%s, isAdmin=%v, baby_id=%s, error=%v", requestID, userIDStr, roleStr, isAdmin, babyIDStr, err)
		if err.Error() == "baby not found" {
			http.Error(w, "baby not found", http.StatusNotFound)
			return
		}
		if err.Error() == "forbidden: only PARENT can create measurements" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Log structured JSON
	logStructured(requestID, userIDStr, isAdmin, "POST", "/babies/"+babyIDStr+"/measurements", http.StatusCreated, time.Since(startTime))

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(measurement); err != nil {
		log.Printf("[%s] Failed to encode response: %v", requestID, err)
	}
}

// GetMeasurements handles GET /babies/{baby_id}/measurements
// ADMIN: any baby, PARENT: owned only
func (h *MeasurementHandler) GetMeasurements(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	requestID := generateRequestID()

	// Extract user info from context
	userIDStr, ok := middleware.GetUserID(r.Context())
	if !ok {
		log.Printf("[%s] Failed to get user ID from context", requestID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("[%s] Invalid user ID: %v", requestID, err)
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	isAdmin := middleware.IsAdmin(r.Context())

	// Extract baby_id from URL path
	babyIDStr := r.PathValue("baby_id")
	babyID, err := uuid.Parse(babyIDStr)
	if err != nil {
		log.Printf("[%s] Invalid baby ID: %v", requestID, err)
		http.Error(w, "invalid baby ID", http.StatusBadRequest)
		return
	}

	// Parse query parameters for filtering
	var measurementType *string
	var limit *int

	if typeParam := r.URL.Query().Get("type"); typeParam != "" {
		measurementType = &typeParam
	}

	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		limitInt, err := strconv.Atoi(limitParam)
		if err != nil || limitInt <= 0 {
			log.Printf("[%s] Invalid limit parameter: %s", requestID, limitParam)
			http.Error(w, "invalid limit parameter (must be positive integer)", http.StatusBadRequest)
			return
		}
		limit = &limitInt
	}

	// Get measurements with optional filters
	measurements, err := h.measurementService.GetMeasurements(r.Context(), babyID, userID, isAdmin, measurementType, limit)
	if err != nil {
		roleStr, _ := middleware.GetRole(r.Context())
		log.Printf("[%s] Failed to get measurements: user_id=%s, role=%s, isAdmin=%v, baby_id=%s, error=%v", requestID, userIDStr, roleStr, isAdmin, babyIDStr, err)
		if err.Error() == "baby not found" {
			http.Error(w, "baby not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Log structured JSON
	logStructured(requestID, userIDStr, isAdmin, "GET", "/babies/"+babyIDStr+"/measurements", http.StatusOK, time.Since(startTime))

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(measurements); err != nil {
		log.Printf("[%s] Failed to encode response: %v", requestID, err)
	}
}

// GetMeasurementByID handles GET /measurements/{measurement_id}
// ADMIN: any measurement, PARENT: owned only
func (h *MeasurementHandler) GetMeasurementByID(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	requestID := generateRequestID()

	// Extract user info from context
	userIDStr, ok := middleware.GetUserID(r.Context())
	if !ok {
		log.Printf("[%s] Failed to get user ID from context", requestID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("[%s] Invalid user ID: %v", requestID, err)
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	isAdmin := middleware.IsAdmin(r.Context())

	// Extract measurement_id from URL path
	measurementIDStr := r.PathValue("measurement_id")
	measurementID, err := uuid.Parse(measurementIDStr)
	if err != nil {
		log.Printf("[%s] Invalid measurement ID: %v", requestID, err)
		http.Error(w, "invalid measurement ID", http.StatusBadRequest)
		return
	}

	// Get measurement
	measurement, err := h.measurementService.GetMeasurementByID(r.Context(), measurementID, userID, isAdmin)
	if err != nil {
		roleStr, _ := middleware.GetRole(r.Context())
		log.Printf("[%s] Failed to get measurement: user_id=%s, role=%s, isAdmin=%v, measurement_id=%s, error=%v", requestID, userIDStr, roleStr, isAdmin, measurementIDStr, err)
		errStr := err.Error()
		if errStr == "measurement not found" || strings.Contains(errStr, "measurement not found") {
			http.Error(w, "measurement not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log structured JSON
	logStructured(requestID, userIDStr, isAdmin, "GET", "/measurements/"+measurementIDStr, http.StatusOK, time.Since(startTime))

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(measurement); err != nil {
		log.Printf("[%s] Failed to encode response: %v", requestID, err)
	}
}

// DeleteMeasurement handles DELETE /measurements/{measurement_id}
// PARENT: only measurements they created (ADMIN cannot delete measurements)
func (h *MeasurementHandler) DeleteMeasurement(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	requestID := generateRequestID()

	// Extract user info from context
	userIDStr, ok := middleware.GetUserID(r.Context())
	if !ok {
		log.Printf("[%s] Failed to get user ID from context", requestID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("[%s] Invalid user ID: %v", requestID, err)
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	isAdmin := middleware.IsAdmin(r.Context())

	// Extract measurement_id from URL path
	measurementIDStr := r.PathValue("measurement_id")
	measurementID, err := uuid.Parse(measurementIDStr)
	if err != nil {
		log.Printf("[%s] Invalid measurement ID: %v", requestID, err)
		http.Error(w, "invalid measurement ID", http.StatusBadRequest)
		return
	}

	// Delete measurement
	err = h.measurementService.DeleteMeasurement(r.Context(), measurementID, userID, isAdmin)
	if err != nil {
		roleStr, _ := middleware.GetRole(r.Context())
		log.Printf("[%s] Failed to delete measurement: user_id=%s, role=%s, isAdmin=%v, measurement_id=%s, error=%v", requestID, userIDStr, roleStr, isAdmin, measurementIDStr, err)
		if err.Error() == "measurement not found" {
			http.Error(w, "measurement not found", http.StatusNotFound)
			return
		}
		if err.Error() == "forbidden: only PARENT can delete measurements" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log structured JSON
	logStructured(requestID, userIDStr, isAdmin, "DELETE", "/measurements/"+measurementIDStr, http.StatusNoContent, time.Since(startTime))

	// Return success response
	w.WriteHeader(http.StatusNoContent)
}

