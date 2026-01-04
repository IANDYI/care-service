package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/IANDYI/care-service/internal/adapters/middleware"
	"github.com/IANDYI/care-service/internal/core/ports"
	"github.com/google/uuid"
)

// BabyHandler handles HTTP requests for baby operations
type BabyHandler struct {
	babyService ports.BabyService
}

// NewBabyHandler creates a new baby handler
func NewBabyHandler(babyService ports.BabyService) *BabyHandler {
	return &BabyHandler{
		babyService: babyService,
	}
}

// CreateBabyRequest represents the request body for creating a baby
type CreateBabyRequest struct {
	LastName     string    `json:"last_name"`
	RoomNumber   string    `json:"room_number"`
	ParentUserID uuid.UUID `json:"parent_user_id"`
}

// CreateBaby handles POST /babies
// ADMIN only - creates a baby and assigns to parent_user_id
func (h *BabyHandler) CreateBaby(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	requestID := generateRequestID()

	// Extract user info from context
	userIDStr, ok := middleware.GetUserID(r.Context())
	if !ok {
		log.Printf("[%s] Failed to get user ID from context", requestID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("[%s] Invalid user ID: %v", requestID, err)
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	isAdmin := middleware.IsAdmin(r.Context())

	// Parse request body
	var req CreateBabyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[%s] Failed to decode request: %v", requestID, err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Create baby
	baby, err := h.babyService.CreateBaby(r.Context(), req.LastName, req.RoomNumber, req.ParentUserID, userID, isAdmin)
	if err != nil {
		log.Printf("[%s] Failed to create baby: user_id=%s, role=%v, error=%v", requestID, userIDStr, isAdmin, err)
		if err.Error() == "forbidden: only ADMIN can create babies" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Log structured JSON
	logStructured(requestID, userIDStr, isAdmin, "POST", "/babies", http.StatusCreated, time.Since(startTime))

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(baby)
}

// GetBaby handles GET /babies/{baby_id}
// ADMIN: any baby, PARENT: owned only
func (h *BabyHandler) GetBaby(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	requestID := generateRequestID()

	// Extract user info from context
	userIDStr, ok := middleware.GetUserID(r.Context())
	if !ok {
		log.Printf("[%s] Failed to get user ID from context", requestID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("[%s] Invalid user ID: %v", requestID, err)
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	isAdmin := middleware.IsAdmin(r.Context())
	role, _ := middleware.GetRole(r.Context())

	// Extract baby_id from URL path
	babyIDStr := r.PathValue("baby_id")
	log.Printf("[%s] GetBaby - user_id=%s, role=%s, isAdmin=%v, baby_id=%s", requestID, userIDStr, role, isAdmin, babyIDStr)
	babyID, err := uuid.Parse(babyIDStr)
	if err != nil {
		log.Printf("[%s] Invalid baby ID: %v", requestID, err)
		http.Error(w, "invalid baby ID", http.StatusBadRequest)
		return
	}

	// Get baby
	baby, err := h.babyService.GetBaby(r.Context(), babyID, userID, isAdmin)
	if err != nil {
		log.Printf("[%s] Failed to get baby: user_id=%s, role=%s, isAdmin=%v, baby_id=%s, error=%v", requestID, userIDStr, role, isAdmin, babyIDStr, err)
		if err.Error() == "baby not found" {
			http.Error(w, "baby not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log structured JSON
	logStructured(requestID, userIDStr, isAdmin, "GET", "/babies/"+babyIDStr, http.StatusOK, time.Since(startTime))

	// Return response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(baby)
}

// ListBabies handles GET /babies
// ADMIN: all babies, PARENT: owned only
func (h *BabyHandler) ListBabies(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	requestID := generateRequestID()

	// Extract user info from context
	userIDStr, ok := middleware.GetUserID(r.Context())
	if !ok {
		log.Printf("[%s] Failed to get user ID from context", requestID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("[%s] Invalid user ID: %v", requestID, err)
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	isAdmin := middleware.IsAdmin(r.Context())

	// List babies
	babies, err := h.babyService.ListBabies(r.Context(), userID, isAdmin)
	if err != nil {
		log.Printf("[%s] Failed to list babies: user_id=%s, role=%v, error=%v", requestID, userIDStr, isAdmin, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log structured JSON
	logStructured(requestID, userIDStr, isAdmin, "GET", "/babies", http.StatusOK, time.Since(startTime))

	// Return response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(babies)
}

