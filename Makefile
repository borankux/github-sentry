# Makefile for github-sentry

# Application name
APP_NAME := github-sentry
BINARY := $(APP_NAME)
LOG_DIR := /var/logs/github-sentry
LOG_FILE := $(LOG_DIR)/app.log
PID_FILE := /var/run/$(APP_NAME).pid

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# Build flags
LDFLAGS := -s -w
BUILD_FLAGS := -ldflags "$(LDFLAGS)"

.PHONY: all build rebuild clean test deps run stop install uninstall help

# Default target
all: clean build

# Build the application
build:
	@echo "Building $(BINARY)..."
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY) .
	@echo "Build complete: $(BINARY)"

# Rebuild with cleanup
rebuild: clean build
	@echo "Rebuild complete"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -f $(BINARY)
	@rm -f $(BINARY).exe
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Create log directory
create-log-dir:
	@echo "Creating log directory..."
	@sudo mkdir -p $(LOG_DIR)
	@sudo chmod 755 $(LOG_DIR)
	@echo "Log directory created: $(LOG_DIR)"

# Run the application in foreground
run:
	@echo "Running $(BINARY)..."
	./$(BINARY)

# Run the application in background with nohup
run-background: create-log-dir
	@echo "Starting $(BINARY) in background..."
	@if [ -f $(PID_FILE) ]; then \
		echo "Application already running (PID: $$(cat $(PID_FILE)))"; \
		exit 1; \
	fi
	@nohup ./$(BINARY) > $(LOG_FILE) 2>&1 & \
	echo $$! > $(PID_FILE)
	@echo "Application started in background"
	@echo "PID: $$(cat $(PID_FILE))"
	@echo "Log file: $(LOG_FILE)"
	@echo "To view logs: tail -f $(LOG_FILE)"

# Stop the application
stop:
	@echo "Stopping $(BINARY)..."
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if ps -p $$PID > /dev/null 2>&1; then \
			kill $$PID; \
			rm -f $(PID_FILE); \
			echo "Application stopped (PID: $$PID)"; \
		else \
			echo "Process not running"; \
			rm -f $(PID_FILE); \
		fi \
	else \
		echo "PID file not found. Application may not be running."; \
	fi

# Restart the application
restart: stop run-background
	@echo "Application restarted"

# Install the application (copy binary to /usr/local/bin)
install: build create-log-dir
	@echo "Installing $(BINARY)..."
	@sudo cp $(BINARY) /usr/local/bin/$(BINARY)
	@sudo chmod +x /usr/local/bin/$(BINARY)
	@echo "Installed to /usr/local/bin/$(BINARY)"

# Uninstall the application
uninstall: stop
	@echo "Uninstalling $(BINARY)..."
	@sudo rm -f /usr/local/bin/$(BINARY)
	@sudo rm -rf $(LOG_DIR)
	@echo "Uninstalled"

# View logs
logs:
	@if [ -f $(LOG_FILE) ]; then \
		tail -f $(LOG_FILE); \
	else \
		echo "Log file not found: $(LOG_FILE)"; \
	fi

# Check if application is running
status:
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if ps -p $$PID > /dev/null 2>&1; then \
			echo "Application is running (PID: $$PID)"; \
		else \
			echo "Application is not running (stale PID file)"; \
			rm -f $(PID_FILE); \
		fi \
	else \
		echo "Application is not running"; \
	fi

# Help target
help:
	@echo "Available targets:"
	@echo "  make build          - Build the application"
	@echo "  make rebuild        - Clean and rebuild the application"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make deps           - Download and tidy dependencies"
	@echo "  make run            - Run application in foreground"
	@echo "  make run-background - Run application in background with nohup (logs to $(LOG_FILE))"
	@echo "  make stop           - Stop background application"
	@echo "  make restart        - Restart background application"
	@echo "  make install        - Install binary to /usr/local/bin"
	@echo "  make uninstall       - Uninstall application"
	@echo "  make logs            - View application logs (tail -f)"
	@echo "  make status          - Check if application is running"
	@echo "  make help            - Show this help message"

