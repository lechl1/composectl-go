package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SessionStore holds active sessions in memory
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*SessionInfo
}

// SessionInfo contains information about an active session
type SessionInfo struct {
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// Claims represents JWT claims
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Global session store
var sessionStore = &SessionStore{
	sessions: make(map[string]*SessionInfo),
}

// AddSession adds a new session to the store
func (s *SessionStore) AddSession(token string, info *SessionInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = info
}

// GetSession retrieves a session from the store
func (s *SessionStore) GetSession(token string) (*SessionInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, exists := s.sessions[token]
	return info, exists
}

// RemoveSession removes a session from the store
func (s *SessionStore) RemoveSession(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// RenewSession extends the expiration time of an existing session
func (s *SessionStore) RenewSession(token string, newExpiresAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if info, exists := s.sessions[token]; exists {
		info.ExpiresAt = newExpiresAt
	}
}

// CleanupExpiredSessions removes expired sessions from the store
func (s *SessionStore) CleanupExpiredSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for token, info := range s.sessions {
		if now.After(info.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
}

// readSecretFile reads a secret from a file and returns its trimmed content
func readSecretFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Trim whitespace and newlines from the secret
	return strings.TrimSpace(string(content)), nil
}

// isAuthDisabled returns true when authentication is explicitly disabled via env/flags
func isAuthDisabled() bool {
	val := strings.ToLower(getConfig("auth_disabled", "false"))
	switch val {
	case "true":
		return true
	default:
		return false
	}
}

// HandleLogin handles the /login endpoint - accepts Basic Auth only
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	// If auth is disabled globally, return a static token and create a long-lived session
	if isAuthDisabled() {
		log.Println("AUTH_DISABLED is set; skipping login authentication")
		// Use a fixed token value so clients can send any token or this one. Store it so middleware can find it.
		const disabledToken = "AUTH_DISABLED"
		expiresAt := time.Now().Add(100 * 365 * 24 * time.Hour) // very long lived
		sessionStore.AddSession(disabledToken, &SessionInfo{
			Username:  getConfig("admin_username", "admin"),
			ExpiresAt: expiresAt,
			CreatedAt: time.Now(),
		})
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, disabledToken)
		return
	}

	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get credentials from Basic Auth header
	username, password, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="dc - Login"`)
		http.Error(w, "Basic authentication required", http.StatusUnauthorized)
		return
	}

	// Get admin credentials
	adminUsername := getConfig("admin_username", "admin")
	if username == "" {
		fmt.Println("Warning: admin_username not set. Using default 'admin'")
	}
	adminPassword := getConfig("admin_password", "Admin_123")
	if password == "" {
		fmt.Fprintln(os.Stderr, "Warning: admin_password not set. Using default 'Admin_123'")
	}

	// Validate credentials using constant-time comparison
	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(adminUsername)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(adminPassword)) == 1

	if !usernameMatch || !passwordMatch {
		w.Header().Set("WWW-Authenticate", `Basic realm="dc - Login"`)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate JWT token
	secretKey := GetSecretKey(os.Args)
	expiresAt := time.Now().Add(12 * time.Hour) // Token valid for 12 hours

	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "dc",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secretKey))
	if err != nil {
		log.Printf("Error signing token: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Store session in memory
	sessionStore.AddSession(tokenString, &SessionInfo{
		Username:  username,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	})
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, tokenString)
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	// If auth is disabled, just return success (noop)
	if isAuthDisabled() {
		log.Println("AUTH_DISABLED is set; skipping logout")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	// Expect "Bearer <token>"
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		http.Error(w, "Invalid authorization format", http.StatusUnauthorized)
		return
	}

	tokenString := strings.TrimPrefix(authHeader, prefix)

	// Optional: Validate token signature before removing
	secretKey := GetSecretKey(os.Args)
	_, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secretKey), nil
	})
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Remove session
	sessionStore.RemoveSession(tokenString)
	w.WriteHeader(http.StatusNoContent)
}

// validateBearerToken validates a JWT bearer token and renews the session
func validateBearerToken(tokenString string) (*Claims, error) {
	// If auth disabled, allow all requests and return a synthetic claim
	if isAuthDisabled() {
		return &Claims{
			Username: getConfig("admin_username", "admin"),
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(100 * 365 * 24 * time.Hour)),
			},
		}, nil
	}

	// Check if session exists and is not expired
	sessionInfo, exists := sessionStore.GetSession(tokenString)
	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	if time.Now().After(sessionInfo.ExpiresAt) {
		sessionStore.RemoveSession(tokenString)
		return nil, fmt.Errorf("session expired")
	}

	// Parse and validate JWT token
	secretKey := GetSecretKey(os.Args)
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Renew session - extend expiration by 12 hours from now
	newExpiresAt := time.Now().Add(12 * time.Hour)
	sessionStore.RenewSession(tokenString, newExpiresAt)
	return claims, nil
}

func JwtAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If auth is disabled, skip auth checks entirely
		if isAuthDisabled() {
			// Allow the request through
			next(w, r)
			return
		}

		// Only accept Bearer token (no Basic Auth fallback)
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			log.Printf("Missing or invalid Authorization header")
			w.Header().Set("WWW-Authenticate", `Bearer realm="dcapi"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("401 Unauthorized\n"))
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate bearer token (also renews session)
		_, err := validateBearerToken(tokenString)
		if err != nil {
			log.Printf("Bearer token validation failed: %v", err)
			w.Header().Set("WWW-Authenticate", `Bearer realm="dcapi"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("401 Unauthorized\n"))
			return
		}
		next(w, r)
	}
}

func SessionCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Cleaning up expired sessions...")
		sessionStore.CleanupExpiredSessions()
	}
}

func getConfig(key string, defaultValue string) string {
	keyLower := strings.ToLower(key)
	keyUpper := strings.ToUpper(key)
	// Create title case manually (first char upper, rest lower)
	keyTitle := ""
	if len(keyLower) > 0 {
		keyTitle = strings.ToUpper(string(keyLower[0])) + keyLower[1:]
	}

	// Check program arguments first
	args := os.Args[1:] // Skip program name
	for i, arg := range args {
		// Replace underscores with dashes for command-line flag names
		keyFlag := strings.ReplaceAll(keyLower, "_", "-")
		argFlag := "-" + keyFlag
		argFlagDouble := "--" + keyFlag

		if (arg == argFlag || arg == argFlagDouble) && i+1 < len(args) {
			log.Printf("Loaded %s from program arguments: %s", keyUpper, args[i+1])
			return args[i+1]
		}
		// Handle --key=value format
		if strings.HasPrefix(arg, argFlagDouble+"=") {
			value := strings.TrimPrefix(arg, argFlagDouble+"=")
			log.Printf("Loaded %s from program arguments: %s", keyUpper, value)
			return value
		}
		if strings.HasPrefix(arg, argFlag+"=") {
			value := strings.TrimPrefix(arg, argFlag+"=")
			log.Printf("Loaded %s from program arguments: %s", keyUpper, value)
			return value
		}
	}

	// Try to read from file specified in KEY_FILE env var
	fileEnvVar := keyUpper + "_FILE"
	if configFile := os.Getenv(fileEnvVar); configFile != "" {
		if content, err := readSecretFile(configFile); err == nil {
			log.Printf("Loaded %s from file: %s", keyUpper, configFile)
			return content
		} else {
			log.Printf("Warning: Failed to read %s (%s): %v", fileEnvVar, configFile, err)
		}
	}

	// Check direct environment variable
	if value := os.Getenv(keyUpper); value != "" {
		return value
	}

	// Try default Docker secrets location (case insensitive)
	secretPaths := []string{
		"/run/secrets/" + keyUpper,
		"/run/secrets/" + keyLower,
		"/run/secrets/" + keyTitle,
	}
	for _, secretPath := range secretPaths {
		if content, err := readSecretFile(secretPath); err == nil {
			log.Printf("Loaded %s from Docker secrets: %s", keyUpper, secretPath)
			return content
		}
	}

	// Return default value
	return defaultValue
}

// GetSecretKey retrieves the SECRET_KEY configuration with the following priority:
// 1. Check program arguments for -secret-key or --secret-key flag
// 2. Check SECRET_KEY_FILE env var (Docker secrets pattern)
// 3. Check SECRET_KEY env var
// 4. Check prod.env file (case insensitive)
// 5. Check default Docker secrets location (/run/secrets/SECRET_KEY - case insensitive)
// 6. Generate and save a new random secret key to prod.env
func GetSecretKey(args []string) string {
	secretKey := getConfig("secret_key", "")

	// If no secret key found, generate one and save it to prod.env
	if secretKey == "" {
		var err error
		secretKey, err = generateAndSaveSecretKey()
		if err != nil {
			log.Fatalf("Failed to generate secret key: %v", err)
		}
	}

	return secretKey
}

// generateAndSaveSecretKey generates a new random secret key and saves it to prod.env
func generateAndSaveSecretKey() (string, error) {
	secretKey := getConfig("auth_secret_key", "")
	if secretKey != "" {
		return secretKey, nil
	}
	// Generate a 64-character random secret key
	secretKey, err := generateURLSafePassword(64)
	if err != nil {
		return "", fmt.Errorf("failed to generate secret key: %w", err)
	}

	fmt.Errorf("Using generated secret key. %s", secretKey)
	return secretKey, nil
}

// generateURLSafePassword generates a cryptographically secure URL-safe random password
func generateURLSafePassword(length int) (string, error) {
	// Generate random bytes (we need more bytes than the final length due to base64 encoding)
	numBytes := (length * 6) / 8
	if numBytes < length {
		numBytes = length
	}

	randomBytes := make([]byte, numBytes)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64 URL-safe format and remove padding
	password := base64.URLEncoding.EncodeToString(randomBytes)
	password = strings.TrimRight(password, "=")

	// Truncate to desired length
	if len(password) > length {
		password = password[:length]
	}

	return password, nil
}

func HandleAuthStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK) // If we got here, the token is valid
}
