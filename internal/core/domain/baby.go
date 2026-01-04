package domain

import (
	"time"

	"github.com/google/uuid"
)

// Baby represents a baby in the system
// Parent ownership is enforced via parent_user_id from JWT claims
type Baby struct {
	ID           uuid.UUID `json:"id"`
	LastName     string    `json:"last_name"`
	RoomNumber   string    `json:"room_number"`
	ParentUserID uuid.UUID `json:"parent_user_id"` // From Identity Service JWT
	CreatedAt    time.Time `json:"created_at"`
}

// SafetyStatus represents the safety status of a measurement
type SafetyStatus string

const (
	SafetyStatusGreen  SafetyStatus = "green"  // Normal - within acceptable range
	SafetyStatusYellow SafetyStatus = "yellow" // Borderline - slightly outside normal range
	SafetyStatusRed    SafetyStatus = "red"    // Critical - abnormal, requires immediate attention
)

// FeedingType represents the type of feeding
type FeedingType string

const (
	FeedingTypeBottle FeedingType = "bottle" // Bottle feeding with amount in ml
	FeedingTypeBreast FeedingType = "breast" // Breast feeding with duration in minutes
)

// BreastfeedingPosition represents the position used for breastfeeding
type BreastfeedingPosition string

const (
	PositionCrossCradle BreastfeedingPosition = "cross_cradle" // Best for latch control
	PositionCradle      BreastfeedingPosition = "cradle"        // Standard/Comfortable
	PositionFootball   BreastfeedingPosition = "football"       // Best for C-section recovery
	PositionSideLying   BreastfeedingPosition = "side_lying"    // Best for night feedings
	PositionLaidBack    BreastfeedingPosition = "laid_back"     // Encourages natural instincts
)

// BreastfeedingSide represents which side(s) were used for breastfeeding
type BreastfeedingSide string

const (
	SideLeft  BreastfeedingSide = "left"  // Left side only
	SideRight BreastfeedingSide = "right"  // Right side only
	SideBoth  BreastfeedingSide = "both"   // Both sides
)

// DiaperStatus represents the status of a diaper change
type DiaperStatus string

const (
	DiaperStatusDry   DiaperStatus = "dry"   // Dry diaper
	DiaperStatusWet   DiaperStatus = "wet"   // Wet diaper
	DiaperStatusDirty DiaperStatus = "dirty" // Dirty diaper
	DiaperStatusBoth  DiaperStatus = "both"  // Both wet and dirty
)

// Measurement represents a measurement taken for a baby
// Types: feeding, weight, temperature, diaper
type Measurement struct {
	ID           uuid.UUID     `json:"id"`
	ParentID     uuid.UUID     `json:"parent_id"`     // Parent who logged the measurement
	BabyID       uuid.UUID     `json:"baby_id"`
	Type         string        `json:"type"`          // feeding, weight, temperature, diaper
	Value        float64       `json:"value"`         // Numeric value (weight in grams, temperature in Celsius)
	SafetyStatus SafetyStatus  `json:"safety_status"` // Green, Yellow, or Red
	Note         string        `json:"note"`          // Optional contextual metadata
	Timestamp    time.Time     `json:"timestamp"`    // When the measurement was taken
	CreatedAt    time.Time     `json:"created_at"`   // When the record was created
	
	// Feeding-specific fields (only used when Type == "feeding")
	FeedingType     FeedingType         `json:"feeding_type,omitempty"`     // bottle or breast
	VolumeML        *int                `json:"volume_ml,omitempty"`      // ml for bottle feeding
	Position         *BreastfeedingPosition `json:"position,omitempty"`   // Position for breast feeding
	Side             *BreastfeedingSide    `json:"side,omitempty"`       // Side(s) for breast feeding
	LeftDuration     *int                `json:"left_duration,omitempty"`  // Duration in seconds for left side
	RightDuration    *int                `json:"right_duration,omitempty"` // Duration in seconds for right side
	Duration         *int                `json:"duration,omitempty"`       // Total duration in seconds (for single side)
	
	// Temperature-specific fields (only used when Type == "temperature")
	ValueCelsius     *float64           `json:"value_celsius,omitempty"`  // Temperature in Celsius
	
	// Diaper-specific fields (only used when Type == "diaper")
	DiaperStatus     *DiaperStatus      `json:"diaper_status,omitempty"`  // Status of diaper change
}

// MeasurementType constants for validation
const (
	MeasurementTypeFeeding     = "feeding"
	MeasurementTypeWeight      = "weight"
	MeasurementTypeTemperature = "temperature"
	MeasurementTypeDiaper      = "diaper"
)

// ValidMeasurementTypes returns a slice of valid measurement types
func ValidMeasurementTypes() []string {
	return []string{
		MeasurementTypeFeeding,
		MeasurementTypeWeight,
		MeasurementTypeTemperature,
		MeasurementTypeDiaper,
	}
}

// IsValidMeasurementType checks if a measurement type is valid
func IsValidMeasurementType(measurementType string) bool {
	validTypes := ValidMeasurementTypes()
	for _, t := range validTypes {
		if t == measurementType {
			return true
		}
	}
	return false
}

// TemperatureNormalRange defines the normal temperature range in Celsius
const (
	TemperatureNormalMin = 36.5
	TemperatureNormalMax = 37.5
	TemperatureYellowMin = 36.0 // Below this is yellow
	TemperatureYellowMax = 38.0 // Above this is yellow
)

// CalculateSafetyStatus calculates the safety status based on measurement type and value
// Temperature: Green (36.5-37.5°C), Yellow (36.0-36.5 or 37.5-38.0°C), Red (<36.0 or >38.0°C)
// Weight: Green (valid positive value), Yellow (0 or negative), Red (not applicable for weight)
// Feeding: Green (valid feeding), Yellow/Red (not applicable for feeding)
func CalculateSafetyStatus(measurementType string, value float64) SafetyStatus {
	switch measurementType {
	case MeasurementTypeTemperature:
		if value >= TemperatureNormalMin && value <= TemperatureNormalMax {
			return SafetyStatusGreen
		}
		if value >= TemperatureYellowMin && value < TemperatureNormalMin {
			return SafetyStatusYellow // Slightly below normal
		}
		if value > TemperatureNormalMax && value <= TemperatureYellowMax {
			return SafetyStatusYellow // Slightly above normal
		}
		return SafetyStatusRed // Critical: <36.0 or >38.0°C
	case MeasurementTypeWeight:
		if value > 0 {
			return SafetyStatusGreen // Valid weight
		}
		return SafetyStatusYellow // Invalid weight (0 or negative)
	case MeasurementTypeFeeding:
		// Feeding measurements are always considered safe (Green)
		// Validation happens at the service level
		return SafetyStatusGreen
	case MeasurementTypeDiaper:
		// Diaper changes are always considered safe (Green)
		return SafetyStatusGreen
	default:
		return SafetyStatusGreen // Default to safe
	}
}

// IsAbnormalMeasurement checks if a measurement requires an alert (Red status)
// Returns true if SafetyStatus is Red
func IsAbnormalMeasurement(m *Measurement) bool {
	return m.SafetyStatus == SafetyStatusRed
}

