.PHONY: build clean test install start stop restart status update setup-auth enable disable reinstall uninstall

build:
	$(MAKE) -C dcgui build
	rm -rf dcapi/ui && cp -r dcgui/build dcapi/ui
	$(MAKE) -C dcapi build
	$(MAKE) -C dc build

clean:
	$(MAKE) -C dcapi clean
	$(MAKE) -C dc clean
	$(MAKE) -C dcgui clean
	rm -rf dcapi/ui && mkdir dcapi/ui && echo '<!doctype html><html><body>UI not built. Run <code>make build</code> from the repo root.</body></html>' > dcapi/ui/index.html

test:
	$(MAKE) -C dcapi test
	$(MAKE) -C dc test
	$(MAKE) -C dcgui test

install:
	$(MAKE) -C dcapi install
	$(MAKE) -C dc install
	# $(MAKE) -C dcgui install // Bundled with dcapi

start stop restart status update setup-auth enable disable reinstall uninstall:
	$(MAKE) -C dcapi $@
