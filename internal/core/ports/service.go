package ports

import (
	"context"
	"time"

	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/google/uuid"
)

// BabyService defines the business logic interface for baby operations
type BabyService interface {
	// CreateBaby creates a new baby (ADMIN only)
	// Validates input and enforces RBAC
	CreateBaby(ctx context.Context, lastName string, roomNumber string, parentUserID uuid.UUID, createdByUserID uuid.UUID, isAdmin bool) (*domain.Baby, error)

	// GetBaby retrieves a baby by ID
	// Enforces ownership: ADMIN can access any, PARENT only their own
	GetBaby(ctx context.Context, babyID uuid.UUID, userID uuid.UUID, isAdmin bool) (*domain.Baby, error)

	// ListBabies retrieves babies based on role
	// ADMIN: all babies, PARENT: only owned babies
	ListBabies(ctx context.Context, userID uuid.UUID, isAdmin bool) ([]*domain.Baby, error)
}

// MeasurementService defines the business logic interface for measurement operations
type MeasurementService interface {
	// CreateMeasurement creates a new measurement for a baby (backward compatible)
	// Enforces ownership: Only PARENT can add measurements to their own babies
	// ADMIN cannot create measurements (read-only access)
	// Publishes alerts for Red status measurements
	CreateMeasurement(ctx context.Context, babyID uuid.UUID, measurementType string, value float64, note string, userID uuid.UUID, isAdmin bool) (*domain.Measurement, error)

	// CreateMeasurementWithDetails creates a measurement with full details including feeding-specific fields
	// This method supports feeding types (bottle/breast) with amount/duration
	// Only PARENT can create measurements for their own babies
	CreateMeasurementWithDetails(ctx context.Context, babyID uuid.UUID, req CreateMeasurementRequest, userID uuid.UUID, isAdmin bool) (*domain.Measurement, error)

	// GetMeasurements retrieves all measurements for a baby
	// Enforces ownership: ADMIN can access any, PARENT only their own babies
	// Optional filters: measurementType (filter by type), limit (max results)
	GetMeasurements(ctx context.Context, babyID uuid.UUID, userID uuid.UUID, isAdmin bool, measurementType *string, limit *int) ([]*domain.Measurement, error)

	// GetMeasurementByID retrieves a specific measurement by ID
	// Enforces ownership: ADMIN can access any, PARENT only their own babies' measurements
	GetMeasurementByID(ctx context.Context, measurementID uuid.UUID, userID uuid.UUID, isAdmin bool) (*domain.Measurement, error)

	// DeleteMeasurement deletes a measurement by ID
	// Enforces ownership: Only the parent who created the measurement can delete it
	// ADMIN cannot delete measurements (read-only access)
	DeleteMeasurement(ctx context.Context, measurementID uuid.UUID, userID uuid.UUID, isAdmin bool) error
}

// CreateMeasurementRequest represents the input for creating a measurement with full details
type CreateMeasurementRequest struct {
	Type        string    `json:"type"`          // feeding, weight, temperature, diaper
	Value       float64   `json:"value"`        // Numeric value (weight in grams, temperature in Celsius)
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

