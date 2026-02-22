package main

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
)

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

// BasicAuthMiddleware wraps an http.HandlerFunc with Basic Authentication
func BasicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()

		// Get credentials using GetAdminCredentials which ensures they exist
		adminUsername, adminPassword, err := GetAdminCredentials(os.Args)
		if err != nil {
			log.Printf("Error retrieving admin credentials: %v", err)
			unauthorizedResponse(w)
			return
		}

		// If still no credentials, deny access
		if adminUsername == "" || adminPassword == "" {
			log.Printf("Warning: ADMIN_USERNAME or ADMIN_PASSWORD not found in any source")
			unauthorizedResponse(w)
			return
		}

		// Validate credentials using constant-time comparison to prevent timing attacks
		usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(adminUsername)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(adminPassword)) == 1

		if !ok || !usernameMatch || !passwordMatch {
			unauthorizedResponse(w)
			return
		}

		// Authentication successful, call the next handler
		next(w, r)
	}
}

// unauthorizedResponse sends a 401 Unauthorized response with WWW-Authenticate header
func unauthorizedResponse(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="ComposeCTL - Restricted Access"`)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("401 Unauthorized\n"))
}
