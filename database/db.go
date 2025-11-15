package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/allintech/github-sentry/config"
	_ "github.com/lib/pq"
)

var db *sql.DB

// InitDB initializes the database connection and creates tables
func InitDB(cfg *config.Config) error {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	if err := createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// createTables creates the triggers and executions tables
func createTables() error {
	triggersTable := `
	CREATE TABLE IF NOT EXISTS triggers (
		id SERIAL PRIMARY KEY,
		time TIMESTAMP NOT NULL,
		commit_id VARCHAR(40) NOT NULL,
		commit_message TEXT NOT NULL,
		branch VARCHAR(255) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	executionsTable := `
	CREATE TABLE IF NOT EXISTS executions (
		id SERIAL PRIMARY KEY,
		trigger_id INTEGER NOT NULL REFERENCES triggers(id) ON DELETE CASCADE,
		script_name VARCHAR(255) NOT NULL,
		status VARCHAR(20) NOT NULL,
		output TEXT,
		error TEXT,
		executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(triggersTable); err != nil {
		return fmt.Errorf("failed to create triggers table: %w", err)
	}

	if _, err := db.Exec(executionsTable); err != nil {
		return fmt.Errorf("failed to create executions table: %w", err)
	}

	return nil
}

// GetDB returns the database connection
func GetDB() *sql.DB {
	return db
}

// Trigger represents a webhook trigger record
type Trigger struct {
	ID            int64
	Time          time.Time
	CommitID      string
	CommitMessage string
	Branch        string
	CreatedAt     time.Time
}

// RecordTrigger records a new trigger in the database
func RecordTrigger(time time.Time, commitID, commitMessage, branch string) (int64, error) {
	query := `
		INSERT INTO triggers (time, commit_id, commit_message, branch)
		VALUES ($1, $2, $3, $4)
		RETURNING id`

	var id int64
	err := db.QueryRow(query, time, commitID, commitMessage, branch).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to record trigger: %w", err)
	}

	return id, nil
}

// Execution represents a script execution record
type Execution struct {
	ID        int64
	TriggerID int64
	ScriptName string
	Status    string
	Output    string
	Error     string
	ExecutedAt time.Time
}

// RecordExecution records a script execution in the database
func RecordExecution(triggerID int64, scriptName, status, output, errorMsg string) error {
	query := `
		INSERT INTO executions (trigger_id, script_name, status, output, error)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := db.Exec(query, triggerID, scriptName, status, output, errorMsg)
	if err != nil {
		return fmt.Errorf("failed to record execution: %w", err)
	}

	return nil
}

// Close closes the database connection
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

