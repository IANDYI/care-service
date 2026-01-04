package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/IANDYI/care-service/internal/core/domain"
	"github.com/IANDYI/care-service/internal/core/ports"
	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	"github.com/sony/gobreaker"
)

// RabbitMQPublisher implements AlertPublisher for publishing alerts to RabbitMQ
// Includes retry logic and circuit breaker for resilience
// Target pipeline latency < 15s
type RabbitMQPublisher struct {
	conn         *amqp091.Connection
	channel      *amqp091.Channel
	queueName    string
	cb           *gobreaker.CircuitBreaker
	maxRetries   int
	retryDelay   time.Duration
	connMutex    sync.RWMutex
	reconnectCh  chan bool
	stopReconnect chan bool
}

// AlertEvent represents an alert event published to RabbitMQ
// Published only for Red status measurements (critical alerts)
type AlertEvent struct {
	BabyID       uuid.UUID            `json:"baby_id"`
	Measurement  *domain.Measurement  `json:"measurement"`
	Timestamp    time.Time            `json:"timestamp"`
	AlertType    string               `json:"alert_type"`
	SafetyStatus string               `json:"safety_status"`
	Severity     string               `json:"severity"` // "critical" for Red status
}

// NewRabbitMQPublisher creates a new RabbitMQ publisher with circuit breaker
func NewRabbitMQPublisher(rabbitMQURL string, queueName string) (*RabbitMQPublisher, error) {
	if queueName == "" {
		queueName = "baby_alerts"
	}

	publisher := &RabbitMQPublisher{
		queueName:     queueName,
		maxRetries:    3,
		retryDelay:    1 * time.Second,
		reconnectCh:   make(chan bool, 1),
		stopReconnect: make(chan bool),
	}

	// Circuit breaker settings
	settings := gobreaker.Settings{
		Name:        "rabbitmq",
		MaxRequests: 5,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 5
		},
	}
	publisher.cb = gobreaker.NewCircuitBreaker(settings)

	// Connect to RabbitMQ
	if err := publisher.connect(rabbitMQURL); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Start reconnection handler
	go publisher.handleReconnection(rabbitMQURL)

	return publisher, nil
}

// connect establishes connection to RabbitMQ
func (p *RabbitMQPublisher) connect(rabbitMQURL string) error {
	var err error
	for i := 0; i < p.maxRetries; i++ {
		p.conn, err = amqp091.Dial(rabbitMQURL)
		if err == nil {
			break
		}
		log.Printf("Failed to connect to RabbitMQ (attempt %d/%d): %v", i+1, p.maxRetries, err)
		if i < p.maxRetries-1 {
			time.Sleep(p.retryDelay)
		}
	}

	if err != nil {
		return err
	}

	p.channel, err = p.conn.Channel()
	if err != nil {
		p.conn.Close()
		return err
	}

	// Declare queue (idempotent)
	_, err = p.channel.QueueDeclare(
		p.queueName, // name
		true,        // durable
		false,       // delete when unused
		false,       // exclusive
		false,       // no-wait
		nil,         // arguments
	)

	if err != nil {
		p.channel.Close()
		p.conn.Close()
		return err
	}

	log.Println("Connected to RabbitMQ successfully")
	return nil
}

// handleReconnection handles automatic reconnection to RabbitMQ
func (p *RabbitMQPublisher) handleReconnection(rabbitMQURL string) {
	for {
		select {
		case <-p.reconnectCh:
			log.Println("Attempting to reconnect to RabbitMQ...")
			p.connMutex.Lock()
			if p.channel != nil {
				p.channel.Close()
			}
			if p.conn != nil {
				p.conn.Close()
			}
			p.connMutex.Unlock()

			if err := p.connect(rabbitMQURL); err != nil {
				log.Printf("Reconnection failed: %v", err)
			}
		case <-p.stopReconnect:
			return
		}
	}
}

// PublishAlert publishes an alert event to RabbitMQ
// Implements AlertPublisher interface
func (p *RabbitMQPublisher) PublishAlert(ctx context.Context, babyID uuid.UUID, measurement *domain.Measurement) error {
	_, err := p.cb.Execute(func() (interface{}, error) {
		return nil, p.publishWithRetry(ctx, babyID, measurement)
	})
	return err
}

// publishWithRetry publishes with retry logic
func (p *RabbitMQPublisher) publishWithRetry(ctx context.Context, babyID uuid.UUID, measurement *domain.Measurement) error {
	startTime := time.Now()

	// Determine alert type based on measurement type and safety status
	alertType := "critical_measurement"
	if measurement.Type == domain.MeasurementTypeTemperature {
		if measurement.Value > domain.TemperatureYellowMax {
			alertType = "high_temperature_critical"
		} else if measurement.Value < domain.TemperatureYellowMin {
			alertType = "low_temperature_critical"
		}
	} else if measurement.Type == domain.MeasurementTypeWeight {
		alertType = "invalid_weight"
	}

	event := AlertEvent{
		BabyID:       babyID,
		Measurement:  measurement,
		Timestamp:    time.Now(),
		AlertType:    alertType,
		SafetyStatus: string(measurement.SafetyStatus),
		Severity:     "critical", // Red status alerts are always critical
	}

	// Log structured JSON for alert publishing
	logEntry := map[string]interface{}{
		"event":         "alert_publish_attempt",
		"baby_id":       babyID.String(),
		"measurement_id": measurement.ID.String(),
		"alert_type":    alertType,
		"safety_status":  string(measurement.SafetyStatus),
		"timestamp":      time.Now().Format(time.RFC3339),
	}
	jsonBytes, _ := json.Marshal(logEntry)
	log.Printf("%s", string(jsonBytes))

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal alert event: %w", err)
	}

	var lastErr error
	for i := 0; i < p.maxRetries; i++ {
		p.connMutex.RLock()
		ch := p.channel
		conn := p.conn
		p.connMutex.RUnlock()

		if ch == nil || conn == nil || conn.IsClosed() {
			// Trigger reconnection
			select {
			case p.reconnectCh <- true:
			default:
			}
			time.Sleep(p.retryDelay)
			continue
		}

		err = ch.PublishWithContext(
			ctx,
			"",           // exchange
			p.queueName,  // routing key
			false,        // mandatory
			false,        // immediate
			amqp091.Publishing{
				ContentType:  "application/json",
				Body:         body,
				DeliveryMode: amqp091.Persistent, // Make message persistent
				Timestamp:    time.Now(),
			},
		)

		if err == nil {
			latency := time.Since(startTime)
			if latency > 15*time.Second {
				log.Printf("Warning: Alert publishing latency exceeded 15s: %v", latency)
			}
			return nil
		}

		lastErr = err
		log.Printf("Failed to publish alert (attempt %d/%d): %v", i+1, p.maxRetries, err)

		if i < p.maxRetries-1 {
			// Trigger reconnection on error
			select {
			case p.reconnectCh <- true:
			default:
			}
			time.Sleep(p.retryDelay)
		}
	}

	return fmt.Errorf("failed to publish alert after %d retries: %w", p.maxRetries, lastErr)
}

// Close closes the RabbitMQ connection
func (p *RabbitMQPublisher) Close() error {
	close(p.stopReconnect)
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	if p.channel != nil {
		p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Ensure RabbitMQPublisher implements the interface
var _ ports.AlertPublisher = (*RabbitMQPublisher)(nil)

