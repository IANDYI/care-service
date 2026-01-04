package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/IANDYI/care-service/internal/core/ports"
	"github.com/google/uuid"
)

// CreateMeasurementRequest is imported from ports package
type CreateMeasurementRequest = ports.CreateMeasurementRequest

// MeasurementService implements business logic for measurement operations
// Enforces RBAC and ownership rules, publishes alerts for Red status measurements
type MeasurementService struct {
	measurementRepo ports.MeasurementRepository
	babyRepo        ports.BabyRepository
	alertPublisher  ports.AlertPublisher
}

// NewMeasurementService creates a new measurement service
func NewMeasurementService(
	measurementRepo ports.MeasurementRepository,
	babyRepo ports.BabyRepository,
	alertPublisher ports.AlertPublisher,
) *MeasurementService {
	return &MeasurementService{
		measurementRepo: measurementRepo,
		babyRepo:        babyRepo,
		alertPublisher:  alertPublisher,
	}
}


// CreateMeasurement creates a new measurement for a baby
// Enforces ownership: Only PARENT can add measurements to their own babies
// ADMIN cannot create measurements (read-only access)
// Publishes alerts for Red status measurements (asynchronously)
// Response time must be < 2s
func (s *MeasurementService) CreateMeasurement(
	ctx context.Context,
	babyID uuid.UUID,
	measurementType string,
	value float64,
	note string,
	userID uuid.UUID,
	isAdmin bool,
) (*domain.Measurement, error) {
	return s.CreateMeasurementWithDetails(ctx, babyID, CreateMeasurementRequest{
		Type:  measurementType,
		Value: value,
		Note:  note,
	}, userID, isAdmin)
}

// CreateMeasurementWithDetails creates a measurement with full details including feeding-specific fields
func (s *MeasurementService) CreateMeasurementWithDetails(
	ctx context.Context,
	babyID uuid.UUID,
	req CreateMeasurementRequest,
	userID uuid.UUID,
	isAdmin bool,
) (*domain.Measurement, error) {
	startTime := time.Now()

	// Input validation
	if !domain.IsValidMeasurementType(req.Type) {
		return nil, fmt.Errorf("invalid measurement type: %s", req.Type)
	}

	// Type-specific validation
	if err := s.validateMeasurement(req); err != nil {
		return nil, err
	}

	// Check if baby exists
	exists, err := s.babyRepo.BabyExists(ctx, babyID)
	if err != nil {
		return nil, fmt.Errorf("failed to check baby existence: %w", err)
	}
	if !exists {
		// Don't leak ownership info
		return nil, fmt.Errorf("baby not found")
	}

	// RBAC enforcement: Only PARENT can create measurements, and only for their own babies
	// ADMIN cannot create measurements (read-only access)
	if isAdmin {
		return nil, fmt.Errorf("forbidden: only PARENT can create measurements")
	}

	// Verify parent owns the baby
	owned, err := s.babyRepo.CheckBabyOwnership(ctx, babyID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check ownership: %w", err)
	}
	if !owned {
		// Don't leak ownership info - return generic not found
		return nil, fmt.Errorf("baby not found")
	}

	// Calculate safety status based on type and value
	safetyStatus := domain.CalculateSafetyStatus(req.Type, req.Value)

	// Set timestamp if not provided (default to now)
	timestamp := req.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	// Create measurement
	measurement := &domain.Measurement{
		ID:           uuid.New(),
		ParentID:     userID,
		BabyID:       babyID,
		Type:         req.Type,
		Value:        req.Value,
		SafetyStatus: safetyStatus,
		Note:         req.Note,
		Timestamp:    timestamp,
		CreatedAt:    time.Now(),
	}

	// Set type-specific fields based on measurement type
	switch req.Type {
	case domain.MeasurementTypeFeeding:
		if err := s.setFeedingFields(measurement, req); err != nil {
			return nil, err
		}
	case domain.MeasurementTypeTemperature:
		if err := s.setTemperatureFields(measurement, req); err != nil {
			return nil, err
		}
	case domain.MeasurementTypeDiaper:
		if err := s.setDiaperFields(measurement, req); err != nil {
			return nil, err
		}
	}

	// Save measurement
	if err := s.measurementRepo.CreateMeasurement(ctx, measurement); err != nil {
		return nil, fmt.Errorf("failed to create measurement: %w", err)
	}

	// Log structured JSON for measurement creation
	s.logMeasurement(measurement, "created")

	// Check if measurement requires alert (Red status) and publish asynchronously
	// This is done in a goroutine to avoid blocking the response
	if measurement.SafetyStatus == domain.SafetyStatusRed {
		go func() {
			// Use background context to avoid cancellation
			bgCtx := context.Background()
			if err := s.alertPublisher.PublishAlert(bgCtx, babyID, measurement); err != nil {
				// Log error but don't fail the request
				log.Printf("Failed to publish alert for Red status measurement: %v", err)
			} else {
				s.logMeasurement(measurement, "alert_published")
			}
		}()
	}

	// Ensure response time < 2s
	elapsed := time.Since(startTime)
	if elapsed > 2*time.Second {
		return nil, fmt.Errorf("operation exceeded 2s timeout")
	}

	return measurement, nil
}

// validateMeasurement validates measurement-specific requirements
func (s *MeasurementService) validateMeasurement(req CreateMeasurementRequest) error {
	switch req.Type {
	case domain.MeasurementTypeTemperature:
		// Temperature validation: reasonable range for babies (30-42°C)
		if req.Value < 30.0 || req.Value > 42.0 {
			return fmt.Errorf("temperature must be between 30.0 and 42.0°C")
		}
		return nil

	case domain.MeasurementTypeWeight:
		// Weight validation: must be positive (in grams)
		if req.Value <= 0 {
			return fmt.Errorf("weight must be greater than 0 grams")
		}
		// Reasonable upper bound (e.g., 10kg = 10000g)
		if req.Value > 10000 {
			return fmt.Errorf("weight exceeds reasonable maximum (10000g)")
		}
		return nil

	case domain.MeasurementTypeFeeding:
		// Feeding validation is handled in setFeedingFields
		// Basic check here
		if req.FeedingType == "" {
			return fmt.Errorf("feeding type must be specified (bottle or breast)")
		}
		return nil

	case domain.MeasurementTypeDiaper:
		// Diaper validation is handled in setDiaperFields
		// Basic check here
		if req.DiaperStatus == "" {
			return fmt.Errorf("diaper status must be specified (dry, wet, dirty, or both)")
		}
		return nil

	default:
		return fmt.Errorf("unsupported measurement type: %s", req.Type)
	}
}

// logMeasurement logs structured JSON for measurement events
func (s *MeasurementService) logMeasurement(m *domain.Measurement, event string) {
	logEntry := map[string]interface{}{
		"event":          event,
		"measurement_id": m.ID.String(),
		"baby_id":        m.BabyID.String(),
		"type":           m.Type,
		"value":          m.Value,
		"safety_status":  string(m.SafetyStatus),
		"created_at":     m.CreatedAt.Format(time.RFC3339),
	}

	if m.Note != "" {
		logEntry["note"] = m.Note
	}

	if m.Type == domain.MeasurementTypeFeeding {
		logEntry["feeding_type"] = string(m.FeedingType)
		if m.FeedingType == domain.FeedingTypeBottle && m.VolumeML != nil {
			logEntry["volume_ml"] = *m.VolumeML
		}
		if m.FeedingType == domain.FeedingTypeBreast {
			if m.Side != nil {
				logEntry["side"] = string(*m.Side)
			}
			if m.Position != nil {
				logEntry["position"] = string(*m.Position)
			}
			if m.Side != nil && *m.Side == domain.SideBoth {
				if m.LeftDuration != nil {
					logEntry["left_duration_seconds"] = *m.LeftDuration
				}
				if m.RightDuration != nil {
					logEntry["right_duration_seconds"] = *m.RightDuration
				}
			} else if m.Duration != nil {
				logEntry["duration_seconds"] = *m.Duration
			}
		}
	}
	
	if m.Type == domain.MeasurementTypeTemperature && m.ValueCelsius != nil {
		logEntry["value_celsius"] = *m.ValueCelsius
	}
	
	if m.Type == domain.MeasurementTypeDiaper && m.DiaperStatus != nil {
		logEntry["diaper_status"] = string(*m.DiaperStatus)
	}
	
	if !m.Timestamp.IsZero() {
		logEntry["timestamp"] = m.Timestamp.Format(time.RFC3339)
	}

	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("Failed to marshal measurement log entry: %v", err)
		return
	}

	log.Printf("%s", string(jsonBytes))
}

// GetMeasurements retrieves all measurements for a baby
// Enforces ownership: ADMIN can access any, PARENT only their own babies
// Optional filters: measurementType (filter by type), limit (max results)
func (s *MeasurementService) GetMeasurements(
	ctx context.Context,
	babyID uuid.UUID,
	userID uuid.UUID,
	isAdmin bool,
	measurementType *string,
	limit *int,
) ([]*domain.Measurement, error) {
	// Check if baby exists
	exists, err := s.babyRepo.BabyExists(ctx, babyID)
	if err != nil {
		return nil, fmt.Errorf("failed to check baby existence: %w", err)
	}
	if !exists {
		// Don't leak ownership info
		return nil, fmt.Errorf("baby not found")
	}

	// RBAC enforcement: PARENT can only access their own babies
	if !isAdmin {
		owned, err := s.babyRepo.CheckBabyOwnership(ctx, babyID, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to check ownership: %w", err)
		}
		if !owned {
			// Don't leak ownership info - return generic not found
			return nil, fmt.Errorf("baby not found")
		}
	}

	// Validate measurement type filter if provided
	if measurementType != nil && !domain.IsValidMeasurementType(*measurementType) {
		return nil, fmt.Errorf("invalid measurement type filter: %s", *measurementType)
	}

	// Validate limit if provided
	if limit != nil && *limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than 0")
	}

	measurements, err := s.measurementRepo.GetMeasurementsByBabyID(ctx, babyID, measurementType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get measurements: %w", err)
	}

	return measurements, nil
}

// GetMeasurementByID retrieves a specific measurement by ID
// Enforces ownership: ADMIN can access any, PARENT only their own babies' measurements
func (s *MeasurementService) GetMeasurementByID(
	ctx context.Context,
	measurementID uuid.UUID,
	userID uuid.UUID,
	isAdmin bool,
) (*domain.Measurement, error) {
	// Get measurement
	measurement, err := s.measurementRepo.GetMeasurementByID(ctx, measurementID)
	if err != nil {
		// Check if the underlying error is sql.ErrNoRows or "measurement not found"
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("measurement not found")
		}
		errStr := strings.ToLower(err.Error())
		// Check for "measurement not found" or "no rows" in error message (case-insensitive)
		// This catches errors wrapped by retry logic like "operation failed after X retries: sql: no rows in result set"
		if strings.Contains(errStr, "measurement not found") || 
			strings.Contains(errStr, "no rows") ||
			strings.Contains(errStr, "sql: no rows") {
			return nil, fmt.Errorf("measurement not found")
		}
		// For other errors, wrap but preserve the original error message for debugging
		return nil, fmt.Errorf("failed to get measurement: %w", err)
	}
	
	// Safety check: measurement should never be nil if err is nil, but check anyway
	if measurement == nil {
		return nil, fmt.Errorf("measurement not found")
	}

	// Check if baby exists and enforce ownership
	exists, err := s.babyRepo.BabyExists(ctx, measurement.BabyID)
	if err != nil {
		return nil, fmt.Errorf("failed to check baby existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("measurement not found")
	}

	// RBAC enforcement: PARENT can only access their own babies' measurements
	if !isAdmin {
		owned, err := s.babyRepo.CheckBabyOwnership(ctx, measurement.BabyID, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to check ownership: %w", err)
		}
		if !owned {
			// Don't leak ownership info - return generic not found
			return nil, fmt.Errorf("measurement not found")
		}
	}

	return measurement, nil
}

// DeleteMeasurement deletes a measurement by ID
// Enforces ownership: Only the parent who created the measurement can delete it
// ADMIN cannot delete measurements (read-only access)
func (s *MeasurementService) DeleteMeasurement(
	ctx context.Context,
	measurementID uuid.UUID,
	userID uuid.UUID,
	isAdmin bool,
) error {
	// RBAC enforcement: ADMIN cannot delete measurements
	if isAdmin {
		return fmt.Errorf("forbidden: only PARENT can delete measurements")
	}

	// Get measurement first to validate ownership
	measurement, err := s.measurementRepo.GetMeasurementByID(ctx, measurementID)
	if err != nil {
		// Check if the underlying error is sql.ErrNoRows or "measurement not found"
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("measurement not found")
		}
		errStr := strings.ToLower(err.Error())
		// Check for "measurement not found" or "no rows" in error message (case-insensitive)
		// This catches errors wrapped by retry logic
		if strings.Contains(errStr, "measurement not found") || 
			strings.Contains(errStr, "no rows") ||
			strings.Contains(errStr, "sql: no rows") {
			return fmt.Errorf("measurement not found")
		}
		return fmt.Errorf("failed to get measurement: %w", err)
	}

	// RBAC enforcement: Only the parent who created the measurement can delete
	if measurement.ParentID != userID {
		// Don't leak ownership info - return generic not found
		return fmt.Errorf("measurement not found")
	}

	// Delete measurement - pass userID to validate ownership
	err = s.measurementRepo.DeleteMeasurement(ctx, measurementID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete measurement: %w", err)
	}

	return nil
}
