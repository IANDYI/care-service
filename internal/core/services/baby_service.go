package services

import (
	"context"
	"fmt"
	"time"

	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/IANDYI/care-service/internal/core/ports"
	"github.com/google/uuid"
)

// BabyService implements business logic for baby operations
// Enforces RBAC and ownership rules
type BabyService struct {
	babyRepo ports.BabyRepository
}

// NewBabyService creates a new baby service
func NewBabyService(babyRepo ports.BabyRepository) *BabyService {
	return &BabyService{
		babyRepo: babyRepo,
	}
}

// CreateBaby creates a new baby (ADMIN only)
// Validates input and enforces RBAC
func (s *BabyService) CreateBaby(ctx context.Context, lastName string, roomNumber string, parentUserID uuid.UUID, createdByUserID uuid.UUID, isAdmin bool) (*domain.Baby, error) {
	// RBAC enforcement: Only ADMIN can create babies
	if !isAdmin {
		return nil, fmt.Errorf("forbidden: only ADMIN can create babies")
	}

	// Input validation
	if lastName == "" {
		return nil, fmt.Errorf("baby last_name cannot be empty")
	}
	if roomNumber == "" {
		return nil, fmt.Errorf("baby room_number cannot be empty")
	}

	// Create baby
	baby := &domain.Baby{
		ID:           uuid.New(),
		LastName:     lastName,
		RoomNumber:   roomNumber,
		ParentUserID: parentUserID,
		CreatedAt:    time.Now(),
	}

	if err := s.babyRepo.CreateBaby(ctx, baby); err != nil {
		return nil, fmt.Errorf("failed to create baby: %w", err)
	}

	return baby, nil
}

// GetBaby retrieves a baby by ID
// Enforces ownership: ADMIN can access any, PARENT only their own
func (s *BabyService) GetBaby(ctx context.Context, babyID uuid.UUID, userID uuid.UUID, isAdmin bool) (*domain.Baby, error) {
	// Check if baby exists
	exists, err := s.babyRepo.BabyExists(ctx, babyID)
	if err != nil {
		return nil, fmt.Errorf("failed to check baby existence: %w", err)
	}
	if !exists {
		// Don't leak ownership info - return generic not found
		return nil, fmt.Errorf("baby not found")
	}

	// ADMIN can access any baby
	if isAdmin {
		baby, err := s.babyRepo.GetBabyByID(ctx, babyID)
		if err != nil {
			return nil, fmt.Errorf("failed to get baby: %w", err)
		}
		return baby, nil
	}

	// PARENT can only access their own babies
	owned, err := s.babyRepo.CheckBabyOwnership(ctx, babyID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check ownership: %w", err)
	}
	if !owned {
		// Don't leak ownership info - return generic not found
		return nil, fmt.Errorf("baby not found")
	}

	baby, err := s.babyRepo.GetBabyByID(ctx, babyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get baby: %w", err)
	}

	return baby, nil
}

// ListBabies retrieves babies based on role
// ADMIN: all babies, PARENT: only owned babies
func (s *BabyService) ListBabies(ctx context.Context, userID uuid.UUID, isAdmin bool) ([]*domain.Baby, error) {
	parentUserID := userID
	if isAdmin {
		// ADMIN can see all babies, parentUserID is ignored
		parentUserID = uuid.Nil
	}

	babies, err := s.babyRepo.ListBabies(ctx, parentUserID, isAdmin)
	if err != nil {
		return nil, fmt.Errorf("failed to list babies: %w", err)
	}

	return babies, nil
}

