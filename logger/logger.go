package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

var (
	logFile *os.File
	logger  *log.Logger
)

// InitLogger initializes the logger with a file in the specified log folder
func InitLogger(logFolder string) error {
	// Ensure log folder exists
	if err := os.MkdirAll(logFolder, 0755); err != nil {
		return fmt.Errorf("failed to create log folder: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02")
	logFileName := filepath.Join(logFolder, fmt.Sprintf("webhook-%s.log", timestamp))

	var err error
	logFile, err = os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Create logger with both file and stdout output
	logger = log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)

	return nil
}

// Log writes a log message
func Log(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	logger.Println(message)
	// Also output to stdout for immediate visibility
	fmt.Println(message)
}

// LogTrigger logs a webhook trigger event
func LogTrigger(commitID, commitMessage, branch string) {
	Log("TRIGGER: branch=%s commit_id=%s message=%s", branch, commitID, commitMessage)
}

// LogExecution logs a script execution
func LogExecution(scriptName string, success bool, output, errorMsg string) {
	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}
	Log("EXECUTION: script=%s status=%s", scriptName, status)
	if output != "" {
		Log("OUTPUT: %s", output)
	}
	if errorMsg != "" {
		Log("ERROR: %s", errorMsg)
	}
}

// LogError logs an error
func LogError(format string, v ...interface{}) {
	Log("ERROR: "+format, v...)
}

// LogInfo logs an info message
func LogInfo(format string, v ...interface{}) {
	Log("INFO: "+format, v...)
}

// Close closes the log file
func Close() error {
	if logFile != nil {
		return logFile.Close()
	}
	return nil
}

