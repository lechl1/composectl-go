FROM golang:1.21-bullseye

# Install Docker CLI from Docker's official Debian repo to ensure a recent client
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl gnupg; \
    mkdir -p /etc/apt/keyrings; \
    curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg; \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian bullseye stable" > /etc/apt/sources.list.d/docker.list; \
    apt-get update; \
    apt-get install -y --no-install-recommends docker-ce-cli docker-compose-plugin; \
    rm -rf /var/lib/apt/lists/* /etc/apt/keyrings/docker.gpg

# Create app directory
WORKDIR /app

# Copy the built binary from included build context
COPY dcapi /usr/local/bin/dcapi
RUN chmod +x /usr/local/bin/dcapi

COPY dc /usr/local/bin/dc
RUN chmod +x /usr/local/bin/dc

COPY pw /usr/local/bin/pw
RUN chmod +x /usr/local/bin/pw

EXPOSE 8882
ENTRYPOINT ["/usr/local/bin/dcapi"]
