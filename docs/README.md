# dc - Docker Compose Management Service

A web-based management interface for Docker Compose stacks with real-time monitoring and control.

## Features

- **Real-time Stack Management**: Monitor and control Docker Compose stacks
- **WebSocket Updates**: Live updates for container status changes
- **Basic Authentication**: Secure access with username/password authentication
- **File Watching**: Automatic updates when stack files change
- **Thumbnail Generation**: Visual previews for containers
- **YAML Enrichment**: Enhance and validate Docker Compose files

## Installation

### Quick Start

1. **Clone the repository** (or download the source code)

2. **Set up credentials** in `prod.env`:
   ```bash
   # Edit prod.env and set:
   ADMIN_USERNAME=your_username
   ADMIN_PASSWORD=your_secure_password
   ```

3. **Install and run**:
   ```bash
   make install
   make enable
   make start
   ```

### Available Make Targets

- `make build` - Build the application binary
- `make install` - Install the application and set up systemd user service
- `make enable` - Enable service to start automatically after login
- `make start` - Start the service
- `make stop` - Stop the service
- `make restart` - Restart the service
- `make status` - Show service status
- `make logs` - Show service logs (follow mode)
- `make update` - Update the binary and restart service
- `make uninstall` - Uninstall the application and remove systemd service
- `make reinstall` - Complete reinstall (uninstall, install, enable and start)
- `make clean` - Clean build artifacts
- `make test` - Run tests
- `make help` - Show all available targets

## Configuration

### Authentication

The application uses Basic Authentication to protect all HTTP endpoints. Credentials must be configured in `prod.env`:

```bash
ADMIN_USERNAME=admin
ADMIN_PASSWORD=your_secure_password_here
```

**Important**: 
- Set strong passwords in production
- The `prod.env` file should be protected with appropriate file permissions
- All HTTP endpoints require authentication

### Working Directory

The service runs from the installation directory and requires access to:
- `stacks/` - Docker Compose stack definitions
- `thumbnails/` - Container thumbnail images
- `prod.env` - Environment variables and credentials

## Usage

### Starting the Service

After installation, the service can be managed with systemctl:

```bash
# Start now
make start

# Enable automatic start after login
make enable

# Check status
make status

# View logs
make logs
```

### Accessing the Web Interface

Once running, access the interface at:
```
http://localhost:8080
```

You will be prompted for the username and password configured in `prod.env`.

### Managing Stacks

Place your Docker Compose YAML files in the `stacks/` directory. The service will automatically detect and manage them.

## Development

### Building from Source

```bash
# Build only
make build

# Build and test
make test
make build
```

### Project Structure

```
dcapi-go/
├── main.go           # Application entry point
├── auth.go           # Basic Authentication middleware
├── server.go         # HTTP server and routing
├── stack.go          # Docker Compose stack management
├── container.go      # Container operations
├── websocket.go      # WebSocket handling
├── watch.go          # File watching
├── thumbnail.go      # Thumbnail generation
├── yaml.go           # YAML processing
├── prod.env          # Environment configuration
├── stacks/           # Docker Compose files
├── thumbnails/       # Generated thumbnails
└── docs/             # Documentation

```

## Security Considerations

1. **Change Default Credentials**: Always set strong, unique credentials in `prod.env`
2. **File Permissions**: Restrict access to `prod.env`:
   ```bash
   chmod 600 prod.env
   ```
3. **HTTPS**: Consider using a reverse proxy (like Traefik or Nginx) with HTTPS
4. **Firewall**: Restrict access to port 8080 if exposed to the network

## Troubleshooting

### Service won't start

Check the logs:
```bash
make logs
```

Common issues:
- Missing credentials in `prod.env`
- Port 8080 already in use
- Insufficient permissions for Docker operations

### Authentication fails

Verify credentials in `prod.env`:
```bash
cat prod.env | grep ADMIN
```

Ensure both `ADMIN_USERNAME` and `ADMIN_PASSWORD` are set.

### Service not starting after login

Check if the service is enabled:
```bash
systemctl --user is-enabled dcapi.service
```

Enable if needed:
```bash
make enable
```

## License

[Add your license information here]

## Contributing

[Add contribution guidelines here]

