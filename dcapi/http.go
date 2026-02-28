package main

import (
	"net/http"
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
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	path = strings.TrimPrefix(path, "/stacks")
	// bare GET /api/stacks → list all stacks
	if path == "" || path == "/" {
		if r.Method == http.MethodGet {
			HandleAction(w, "dc", "ls")
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) < 1 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	stackName := segments[0]
	actionName := segments[1]
	switch actionName {
	case "stop", "start", "up", "down", "create", "view":
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			HandleAction(w, "dc", actionName, stackName)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "logs", "list", "ls":
		if r.Method == http.MethodGet {
			HandleAction(w, "dc", actionName, stackName)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func HandleAction(w http.ResponseWriter, args ...string) {
	if len(args) < 2 {
		http.Error(w, "invalid action args", http.StatusInternalServerError)
		return
	}
	binary := args[0]
	cmdArgs := []string{"stacks", args[1]}
	for _, a := range args[2:] {
		if a != "" {
			cmdArgs = append(cmdArgs, a)
		}
	}
	cmd := exec.Command(binary, cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, string(out), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}
