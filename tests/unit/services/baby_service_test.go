package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/IANDYI/care-service/internal/core/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockBabyRepository is a mock implementation of BabyRepository
type MockBabyRepository struct {
	mock.Mock
}

func (m *MockBabyRepository) CreateBaby(ctx context.Context, baby *domain.Baby) error {
	args := m.Called(ctx, baby)
	return args.Error(0)
}

func (m *MockBabyRepository) GetBabyByID(ctx context.Context, babyID uuid.UUID) (*domain.Baby, error) {
	args := m.Called(ctx, babyID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Baby), args.Error(1)
}

func (m *MockBabyRepository) ListBabies(ctx context.Context, parentUserID uuid.UUID, isAdmin bool) ([]*domain.Baby, error) {
	args := m.Called(ctx, parentUserID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Baby), args.Error(1)
}

func (m *MockBabyRepository) BabyExists(ctx context.Context, babyID uuid.UUID) (bool, error) {
	args := m.Called(ctx, babyID)
	return args.Bool(0), args.Error(1)
}

func (m *MockBabyRepository) CheckBabyOwnership(ctx context.Context, babyID uuid.UUID, parentUserID uuid.UUID) (bool, error) {
	args := m.Called(ctx, babyID, parentUserID)
	return args.Bool(0), args.Error(1)
}

func TestNewBabyService(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)
	assert.NotNil(t, babyService)
}

func TestBabyService_CreateBaby_Success(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	parentUserID := uuid.New()
	createdByUserID := uuid.New()

	mockRepo.On("CreateBaby", mock.Anything, mock.MatchedBy(func(b *domain.Baby) bool {
		return b.LastName == "Doe" && b.RoomNumber == "101" && b.ParentUserID == parentUserID
	})).Return(nil)

	result, err := babyService.CreateBaby(context.Background(), "Doe", "101", parentUserID, createdByUserID, true)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Doe", result.LastName)
	assert.Equal(t, "101", result.RoomNumber)
	assert.Equal(t, parentUserID, result.ParentUserID)
	mockRepo.AssertExpectations(t)
}

func TestBabyService_CreateBaby_Forbidden(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	parentUserID := uuid.New()
	createdByUserID := uuid.New()

	result, err := babyService.CreateBaby(context.Background(), "Doe", "101", parentUserID, createdByUserID, false)
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "forbidden")
	mockRepo.AssertNotCalled(t, "CreateBaby")
}

func TestBabyService_CreateBaby_EmptyLastName(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	parentUserID := uuid.New()
	createdByUserID := uuid.New()

	result, err := babyService.CreateBaby(context.Background(), "", "101", parentUserID, createdByUserID, true)
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "last_name cannot be empty")
	mockRepo.AssertNotCalled(t, "CreateBaby")
}

func TestBabyService_CreateBaby_EmptyRoomNumber(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	parentUserID := uuid.New()
	createdByUserID := uuid.New()

	result, err := babyService.CreateBaby(context.Background(), "Doe", "", parentUserID, createdByUserID, true)
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "room_number cannot be empty")
	mockRepo.AssertNotCalled(t, "CreateBaby")
}

func TestBabyService_GetBaby_Success_Admin(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	userID := uuid.New()
	babyID := uuid.New()
	parentUserID := uuid.New()

	expectedBaby := &domain.Baby{
		ID:           babyID,
		LastName:     "Doe",
		RoomNumber:   "101",
		ParentUserID: parentUserID,
		CreatedAt:    time.Now(),
	}

	mockRepo.On("BabyExists", mock.Anything, babyID).Return(true, nil)
	mockRepo.On("GetBabyByID", mock.Anything, babyID).Return(expectedBaby, nil)

	result, err := babyService.GetBaby(context.Background(), babyID, userID, true)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, babyID, result.ID)
	mockRepo.AssertExpectations(t)
}

func TestBabyService_GetBaby_Success_Parent(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	userID := uuid.New()
	babyID := uuid.New()

	expectedBaby := &domain.Baby{
		ID:           babyID,
		LastName:     "Doe",
		RoomNumber:   "101",
		ParentUserID: userID,
		CreatedAt:    time.Now(),
	}

	mockRepo.On("BabyExists", mock.Anything, babyID).Return(true, nil)
	mockRepo.On("CheckBabyOwnership", mock.Anything, babyID, userID).Return(true, nil)
	mockRepo.On("GetBabyByID", mock.Anything, babyID).Return(expectedBaby, nil)

	result, err := babyService.GetBaby(context.Background(), babyID, userID, false)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, babyID, result.ID)
	mockRepo.AssertExpectations(t)
}

func TestBabyService_GetBaby_NotFound(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	userID := uuid.New()
	babyID := uuid.New()

	mockRepo.On("BabyExists", mock.Anything, babyID).Return(false, nil)

	result, err := babyService.GetBaby(context.Background(), babyID, userID, true)
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "baby not found")
	mockRepo.AssertNotCalled(t, "GetBabyByID")
}

func TestBabyService_GetBaby_NotOwned(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	userID := uuid.New()
	babyID := uuid.New()

	mockRepo.On("BabyExists", mock.Anything, babyID).Return(true, nil)
	mockRepo.On("CheckBabyOwnership", mock.Anything, babyID, userID).Return(false, nil)

	result, err := babyService.GetBaby(context.Background(), babyID, userID, false)
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "baby not found")
	mockRepo.AssertNotCalled(t, "GetBabyByID")
}

func TestBabyService_ListBabies_Success_Admin(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	userID := uuid.New()

	expectedBabies := []*domain.Baby{
		{
			ID:           uuid.New(),
			LastName:     "Doe",
			RoomNumber:   "101",
			ParentUserID: uuid.New(),
			CreatedAt:    time.Now(),
		},
		{
			ID:           uuid.New(),
			LastName:     "Smith",
			RoomNumber:   "102",
			ParentUserID: uuid.New(),
			CreatedAt:    time.Now(),
		},
	}

	mockRepo.On("ListBabies", mock.Anything, uuid.Nil, true).Return(expectedBabies, nil)

	result, err := babyService.ListBabies(context.Background(), userID, true)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2)
	mockRepo.AssertExpectations(t)
}

func TestBabyService_ListBabies_Success_Parent(t *testing.T) {
	mockRepo := new(MockBabyRepository)
	babyService := services.NewBabyService(mockRepo)

	userID := uuid.New()

	expectedBabies := []*domain.Baby{
		{
			ID:           uuid.New(),
			LastName:     "Doe",
			RoomNumber:   "101",
			ParentUserID: userID,
			CreatedAt:    time.Now(),
		},
	}

	mockRepo.On("ListBabies", mock.Anything, userID, false).Return(expectedBabies, nil)

	result, err := babyService.ListBabies(context.Background(), userID, false)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 1)
	mockRepo.AssertExpectations(t)
}
