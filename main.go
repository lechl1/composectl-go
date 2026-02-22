package main

import (
	"log"
	"net/http"
)

func main() {
	go HandleBroadcast()
	go WatchFiles()

	// Wrap all handlers with Basic Auth middleware
	http.HandleFunc("/ws", BasicAuthMiddleware(HandleWebSocket))
	http.HandleFunc("/thumbnail/", BasicAuthMiddleware(HandleThumbnail))
	http.HandleFunc("/api/containers/", BasicAuthMiddleware(handleContainerAPI))
	http.HandleFunc("/api/stacks/", BasicAuthMiddleware(handleStackAPI))
	http.HandleFunc("/api/enrich/", BasicAuthMiddleware(HandleEnrichYAML))
	http.HandleFunc("/", BasicAuthMiddleware(HandleRoot))

	log.Println("Server running on http://localhost:8080")
	log.Println("Basic Authentication enabled - credentials from prod.env (ADMIN_USERNAME, ADMIN_PASSWORD)")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
