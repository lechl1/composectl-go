BINARY_NAME=dc
BUILD_DIR=build
BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)
INSTALL_DIR=$(HOME)/.local/bin
GO=go
GOFLAGS=-ldflags="-s -w"

.PHONY: build clean install test docker uninstall

build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BINARY_PATH)
	cp $(BINARY_PATH) $(INSTALL_DIR)/$(BINARY_NAME)
	cp pw $(BUILD_DIR)/pw

clean:
	rm -f $(BINARY_PATH)

install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY_PATH) $(INSTALL_DIR)/$(BINARY_NAME)
	cp $(BUILD_DIR)/pw $(INSTALL_DIR)/pw
	chmod +x $(INSTALL_DIR)/$(BINARY_NAME)
	chmod +x $(INSTALL_DIR)/pw

test:
	$(GO) test -v ./...

uninstall: ## Remove installed binary and pw helper if it was installed by this project
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Removed binary from $(INSTALL_DIR)/$(BINARY_NAME)"
	# Only remove pw helper if it matches the repo identifier to avoid deleting a user's custom helper
	@if [ -f "$(INSTALL_DIR)/pw" ]; then \
		ID="COMPOSECTL_PW_ID=composectl_pw_default_v1"; \
		if grep -q "$$ID" "$(INSTALL_DIR)/pw" 2>/dev/null; then \
			rm -f "$(INSTALL_DIR)/pw"; \
			echo "Removed pw helper at $(INSTALL_DIR)/pw"; \
		else \
			echo "Skipping removal of pw helper: custom pw detected at $(INSTALL_DIR)/pw"; \
		fi; \
	fi
