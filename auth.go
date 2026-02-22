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

// getAdminCredentials retrieves admin credentials with the following priority:
// 1. Check ADMIN_USERNAME_FILE and ADMIN_PASSWORD_FILE env vars (Docker secrets pattern)
// 2. Check default Docker secrets location (/run/secrets/ADMIN_USERNAME and /run/secrets/ADMIN_PASSWORD)
// 3. Check ADMIN_USERNAME and ADMIN_PASSWORD env vars
// 4. Check prod.env file
func getAdminCredentials() (username, password string) {
	// Try to read from files specified in *_FILE env vars
	if usernameFile := os.Getenv("ADMIN_USERNAME_FILE"); usernameFile != "" {
		if content, err := readSecretFile(usernameFile); err == nil {
			username = content
			log.Printf("Loaded ADMIN_USERNAME from file: %s", usernameFile)
		} else {
			log.Printf("Warning: Failed to read ADMIN_USERNAME_FILE (%s): %v", usernameFile, err)
		}
	}

	if passwordFile := os.Getenv("ADMIN_PASSWORD_FILE"); passwordFile != "" {
		if content, err := readSecretFile(passwordFile); err == nil {
			password = content
			log.Printf("Loaded ADMIN_PASSWORD from file: %s", passwordFile)
		} else {
			log.Printf("Warning: Failed to read ADMIN_PASSWORD_FILE (%s): %v", passwordFile, err)
		}
	}

	// If not found, try default Docker secrets location
	if username == "" {
		if content, err := readSecretFile("/run/secrets/ADMIN_USERNAME"); err == nil {
			username = content
			log.Printf("Loaded ADMIN_USERNAME from default Docker secrets location")
		}
	}

	if password == "" {
		if content, err := readSecretFile("/run/secrets/ADMIN_PASSWORD"); err == nil {
			password = content
			log.Printf("Loaded ADMIN_PASSWORD from default Docker secrets location")
		}
	}

	// Fall back to direct environment variables
	if username == "" {
		username = os.Getenv("ADMIN_USERNAME")
	}
	if password == "" {
		password = os.Getenv("ADMIN_PASSWORD")
	}

	// If credentials are still not set, load from prod.env
	if username == "" || password == "" {
		envVars, err := readProdEnv(ProdEnvPath)
		if err != nil {
			log.Printf("Warning: Failed to read prod.env for authentication: %v", err)
		} else {
			if username == "" {
				username = envVars["ADMIN_USERNAME"]
			}
			if password == "" {
				password = envVars["ADMIN_PASSWORD"]
			}
		}
	}

	return username, password
}

// BasicAuthMiddleware wraps an http.HandlerFunc with Basic Authentication
func BasicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()

		// Get credentials using the priority-based lookup
		adminUsername, adminPassword := getAdminCredentials()

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
