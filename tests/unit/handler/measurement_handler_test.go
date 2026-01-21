package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/IANDYI/care-service/internal/adapters/handler" //nolint:staticcheck // handler package contains non-deprecated code
	"github.com/IANDYI/care-service/internal/adapters/middleware"
	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/IANDYI/care-service/internal/core/ports"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockMeasurementService is a mock implementation of ports.MeasurementService
type MockMeasurementService struct {
	mock.Mock
}

func (m *MockMeasurementService) CreateMeasurement(ctx context.Context, babyID uuid.UUID, measurementType string, value float64, note string, userID uuid.UUID, isAdmin bool) (*domain.Measurement, error) {
	args := m.Called(ctx, babyID, measurementType, value, note, userID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Measurement), args.Error(1)
}

func (m *MockMeasurementService) CreateMeasurementWithDetails(ctx context.Context, babyID uuid.UUID, req ports.CreateMeasurementRequest, userID uuid.UUID, isAdmin bool) (*domain.Measurement, error) {
	args := m.Called(ctx, babyID, req, userID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Measurement), args.Error(1)
}

func (m *MockMeasurementService) GetMeasurements(ctx context.Context, babyID uuid.UUID, userID uuid.UUID, isAdmin bool, measurementType *string, limit *int) ([]*domain.Measurement, error) {
	args := m.Called(ctx, babyID, userID, isAdmin, measurementType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Measurement), args.Error(1)
}

func (m *MockMeasurementService) GetMeasurementByID(ctx context.Context, measurementID uuid.UUID, userID uuid.UUID, isAdmin bool) (*domain.Measurement, error) {
	args := m.Called(ctx, measurementID, userID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Measurement), args.Error(1)
}

func (m *MockMeasurementService) DeleteMeasurement(ctx context.Context, measurementID uuid.UUID, userID uuid.UUID, isAdmin bool) error {
	args := m.Called(ctx, measurementID, userID, isAdmin)
	return args.Error(0)
}

func TestNewMeasurementHandler(t *testing.T) {
	mockService := new(MockMeasurementService)
	measurementHandler := handler.NewMeasurementHandler(mockService)
	assert.NotNil(t, measurementHandler)
}

func TestMeasurementHandler_CreateMeasurement_Success(t *testing.T) {
	mockService := new(MockMeasurementService)
	measurementHandler := handler.NewMeasurementHandler(mockService)

	userID := uuid.New()
	babyID := uuid.New()
	measurementID := uuid.New()

	reqBody := handler.CreateMeasurementRequest{
		Type:  "temperature",
		Value: 37.0,
		Note:  "Normal temperature",
	}

	expectedMeasurement := &domain.Measurement{
		ID:           measurementID,
		ParentID:     userID,
		BabyID:       babyID,
		Type:         "temperature",
		Value:        37.0,
		SafetyStatus: domain.SafetyStatusGreen,
		Note:         "Normal temperature",
		Timestamp:    time.Now(),
		CreatedAt:    time.Now(),
	}

	mockService.On("CreateMeasurementWithDetails", mock.Anything, babyID, mock.Anything, userID, false).
		Return(expectedMeasurement, nil)

	// Use a router to properly set path values
	mux := http.NewServeMux()
	mux.HandleFunc("POST /babies/{baby_id}/measurements", measurementHandler.CreateMeasurement)
	
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/babies/"+babyID.String()+"/measurements", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "PARENT")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	mockService.AssertExpectations(t)
}

func TestMeasurementHandler_CreateMeasurement_Forbidden(t *testing.T) {
	mockService := new(MockMeasurementService)
	measurementHandler := handler.NewMeasurementHandler(mockService)

	userID := uuid.New()
	babyID := uuid.New()

	reqBody := handler.CreateMeasurementRequest{
		Type:  "temperature",
		Value: 37.0,
	}

	mockService.On("CreateMeasurementWithDetails", mock.Anything, babyID, mock.Anything, userID, true).
		Return(nil, assert.AnError)

	// Use a router to properly set path values
	mux := http.NewServeMux()
	mux.HandleFunc("POST /babies/{baby_id}/measurements", measurementHandler.CreateMeasurement)
	
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/babies/"+babyID.String()+"/measurements", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "ADMIN")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	mockService.AssertExpectations(t)
}

func TestMeasurementHandler_GetMeasurements_Success(t *testing.T) {
	mockService := new(MockMeasurementService)
	measurementHandler := handler.NewMeasurementHandler(mockService)

	userID := uuid.New()
	babyID := uuid.New()

	expectedMeasurements := []*domain.Measurement{
		{
			ID:           uuid.New(),
			ParentID:     userID,
			BabyID:       babyID,
			Type:         "temperature",
			Value:        37.0,
			SafetyStatus: domain.SafetyStatusGreen,
			Timestamp:    time.Now(),
			CreatedAt:    time.Now(),
		},
	}

	mockService.On("GetMeasurements", mock.Anything, babyID, userID, true, (*string)(nil), (*int)(nil)).
		Return(expectedMeasurements, nil)

	// Use a router to properly set path values
	mux := http.NewServeMux()
	mux.HandleFunc("GET /babies/{baby_id}/measurements", measurementHandler.GetMeasurements)
	
	req := httptest.NewRequest("GET", "/babies/"+babyID.String()+"/measurements", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "ADMIN")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var measurements []*domain.Measurement
	err := json.NewDecoder(w.Body).Decode(&measurements)
	require.NoError(t, err)
	assert.Len(t, measurements, 1)
	mockService.AssertExpectations(t)
}

func TestMeasurementHandler_GetMeasurementByID_Success(t *testing.T) {
	mockService := new(MockMeasurementService)
	measurementHandler := handler.NewMeasurementHandler(mockService)

	userID := uuid.New()
	measurementID := uuid.New()

	expectedMeasurement := &domain.Measurement{
		ID:           measurementID,
		ParentID:     userID,
		BabyID:       uuid.New(),
		Type:         "temperature",
		Value:        37.0,
		SafetyStatus: domain.SafetyStatusGreen,
		Timestamp:    time.Now(),
		CreatedAt:    time.Now(),
	}

	mockService.On("GetMeasurementByID", mock.Anything, measurementID, userID, true).
		Return(expectedMeasurement, nil)

	// Use a router to properly set path values
	mux := http.NewServeMux()
	mux.HandleFunc("GET /measurements/{measurement_id}", measurementHandler.GetMeasurementByID)
	
	req := httptest.NewRequest("GET", "/measurements/"+measurementID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "ADMIN")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var measurement domain.Measurement
	err := json.NewDecoder(w.Body).Decode(&measurement)
	require.NoError(t, err)
	assert.Equal(t, measurementID, measurement.ID)
	mockService.AssertExpectations(t)
}

func TestMeasurementHandler_DeleteMeasurement_Success(t *testing.T) {
	mockService := new(MockMeasurementService)
	measurementHandler := handler.NewMeasurementHandler(mockService)

	userID := uuid.New()
	measurementID := uuid.New()

	mockService.On("DeleteMeasurement", mock.Anything, measurementID, userID, false).
		Return(nil)

	// Use a router to properly set path values
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /measurements/{measurement_id}", measurementHandler.DeleteMeasurement)
	
	req := httptest.NewRequest("DELETE", "/measurements/"+measurementID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "PARENT")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	mockService.AssertExpectations(t)
}
