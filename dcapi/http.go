package main

import (
	"fmt"
	"net/http"
	"strings"
)

func RegisterHTTPHandlers() {
	http.HandleFunc("/api/auth/login", HandleLogin)
	http.HandleFunc("/api/auth/logout", JwtAuthMiddleware(HandleLogout))
	http.HandleFunc("/ws", JwtAuthMiddleware(HandleWebSocket))
	http.HandleFunc("/api/thumbnail", JwtAuthMiddleware(HandleThumbnail))
	http.HandleFunc("/api/stacks", JwtAuthMiddleware(HandleStackAPI))
}

// handleStackAPI routes stack API requests to appropriate handlers
func HandleStackAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	path = strings.TrimSuffix(path, "/api")

	if strings.HasPrefix("/auth", path) {
		path = strings.TrimPrefix(path, "/auth")
		segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
		stackName := segments[0]
		actionName := segments[1]
		switch actionName {
		case "stop", "start", "up", "down", "create", "view":
			if r.Method == http.MethodPost || r.Method == http.MethodPut {
				HandleAction(w, r, "dc", actionName, stackName)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case "logs", "list", "ls":
			if r.Method == http.MethodGet {
				HandleAction(w, r, "dc", actionName, stackName)
			}
		}
	} else if strings.HasPrefix("/stacks", path) {
		path = strings.TrimPrefix(path, "/stacks")
		segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
		stackName := segments[0]
		actionName := segments[1]
		switch actionName {
		case "stop", "start", "up", "down", "create", "view":
			if r.Method == http.MethodPost || r.Method == http.MethodPut {
				HandleAction(w, r, "dc", actionName, stackName)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case "logs", "list", "ls":
			if r.Method == http.MethodGet {
				HandleAction(w, r, "dc", actionName, stackName)
			}
		}
	}
}

func HandleAction(w http.ResponseWriter, r *http.Request, args ...string) {
	c := fmt.Sprintln(args)
	// Execute the command and handle the response
	// This is a placeholder for the actual implementation
	w.Write([]byte(fmt.Sprintf("Executed: %s", c)))
}
