# JWT Bearer Token Authentication

This document describes the JWT bearer token authentication implementation in dc-go.

## Overview

dc uses a token-based authentication system:
1. **Login Endpoint** - POST to `/login` with Basic Auth to obtain a JWT token
2. **Bearer Token Authentication** - All other endpoints require JWT bearer tokens

## Features

- **Secure Login Endpoint**: POST to `/login` with Basic Auth credentials to obtain a JWT token
- **Token-Based Authentication**: Use bearer tokens for all API requests
- **In-Memory Session Store**: Active sessions are tracked in memory
- **Automatic Expiration**: Tokens expire after 12 hours of inactivity
- **Session Renewal**: Each authenticated request extends the session by 12 hours
- **Session Cleanup**: Expired sessions are automatically cleaned up every hour
- **Separation of Concerns**: Basic Auth only for login, bearer tokens for all API access

## Usage

### 1. Login to Obtain Token

Send a POST request to `/login` with Basic Auth:

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

### 2. Use Token for API Requests

Include the token in the `Authorization` header:

```bash
curl -X GET http://localhost:8882/api/stacks/ \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

### 3. Token Validation and Renewal

The middleware automatically:
- Checks if the token exists in the session store
- Verifies the token hasn't expired
- Validates the JWT signature
- **Renews the session expiration by 12 hours** on each successful request
- Removes expired sessions on access

Active users never need to re-login as long as they make at least one request every 12 hours.

## Configuration

### Secret Key

The JWT tokens are signed using a `SECRET_KEY`. This key is retrieved using the standard config system with the following priority:

1. Command-line argument: `--secret-key`
2. Environment variable file: `$SECRET_KEY_FILE`
3. Environment variable: `$SECRET_KEY`
4. prod.env file
5. Docker secrets: `/run/secrets/SECRET_KEY`
6. **Auto-generated** if not found (saved to prod.env)

### Token Expiration

Default: **12 hours** (renewable on each request)

Sessions are automatically extended by 12 hours every time a valid request is made. This means:
- Active users stay logged in indefinitely
- Inactive sessions expire after 12 hours

To modify the expiration time, edit in `auth.go`:
```go
expiresAt := time.Now().Add(12 * time.Hour) // Change this duration
```

## Implementation Details

### Session Store

The in-memory session store tracks active tokens:

```go
type SessionStore struct {
    mu       sync.RWMutex
    sessions map[string]*SessionInfo
}

type SessionInfo struct {
    Username  string
    ExpiresAt time.Time
    CreatedAt time.Time
}
```

### JWT Claims

Tokens include standard JWT claims:

```go
type Claims struct {
    Username string
    jwt.RegisteredClaims
}
```

Claims include:
- `username`: The authenticated user
- `exp`: Expiration timestamp
- `iat`: Issued at timestamp
- `iss`: Issuer ("dc")

### Authentication Flow

1. **Login Request** (Basic Auth) → Validates credentials → Generates JWT → Stores session → Returns token
2. **API Request with Bearer Token** → Checks session store → Validates JWT → Renews session → Grants access
3. **API Request without Bearer Token** → Returns 401 Unauthorized

### Middleware

The `BasicAuthMiddleware` now only accepts bearer tokens for protected endpoints:

```go
func BasicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
    // 1. Check for Bearer token (required)
    // 2. Validate and renew session
    // 3. Deny access if token is missing or invalid
}
```

The `/login` endpoint accepts only Basic Auth (not bearer tokens).

## Security Considerations

### Token Security

- Tokens are signed with HMAC-SHA256
- Secret key is auto-generated if not provided (64 characters, URL-safe)
- Constant-time comparison prevents timing attacks
- Sessions are tracked and can be invalidated

### Best Practices

1. **Use HTTPS** in production to protect tokens in transit
2. **Store tokens securely** on the client side
3. **Rotate secret keys** periodically
4. **Monitor session activity** via logs
5. **Set appropriate token expiration** based on your security requirements

### Memory Considerations

- Sessions are stored in memory (lost on restart)
- Expired sessions are cleaned up every hour
- Consider implementing persistent storage for production use

## Testing

Run the included test script:

```bash
./test_login.sh
```

This script will:
1. Attempt to login with credentials from prod.env
2. Verify token is received
3. Test API access with the bearer token
4. Verify invalid credentials are rejected

## API Endpoints

### POST /login

**Authentication:** Basic Auth required

**Request:**
```bash
curl -X POST http://localhost:8882/login -u admin:password
```

**Response (200 OK):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2026-02-23T00:00:00Z"
}
```

**Response (401 Unauthorized):**
```
Invalid credentials
```

### All Other Endpoints

**Authentication Required** - Bearer token only:

```
Authorization: Bearer <token>
```

Basic Auth is **not accepted** on regular endpoints.

## Examples

### JavaScript/Fetch

```javascript
// Login with Basic Auth
const credentials = btoa('admin:password'); // Base64 encode
const loginResponse = await fetch('http://localhost:8882/login', {
  method: 'POST',
  headers: { 'Authorization': `Basic ${credentials}` }
});

const { token } = await loginResponse.json();

// Use token for API requests
const apiResponse = await fetch('http://localhost:8882/api/stacks/', {
  headers: { 'Authorization': `Bearer ${token}` }
});
```

### Python

```python
import requests

# Login with Basic Auth
response = requests.post('http://localhost:8882/login', 
    auth=('admin', 'password'))
token = response.json()['token']

# Use token for API requests
headers = {'Authorization': f'Bearer {token}'}
api_response = requests.get('http://localhost:8882/api/stacks/', headers=headers)
```

### cURL

```bash
# Login with Basic Auth and extract token
TOKEN=$(curl -s -X POST http://localhost:8882/login \
  -u admin:your_password \
  | jq -r '.token')

# Use token for API requests
curl -X GET http://localhost:8882/api/stacks/ \
  -H "Authorization: Bearer $TOKEN"
```

## Troubleshooting

### Token Validation Fails

- **Session expired**: Login again to get a new token
- **Server restarted**: Sessions are in-memory, login again
- **Invalid token**: Verify token wasn't modified

### Can't Login

- **Check credentials**: Verify ADMIN_USERNAME and ADMIN_PASSWORD in prod.env
- **Check logs**: Server logs show authentication attempts

### Server Errors

- **SECRET_KEY generation failed**: Check file permissions for prod.env
- **Can't read prod.env**: Verify file exists and is readable

## Future Enhancements

Potential improvements:
- Persistent session storage (Redis, database)
- Token refresh mechanism
- Token revocation endpoint
- Multiple user support
- Role-based access control
- Session listing and management endpoints

