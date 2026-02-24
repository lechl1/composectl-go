package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	go SessionCleanup()
	go HandleBroadcast()
	// go WatchFiles()

	go RegisterHTTPHandlers()

	port := getConfig("port", "8882")
	addr := getConfig("addr", "0.0.0.0")
	listenAddr := fmt.Sprintf("%s:%s", addr, port)

	log.Printf("Server running on http://%s:%s", addr, port)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
