.PHONY: build clean test install start stop restart status update setup-auth enable disable reinstall uninstall

build:
	$(MAKE) -C dcapi build
	$(MAKE) -C dc build

clean:
	$(MAKE) -C dcapi clean
	$(MAKE) -C dc clean

test:
	$(MAKE) -C dcapi test
	$(MAKE) -C dc test

install:
	$(MAKE) -C dcapi install
	$(MAKE) -C dc install

start stop restart status update setup-auth enable disable reinstall uninstall:
	$(MAKE) -C dcapi $@
