package config

import (
	"crypto/rsa"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

// Config holds all configuration for the Care Service
type Config struct {
	// JWT configuration - public key from Identity Service
	JWTPublicKey *rsa.PublicKey

	// Database configuration
	DatabaseURL string

	// RabbitMQ configuration
	RabbitMQURL string

	// Baby queue name
	BABY_QUEUE_NAME string

	// Server configuration
	Port string

	// Circuit breaker configuration
	CircuitBreakerMaxRequests uint32
	CircuitBreakerInterval    string
	CircuitBreakerTimeout     string
}

// Load reads configuration from environment variables
// Public key is loaded from /etc/identity/public.pem (mounted via ConfigMap)
func Load() *Config {
	// Load JWT public key from mounted ConfigMap
	publicKeyPath := os.Getenv("PUBLIC_KEY_PATH")
	if publicKeyPath == "" {
		publicKeyPath = "/etc/identity/public.pem"
	}
	publicKey, err := loadPublicKey(publicKeyPath)
	if err != nil {
		panic("Failed to load public key: " + err.Error())
	}

	// Database connection string
	dbURL := os.Getenv("DB_CONNECTION_STRING")
	if dbURL == "" {
		panic("DB_CONNECTION_STRING environment variable is required")
	}

	// RabbitMQ connection string
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}

	babyQueueName := os.Getenv("BABY_QUEUE_NAME")
	if babyQueueName == "" {
		babyQueueName = "babies"
	}

	// Server port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Circuit breaker settings (optional, with defaults)
	cbMaxRequests := uint32(5)
	if val := os.Getenv("CIRCUIT_BREAKER_MAX_REQUESTS"); val != "" {
		// Parse if needed, for now use default
	}
	cbInterval := os.Getenv("CIRCUIT_BREAKER_INTERVAL")
	if cbInterval == "" {
		cbInterval = "60s"
	}
	cbTimeout := os.Getenv("CIRCUIT_BREAKER_TIMEOUT")
	if cbTimeout == "" {
		cbTimeout = "30s"
	}

	return &Config{
		JWTPublicKey:              publicKey,
		DatabaseURL:               dbURL,
		RabbitMQURL:               rabbitMQURL,
		BABY_QUEUE_NAME:           babyQueueName,
		Port:                      port,
		CircuitBreakerMaxRequests: cbMaxRequests,
		CircuitBreakerInterval:    cbInterval,
		CircuitBreakerTimeout:     cbTimeout,
	}
}

// loadPublicKey loads an RSA public key from a PEM file
func loadPublicKey(path string) (*rsa.PublicKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	publicKey, err := jwt.ParseRSAPublicKeyFromPEM(keyData)
	if err != nil {
		return nil, err
	}
	return publicKey, nil
}

