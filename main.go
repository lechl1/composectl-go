package main

import (
	"log"
	"net/http"
)

func main() {
	go HandleBroadcast()
	go WatchFiles()
	http.HandleFunc("/ws", HandleWebSocket)
	http.HandleFunc("/thumbnail/", HandleThumbnail)
	http.HandleFunc("/api/containers/", handleContainerAPI)
	http.HandleFunc("/api/stacks/", handleStackAPI)
	http.HandleFunc("/", HandleRoot)

	log.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
