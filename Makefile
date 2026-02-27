.PHONY: build clean test install docker start stop restart status update setup-auth enable disable reinstall uninstall

BUILD_DIR=build

build:
	$(MAKE) -C dc build
	$(MAKE) -C dcapi build
	$(MAKE) -C dcgui build

docker: build
	$(MAKE) -C dc docker
	$(MAKE) -C dcapi docker
	$(MAKE) -C dcgui docker

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

start:
	$(MAKE) -C dcapi start


start stop restart status update setup-auth enable disable reinstall uninstall:
	$(MAKE) -C dcapi $@
