# ComposeCTL - Docker Compose Management Service

A web-based management interface for Docker Compose stacks with real-time monitoring and control.

## Features

- **Real-time Stack Management**: Monitor and control Docker Compose stacks
- **WebSocket Updates**: Live updates for container status changes
- **Basic Authentication**: Secure access with username/password authentication
- **File Watching**: Automatic updates when stack files change
- **Thumbnail Generation**: Visual previews for containers
- **YAML Enrichment**: Enhance and validate Docker Compose files
- **Centralized Configuration**: All data stored in `$HOME/.local/containers`

## Installation

### Quick Start

1. **Build the application**:
   ```bash
   go build -o composectl
   ```

2. **Install and configure**:
   ```bash
   make install    # Creates directory structure and prompts for credentials
   make enable     # Enable auto-start after login
   make start      # Start the service
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
- `make setup-auth` - Configure authentication credentials
- `make update` - Update the binary and restart service
- `make uninstall` - Uninstall the application and remove systemd service
- `make reinstall` - Complete reinstall (uninstall, install, enable and start)
- `make clean` - Clean build artifacts
- `make test` - Run tests
- `make help` - Show all available targets

## Configuration

### Directory Structure

ComposeCTL uses `$HOME/.local/containers` as its base directory:

```
$HOME/.local/containers/
├── prod.env          # Environment variables and credentials
├── stacks/           # Docker Compose stack YAML files
│   ├── app1.yml
│   ├── app1.effective.yml
│   ├── app2.yml
│   └── app2.effective.yml
└── thumbnails/       # Container thumbnail images
```

This directory is automatically created during installation.

### Authentication

The application uses Basic Authentication to protect all HTTP endpoints. Credentials are configured in `$HOME/.local/containers/prod.env`:

```bash
ADMIN_USERNAME=admin
ADMIN_PASSWORD=your_secure_password_here
```

**Important**: 
- Set strong passwords in production
- The `prod.env` file is automatically created with chmod 600 permissions
- All HTTP endpoints require authentication
- Use `make setup-auth` to configure credentials interactively

### Working Directory

The service runs from `$HOME/.local/containers` and manages:
- `prod.env` - Environment variables and credentials  
- `stacks/` - Docker Compose stack definitions
- `thumbnails/` - Container thumbnail images

All paths are relative to this base directory.

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

You will be prompted for the username and password configured in `$HOME/.local/containers/prod.env`.

### Managing Stacks

Place your Docker Compose YAML files in `$HOME/.local/containers/stacks/`. The service will automatically detect and manage them.

Example:
```bash
# Create or edit a stack
nano ~/.local/containers/stacks/myapp.yml

# The service will automatically pick it up
# Access via the web interface or API
```

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
composectl-go/
├── main.go           # Application entry point
├── config.go         # Path configuration
├── auth.go           # Basic Authentication middleware
├── server.go         # HTTP server and routing
├── stack.go          # Docker Compose stack management
├── container.go      # Container operations
├── websocket.go      # WebSocket handling
├── watch.go          # File watching
├── thumbnail.go      # Thumbnail generation
├── yaml.go           # YAML processing
└── docs/             # Documentation
```

### Configuration Paths

The application uses these paths (defined in `config.go`):
- `ContainersDir`: `$HOME/.local/containers`
- `StacksDir`: `$HOME/.local/containers/stacks`
- `ProdEnvPath`: `$HOME/.local/containers/prod.env`

## Security Considerations

1. **Change Default Credentials**: Always set strong, unique credentials
2. **File Permissions**: `prod.env` is automatically created with chmod 600
3. **HTTPS**: Consider using a reverse proxy (like Traefik or Nginx) with HTTPS
4. **Firewall**: Restrict access to port 8080 if exposed to the network
5. **Strong Passwords**: Use `openssl rand -base64 24` to generate secure passwords

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

Verify credentials:
```bash
cat ~/.local/containers/prod.env | grep ADMIN
```

Ensure both `ADMIN_USERNAME` and `ADMIN_PASSWORD` are set.

Update credentials:
```bash
make setup-auth
make restart
```

### Service not starting after login

Check if the service is enabled:
```bash
systemctl --user is-enabled composectl.service
```

Enable if needed:
```bash
make enable
```

### Directory not found errors

Ensure the containers directory exists:
```bash
ls -la ~/.local/containers
```

If missing, reinstall:
```bash
make install
```

## Migration from Old Version

If you're upgrading from a version that used local `stacks/` and `prod.env`:

1. **Backup your data**:
   ```bash
   cp -r stacks ~/backup-stacks
   cp prod.env ~/backup-prod.env
   ```

2. **Install new version**:
   ```bash
   make install
   ```

3. **Move your stacks and configuration**:
   ```bash
   cp -r ~/backup-stacks/* ~/.local/containers/stacks/
   cp ~/backup-prod.env ~/.local/containers/prod.env
   chmod 600 ~/.local/containers/prod.env
   ```

4. **Restart the service**:
   ```bash
   make restart
   ```

## API Reference

All endpoints require Basic Authentication:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Web interface |
| `/ws` | GET | WebSocket connection |
| `/api/stacks/` | GET | List all stacks |
| `/api/stacks/{name}` | GET | Get stack details |
| `/api/stacks/{name}` | PUT | Create/update stack |
| `/api/stacks/{name}` | DELETE | Delete stack |
| `/api/stacks/{name}/start` | POST | Start stack |
| `/api/stacks/{name}/stop` | POST | Stop stack |
| `/api/containers/` | GET | List containers |
| `/api/enrich/` | POST | Enrich YAML |
| `/thumbnail/{id}` | GET | Get container thumbnail |

## License

[Add your license information here]

## Contributing

[Add contribution guidelines here]

## Support

For detailed authentication information, see:
- [Basic Auth Guide](docs/BASIC_AUTH_GUIDE.md)
- [Basic Auth Implementation](docs/BASIC_AUTH_IMPLEMENTATION.md)
- [Quick Reference](docs/AUTH_QUICK_REFERENCE.md)

