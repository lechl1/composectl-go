# JWT Authentication - Quick Reference

## Summary

✅ **Implemented:**
- `/login` endpoint accepts **Basic Auth only** to obtain JWT bearer tokens
- All other endpoints accept **Bearer Token only** (no Basic Auth)
- Token signing using SECRET_KEY from getConfig()
- **12-hour token expiration** (renewed on each request)
- In-memory session store tracking active logins
- **Automatic session renewal** - each authenticated request extends expiration by 12 hours
- Automatic session cleanup (hourly)

## Quick Start

### 1. Login (Basic Auth)
```bash
curl -X POST http://localhost:8882/login \
  -u admin:your_password
```

Response:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2026-02-23T00:00:00Z"
}
```

### 2. Use Token (Bearer Token Required)
```bash
curl http://localhost:8882/api/stacks/ \
  -H "Authorization: Bearer <token>"
```

**Note:** Each authenticated request automatically renews the session for another 12 hours.

## Implementation Details

### Files Modified

1. **config.go**
   - Added `GetSecretKey()` function
   - Added `generateAndSaveSecretKey()` function
   - Auto-generates 64-character URL-safe secret key if not found

2. **auth.go**
   - Added JWT token generation and validation
   - Added `SessionStore` for tracking active sessions
   - Added `HandleLogin()` endpoint handler
   - Extended `BasicAuthMiddleware()` to support bearer tokens
   - Added `StartSessionCleanup()` for periodic cleanup

3. **main.go**
   - Registered `/login` endpoint (public, no auth)
   - Started session cleanup goroutine
   - Updated logging to mention JWT support

### Key Components

**Session Store:**
```go
type SessionStore struct {
    sessions map[string]*SessionInfo  // token → session info
}

type SessionInfo struct {
    Username  string
    ExpiresAt time.Time
    CreatedAt time.Time
}
```

**JWT Claims:**
```go
type Claims struct {
    Username string
    jwt.RegisteredClaims  // exp, iat, iss
}
```

**Authentication Flow:**
1. `/login` endpoint: Accept Basic Auth → validate credentials → return JWT token
2. All other endpoints: Require Bearer token → validate JWT + check session → renew session → grant access
3. If Bearer token missing/invalid → return 401 Unauthorized

**Session Renewal:**
- Each successful authentication extends the session expiration by 12 hours from the current time
- Active users never need to re-login as long as they make requests within 12 hours
- Inactive sessions expire after 12 hours of no activity

## Security Features

- ✅ HMAC-SHA256 token signing
- ✅ Constant-time credential comparison
- ✅ Session tracking prevents token reuse after expiration
- ✅ Auto-generated cryptographically secure secret key
- ✅ Token expiration (12 hours, renewable)
- ✅ Automatic session renewal on each request
- ✅ Automatic cleanup of expired sessions
- ✅ Separation of concerns: Basic Auth only for login, Bearer token for API access

## Testing

Run the test script:
```bash
./test_login.sh
```

Manual testing:
```bash
# Get credentials from prod.env
PASSWORD=$(grep ADMIN_PASSWORD prod.env | cut -d'=' -f2)

# Login with Basic Auth
TOKEN=$(curl -s -X POST http://localhost:8882/login \
  -u "admin:$PASSWORD" \
  | jq -r '.token')

# Test API with Bearer token
curl http://localhost:8882/api/stacks/ \
  -H "Authorization: Bearer $TOKEN"
```

## Configuration

**Secret Key Priority:**
1. `--secret-key` command-line arg
2. `$SECRET_KEY_FILE` env var
3. `$SECRET_KEY` env var
4. `prod.env` file
5. `/run/secrets/SECRET_KEY` (Docker)
6. **Auto-generated** (saved to prod.env)

## Dependencies Added

```
github.com/golang-jwt/jwt/v5 v5.3.1
```

## API Endpoints

| Endpoint | Auth Required | Method | Description |
|----------|--------------|--------|-------------|
| `/login` | Basic Auth | POST | Obtain JWT token |
| All others | Bearer Token | Various | API access with JWT token |

## Notes

- Sessions are **in-memory** (lost on server restart)
- Expired sessions cleaned up every **1 hour**
- Token expiration: **12 hours** (renewable on each request)
- **No Basic Auth** on regular endpoints - only Bearer tokens accepted
- `/login` endpoint **only accepts Basic Auth** (not Bearer tokens)

## Future Enhancements

Consider implementing:
- Persistent session storage (Redis/DB)
- Token refresh mechanism  
- Logout/revocation endpoint
- Session management API
- Role-based access control

