package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/IANDYI/care-service/internal/core/ports"
	"github.com/IANDYI/care-service/internal/core/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockMeasurementRepository is a mock implementation of ports.MeasurementRepository
type MockMeasurementRepository struct {
	mock.Mock
}

func (m *MockMeasurementRepository) CreateMeasurement(ctx context.Context, measurement *domain.Measurement) error {
	args := m.Called(ctx, measurement)
	return args.Error(0)
}

func (m *MockMeasurementRepository) GetMeasurementsByBabyID(ctx context.Context, babyID uuid.UUID, measurementType *string, limit *int) ([]*domain.Measurement, error) {
	args := m.Called(ctx, babyID, measurementType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Measurement), args.Error(1)
}

func (m *MockMeasurementRepository) GetMeasurementByID(ctx context.Context, measurementID uuid.UUID) (*domain.Measurement, error) {
	args := m.Called(ctx, measurementID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Measurement), args.Error(1)
}

func (m *MockMeasurementRepository) DeleteMeasurement(ctx context.Context, measurementID uuid.UUID, parentID uuid.UUID) error {
	args := m.Called(ctx, measurementID, parentID)
	return args.Error(0)
}

// MockBabyRepository for measurement service tests
type MockBabyRepositoryForMeasurement struct {
	mock.Mock
}

func (m *MockBabyRepositoryForMeasurement) CreateBaby(ctx context.Context, baby *domain.Baby) error {
	args := m.Called(ctx, baby)
	return args.Error(0)
}

func (m *MockBabyRepositoryForMeasurement) GetBabyByID(ctx context.Context, babyID uuid.UUID) (*domain.Baby, error) {
	args := m.Called(ctx, babyID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Baby), args.Error(1)
}

func (m *MockBabyRepositoryForMeasurement) ListBabies(ctx context.Context, parentUserID uuid.UUID, isAdmin bool) ([]*domain.Baby, error) {
	args := m.Called(ctx, parentUserID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Baby), args.Error(1)
}

func (m *MockBabyRepositoryForMeasurement) BabyExists(ctx context.Context, babyID uuid.UUID) (bool, error) {
	args := m.Called(ctx, babyID)
	return args.Bool(0), args.Error(1)
}

func (m *MockBabyRepositoryForMeasurement) CheckBabyOwnership(ctx context.Context, babyID uuid.UUID, parentUserID uuid.UUID) (bool, error) {
	args := m.Called(ctx, babyID, parentUserID)
	return args.Bool(0), args.Error(1)
}

// MockAlertPublisher is a mock implementation of ports.AlertPublisher
type MockAlertPublisher struct {
	mock.Mock
}

func (m *MockAlertPublisher) PublishAlert(ctx context.Context, babyID uuid.UUID, measurement *domain.Measurement) error {
	args := m.Called(ctx, babyID, measurement)
	return args.Error(0)
}

func TestNewMeasurementService(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)
	assert.NotNil(t, measurementService)
}

func TestMeasurementService_CreateMeasurement_Success(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)

	userID := uuid.New()
	babyID := uuid.New()

	mockBabyRepo.On("BabyExists", mock.Anything, babyID).Return(true, nil)
	mockBabyRepo.On("CheckBabyOwnership", mock.Anything, babyID, userID).Return(true, nil)
	mockMeasurementRepo.On("CreateMeasurement", mock.Anything, mock.AnythingOfType("*domain.Measurement")).Return(nil)

	req := ports.CreateMeasurementRequest{
		Type:  "temperature",
		Value: 37.0,
		Note:  "Normal temperature",
	}

	result, err := measurementService.CreateMeasurementWithDetails(context.Background(), babyID, req, userID, false)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "temperature", result.Type)
	assert.Equal(t, 37.0, result.Value)
	assert.Equal(t, domain.SafetyStatusGreen, result.SafetyStatus)
	mockBabyRepo.AssertExpectations(t)
	mockMeasurementRepo.AssertExpectations(t)
}

func TestMeasurementService_CreateMeasurement_Forbidden_Admin(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)

	userID := uuid.New()
	babyID := uuid.New()

	mockBabyRepo.On("BabyExists", mock.Anything, babyID).Return(true, nil)

	req := ports.CreateMeasurementRequest{
		Type:  "temperature",
		Value: 37.0,
	}

	result, err := measurementService.CreateMeasurementWithDetails(context.Background(), babyID, req, userID, true)
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "forbidden")
	mockBabyRepo.AssertExpectations(t)
	mockBabyRepo.AssertNotCalled(t, "CheckBabyOwnership")
	mockMeasurementRepo.AssertNotCalled(t, "CreateMeasurement")
}

func TestMeasurementService_CreateMeasurement_InvalidType(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)

	userID := uuid.New()
	babyID := uuid.New()

	req := ports.CreateMeasurementRequest{
		Type:  "invalid_type",
		Value: 37.0,
	}

	result, err := measurementService.CreateMeasurementWithDetails(context.Background(), babyID, req, userID, false)
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid measurement type")
	mockBabyRepo.AssertNotCalled(t, "BabyExists")
	mockMeasurementRepo.AssertNotCalled(t, "CreateMeasurement")
}

func TestMeasurementService_CreateMeasurement_BabyNotFound(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)

	userID := uuid.New()
	babyID := uuid.New()

	mockBabyRepo.On("BabyExists", mock.Anything, babyID).Return(false, nil)

	req := ports.CreateMeasurementRequest{
		Type:  "temperature",
		Value: 37.0,
	}

	result, err := measurementService.CreateMeasurementWithDetails(context.Background(), babyID, req, userID, false)
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "baby not found")
	mockBabyRepo.AssertExpectations(t)
	mockMeasurementRepo.AssertNotCalled(t, "CreateMeasurement")
}

func TestMeasurementService_CreateMeasurement_RedStatus(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)

	userID := uuid.New()
	babyID := uuid.New()

	mockBabyRepo.On("BabyExists", mock.Anything, babyID).Return(true, nil)
	mockBabyRepo.On("CheckBabyOwnership", mock.Anything, babyID, userID).Return(true, nil)
	mockMeasurementRepo.On("CreateMeasurement", mock.Anything, mock.MatchedBy(func(m *domain.Measurement) bool {
		return m.SafetyStatus == domain.SafetyStatusRed
	})).Return(nil)
	// Alert publisher might be called asynchronously, so we don't assert it here

	req := ports.CreateMeasurementRequest{
		Type:  "temperature",
		Value: 39.0, // Red status (>38.0)
		Note:  "High temperature",
	}

	result, err := measurementService.CreateMeasurementWithDetails(context.Background(), babyID, req, userID, false)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.SafetyStatusRed, result.SafetyStatus)
	mockBabyRepo.AssertExpectations(t)
	mockMeasurementRepo.AssertExpectations(t)
}

func TestMeasurementService_GetMeasurements_Success(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)

	userID := uuid.New()
	babyID := uuid.New()

	mockBabyRepo.On("BabyExists", mock.Anything, babyID).Return(true, nil)
	mockBabyRepo.On("CheckBabyOwnership", mock.Anything, babyID, userID).Return(true, nil)

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

	mockMeasurementRepo.On("GetMeasurementsByBabyID", mock.Anything, babyID, (*string)(nil), (*int)(nil)).
		Return(expectedMeasurements, nil)

	result, err := measurementService.GetMeasurements(context.Background(), babyID, userID, false, nil, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 1)
	mockBabyRepo.AssertExpectations(t)
	mockMeasurementRepo.AssertExpectations(t)
}

func TestMeasurementService_GetMeasurementByID_Success(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)

	userID := uuid.New()
	measurementID := uuid.New()
	babyID := uuid.New()

	expectedMeasurement := &domain.Measurement{
		ID:           measurementID,
		ParentID:     userID,
		BabyID:       babyID,
		Type:         "temperature",
		Value:        37.0,
		SafetyStatus: domain.SafetyStatusGreen,
		Timestamp:    time.Now(),
		CreatedAt:    time.Now(),
	}

	mockMeasurementRepo.On("GetMeasurementByID", mock.Anything, measurementID).Return(expectedMeasurement, nil)
	mockBabyRepo.On("BabyExists", mock.Anything, babyID).Return(true, nil)
	mockBabyRepo.On("CheckBabyOwnership", mock.Anything, babyID, userID).Return(true, nil)

	result, err := measurementService.GetMeasurementByID(context.Background(), measurementID, userID, false)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, measurementID, result.ID)
	mockBabyRepo.AssertExpectations(t)
	mockMeasurementRepo.AssertExpectations(t)
}

func TestMeasurementService_DeleteMeasurement_Success(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)

	userID := uuid.New()
	measurementID := uuid.New()
	babyID := uuid.New()

	expectedMeasurement := &domain.Measurement{
		ID:           measurementID,
		ParentID:     userID,
		BabyID:       babyID,
		Type:         "temperature",
		Value:        37.0,
		SafetyStatus: domain.SafetyStatusGreen,
		Timestamp:    time.Now(),
		CreatedAt:    time.Now(),
	}

	mockMeasurementRepo.On("GetMeasurementByID", mock.Anything, measurementID).Return(expectedMeasurement, nil)
	mockMeasurementRepo.On("DeleteMeasurement", mock.Anything, measurementID, userID).Return(nil)

	err := measurementService.DeleteMeasurement(context.Background(), measurementID, userID, false)
	
	require.NoError(t, err)
	mockMeasurementRepo.AssertExpectations(t)
}

func TestMeasurementService_DeleteMeasurement_Forbidden_Admin(t *testing.T) {
	mockMeasurementRepo := new(MockMeasurementRepository)
	mockBabyRepo := new(MockBabyRepositoryForMeasurement)
	mockAlertPublisher := new(MockAlertPublisher)
	
	measurementService := services.NewMeasurementService(mockMeasurementRepo, mockBabyRepo, mockAlertPublisher)

	userID := uuid.New()
	measurementID := uuid.New()

	err := measurementService.DeleteMeasurement(context.Background(), measurementID, userID, true)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")
	mockMeasurementRepo.AssertNotCalled(t, "GetMeasurementByID")
	mockMeasurementRepo.AssertNotCalled(t, "DeleteMeasurement")
}
