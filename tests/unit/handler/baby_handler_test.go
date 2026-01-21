package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/IANDYI/care-service/internal/adapters/handler"
	"github.com/IANDYI/care-service/internal/adapters/middleware"
	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockBabyService is a mock implementation of BabyService
type MockBabyService struct {
	mock.Mock
}

func (m *MockBabyService) CreateBaby(ctx context.Context, lastName string, roomNumber string, parentUserID uuid.UUID, createdByUserID uuid.UUID, isAdmin bool) (*domain.Baby, error) {
	args := m.Called(ctx, lastName, roomNumber, parentUserID, createdByUserID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Baby), args.Error(1)
}

func (m *MockBabyService) GetBaby(ctx context.Context, babyID uuid.UUID, userID uuid.UUID, isAdmin bool) (*domain.Baby, error) {
	args := m.Called(ctx, babyID, userID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Baby), args.Error(1)
}

func (m *MockBabyService) ListBabies(ctx context.Context, userID uuid.UUID, isAdmin bool) ([]*domain.Baby, error) {
	args := m.Called(ctx, userID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Baby), args.Error(1)
}

func TestNewBabyHandler(t *testing.T) {
	mockService := new(MockBabyService)
	babyHandler := handler.NewBabyHandler(mockService)
	assert.NotNil(t, babyHandler)
}

func TestBabyHandler_CreateBaby_Success(t *testing.T) {
	mockService := new(MockBabyService)
	babyHandler := handler.NewBabyHandler(mockService)

	userID := uuid.New()
	parentUserID := uuid.New()
	babyID := uuid.New()

	expectedBaby := &domain.Baby{
		ID:           babyID,
		LastName:     "Doe",
		RoomNumber:   "101",
		ParentUserID: parentUserID,
		CreatedAt:    time.Now(),
	}

	mockService.On("CreateBaby", mock.Anything, "Doe", "101", parentUserID, userID, true).Return(expectedBaby, nil)

	reqBody := handler.CreateBabyRequest{
		LastName:     "Doe",
		RoomNumber:   "101",
		ParentUserID: parentUserID,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/babies", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "ADMIN")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	babyHandler.CreateBaby(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	mockService.AssertExpectations(t)
}

func TestBabyHandler_CreateBaby_Unauthorized(t *testing.T) {
	mockService := new(MockBabyService)
	babyHandler := handler.NewBabyHandler(mockService)

	reqBody := handler.CreateBabyRequest{
		LastName:     "Doe",
		RoomNumber:   "101",
		ParentUserID: uuid.New(),
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/babies", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	
	// No user ID in context
	req = req.WithContext(context.Background())

	w := httptest.NewRecorder()
	babyHandler.CreateBaby(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockService.AssertNotCalled(t, "CreateBaby")
}

func TestBabyHandler_CreateBaby_Forbidden(t *testing.T) {
	mockService := new(MockBabyService)
	babyHandler := handler.NewBabyHandler(mockService)

	userID := uuid.New()
	parentUserID := uuid.New()

	mockService.On("CreateBaby", mock.Anything, "Doe", "101", parentUserID, userID, false).
		Return(nil, assert.AnError)

	reqBody := handler.CreateBabyRequest{
		LastName:     "Doe",
		RoomNumber:   "101",
		ParentUserID: parentUserID,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/babies", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "PARENT")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	babyHandler.CreateBaby(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	mockService.AssertExpectations(t)
}

func TestBabyHandler_GetBaby_Success(t *testing.T) {
	mockService := new(MockBabyService)
	babyHandler := handler.NewBabyHandler(mockService)

	userID := uuid.New()
	babyID := uuid.New()

	expectedBaby := &domain.Baby{
		ID:           babyID,
		LastName:     "Doe",
		RoomNumber:   "101",
		ParentUserID: uuid.New(),
		CreatedAt:    time.Now(),
	}

	mockService.On("GetBaby", mock.Anything, babyID, userID, true).Return(expectedBaby, nil)

	// Use a router to properly set path values
	mux := http.NewServeMux()
	mux.HandleFunc("GET /babies/{baby_id}", babyHandler.GetBaby)
	
	req := httptest.NewRequest("GET", "/babies/"+babyID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "ADMIN")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var baby domain.Baby
	err := json.NewDecoder(w.Body).Decode(&baby)
	require.NoError(t, err)
	assert.Equal(t, babyID, baby.ID)
	mockService.AssertExpectations(t)
}

func TestBabyHandler_GetBaby_NotFound(t *testing.T) {
	mockService := new(MockBabyService)
	babyHandler := handler.NewBabyHandler(mockService)

	userID := uuid.New()
	babyID := uuid.New()

	mockService.On("GetBaby", mock.Anything, babyID, userID, true).
		Return(nil, assert.AnError)

	// Use a router to properly set path values
	mux := http.NewServeMux()
	mux.HandleFunc("GET /babies/{baby_id}", babyHandler.GetBaby)
	
	req := httptest.NewRequest("GET", "/babies/"+babyID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "ADMIN")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}

func TestBabyHandler_ListBabies_Success(t *testing.T) {
	mockService := new(MockBabyService)
	babyHandler := handler.NewBabyHandler(mockService)

	userID := uuid.New()
	parentUserID := uuid.New()

	expectedBabies := []*domain.Baby{
		{
			ID:           uuid.New(),
			LastName:     "Doe",
			RoomNumber:   "101",
			ParentUserID: parentUserID,
			CreatedAt:    time.Now(),
		},
	}

	mockService.On("ListBabies", mock.Anything, userID, true).Return(expectedBabies, nil)

	req := httptest.NewRequest("GET", "/babies", nil)
	
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.RoleKey, "ADMIN")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	babyHandler.ListBabies(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var babies []*domain.Baby
	err := json.NewDecoder(w.Body).Decode(&babies)
	require.NoError(t, err)
	assert.Len(t, babies, 1)
	mockService.AssertExpectations(t)
}
