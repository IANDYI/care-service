package config

import (
	"crypto/rsa"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

type AlertConsumerConfig struct {
	JWTPublicKey  *rsa.PublicKey
	RabbitMQURL   string
	QueueName     string
	WebSocketPort string
	PublicKeyPath string
}

func LoadAlertConsumerConfig() *AlertConsumerConfig {
	publicKeyPath := os.Getenv("PUBLIC_KEY_PATH")
	if publicKeyPath == "" {
		publicKeyPath = "/etc/certs/public.pem"
	}
	
	publicKey, err := loadPublicKey(publicKeyPath)
	if err != nil {
		publicKey = nil
	}

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}

	queueName := os.Getenv("QUEUE_NAME")
	if queueName == "" {
		queueName = "baby_alerts"
	}

	wsPort := os.Getenv("WEBSOCKET_PORT")
	if wsPort == "" {
		wsPort = "8081"
	}

	return &AlertConsumerConfig{
		JWTPublicKey:  publicKey,
		RabbitMQURL:   rabbitMQURL,
		QueueName:     queueName,
		WebSocketPort: wsPort,
		PublicKeyPath: publicKeyPath,
	}
}
