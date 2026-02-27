BINARY_NAME=dc
BUILD_DIR=build
BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)
INSTALL_DIR=$(HOME)/.local/bin
GO=go
GOFLAGS=-ldflags="-s -w"

.PHONY: build clean install test

build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BINARY_PATH)

clean:
	rm -f $(BINARY_PATH)

install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY_PATH) $(INSTALL_DIR)/$(BINARY_NAME)
	chmod +x $(INSTALL_DIR)/$(BINARY_NAME)

test:
	$(GO) test -v ./...
