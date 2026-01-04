package services

import (
	"fmt"

	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/IANDYI/care-service/internal/core/ports"
)

// setFeedingFields sets feeding-specific fields on a measurement
func (s *MeasurementService) setFeedingFields(measurement *domain.Measurement, req ports.CreateMeasurementRequest) error {
	if req.FeedingType == "" {
		return fmt.Errorf("feeding type must be specified (bottle or breast)")
	}

	feedingType := domain.FeedingType(req.FeedingType)
	if feedingType != domain.FeedingTypeBottle && feedingType != domain.FeedingTypeBreast {
		return fmt.Errorf("feeding type must be 'bottle' or 'breast'")
	}

	measurement.FeedingType = feedingType

	if feedingType == domain.FeedingTypeBottle {
		// Bottle feeding: requires VolumeML
		if req.VolumeML == nil || *req.VolumeML <= 0 {
			return fmt.Errorf("bottle feeding requires volume_ml > 0")
		}
		if *req.VolumeML > 500 {
			return fmt.Errorf("bottle volume exceeds reasonable maximum (500ml)")
		}
		measurement.VolumeML = req.VolumeML
		measurement.Value = float64(*req.VolumeML) // Store volume as value for consistency
	} else {
		// Breast feeding: requires Side and Position
		if req.Side == "" {
			return fmt.Errorf("breast feeding requires side (left, right, or both)")
		}

		side := domain.BreastfeedingSide(req.Side)
		if !domain.IsValidBreastfeedingSide(side) {
			return fmt.Errorf("invalid side: must be 'left', 'right', or 'both'")
		}

		measurement.Side = &side

		if req.Position != "" {
			position := domain.BreastfeedingPosition(req.Position)
			if !domain.IsValidBreastfeedingPosition(position) {
				return fmt.Errorf("invalid breastfeeding position: %s", req.Position)
			}
			measurement.Position = &position
		}

		if side == domain.SideBoth {
			// Both sides: requires LeftDuration and RightDuration
			if req.LeftDuration == nil || *req.LeftDuration <= 0 {
				return fmt.Errorf("breast feeding with both sides requires left_duration > 0")
			}
			if req.RightDuration == nil || *req.RightDuration <= 0 {
				return fmt.Errorf("breast feeding with both sides requires right_duration > 0")
			}
			measurement.LeftDuration = req.LeftDuration
			measurement.RightDuration = req.RightDuration
			// Calculate total duration in seconds
			totalSeconds := *req.LeftDuration + *req.RightDuration
			measurement.Value = float64(totalSeconds)
		} else {
			// Single side: requires Duration (in seconds)
			if req.Duration == nil || *req.Duration <= 0 {
				return fmt.Errorf("breast feeding with single side requires duration > 0 seconds")
			}
			if *req.Duration > 3600 {
				return fmt.Errorf("breast feeding duration exceeds reasonable maximum (3600 seconds / 60 minutes)")
			}
			measurement.Duration = req.Duration
			// Store duration in seconds as value for consistency
			measurement.Value = float64(*req.Duration)
		}
	}

	return nil
}

// setTemperatureFields sets temperature-specific fields on a measurement
func (s *MeasurementService) setTemperatureFields(measurement *domain.Measurement, req ports.CreateMeasurementRequest) error {
	// Use ValueCelsius if provided, otherwise fall back to Value
	var tempValue float64
	if req.ValueCelsius != nil {
		tempValue = *req.ValueCelsius
	} else {
		tempValue = req.Value
	}

	if tempValue < 30.0 || tempValue > 42.0 {
		return fmt.Errorf("temperature must be between 30.0 and 42.0°C")
	}

	measurement.ValueCelsius = &tempValue
	measurement.Value = tempValue

	return nil
}

// setDiaperFields sets diaper-specific fields on a measurement
func (s *MeasurementService) setDiaperFields(measurement *domain.Measurement, req ports.CreateMeasurementRequest) error {
	if req.DiaperStatus == "" {
		return fmt.Errorf("diaper status must be specified (dry, wet, dirty, or both)")
	}

	status := domain.DiaperStatus(req.DiaperStatus)
	if !domain.IsValidDiaperStatus(status) {
		return fmt.Errorf("invalid diaper status: must be 'dry', 'wet', 'dirty', or 'both'")
	}

	measurement.DiaperStatus = &status
	// Diaper changes don't have a numeric value, set to 0
	measurement.Value = 0

	return nil
}

