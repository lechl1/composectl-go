package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
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

	port := GetPort(os.Args)
	addr := GetAddr(os.Args)
	listenAddr := fmt.Sprintf("%s:%s", addr, port)

	log.Printf("Server running on http://%s:%s", addr, port)
	log.Println("Basic Authentication enabled - credentials from prod.env (ADMIN_USERNAME, ADMIN_PASSWORD)")
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
