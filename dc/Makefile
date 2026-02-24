BINARY_NAME=dc
INSTALL_DIR=$(HOME)/.local/bin
GO=go
GOFLAGS=-ldflags="-s -w"

.PHONY: build clean install test

build:
	$(GO) build $(GOFLAGS) -o $(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)

install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	chmod +x $(INSTALL_DIR)/$(BINARY_NAME)

test:
	$(GO) test -v ./...
