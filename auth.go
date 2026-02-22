package main

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
)

// BasicAuthMiddleware wraps an http.HandlerFunc with Basic Authentication
func BasicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()

		// Get credentials from environment/prod.env
		adminUsername := os.Getenv("ADMIN_USERNAME")
		adminPassword := os.Getenv("ADMIN_PASSWORD")

		// If credentials are not set, load from prod.env
		if adminUsername == "" || adminPassword == "" {
			envVars, err := readProdEnv(ProdEnvPath)
			if err != nil {
				log.Printf("Warning: Failed to read prod.env for authentication: %v", err)
			} else {
				if adminUsername == "" {
					adminUsername = envVars["ADMIN_USERNAME"]
				}
				if adminPassword == "" {
					adminPassword = envVars["ADMIN_PASSWORD"]
				}
			}
		}

		// If still no credentials, deny access
		if adminUsername == "" || adminPassword == "" {
			log.Printf("Warning: ADMIN_USERNAME or ADMIN_PASSWORD not set in prod.env")
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
