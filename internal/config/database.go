package config

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// InitDatabase creates the database schema from scratch
// This is POC-friendly: auto-creates tables on startup
// Set DROP_TABLES_ON_STARTUP=true environment variable to drop existing tables
func InitDatabase(db *sql.DB) error {
	// Only drop tables if explicitly requested (via env var)
	// This prevents accidental data loss on restart
	if os.Getenv("DROP_TABLES_ON_STARTUP") == "true" {
		log.Println("Dropping existing tables (DROP_TABLES_ON_STARTUP=true)...")
		if _, err := db.Exec("DROP TABLE IF EXISTS measurements CASCADE"); err != nil {
			log.Printf("Warning: Failed to drop measurements table: %v", err)
		}
		if _, err := db.Exec("DROP TABLE IF EXISTS babies CASCADE"); err != nil {
			log.Printf("Warning: Failed to drop babies table: %v", err)
		}
	} else {
		log.Println("Skipping table drop (set DROP_TABLES_ON_STARTUP=true to drop tables on startup)")
	}
	
	// Create babies table
	log.Println("Creating babies table...")
	babiesSchema := `
	CREATE TABLE babies (
		id UUID PRIMARY KEY,
		last_name TEXT NOT NULL,
		room_number TEXT NOT NULL,
		parent_user_id UUID NOT NULL,
		created_at TIMESTAMP DEFAULT now()
	);`
	
	if _, err := db.Exec(babiesSchema); err != nil {
		return fmt.Errorf("failed to create babies table: %w", err)
	}

	// Create measurements table
	log.Println("Creating measurements table...")
	measurementsSchema := `
	CREATE TABLE measurements (
		id UUID PRIMARY KEY,
		parent_id UUID NOT NULL,
		baby_id UUID NOT NULL REFERENCES babies(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		value NUMERIC NOT NULL,
		safety_status TEXT NOT NULL DEFAULT 'green',
		note TEXT,
		timestamp TIMESTAMP,
		created_at TIMESTAMP DEFAULT now(),
		-- Feeding-specific fields
		feeding_type TEXT,
		volume_ml INTEGER,
		position TEXT,
		side TEXT,
		left_duration INTEGER,
		right_duration INTEGER,
		duration INTEGER,
		-- Temperature-specific fields
		value_celsius NUMERIC,
		-- Diaper-specific fields
		diaper_status TEXT,
		-- CHECK constraints for data integrity
		CONSTRAINT chk_feeding_fields CHECK (
			(type != 'feeding' AND volume_ml IS NULL AND feeding_type IS NULL) OR
			(type = 'feeding' AND feeding_type IS NOT NULL)
		),
		CONSTRAINT chk_temperature_fields CHECK (
			(type = 'temperature' AND value_celsius IS NOT NULL) OR
			(type != 'temperature' AND value_celsius IS NULL)
		),
		CONSTRAINT chk_diaper_fields CHECK (
			(type = 'diaper' AND diaper_status IS NOT NULL) OR
			(type != 'diaper' AND diaper_status IS NULL)
		),
		CONSTRAINT chk_breastfeeding_durations CHECK (
			(side != 'both') OR
			(side = 'both' AND left_duration IS NOT NULL AND right_duration IS NOT NULL)
		)
	);`
	
	if _, err := db.Exec(measurementsSchema); err != nil {
		return fmt.Errorf("failed to create measurements table: %w", err)
	}
	
	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_babies_parent_user_id ON babies(parent_user_id)",
		"CREATE INDEX IF NOT EXISTS idx_measurements_baby_id ON measurements(baby_id)",
		"CREATE INDEX IF NOT EXISTS idx_measurements_parent_id ON measurements(parent_id)",
		"CREATE INDEX IF NOT EXISTS idx_measurements_created_at ON measurements(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_measurements_timestamp ON measurements(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_measurements_safety_status ON measurements(safety_status)",
		"CREATE INDEX IF NOT EXISTS idx_measurements_type ON measurements(type)",
	}
	
	for _, indexSQL := range indexes {
		if _, err := db.Exec(indexSQL); err != nil {
			log.Printf("Warning: Failed to create index: %v", err)
		}
	}

	log.Println("Database schema initialized successfully")
	return nil
}

// ConnectDatabase establishes a connection to PostgreSQL with retry logic
func ConnectDatabase(databaseURL string, maxRetries int, retryDelay time.Duration) (*sql.DB, error) {
	var db *sql.DB
	var err error

	for i := 0; i < maxRetries; i++ {
		db, err = sql.Open("postgres", databaseURL)
		if err != nil {
			log.Printf("Failed to open database connection (attempt %d/%d): %v", i+1, maxRetries, err)
			if i < maxRetries-1 {
				time.Sleep(retryDelay)
				continue
			}
			return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, err)
		}

		// Test the connection
		if err = db.Ping(); err != nil {
			log.Printf("Failed to ping database (attempt %d/%d): %v", i+1, maxRetries, err)
			db.Close()
			if i < maxRetries-1 {
				time.Sleep(retryDelay)
				continue
			}
			return nil, fmt.Errorf("failed to ping database after %d attempts: %w", maxRetries, err)
		}

		// Configure connection pool
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)

		log.Println("Database connection established successfully")
		return db, nil
	}

	return nil, fmt.Errorf("failed to connect to database: %w", err)
}

