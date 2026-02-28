package main

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
)

func RegisterHTTPHandlers() {
	http.HandleFunc("/api/auth/login", HandleLogin)
	http.HandleFunc("/api/auth/logout", JwtAuthMiddleware(HandleLogout))
	http.HandleFunc("/api/auth/status", JwtAuthMiddleware(HandleAuthStatus))
	http.HandleFunc("/ws", JwtAuthMiddleware(HandleWebSocket))
	http.HandleFunc("/api/thumbnail", JwtAuthMiddleware(HandleThumbnail))
	http.HandleFunc("/api/stacks", JwtAuthMiddleware(HandleStackAPI))
	http.HandleFunc("/api/stacks/", JwtAuthMiddleware(HandleStackAPI))
}

// HandleStackAPI routes stack API requests to appropriate handlers
func HandleStackAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/api")

	if !strings.HasPrefix(path, "/stacks") {
		http.Error(w, "Not found "+path, http.StatusNotFound)
		return
	}

	path = strings.TrimPrefix(path, "/stacks")
	path = strings.TrimPrefix(path, "/")
	segments := []string{}
	if strings.Contains(path, "/") {
		segments = strings.Split(path, "/")
	} else if path != "" {
		segments = []string{path}
	}

	if len(segments) == 2 {
		stackName := segments[0]
		actionName := segments[1]
		switch actionName {
		case "stop", "start", "up", "down", "create":
			if r.Method == http.MethodPost || r.Method == http.MethodPut {
				HandleAction(w, "dc", "stack", actionName, stackName)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case "logs":
			if r.Method == http.MethodGet {
				HandleAction(w, "dc", "stack", actionName, stackName)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case "rm", "remove", "del", "delete":
			if r.Method == http.MethodDelete {
				HandleAction(w, "dc", "stack", "rm", stackName)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case "view":
			if r.Method == http.MethodGet {
				HandleAction(w, "dc", "stack", "view", segments[0])
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			http.Error(w, "Not found "+path, http.StatusNotFound)
		}
	} else if len(segments) == 1 {
		if r.Method == http.MethodGet {
			HandleAction(w, "dc", "stack", "view", segments[0])
		} else if r.Method == http.MethodDelete {
			HandleAction(w, "dc", "stack", "rm", segments[0])
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	} else if len(segments) == 0 {
		if r.Method == http.MethodGet {
			HandleAction(w, "dc", "stack", "ls")
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	} else {
		http.Error(w, "Not found "+path, http.StatusNotFound)
	}
}

func HandleAction(w http.ResponseWriter, c string, args ...string) {
	cmd := exec.Command(c, args...)
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, string(out), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write(out)
}
