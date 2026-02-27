FROM golang:1.21-alpine3.18

# Create app directory
WORKDIR /app

# Copy the built binary from included build context
COPY dcapi /usr/local/bin/dcapi
RUN chmod +x /usr/local/bin/dcapi

EXPOSE 8882
ENTRYPOINT ["/usr/local/bin/dcapi"]

