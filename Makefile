.PHONY: build clean test install docker start stop restart status update setup-auth enable disable reinstall uninstall

BUILD_DIR=build

build:
	$(MAKE) -C dc build
	$(MAKE) -C dcapi build
	$(MAKE) -C dcgui build

docker: build
	@echo "Collecting build artifacts into $(BUILD_DIR)..."
	@mkdir -p $(BUILD_DIR)
	@cp dc/build/dc $(BUILD_DIR)/dc
	@cp dcapi/build/dcapi $(BUILD_DIR)/dcapi
	@cp -r dcgui/build $(BUILD_DIR)/frontend
	@echo "Running docker compose build..."
	docker compose build
	./dc/dc up docker-compose

clean:
	$(MAKE) -C dc clean
	$(MAKE) -C dcapi clean
	$(MAKE) -C dcgui clean
	rm -rf $(BUILD_DIR)

test:
	$(MAKE) -C dc test
	$(MAKE) -C dcapi test
	$(MAKE) -C dcgui test

install:
	$(MAKE) -C dc install
	$(MAKE) -C dcapi install
	# $(MAKE) -C dcgui install // Bundled with dcapi

start stop restart status update setup-auth enable disable reinstall uninstall:
	$(MAKE) -C dcapi $@
