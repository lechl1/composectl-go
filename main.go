package main

import (
	"fmt"
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

	port := GetPort()
	addr := fmt.Sprintf(":%s", port)

	log.Printf("Server running on http://localhost:%s", port)
	log.Println("Basic Authentication enabled - credentials from prod.env (ADMIN_USERNAME, ADMIN_PASSWORD)")
	log.Fatal(http.ListenAndServe(addr, nil))
}
