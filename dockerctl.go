package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
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

// matchResult contains the matched template path and extracted parameters
type matchResult struct {
	templatePath string
	params       map[string]string
}

// matchRoute tries to match a URL path to a template, handling dynamic segments like [stack]
func matchRoute(urlPath string) (*matchResult, error) {
	// Remove leading slash and split path
	cleanPath := strings.TrimPrefix(urlPath, "/")
	if cleanPath == "" {
		return nil, fmt.Errorf("empty path")
	}

	urlSegments := strings.Split(cleanPath, "/")
	params := make(map[string]string)

	// Build the path progressively, matching dynamic segments
	currentPath := "pages"
	for _, segment := range urlSegments {
		// Try exact match first
		exactPath := filepath.Join(currentPath, segment)
		if info, err := os.Stat(exactPath); err == nil && info.IsDir() {
			currentPath = exactPath
			continue
		}

		// Try to find a dynamic segment directory [xxx]
		entries, err := os.ReadDir(currentPath)
		if err != nil {
			return nil, err
		}

		matched := false
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			name := entry.Name()
			if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
				// Extract parameter name from [paramName]
				paramName := name[1 : len(name)-1]
				params[paramName] = segment
				currentPath = filepath.Join(currentPath, name)
				matched = true
				break
			}
		}

		if !matched {
			return nil, fmt.Errorf("no match found for segment: %s", segment)
		}
	}

	// Determine the final template file name
	// For the last segment, use the last URL segment or dynamic param name
	lastSegment := urlSegments[len(urlSegments)-1]

	// Check if current path contains a dynamic segment directory
	dirName := filepath.Base(currentPath)
	var templateName string

	if strings.HasPrefix(dirName, "[") && strings.HasSuffix(dirName, "]") {
		templateName = dirName + ".html"
	} else {
		templateName = lastSegment + ".html"
	}

	templatePath := filepath.Join(currentPath, templateName)

	// Verify the template exists
	if _, err := os.Stat(templatePath); err != nil {
		return nil, fmt.Errorf("template not found: %s", templatePath)
	}

	return &matchResult{
		templatePath: templatePath,
		params:       params,
	}, nil
}

// loadComponents loads all component files matching "components/X/X.html" pattern
// and returns them as a map with the filename (without extension) as the key
func loadComponents() (map[string]*template.Template, error) {
	components := make(map[string]*template.Template)
	componentsDir := "components"
	if _, err := os.Stat(componentsDir); os.IsNotExist(err) {
		return components, nil
	}

	entries, err := os.ReadDir(componentsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		componentName := entry.Name()
		componentPath := filepath.Join(componentsDir, componentName, componentName+".html")

		if _, err := os.Stat(componentPath); err == nil {
			content, err := os.ReadFile(componentPath)
			if err != nil {
				log.Printf("Error reading component %s: %v", componentPath, err)
				continue
			}
			components[componentName], err = template.New(componentName).Parse(string(content))
			log.Printf("Loaded component: %s", componentName)
		}
	}

	return components, nil
}

// findDeepestIndexHTML returns the path to the deepest index.html for a given path
func findDeepestIndexHTML(requestPath string) string {
	// Start with the root index.html
	deepestIndex := "pages/index.html"

	// Split the path and check for index.html at each level
	parts := strings.Split(strings.TrimPrefix(requestPath, "/"), "/")
	for i := 0; i < len(parts); i++ {
		potentialIndex := filepath.Join("pages", filepath.Join(parts[:i+1]...), "index.html")
		if _, err := os.Stat(potentialIndex); err == nil {
			deepestIndex = potentialIndex
		}
	}

	return deepestIndex
}

// runScriptsInDirectory finds and executes all .sh scripts in a directory
// and returns their JSON output parsed into a map
func runScriptsInDirectory(dirPath string, params map[string]string) (map[string]interface{}, error) {
	scriptData := make(map[string]interface{})

	// Check if the directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return scriptData, nil
	}

	// Read directory entries
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return scriptData, err
	}

	// Find all .sh files
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
			continue
		}

		scriptPath := filepath.Join(dirPath, entry.Name())
		scriptName := strings.TrimSuffix(entry.Name(), ".sh")

		log.Printf("Executing script: %s", scriptPath)

		// Execute the script
		cmd := exec.Command("/bin/bash", scriptPath)

		// Set environment variables from URL parameters
		env := os.Environ()
		for key, value := range params {
			env = append(env, fmt.Sprintf("%s=%s", strings.ToUpper(key), value))
		}
		cmd.Env = env

		// Capture output
		output, err := cmd.Output()
		if err != nil {
			log.Printf("Error executing script %s: %v", scriptPath, err)
			// If script fails, store error info
			scriptData[scriptName] = map[string]interface{}{
				"error": err.Error(),
			}
			continue
		}

		// Parse JSON output
		var jsonData interface{}
		if err := json.Unmarshal(output, &jsonData); err != nil {
			log.Printf("Error parsing JSON from script %s: %v", scriptPath, err)
			// If JSON parsing fails, store the raw output as a string
			scriptData[scriptName] = string(output)
			continue
		}
		// Store parsed data with script name as key
		scriptData[scriptName] = jsonData
		log.Printf("Script %s executed successfully", scriptName)
	}

	return scriptData, nil
}

// handleWebSocket manages WebSocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
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

// handleBroadcast sends file change messages to all connected clients
func handleBroadcast() {
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

// watchFiles monitors the pages and components directories for changes
func watchFiles() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Error creating file watcher:", err)
	}
	defer watcher.Close()

	// Watch pages directory
	err = addWatchRecursive(watcher, "pages")
	if err != nil {
		log.Println("Error watching pages directory:", err)
	}

	// Watch components directory (if it exists)
	if _, err := os.Stat("components"); err == nil {
		err = addWatchRecursive(watcher, "components")
		if err != nil {
			log.Println("Error watching components directory:", err)
		}
	}

	log.Println("File watcher started for pages and components directories")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
				log.Printf("File change detected: %s [%s]", event.Name, event.Op)
				broadcast <- FileChangeMessage{
					Type: event.Op.String(),
					Path: event.Name,
				}

				// If a new directory was created, watch it too
				if event.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						addWatchRecursive(watcher, event.Name)
					}
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("File watcher error:", err)
		}
	}
}

// addWatchRecursive adds a directory and all its subdirectories to the watcher
func addWatchRecursive(watcher *fsnotify.Watcher, path string) error {
	return filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = watcher.Add(walkPath)
			if err != nil {
				log.Printf("Error watching %s: %v", walkPath, err)
				return err
			}
			log.Printf("Watching: %s", walkPath)
		}
		return nil
	})
}

func main() {
	go handleBroadcast()
	go watchFiles()
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Convention: /X matches pages/X/X.html or pages/X/[param]/[param].html
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			http.NotFound(w, r)
			return
		}

		// Try to match the route with dynamic segments
		match, err := matchRoute(r.URL.Path)
		if err != nil {
			log.Printf("Route match error: %v", err)
			http.NotFound(w, r)
			return
		}

		// Load and render the page template
		bodyTemplate, err := template.ParseFiles(match.templatePath)
		if err != nil {
			log.Printf("Template parse error: %v", err)
			http.NotFound(w, r)
			return
		}

		// Prepare page data with URL parameters
		templateData := map[string]interface{}{
			"Title": strings.ToTitle(path),
		}

		// Add all URL parameters to page data
		for key, value := range match.params {
			templateData[key] = value
		}

		// Find the deepest index.html for the path
		layoutPath := findDeepestIndexHTML(r.URL.Path)
		layoutTemplate := template.Must(template.ParseFiles(layoutPath))

		// Run scripts in the page directory
		pageDir := filepath.Dir(match.templatePath)
		scriptData, err := runScriptsInDirectory(pageDir, match.params)
		if err != nil {
			log.Printf("Error running scripts: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Add all URL parameters to layout data as well
		for key, value := range match.params {
			templateData[key] = value
		}

		// Add all script data to templateData
		for name, data := range scriptData {
			templateData[name] = data
		}

		// Load all components
		components, err := loadComponents()
		if err != nil {
			log.Printf("Error loading components: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Add all components to templateData
		for name, tpl := range components {
			var content bytes.Buffer
			if err := tpl.Execute(&content, templateData); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			templateData[name] = content
		}

		// Render the page template to get its content
		var bodyContent bytes.Buffer
		if err := bodyTemplate.Execute(&bodyContent, templateData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		templateData["Body"] = bodyContent

		if err := layoutTemplate.Execute(w, templateData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	log.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
