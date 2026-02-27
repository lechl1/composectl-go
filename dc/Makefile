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

docker: build
	@echo "Building docker image: dc:local"
	cp pw $(BUILD_DIR)/
	docker build -t dc:local -f Dockerfile build

clean:
	rm -f $(BINARY_PATH)

install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY_PATH) $(INSTALL_DIR)/$(BINARY_NAME)
	chmod +x $(INSTALL_DIR)/$(BINARY_NAME)
	# Install pw helper if present in repo, but avoid overwriting a user's custom pw
	if [ -f pw ]; then \
		TARGET="$(INSTALL_DIR)/pw"; \
		ID="COMPOSECTL_PW_ID=composectl_pw_default_v1"; \
		if [ ! -f "$${TARGET}" ]; then \
			cp pw $${TARGET}; chmod +x $${TARGET}; \
			echo "installed pw helper to $${TARGET}"; \
		else \
			# If the existing pw contains our identifier, it's the repo default and we overwrite it
			if grep -q "$${ID}" "$${TARGET}" 2>/dev/null; then \
				cp pw $${TARGET}; chmod +x $${TARGET}; \
				echo "updated pw helper at $${TARGET}"; \
			else \
				echo "skipping pw install: custom pw detected at $${TARGET} (not overwritten)"; \
			fi; \
		fi; \
	fi

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
