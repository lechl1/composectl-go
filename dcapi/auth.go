package main

import (
	"crypto/subtle"
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

// getAdminCredentials retrieves admin credentials using the unified config system
func getAdminCredentials(args []string) (username, password string) {
	username = GetAdminUsername(args)
	password = GetAdminPassword(args)
	return username, password
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents the login response payload
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// HandleLogin handles the /login endpoint - accepts Basic Auth only
func HandleLogin(w http.ResponseWriter, r *http.Request) {
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
	adminUsername, adminPassword, err := GetAdminCredentials(os.Args)
	if err != nil {
		log.Printf("Error retrieving admin credentials: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
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

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Logged out successfully")
}

// validateBearerToken validates a JWT bearer token and renews the session
func validateBearerToken(tokenString string) (*Claims, error) {
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

// BasicAuthMiddleware wraps an http.HandlerFunc with Bearer Token authentication only
func BasicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept Bearer token (no Basic Auth fallback)
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			log.Printf("Missing or invalid Authorization header")
			w.Header().Set("WWW-Authenticate", `Bearer realm="dc - Restricted Access"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("401 Unauthorized - Bearer token required\n"))
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate bearer token (also renews session)
		_, err := validateBearerToken(tokenString)
		if err != nil {
			log.Printf("Bearer token validation failed: %v", err)
			w.Header().Set("WWW-Authenticate", `Bearer realm="dc - Restricted Access"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("401 Unauthorized - Invalid or expired token\n"))
			return
		}
		next(w, r)
	}
}

// unauthorizedResponse sends a 401 Unauthorized response with WWW-Authenticate header
func unauthorizedResponse(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="dc - Restricted Access"`)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("401 Unauthorized\n"))
}

func SessionCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Cleaning up expired sessions...")
		sessionStore.CleanupExpiredSessions()
	}
}
