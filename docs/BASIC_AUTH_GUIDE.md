# Quick Start Guide - Basic Authentication

## Overview

ComposeCTL now requires Basic Authentication for all HTTP endpoints. This protects your Docker Compose management interface from unauthorized access.

## Initial Setup

### 1. Configure Credentials

Edit the `prod.env` file and set your admin credentials:

```bash
ADMIN_USERNAME=admin
ADMIN_PASSWORD=your_secure_password_here
```

**Or** use the interactive setup:

```bash
make setup-auth
```

### 2. Build and Run

```bash
# Build the application
go build -o composectl

# Run directly
./composectl

# Or install as a systemd service
make install
make enable
make start
```

## Usage

### Accessing the Web Interface

1. Open your browser and navigate to: `http://localhost:8080`
2. You will be prompted for username and password
3. Enter the credentials from `prod.env`

### Using curl

```bash
# With authentication
curl -u admin:password http://localhost:8080/api/stacks/

# Or with explicit header
curl -H "Authorization: Basic $(echo -n 'admin:password' | base64)" http://localhost:8080/
```

### WebSocket Connections

WebSocket connections also require Basic Auth. Include credentials in the connection URL:

```javascript
// JavaScript example
const ws = new WebSocket('ws://admin:password@localhost:8080/ws');
```

## Security Best Practices

### 1. Strong Passwords

Use a strong, unique password for `ADMIN_PASSWORD`:

```bash
# Generate a random password
openssl rand -base64 24
```

### 2. Protect prod.env

Restrict file permissions:

```bash
chmod 600 prod.env
```

### 3. Use HTTPS

In production, use a reverse proxy with HTTPS (Traefik, Nginx, Caddy):

```yaml
# Example Traefik configuration
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.composectl.rule=Host(`composectl.example.com`)"
  - "traefik.http.routers.composectl.entrypoints=websecure"
  - "traefik.http.routers.composectl.tls.certresolver=letsencrypt"
```

### 4. Network Access Control

Restrict access to localhost or specific IPs using firewall rules:

```bash
# Allow only localhost
sudo ufw allow from 127.0.0.1 to any port 8080

# Allow specific IP
sudo ufw allow from 192.168.1.100 to any port 8080
```

## Troubleshooting

### "401 Unauthorized" Error

**Problem**: Cannot access the application, always getting 401 error.

**Solution**:
1. Check credentials in `prod.env`:
   ```bash
   cat prod.env | grep ADMIN
   ```
2. Ensure both `ADMIN_USERNAME` and `ADMIN_PASSWORD` are set
3. Restart the service after changing credentials:
   ```bash
   make restart
   ```

### Credentials Not Working

**Problem**: Using correct credentials but still getting 401.

**Solution**:
1. Check for whitespace or special characters:
   ```bash
   # Username and password should not have leading/trailing spaces
   ADMIN_USERNAME=admin    # ❌ Wrong (trailing spaces)
   ADMIN_USERNAME=admin    # ✓ Correct
   ```
2. Verify the service is reading the correct `prod.env`:
   ```bash
   make logs
   # Look for: "Basic Authentication enabled"
   ```

### Browser Keeps Asking for Password

**Problem**: Browser keeps showing authentication popup even with correct credentials.

**Solution**:
1. This usually means authentication is failing
2. Check browser console for errors
3. Clear browser cache and cookies
4. Try using curl to verify credentials work:
   ```bash
   curl -u admin:password http://localhost:8080/
   ```

## Testing Authentication

Use the provided test script:

```bash
./test-auth.sh
```

This will verify:
- ✓ Credentials are configured in prod.env
- ✓ Server is running
- ✓ Authentication works correctly
- ✓ Unauthorized access is blocked

## Updating Credentials

### Method 1: Edit prod.env Directly

```bash
nano prod.env
# Update ADMIN_USERNAME and ADMIN_PASSWORD
# Save and exit

# Restart service
make restart
```

### Method 2: Interactive Setup

```bash
make setup-auth
# Follow the prompts
# Restart service
make restart
```

## Advanced Configuration

### Environment Variables

Credentials can also be set via environment variables (overrides prod.env):

```bash
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD=secret
./composectl
```

### Systemd Service with Environment

Edit the service file to include environment variables:

```bash
# Edit ~/.config/systemd/user/composectl.service
[Service]
Environment="ADMIN_USERNAME=admin"
Environment="ADMIN_PASSWORD=secret"

# Reload and restart
systemctl --user daemon-reload
make restart
```

## API Endpoints

All endpoints now require Basic Authentication:

| Endpoint | Description |
|----------|-------------|
| `GET /` | Web interface (requires auth) |
| `GET /ws` | WebSocket connection (requires auth) |
| `GET /api/stacks/` | Stack management API (requires auth) |
| `GET /api/containers/` | Container API (requires auth) |
| `GET /api/enrich/` | YAML enrichment API (requires auth) |
| `GET /thumbnail/` | Thumbnail images (requires auth) |

## Next Steps

- [Main README](README.md) - Full documentation
- [Makefile Reference](Makefile) - All available commands
- Configure HTTPS with reverse proxy
- Set up firewall rules
- Enable automatic updates

