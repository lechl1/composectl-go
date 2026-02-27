INSTALL_DIR=$(HOME)/.local/bin
NPM=npm

.PHONY: build clean install test dev docker check-config

build:
	$(NPM) run build

docker: build ## Build Docker image for frontend (dcgui)
	@echo "Building docker image: dcgui:local"
	docker build -t dcgui:local -f Dockerfile build --build-arg BUILDDIR=build

clean:
	rm -rf build

test:
	$(NPM) run check
	# also validate nginx config image (delegates to check-config)
	@echo "Checking nginx config inside container (renders with DC_API_URL if provided)"
	# Run container, let entrypoint render config, then run nginx -t
	# Add a hosts entry for 'dcapi' pointing to localhost so nginx can resolve the upstream during the test.
	docker run --rm --add-host=dcapi:127.0.0.1 $(if $(DC_API_URL),-e DC_API_URL=$(DC_API_URL),) dcgui:local sh -c "env && nginx -t -c /etc/nginx/nginx.conf" || true


dev:
	$(NPM) run dev
