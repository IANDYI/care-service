package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/IANDYI/care-service/internal/adapters/handler"
	"github.com/IANDYI/care-service/internal/adapters/middleware"
	"github.com/IANDYI/care-service/internal/adapters/repository"
	"github.com/IANDYI/care-service/internal/config"
	"github.com/IANDYI/care-service/internal/core/services"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Connect to database with retry logic
	db, err := config.ConnectDatabase(cfg.DatabaseURL, 5, 2*time.Second)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize RabbitMQ publisher
	rabbitMQPublisher, err := repository.NewRabbitMQPublisher(cfg.RabbitMQURL, cfg.ALERTS_QUEUE_NAME)
	if err != nil {
		log.Fatalf("Failed to initialize RabbitMQ publisher: %v", err)
	}
	defer rabbitMQPublisher.Close()

	// Initialize repositories
	sqlRepo := repository.NewSQLRepository(db)

	// Initialize services
	babyService := services.NewBabyService(sqlRepo)
	measurementService := services.NewMeasurementService(sqlRepo, sqlRepo, rabbitMQPublisher)

	// Initialize RabbitMQ consumer for baby creation
	// This consumer runs in the same pod as the care-service and processes
	// baby creation requests from the identity-service via RabbitMQ
	babyConsumer, err := repository.NewBabyConsumer(cfg.RabbitMQURL, cfg.BABY_QUEUE_NAME, babyService)
	if err != nil {
		log.Fatalf("Failed to initialize RabbitMQ baby consumer: %v", err)
	}
	defer babyConsumer.Close()

	// Start baby consumer in background goroutine (non-blocking)
	// The consumer will process messages asynchronously while the HTTP server runs
	// Note: In multi-replica deployments, each replica will have its own consumer,
	// and RabbitMQ will distribute messages across replicas using round-robin
	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	go func() {
		if err := babyConsumer.StartConsuming(consumerCtx); err != nil {
			log.Printf("Baby consumer error: %v", err)
		}
	}()
	log.Println("Baby consumer started in background, listening for baby creation requests")

	// Initialize handlers
	babyHandler := handler.NewBabyHandler(babyService)
	measurementHandler := handler.NewMeasurementHandler(measurementService)
	healthHandler := handler.NewHealthHandler(db)

	// Initialize JWT middleware
	authMiddleware := middleware.NewAuthMiddleware(cfg.JWTPublicKey)

	// Setup HTTP router
	mux := http.NewServeMux()

	// Health endpoints (OpenShift compatible, no auth required)
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("GET /health/ready", healthHandler.Ready)
	mux.HandleFunc("GET /health/live", healthHandler.Live)

	// API endpoints (require authentication)
	// POST /babies - ADMIN only
	mux.HandleFunc("POST /babies", authMiddleware.RequireRole("ADMIN", babyHandler.CreateBaby))

	// GET /babies - ADMIN: all, PARENT: owned only
	mux.HandleFunc("GET /babies", authMiddleware.RequireAuth(babyHandler.ListBabies))

	// GET /babies/{baby_id} - ADMIN: any, PARENT: owned only
	mux.HandleFunc("GET /babies/{baby_id}", authMiddleware.RequireAuth(babyHandler.GetBaby))

	// POST /babies/{baby_id}/measurements - PARENT: owned only (ADMIN cannot create)
	mux.HandleFunc("POST /babies/{baby_id}/measurements", authMiddleware.RequireAuth(measurementHandler.CreateMeasurement))

	// GET /babies/{baby_id}/measurements - ADMIN: any, PARENT: owned only
	mux.HandleFunc("GET /babies/{baby_id}/measurements", authMiddleware.RequireAuth(measurementHandler.GetMeasurements))

	// GET /measurements/{measurement_id} - ADMIN: any, PARENT: owned only
	mux.HandleFunc("GET /measurements/{measurement_id}", authMiddleware.RequireAuth(measurementHandler.GetMeasurementByID))

	// DELETE /measurements/{measurement_id} - PARENT: only measurements they created (ADMIN cannot delete)
	mux.HandleFunc("DELETE /measurements/{measurement_id}", authMiddleware.RequireAuth(measurementHandler.DeleteMeasurement))

	// Wrap mux with metrics middleware to track all HTTP requests
	loggedRouter := middleware.MetricsMiddleware(mux)

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      loggedRouter,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting Care Service on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Give server time to start and log success
	time.Sleep(500 * time.Millisecond)
	log.Println("Care Service is starting...")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Cancel consumer context first to stop processing new messages
	consumerCancel()
	log.Println("Baby consumer stopped")

	// Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

