.PHONY: build install uninstall start stop restart status enable disable clean test help

# Binary and service names
BINARY_NAME=composectl
SERVICE_NAME=composectl

# Installation paths
INSTALL_DIR=$(HOME)/.local/bin
SYSTEMD_USER_DIR=$(HOME)/.config/systemd/user
SERVICE_FILE=$(SYSTEMD_USER_DIR)/$(SERVICE_NAME).service
WORKING_DIR=$(shell pwd)

# Build variables
GO=go
GOFLAGS=-ldflags="-s -w"

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the application
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) -o $(BINARY_NAME)
	@echo "Build complete: $(BINARY_NAME)"

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -f $(BINARY_NAME)
	@echo "Clean complete"

test: ## Run tests
	@echo "Running tests..."
	$(GO) test -v ./...
	@echo "Tests complete"

install: build ## Install the application and set up systemd user service
	@echo "Installing $(BINARY_NAME)..."

	# Create installation directory if it doesn't exist
	@mkdir -p $(INSTALL_DIR)

	# Copy binary to installation directory
	@cp $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@chmod +x $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Binary installed to $(INSTALL_DIR)/$(BINARY_NAME)"

	# Create systemd user directory if it doesn't exist
	@mkdir -p $(SYSTEMD_USER_DIR)

	# Create systemd user service file
	@echo "Creating systemd user service..."
	@echo "[Unit]" > $(SERVICE_FILE)
	@echo "Description=ComposeCTL - Docker Compose Management Service" >> $(SERVICE_FILE)
	@echo "After=network.target docker.service" >> $(SERVICE_FILE)
	@echo "Wants=docker.service" >> $(SERVICE_FILE)
	@echo "" >> $(SERVICE_FILE)
	@echo "[Service]" >> $(SERVICE_FILE)
	@echo "Type=simple" >> $(SERVICE_FILE)
	@echo "WorkingDirectory=$(WORKING_DIR)" >> $(SERVICE_FILE)
	@echo "ExecStart=$(INSTALL_DIR)/$(BINARY_NAME)" >> $(SERVICE_FILE)
	@echo "Restart=on-failure" >> $(SERVICE_FILE)
	@echo "RestartSec=10" >> $(SERVICE_FILE)
	@echo "" >> $(SERVICE_FILE)
	@echo "[Install]" >> $(SERVICE_FILE)
	@echo "WantedBy=default.target" >> $(SERVICE_FILE)
	@echo "Service file created at $(SERVICE_FILE)"

	# Reload systemd user daemon
	@systemctl --user daemon-reload
	@echo "Systemd user daemon reloaded"

	@echo ""
	@echo "Installation complete!"
	@echo ""
	@echo "To enable the service to start automatically after login:"
	@echo "  make enable"
	@echo ""
	@echo "To start the service now:"
	@echo "  make start"

enable: ## Enable service to start automatically after login
	@echo "Enabling $(SERVICE_NAME) service..."
	@systemctl --user enable $(SERVICE_NAME).service
	@echo "Service enabled. It will start automatically after login."
	@echo ""
	@echo "To start the service now:"
	@echo "  make start"

disable: ## Disable automatic start after login
	@echo "Disabling $(SERVICE_NAME) service..."
	@systemctl --user disable $(SERVICE_NAME).service
	@echo "Service disabled"

start: ## Start the service
	@echo "Starting $(SERVICE_NAME) service..."
	@systemctl --user start $(SERVICE_NAME).service
	@echo "Service started"
	@echo ""
	@echo "Check status with: make status"
	@echo "View logs with: journalctl --user -u $(SERVICE_NAME).service -f"

stop: ## Stop the service
	@echo "Stopping $(SERVICE_NAME) service..."
	@systemctl --user stop $(SERVICE_NAME).service
	@echo "Service stopped"

restart: ## Restart the service
	@echo "Restarting $(SERVICE_NAME) service..."
	@systemctl --user restart $(SERVICE_NAME).service
	@echo "Service restarted"

status: ## Show service status
	@systemctl --user status $(SERVICE_NAME).service

logs: ## Show service logs (follow mode)
	@journalctl --user -u $(SERVICE_NAME).service -f

uninstall: stop disable ## Uninstall the application and remove systemd service
	@echo "Uninstalling $(BINARY_NAME)..."

	# Remove binary
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Binary removed from $(INSTALL_DIR)"

	# Remove systemd service file
	@rm -f $(SERVICE_FILE)
	@echo "Service file removed"

	# Reload systemd user daemon
	@systemctl --user daemon-reload
	@echo "Systemd user daemon reloaded"

	@echo "Uninstallation complete"

reinstall: uninstall install enable start ## Uninstall, install, enable and start

update: build stop ## Update the binary and restart service
	@echo "Updating $(BINARY_NAME)..."
	@cp $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@chmod +x $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Binary updated"
	@$(MAKE) start

