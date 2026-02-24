package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	// Initialize paths first (respects --stacks-dir and --env-path arguments)
	InitPaths(os.Args)

	// Ensure admin credentials exist before starting server
	username, _, err := GetAdminCredentials(os.Args)
	if err != nil {
		log.Fatalf("Failed to initialize admin credentials: %v", err)
	}
	log.Printf("Authentication configured for user: %s", username)

	go SessionCleanup()
	go HandleBroadcast()
	// go WatchFiles()

	// Public endpoint (no auth required)
	http.HandleFunc("/api/auth/login", HandleLogin)
	http.HandleFunc("/api/auth/logout", HandleLogout)

	// Wrap all handlers with Basic Auth middleware (supports both Basic Auth and Bearer tokens)
	http.HandleFunc("/ws", BasicAuthMiddleware(HandleWebSocket))
	http.HandleFunc("/thumbnail/", BasicAuthMiddleware(HandleThumbnail))
	http.HandleFunc("/api/containers/", BasicAuthMiddleware(handleContainerAPI))
	http.HandleFunc("/api/stacks/", BasicAuthMiddleware(handleStackAPI))
	http.HandleFunc("/", HandleUI)

	port := GetPort(os.Args)
	addr := GetAddr(os.Args)
	listenAddr := fmt.Sprintf("%s:%s", addr, port)

	log.Printf("Server running on http://%s:%s", addr, port)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
