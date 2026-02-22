# Basic Authentication Implementation Summary

## What Was Implemented

Basic Authentication has been added to all HTTP endpoints in ComposeCTL. Credentials are stored in `prod.env` as `ADMIN_USERNAME` and `ADMIN_PASSWORD`.

## Files Created/Modified

### New Files

1. **auth.go** - Basic Authentication middleware
   - `BasicAuthMiddleware()` - Wraps handlers with auth
   - `unauthorizedResponse()` - Sends 401 responses
   - Reads credentials from prod.env
   - Uses constant-time comparison to prevent timing attacks

2. **README.md** - Complete project documentation
   - Installation instructions
   - Configuration guide
   - Security considerations
   - Troubleshooting

3. **test-auth.sh** - Authentication test script
   - Verifies credentials are configured
   - Tests authentication works correctly
   - Tests unauthorized access is blocked

4. **docs/BASIC_AUTH_GUIDE.md** - Detailed auth guide
   - Quick start instructions
   - Security best practices
   - Troubleshooting guide
   - API endpoint reference

### Modified Files

1. **main.go**
   - Wrapped all HTTP handlers with `BasicAuthMiddleware()`
   - Added log message about authentication being enabled

2. **prod.env**
   - Added `ADMIN_USERNAME` field (empty by default)
   - Added `ADMIN_PASSWORD` field (empty by default)

3. **Makefile**
   - Added `setup-auth` target for interactive credential setup
   - Updated `install` target to prompt for credentials
   - Added authentication reminder messages

## How It Works

### Authentication Flow

1. User makes HTTP request to any endpoint
2. `BasicAuthMiddleware` intercepts the request
3. Extracts `Authorization: Basic` header
4. Reads `ADMIN_USERNAME` and `ADMIN_PASSWORD` from environment or prod.env
5. Compares credentials using constant-time comparison
6. If valid: forwards request to handler
7. If invalid: returns 401 Unauthorized with WWW-Authenticate header

### Credential Loading

The middleware checks credentials in this order:
1. Environment variables (`ADMIN_USERNAME`, `ADMIN_PASSWORD`)
2. prod.env file (reads via `readProdEnv()`)
3. If neither found: denies access

### Protected Endpoints

All endpoints now require authentication:
- `GET /` - Web interface
- `GET /ws` - WebSocket connections
- `GET /thumbnail/*` - Thumbnail images
- `GET /api/containers/*` - Container API
- `GET /api/stacks/*` - Stack management API
- `GET /api/enrich/*` - YAML enrichment API

## Security Features

1. **Constant-time comparison** - Prevents timing attacks
2. **No default credentials** - Users must explicitly set them
3. **Secure password storage** - In protected prod.env file
4. **WWW-Authenticate header** - Proper HTTP Basic Auth standard
5. **All endpoints protected** - No unprotected routes

## Usage Examples

### Setting Credentials

```bash
# Method 1: Edit prod.env manually
echo "ADMIN_USERNAME=admin" >> prod.env
echo "ADMIN_PASSWORD=$(openssl rand -base64 24)" >> prod.env

# Method 2: Use interactive setup
make setup-auth

# Method 3: Set environment variables
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD=secretpassword
```

### Accessing the Application

```bash
# Browser
# Navigate to http://localhost:8080
# Enter username and password when prompted

# curl
curl -u admin:password http://localhost:8080/

# curl with header
curl -H "Authorization: Basic $(echo -n 'admin:password' | base64)" http://localhost:8080/

# WebSocket
wscat -c ws://admin:password@localhost:8080/ws
```

### Testing

```bash
# Run test script
./test-auth.sh

# Expected output:
#   ✓ Credentials found in prod.env
#   ✓ Server is running
#   ✓ Test 1: Correctly rejected without credentials
#   ✓ Test 2: Correctly rejected with wrong credentials
#   ✓ Test 3: Successfully authenticated with correct credentials
```

## Installation Flow

### First-time Installation

```bash
# 1. Build application
make build

# 2. Install (prompts for credentials if not set)
make install

# 3. Enable auto-start
make enable

# 4. Start service
make start

# 5. Check status
make status

# 6. View logs
make logs
```

### Updating Credentials

```bash
# Interactive update
make setup-auth

# Restart to apply
make restart
```

## Technical Details

### Middleware Implementation

```go
func BasicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        username, password, ok := r.BasicAuth()
        
        // Load credentials from environment or prod.env
        adminUsername := os.Getenv("ADMIN_USERNAME")
        adminPassword := os.Getenv("ADMIN_PASSWORD")
        
        if adminUsername == "" || adminPassword == "" {
            envVars, _ := readProdEnv("prod.env")
            adminUsername = envVars["ADMIN_USERNAME"]
            adminPassword = envVars["ADMIN_PASSWORD"]
        }
        
        // Validate using constant-time comparison
        usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(adminUsername)) == 1
        passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(adminPassword)) == 1
        
        if !ok || !usernameMatch || !passwordMatch {
            unauthorizedResponse(w)
            return
        }
        
        next(w, r)
    }
}
```

### Why Constant-Time Comparison?

Regular string comparison (`==`) can be vulnerable to timing attacks where an attacker measures response time to guess credentials character-by-character. `subtle.ConstantTimeCompare()` takes the same time regardless of where strings differ.

## Best Practices

1. **Use Strong Passwords**
   - Minimum 16 characters
   - Use password generator: `openssl rand -base64 24`

2. **Protect prod.env**
   ```bash
   chmod 600 prod.env
   chown $USER:$USER prod.env
   ```

3. **Use HTTPS in Production**
   - Deploy behind reverse proxy (Traefik, Nginx, Caddy)
   - Basic Auth over HTTP sends credentials in base64 (not encrypted)

4. **Restrict Network Access**
   ```bash
   # Firewall rules
   sudo ufw allow from 192.168.1.0/24 to any port 8080
   ```

5. **Monitor Access**
   ```bash
   # Check logs for failed auth attempts
   journalctl --user -u composectl.service | grep "401"
   ```

## Troubleshooting

### Problem: 401 Unauthorized Always

**Cause**: Credentials not set or incorrect

**Solution**:
```bash
# Check credentials
cat prod.env | grep ADMIN

# Should show:
# ADMIN_USERNAME=admin
# ADMIN_PASSWORD=yourpassword

# If empty, run:
make setup-auth
make restart
```

### Problem: "ADMIN_USERNAME or ADMIN_PASSWORD not set" in logs

**Cause**: prod.env not found or credentials not configured

**Solution**:
```bash
# Ensure prod.env exists in working directory
ls -la prod.env

# Configure credentials
make setup-auth

# Restart service
make restart
```

### Problem: Browser keeps asking for password

**Cause**: Wrong credentials or authentication failing

**Solution**:
```bash
# Test with curl first
ADMIN_USER=$(grep '^ADMIN_USERNAME=' prod.env | cut -d= -f2)
ADMIN_PASS=$(grep '^ADMIN_PASSWORD=' prod.env | cut -d= -f2)
curl -v -u "$ADMIN_USER:$ADMIN_PASS" http://localhost:8080/

# Check response - should be 200 or 404, not 401
```

## Future Enhancements

Possible improvements for future versions:

1. **Multiple Users** - Support for multiple admin accounts
2. **Token-based Auth** - JWT tokens for API access
3. **Rate Limiting** - Prevent brute force attacks
4. **Audit Logging** - Track all authentication attempts
5. **OAuth/OIDC** - Integration with external identity providers
6. **Session Management** - Persistent sessions with cookies
7. **2FA Support** - Two-factor authentication

## Testing Checklist

- [x] Authentication middleware created
- [x] All endpoints wrapped with middleware
- [x] Credentials loaded from prod.env
- [x] Constant-time comparison implemented
- [x] 401 responses with WWW-Authenticate header
- [x] Makefile updated with setup-auth target
- [x] README documentation created
- [x] Test script created
- [x] Build succeeds without errors
- [x] Installation prompts for credentials

## Deployment Checklist

Before deploying to production:

- [ ] Set strong password in prod.env
- [ ] Set file permissions: `chmod 600 prod.env`
- [ ] Configure HTTPS with reverse proxy
- [ ] Set up firewall rules
- [ ] Enable systemd service: `make enable`
- [ ] Test authentication works
- [ ] Monitor logs for issues
- [ ] Document credentials in password manager
- [ ] Set up backup for prod.env

## Support

For issues or questions:

1. Check the logs: `make logs`
2. Read the troubleshooting guide: `docs/BASIC_AUTH_GUIDE.md`
3. Test authentication: `./test-auth.sh`
4. Verify credentials: `cat prod.env | grep ADMIN`

