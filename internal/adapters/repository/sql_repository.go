package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/IANDYI/care-service/internal/core/ports"
	"github.com/google/uuid"
	"github.com/sony/gobreaker"
)

// SQLRepository implements BabyRepository and MeasurementRepository using PostgreSQL
// Includes retry logic and circuit breaker for resilience
type SQLRepository struct {
	db            *sql.DB
	babyCB        *gobreaker.CircuitBreaker
	measurementCB *gobreaker.CircuitBreaker
	maxRetries    int
	retryDelay    time.Duration
}

// NewSQLRepository creates a new PostgreSQL repository with circuit breakers
func NewSQLRepository(db *sql.DB) *SQLRepository {
	// Circuit breaker settings
	settings := gobreaker.Settings{
		Name:        "database",
		MaxRequests: 5,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 5
		},
	}

	return &SQLRepository{
		db:            db,
		babyCB:        gobreaker.NewCircuitBreaker(settings),
		measurementCB: gobreaker.NewCircuitBreaker(settings),
		maxRetries:    3,
		retryDelay:    1 * time.Second,
	}
}

// executeWithRetry executes a database operation with retry logic
func (r *SQLRepository) executeWithRetry(ctx context.Context, operation func() error) error {
	var lastErr error
	for i := 0; i < r.maxRetries; i++ {
		err := operation()
		if err == nil {
			return nil
		}
		lastErr = err
		// Don't retry on sql.ErrNoRows - it's not a transient error
		// Check both the error itself and its string representation
		if errors.Is(err, sql.ErrNoRows) || err == sql.ErrNoRows || 
			strings.Contains(strings.ToLower(err.Error()), "no rows") {
			return err
		}
		if i < r.maxRetries-1 {
			time.Sleep(r.retryDelay)
		}
	}
	return fmt.Errorf("operation failed after %d retries: %w", r.maxRetries, lastErr)
}

// BabyRepository implementation

func (r *SQLRepository) CreateBaby(ctx context.Context, baby *domain.Baby) error {
	_, err := r.babyCB.Execute(func() (interface{}, error) {
		return nil, r.executeWithRetry(ctx, func() error {
			query := `INSERT INTO babies (id, last_name, room_number, parent_user_id, created_at) VALUES ($1, $2, $3, $4, $5)`
			_, err := r.db.ExecContext(ctx, query, baby.ID, baby.LastName, baby.RoomNumber, baby.ParentUserID, baby.CreatedAt)
			return err
		})
	})
	return err
}

func (r *SQLRepository) GetBabyByID(ctx context.Context, babyID uuid.UUID) (*domain.Baby, error) {
	result, err := r.babyCB.Execute(func() (interface{}, error) {
		var baby domain.Baby
		err := r.executeWithRetry(ctx, func() error {
			query := `SELECT id, last_name, room_number, parent_user_id, created_at FROM babies WHERE id = $1`
			row := r.db.QueryRowContext(ctx, query, babyID)
			return row.Scan(&baby.ID, &baby.LastName, &baby.RoomNumber, &baby.ParentUserID, &baby.CreatedAt)
		})
		if err != nil {
			return nil, err
		}
		return &baby, nil
	})

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("baby not found")
		}
		return nil, err
	}

	return result.(*domain.Baby), nil
}

func (r *SQLRepository) ListBabies(ctx context.Context, parentUserID uuid.UUID, isAdmin bool) ([]*domain.Baby, error) {
	result, err := r.babyCB.Execute(func() (interface{}, error) {
		var babies []*domain.Baby
		err := r.executeWithRetry(ctx, func() error {
			var rows *sql.Rows
			var queryErr error

			if isAdmin {
				// ADMIN can see all babies
				rows, queryErr = r.db.QueryContext(ctx, `SELECT id, last_name, room_number, parent_user_id, created_at FROM babies ORDER BY created_at DESC`)
			} else {
				// PARENT can only see their own babies
				rows, queryErr = r.db.QueryContext(ctx, `SELECT id, last_name, room_number, parent_user_id, created_at FROM babies WHERE parent_user_id = $1 ORDER BY created_at DESC`, parentUserID)
			}

			if queryErr != nil {
				return queryErr
			}
			defer rows.Close()

			for rows.Next() {
				var baby domain.Baby
				if err := rows.Scan(&baby.ID, &baby.LastName, &baby.RoomNumber, &baby.ParentUserID, &baby.CreatedAt); err != nil {
					return err
				}
				babies = append(babies, &baby)
			}

			return rows.Err()
		})
		if err != nil {
			return nil, err
		}
		return babies, nil
	})

	if err != nil {
		return nil, err
	}

	return result.([]*domain.Baby), nil
}

func (r *SQLRepository) BabyExists(ctx context.Context, babyID uuid.UUID) (bool, error) {
	result, err := r.babyCB.Execute(func() (interface{}, error) {
		var exists bool
		err := r.executeWithRetry(ctx, func() error {
			var count int
			query := `SELECT COUNT(*) FROM babies WHERE id = $1`
			err := r.db.QueryRowContext(ctx, query, babyID).Scan(&count)
			exists = count > 0
			return err
		})
		if err != nil {
			return nil, err
		}
		return exists, nil
	})

	if err != nil {
		return false, err
	}

	return result.(bool), nil
}

func (r *SQLRepository) CheckBabyOwnership(ctx context.Context, babyID uuid.UUID, parentUserID uuid.UUID) (bool, error) {
	result, err := r.babyCB.Execute(func() (interface{}, error) {
		var owned bool
		err := r.executeWithRetry(ctx, func() error {
			var count int
			query := `SELECT COUNT(*) FROM babies WHERE id = $1 AND parent_user_id = $2`
			err := r.db.QueryRowContext(ctx, query, babyID, parentUserID).Scan(&count)
			owned = count > 0
			return err
		})
		if err != nil {
			return nil, err
		}
		return owned, nil
	})

	if err != nil {
		return false, err
	}

	return result.(bool), nil
}

// MeasurementRepository implementation

func (r *SQLRepository) CreateMeasurement(ctx context.Context, measurement *domain.Measurement) error {
	_, err := r.measurementCB.Execute(func() (interface{}, error) {
		return nil, r.executeWithRetry(ctx, func() error {
			query := `INSERT INTO measurements (
				id, parent_id, baby_id, type, value, safety_status, note, timestamp, created_at,
				feeding_type, volume_ml, position, side, left_duration, right_duration, duration,
				value_celsius, diaper_status
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`
			
			var feedingType interface{}
			if measurement.FeedingType != "" {
				feedingType = string(measurement.FeedingType)
			}
			
			var position interface{}
			if measurement.Position != nil {
				position = string(*measurement.Position)
			}
			
			var side interface{}
			if measurement.Side != nil {
				side = string(*measurement.Side)
			}
			
			var diaperStatus interface{}
			if measurement.DiaperStatus != nil {
				diaperStatus = string(*measurement.DiaperStatus)
			}
			
			_, err := r.db.ExecContext(ctx, query,
				measurement.ID,
				measurement.ParentID,
				measurement.BabyID,
				measurement.Type,
				measurement.Value,
				string(measurement.SafetyStatus),
				measurement.Note,
				measurement.Timestamp,
				measurement.CreatedAt,
				feedingType,
				measurement.VolumeML,
				position,
				side,
				measurement.LeftDuration,
				measurement.RightDuration,
				measurement.Duration,
				measurement.ValueCelsius,
				diaperStatus,
			)
			return err
		})
	})
	return err
}

func (r *SQLRepository) GetMeasurementsByBabyID(ctx context.Context, babyID uuid.UUID, measurementType *string, limit *int) ([]*domain.Measurement, error) {
	result, err := r.measurementCB.Execute(func() (interface{}, error) {
		var measurements []*domain.Measurement
		err := r.executeWithRetry(ctx, func() error {
			// Build query with optional filters
			query := `SELECT id, parent_id, baby_id, type, value, safety_status, note, timestamp, created_at,
				feeding_type, volume_ml, position, side, left_duration, right_duration, duration,
				value_celsius, diaper_status
				FROM measurements WHERE baby_id = $1`
			
			args := []interface{}{babyID}
			argIndex := 2
			
			// Add type filter if provided
			if measurementType != nil {
				query += fmt.Sprintf(" AND type = $%d", argIndex)
				args = append(args, *measurementType)
				argIndex++
			}
			
			// Add ordering
			query += " ORDER BY timestamp DESC, created_at DESC"
			
			// Add limit if provided
			if limit != nil {
				query += fmt.Sprintf(" LIMIT $%d", argIndex)
				args = append(args, *limit)
			}
			
			rows, queryErr := r.db.QueryContext(ctx, query, args...)
			if queryErr != nil {
				return queryErr
			}
			defer rows.Close()

			for rows.Next() {
				m, err := r.scanMeasurement(rows)
				if err != nil {
					return err
				}
				measurements = append(measurements, m)
			}

			return rows.Err()
		})
		if err != nil {
			return nil, err
		}
		return measurements, nil
	})

	if err != nil {
		return nil, err
	}

	return result.([]*domain.Measurement), nil
}

// scanMeasurement scans a measurement row from the database
func (r *SQLRepository) scanMeasurement(rows *sql.Rows) (*domain.Measurement, error) {
	var m domain.Measurement
	var safetyStatusStr string
	var timestamp sql.NullTime
	
	// Feeding fields
	var feedingTypeStr sql.NullString
	var volumeML sql.NullInt64
	var positionStr sql.NullString
	var sideStr sql.NullString
	var leftDuration sql.NullInt64
	var rightDuration sql.NullInt64
	var duration sql.NullInt64
	
	// Temperature fields
	var valueCelsius sql.NullFloat64
	
	// Diaper fields
	var diaperStatusStr sql.NullString

	err := rows.Scan(
		&m.ID, &m.ParentID, &m.BabyID, &m.Type, &m.Value, &safetyStatusStr, &m.Note,
		&timestamp, &m.CreatedAt,
		&feedingTypeStr, &volumeML, &positionStr, &sideStr,
		&leftDuration, &rightDuration, &duration,
		&valueCelsius, &diaperStatusStr,
	)
	if err != nil {
		return nil, err
	}

	m.SafetyStatus = domain.SafetyStatus(safetyStatusStr)
	if timestamp.Valid {
		m.Timestamp = timestamp.Time
	}

	// Set feeding fields
	if feedingTypeStr.Valid {
		m.FeedingType = domain.FeedingType(feedingTypeStr.String)
	}
	if volumeML.Valid {
		vol := int(volumeML.Int64)
		m.VolumeML = &vol
	}
	if positionStr.Valid {
		pos := domain.BreastfeedingPosition(positionStr.String)
		m.Position = &pos
	}
	if sideStr.Valid {
		side := domain.BreastfeedingSide(sideStr.String)
		m.Side = &side
	}
	if leftDuration.Valid {
		dur := int(leftDuration.Int64)
		m.LeftDuration = &dur
	}
	if rightDuration.Valid {
		dur := int(rightDuration.Int64)
		m.RightDuration = &dur
	}
	if duration.Valid {
		durInt := int(duration.Int64)
		m.Duration = &durInt
	}

	// Set temperature fields
	if valueCelsius.Valid {
		m.ValueCelsius = &valueCelsius.Float64
	}

	// Set diaper fields
	if diaperStatusStr.Valid {
		status := domain.DiaperStatus(diaperStatusStr.String)
		m.DiaperStatus = &status
	}

	return &m, nil
}

func (r *SQLRepository) GetMeasurementByID(ctx context.Context, measurementID uuid.UUID) (*domain.Measurement, error) {
	result, err := r.measurementCB.Execute(func() (interface{}, error) {
		var measurement *domain.Measurement

		err := r.executeWithRetry(ctx, func() error {
			query := `SELECT id, parent_id, baby_id, type, value, safety_status, note, timestamp, created_at,
				feeding_type, volume_ml, position, side, left_duration, right_duration, duration,
				value_celsius, diaper_status
				FROM measurements WHERE id = $1`
			
			rows, err := r.db.QueryContext(ctx, query, measurementID)
			if err != nil {
				return err
			}
			defer rows.Close()
			
			if !rows.Next() {
				return sql.ErrNoRows
			}
			
			measurement, err = r.scanMeasurement(rows)
			return err
		})
		if err != nil {
			return nil, err
		}

		return measurement, nil
	})

	if err != nil {
		// Check if the error is sql.ErrNoRows (even if wrapped)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("measurement not found")
		}
		// Check error message for wrapped errors from retry logic or circuit breaker
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "no rows") || 
			strings.Contains(errStr, "measurement not found") ||
			strings.Contains(errStr, "sql: no rows") {
			return nil, fmt.Errorf("measurement not found")
		}
		return nil, err
	}

	if result == nil {
		return nil, fmt.Errorf("measurement not found")
	}

	return result.(*domain.Measurement), nil
}

// DeleteMeasurement deletes a measurement by ID
// If parentID is provided (non-nil UUID), validates that the measurement belongs to that parent
// If parentID is nil (uuid.Nil), allows deletion without parent validation (for ADMIN)
func (r *SQLRepository) DeleteMeasurement(ctx context.Context, measurementID uuid.UUID, parentID uuid.UUID) error {
	_, err := r.measurementCB.Execute(func() (interface{}, error) {
		return nil, r.executeWithRetry(ctx, func() error {
			var query string
			var args []interface{}
			
			if parentID != uuid.Nil {
				// Validate ownership: check measurement exists and belongs to parent
				var count int
				checkQuery := `SELECT COUNT(*) FROM measurements WHERE id = $1 AND parent_id = $2`
				err := r.db.QueryRowContext(ctx, checkQuery, measurementID, parentID).Scan(&count)
				if err != nil {
					return fmt.Errorf("failed to verify measurement ownership: %w", err)
				}
				if count == 0 {
					return fmt.Errorf("measurement not found")
				}

				// Delete with parent validation
				query = `DELETE FROM measurements WHERE id = $1 AND parent_id = $2`
				args = []interface{}{measurementID, parentID}
			} else {
				// ADMIN deletion: no parent validation
				// First verify measurement exists
				var count int
				checkQuery := `SELECT COUNT(*) FROM measurements WHERE id = $1`
				err := r.db.QueryRowContext(ctx, checkQuery, measurementID).Scan(&count)
				if err != nil {
					return fmt.Errorf("failed to verify measurement exists: %w", err)
				}
				if count == 0 {
					return fmt.Errorf("measurement not found")
				}

				// Delete without parent validation
				query = `DELETE FROM measurements WHERE id = $1`
				args = []interface{}{measurementID}
			}

			result, err := r.db.ExecContext(ctx, query, args...)
			if err != nil {
				return err
			}

			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if rowsAffected == 0 {
				return fmt.Errorf("measurement not found")
			}

			return nil
		})
	})
	return err
}

// Ensure SQLRepository implements the interfaces
var _ ports.BabyRepository = (*SQLRepository)(nil)
var _ ports.MeasurementRepository = (*SQLRepository)(nil)

