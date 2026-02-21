package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for development
		},
	}
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
	broadcast = make(chan FileChangeMessage)
)

// FileChangeMessage represents a file change notification
type FileChangeMessage struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// HandleWebSocket manages WebSocket connections
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	// Register client
	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	log.Println("Client connected")

	// Unregister client on disconnect
	defer func() {
		clientsMu.Lock()
		delete(clients, conn)
		clientsMu.Unlock()
		log.Println("Client disconnected")
	}()

	// Keep connection alive and handle client messages
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// HandleBroadcast sends file change messages to all connected clients
func HandleBroadcast() {
	for msg := range broadcast {
		clientsMu.Lock()
		for conn := range clients {
			err := conn.WriteJSON(msg)
			if err != nil {
				log.Println("Error sending to client:", err)
				conn.Close()
				delete(clients, conn)
			}
		}
		clientsMu.Unlock()
	}
}
