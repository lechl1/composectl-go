package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

// handleContainerAPI routes container API requests to appropriate handlers
func handleContainerAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/stop") {
		HandleStopContainer(w, r)
	} else if strings.HasSuffix(path, "/start") {
		HandleStartContainer(w, r)
	} else if r.Method == http.MethodDelete {
		HandleDeleteContainer(w, r)
	} else {
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// HandleStopContainer handles POST /api/containers/{id}/stop
// Stops a Docker container by ID
func HandleStopContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract container ID from URL path
	// Expected format: /api/containers/{id}/stop
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "containers" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	containerID := pathParts[2]
	if containerID == "" {
		http.Error(w, "Container ID is required", http.StatusBadRequest)
		return
	}

	log.Printf("Stopping container: %s", containerID)

	// Execute docker stop command
	cmd := exec.Command("docker", "stop", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error stopping container %s: %v, output: %s", containerID, err, string(output))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to stop container: %v", err),
			"output":  string(output),
		})
		return
	}

	log.Printf("Successfully stopped container: %s", containerID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"containerID": containerID,
		"message":     "Container stopped successfully",
		"output":      string(output),
	})
}

// HandleStartContainer handles POST /api/containers/{id}/start
// Starts a Docker container by ID
func HandleStartContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract container ID from URL path
	// Expected format: /api/containers/{id}/start
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "containers" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	containerID := pathParts[2]
	if containerID == "" {
		http.Error(w, "Container ID is required", http.StatusBadRequest)
		return
	}

	log.Printf("Starting container: %s", containerID)

	// Execute docker start command
	cmd := exec.Command("docker", "start", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error starting container %s: %v, output: %s", containerID, err, string(output))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to start container: %v", err),
			"output":  string(output),
		})
		return
	}

	log.Printf("Successfully started container: %s", containerID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"containerID": containerID,
		"message":     "Container started successfully",
		"output":      string(output),
	})
}

// HandleDeleteContainer handles DELETE /api/containers/{id}
// Removes a Docker container by ID
func HandleDeleteContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract container ID from URL path
	// Expected format: /api/containers/{id}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "containers" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	containerID := pathParts[2]
	if containerID == "" {
		http.Error(w, "Container ID is required", http.StatusBadRequest)
		return
	}

	log.Printf("Deleting container: %s", containerID)

	// Execute docker rm command with force flag
	cmd := exec.Command("docker", "rm", "-f", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error deleting container %s: %v, output: %s", containerID, err, string(output))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to delete container: %v", err),
			"output":  string(output),
		})
		return
	}

	log.Printf("Successfully deleted container: %s", containerID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"containerID": containerID,
		"message":     "Container deleted successfully",
		"output":      string(output),
	})
}

// getAllContainers executes docker ps -a and returns all containers (running and stopped)
func getAllContainers() ([]map[string]interface{}, error) {
	// Execute docker ps command with -a to include stopped containers
	cmd := exec.Command("docker", "ps", "-a", "--no-trunc", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute docker ps: %w", err)
	}

	// Parse each line as a separate JSON object
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var containers []map[string]interface{}

	for _, line := range lines {
		if line == "" {
			continue
		}

		var container map[string]interface{}
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			log.Printf("Error parsing container JSON: %v", err)
			continue
		}

		// Parse Labels from comma-separated string to map
		if labelsStr, ok := container["Labels"].(string); ok {
			labels := make(map[string]interface{})
			if labelsStr != "" {
				pairs := strings.Split(labelsStr, ",")
				for _, pair := range pairs {
					if parts := strings.SplitN(pair, "=", 2); len(parts) == 2 {
						labels[parts[0]] = parts[1]
					}
				}
			}
			container["Labels"] = labels
		}

		containers = append(containers, container)
	}

	return containers, nil
}