package ports

import (
	"context"

	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/google/uuid"
)

// BabyRepository defines the interface for baby data persistence
type BabyRepository interface {
	// CreateBaby creates a new baby (ADMIN only)
	CreateBaby(ctx context.Context, baby *domain.Baby) error

	// GetBabyByID retrieves a baby by ID
	// Returns error if baby doesn't exist or user doesn't have access
	GetBabyByID(ctx context.Context, babyID uuid.UUID) (*domain.Baby, error)

	// ListBabies retrieves babies based on role:
	// ADMIN: all babies
	// PARENT: only babies where parent_user_id matches
	ListBabies(ctx context.Context, parentUserID uuid.UUID, isAdmin bool) ([]*domain.Baby, error)

	// BabyExists checks if a baby exists
	BabyExists(ctx context.Context, babyID uuid.UUID) (bool, error)

	// CheckBabyOwnership checks if a baby belongs to a specific parent
	CheckBabyOwnership(ctx context.Context, babyID uuid.UUID, parentUserID uuid.UUID) (bool, error)
}

// MeasurementRepository defines the interface for measurement data persistence
type MeasurementRepository interface {
	// CreateMeasurement creates a new measurement for a baby
	CreateMeasurement(ctx context.Context, measurement *domain.Measurement) error

	// GetMeasurementsByBabyID retrieves all measurements for a baby
	// Optional filters: measurementType (filter by type), limit (max results)
	GetMeasurementsByBabyID(ctx context.Context, babyID uuid.UUID, measurementType *string, limit *int) ([]*domain.Measurement, error)

	// GetMeasurementByID retrieves a specific measurement
	GetMeasurementByID(ctx context.Context, measurementID uuid.UUID) (*domain.Measurement, error)

	// DeleteMeasurement deletes a measurement by ID
	// Validates that the measurement belongs to the specified parent before deletion
	DeleteMeasurement(ctx context.Context, measurementID uuid.UUID, parentID uuid.UUID) error
}

// AlertPublisher defines the interface for publishing alerts to RabbitMQ
type AlertPublisher interface {
	// PublishAlert publishes an alert event for abnormal measurements
	PublishAlert(ctx context.Context, babyID uuid.UUID, measurement *domain.Measurement) error
}

