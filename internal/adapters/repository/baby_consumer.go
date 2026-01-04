package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/IANDYI/care-service/internal/core/ports"
	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
)

// BabyCreationRequest represents a message from RabbitMQ for creating a baby
// This matches the message format sent by the identity-service
// Identity service sends: { "user_id": "uuid-string", "last_name": "string", "room_number": "string" }
type BabyCreationRequest struct {
	UserID     string `json:"user_id"`      // Parent user ID (UUID as string from identity service)
	LastName   string `json:"last_name"`    // Baby's last name
	RoomNumber string `json:"room_number"`  // Room number
}

// BabyConsumer consumes messages from RabbitMQ for automatic baby creation
// Runs in background as a goroutine within the care-service pod
// Duplicate prevention checks ensure only one consumer per pod instance
// (For multi-replica deployments, RabbitMQ distributes messages across replicas)
type BabyConsumer struct {
	conn          *amqp091.Connection
	channel       *amqp091.Channel
	queueName     string
	babyService   ports.BabyService
	connMutex     sync.RWMutex
	reconnectCh   chan bool
	stopReconnect chan bool
	maxRetries    int
	retryDelay    time.Duration
	consumingCtx  context.Context
	consumingMutex sync.Mutex
	isConsuming   bool
}

// NewBabyConsumer creates a new RabbitMQ consumer for baby creation
func NewBabyConsumer(rabbitMQURL string, queueName string, babyService ports.BabyService) (*BabyConsumer, error) {
	if queueName == "" {
		queueName = "baby.creation.requests"
	}

	consumer := &BabyConsumer{
		queueName:     queueName,
		babyService:   babyService,
		maxRetries:    3,
		retryDelay:    1 * time.Second,
		reconnectCh:   make(chan bool, 1),
		stopReconnect: make(chan bool),
	}

	// Connect to RabbitMQ
	if err := consumer.connect(rabbitMQURL); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Start reconnection handler
	go consumer.handleReconnection(rabbitMQURL)

	return consumer, nil
}

// connect establishes connection to RabbitMQ
func (c *BabyConsumer) connect(rabbitMQURL string) error {
	var err error
	for i := 0; i < c.maxRetries; i++ {
		c.conn, err = amqp091.Dial(rabbitMQURL)
		if err == nil {
			break
		}
		log.Printf("Failed to connect to RabbitMQ (attempt %d/%d): %v", i+1, c.maxRetries, err)
		if i < c.maxRetries-1 {
			time.Sleep(c.retryDelay)
		}
	}

	if err != nil {
		return err
	}

	c.channel, err = c.conn.Channel()
	if err != nil {
		c.conn.Close()
		return err
	}

	// Declare queue (idempotent)
	_, err = c.channel.QueueDeclare(
		c.queueName, // name
		true,        // durable
		false,       // delete when unused
		false,       // exclusive
		false,       // no-wait
		nil,         // arguments
	)

	if err != nil {
		c.channel.Close()
		c.conn.Close()
		return err
	}

	log.Println("Baby consumer connected to RabbitMQ successfully")
	return nil
}

// handleReconnection handles automatic reconnection to RabbitMQ
func (c *BabyConsumer) handleReconnection(rabbitMQURL string) {
	for {
		select {
		case <-c.reconnectCh:
			log.Println("Attempting to reconnect to RabbitMQ...")
			c.connMutex.Lock()
			if c.conn != nil && !c.conn.IsClosed() {
				c.conn.Close()
			}
			if c.channel != nil && !c.channel.IsClosed() {
				c.channel.Close()
			}
			c.connMutex.Unlock()

			if err := c.connect(rabbitMQURL); err != nil {
				log.Printf("Reconnection failed: %v", err)
				time.Sleep(5 * time.Second)
				c.reconnectCh <- true
			} else {
				// Restart consuming after reconnection using the original context
				c.consumingMutex.Lock()
				if c.consumingCtx != nil && c.consumingCtx.Err() == nil {
					// Only restart if we have a valid context and we're not already consuming
					if !c.isConsuming {
						go c.StartConsuming(c.consumingCtx)
					}
				}
				c.consumingMutex.Unlock()
			}
		case <-c.stopReconnect:
			return
		}
	}
}

// StartConsuming starts consuming messages from the queue in a background goroutine
// This method is called from main.go and runs asynchronously
// Duplicate prevention: ensures only one consumer per pod instance
// (In multi-replica scenarios, each replica will have its own consumer, and RabbitMQ
// will distribute messages across them using round-robin)
func (c *BabyConsumer) StartConsuming(ctx context.Context) error {
	// Prevent multiple consumers from starting in the same pod instance
	// This check is important when there are multiple care-service replicas
	c.consumingMutex.Lock()
	if c.isConsuming {
		c.consumingMutex.Unlock()
		log.Println("Baby consumer is already running in this pod, skipping duplicate start")
		return nil
	}
	c.isConsuming = true
	c.consumingCtx = ctx
	c.consumingMutex.Unlock()

	c.connMutex.RLock()
	channel := c.channel
	conn := c.conn
	c.connMutex.RUnlock()

	if channel == nil || channel.IsClosed() || conn == nil || conn.IsClosed() {
		c.consumingMutex.Lock()
		c.isConsuming = false
		c.consumingMutex.Unlock()
		return fmt.Errorf("RabbitMQ connection is closed")
	}

	// Set QoS to process one message at a time (ensures only one unacknowledged message per consumer)
	err := channel.Qos(
		1,     // prefetch count - only one message at a time
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		c.consumingMutex.Lock()
		c.isConsuming = false
		c.consumingMutex.Unlock()
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	// Register consumer with a unique consumer tag to identify this instance
	consumerTag := fmt.Sprintf("baby-consumer-%d", time.Now().UnixNano())
	msgs, err := channel.Consume(
		c.queueName, // queue
		consumerTag, // consumer tag (unique identifier)
		false,       // auto-ack (manual ack - we acknowledge only after successful baby creation)
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
	)
	if err != nil {
		c.consumingMutex.Lock()
		c.isConsuming = false
		c.consumingMutex.Unlock()
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	log.Printf("Baby consumer started (tag: %s), waiting for messages on queue: %s", consumerTag, c.queueName)

	// Process messages sequentially (QoS=1 ensures only one message is delivered at a time)
	go func() {
		defer func() {
			c.consumingMutex.Lock()
			c.isConsuming = false
			c.consumingMutex.Unlock()
		}()

		for {
			select {
			case <-ctx.Done():
				log.Println("Baby consumer context cancelled")
				return
			case msg, ok := <-msgs:
				if !ok {
					log.Println("Baby consumer channel closed, attempting reconnection...")
					c.reconnectCh <- true
					return
				}

				// Process message sequentially (no goroutine - ensures only one message at a time)
				// Acknowledgment happens only after successful baby creation in processMessage
				c.processMessage(ctx, msg)
			}
		}
	}()

	return nil
}

// processMessage processes a single message from RabbitMQ
// IMPORTANT: Message is acknowledged ONLY after successful baby creation
// If baby creation fails, message is nacked and requeued for retry
func (c *BabyConsumer) processMessage(ctx context.Context, msg amqp091.Delivery) {
	var req BabyCreationRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		log.Printf("Failed to unmarshal baby creation request: %v", err)
		// Invalid message format - reject and don't requeue (will be lost)
		msg.Nack(false, false)
		return
	}

	log.Printf("Received baby creation request: user_id=%s, last_name=%s, room_number=%s",
		req.UserID, req.LastName, req.RoomNumber)

	// Validate request
	if req.UserID == "" {
		log.Printf("Invalid baby creation request: user_id is required")
		// Invalid data - reject and don't requeue
		msg.Nack(false, false)
		return
	}
	if req.LastName == "" {
		log.Printf("Invalid baby creation request: last_name is required")
		// Invalid data - reject and don't requeue
		msg.Nack(false, false)
		return
	}
	if req.RoomNumber == "" {
		log.Printf("Invalid baby creation request: room_number is required")
		// Invalid data - reject and don't requeue
		msg.Nack(false, false)
		return
	}

	// Parse user_id (UUID string) to uuid.UUID
	parentUserID, err := uuid.Parse(req.UserID)
	if err != nil {
		log.Printf("Invalid baby creation request: user_id is not a valid UUID: %v", err)
		// Invalid UUID format - reject and don't requeue
		msg.Nack(false, false)
		return
	}

	// Create baby using the service (ADMIN context - automated creation)
	// Note: We use a system/admin context for automated creation
	// In production, you might want to pass a system user ID or use a different approach
	adminUserID := uuid.Nil // System user for automated creation
	baby, err := c.babyService.CreateBaby(ctx, req.LastName, req.RoomNumber, parentUserID, adminUserID, true)
	if err != nil {
		log.Printf("Failed to create baby from RabbitMQ message: %v", err)
		// Baby creation failed - reject and requeue for retry
		// This ensures the message will be redelivered and we can try again
		msg.Nack(false, true)
		return
	}

	// Baby creation succeeded - log success
	log.Printf("Successfully created baby from RabbitMQ: id=%s, last_name=%s, room_number=%s",
		baby.ID, baby.LastName, baby.RoomNumber)

	// CRITICAL: Acknowledge message ONLY after successful baby creation
	// This ensures the message is removed from the queue only when baby creation succeeds
	// If acknowledgment fails, the message will be redelivered (at-least-once delivery)
	if err := msg.Ack(false); err != nil {
		log.Printf("Failed to acknowledge message after baby creation: %v", err)
		// If ack fails, message will be redelivered, which is safe (idempotent operation)
	}
}

// Close closes the RabbitMQ connection and stops consuming
// Note: The consuming context is cancelled by main.go during graceful shutdown
func (c *BabyConsumer) Close() error {
	// Stop reconnection handler
	close(c.stopReconnect)

	// Mark as not consuming (context cancellation is handled by main.go)
	c.consumingMutex.Lock()
	c.isConsuming = false
	c.consumingMutex.Unlock()

	// Close RabbitMQ connection
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	if c.channel != nil && !c.channel.IsClosed() {
		if err := c.channel.Close(); err != nil {
			log.Printf("Error closing RabbitMQ channel: %v", err)
		}
	}

	if c.conn != nil && !c.conn.IsClosed() {
		if err := c.conn.Close(); err != nil {
			log.Printf("Error closing RabbitMQ connection: %v", err)
		}
	}

	log.Println("Baby consumer closed")
	return nil
}
