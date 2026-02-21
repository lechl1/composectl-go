package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComposeFile represents a docker-compose.yml structure
type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
	Volumes  map[string]ComposeVolume  `yaml:"volumes,omitempty"`
	Networks map[string]ComposeNetwork `yaml:"networks,omitempty"`
}

// ComposeVolume represents a volume configuration
type ComposeVolume struct {
	External bool `yaml:"external"`
}

// ComposeNetwork represents a network configuration
type ComposeNetwork struct {
	External bool `yaml:"external"`
}

// ComposeService represents a service in docker-compose.yml
type ComposeService struct {
	Image         string                 `yaml:"image"`
	ContainerName string                 `yaml:"container_name"`
	Volumes       []string               `yaml:"volumes"`
	Ports         []string               `yaml:"ports"`
	Environment   []string               `yaml:"environment,omitempty"`
	Networks      interface{}            `yaml:"networks,omitempty"` // Can be array or map
	Labels        interface{}            `yaml:"labels,omitempty"`   // Can be array or map
	Command       interface{}            `yaml:"command,omitempty"`  // Can be string or array
}

// handleStackAPI routes stack API requests to appropriate handlers
func handleStackAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle listing all stacks at /api/stacks
	if path == "/api/stacks" || path == "/api/stacks/" {
		if r.Method == http.MethodGet {
			HandleListStacks(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if strings.HasSuffix(path, "/stop") {
		HandleStopStack(w, r)
	} else if strings.HasSuffix(path, "/start") {
		HandleStartStack(w, r)
	} else if r.Method == http.MethodDelete {
		HandleDeleteStack(w, r)
	} else if r.Method == http.MethodGet {
		HandleGetStack(w, r)
	} else if r.Method == http.MethodPut {
		HandlePutStack(w, r)
	} else {
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// getStacksList returns a combined list of running stacks from Docker and available YAML files
func getStacksList() ([]map[string]interface{}, error) {
	// Get all containers from Docker (running and stopped)
	allContainers, err := getAllContainers()
	if err != nil {
		return nil, fmt.Errorf("failed to get containers: %w", err)
	}

	// Get running stacks from Docker
	runningStacks, err := getRunningStacks()
	if err != nil {
		return nil, fmt.Errorf("failed to get running stacks: %w", err)
	}

	// Get available YAML files from stacks directory
	stacksDir := "stacks"
	ymlStacks := make(map[string]string) // stackName -> filePath

	entries, err := os.ReadDir(stacksDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read stacks directory: %w", err)
	}

	if err == nil {
		// Collect YAML file stack names and paths
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yml") {
				stackName := strings.TrimSuffix(entry.Name(), ".yml")
				ymlStacks[stackName] = filepath.Join(stacksDir, entry.Name())
			}
		}
	}

	// Create a map to track which stacks are already running
	runningStackNames := make(map[string]bool)
	for _, stack := range runningStacks {
		if name, ok := stack["name"].(string); ok {
			runningStackNames[name] = true
		}
	}

	// Add YAML stacks that are not running (with simulated containers)
	for stackName, filePath := range ymlStacks {
		if !runningStackNames[stackName] {
			// Parse YAML file and create simulated containers
			simulatedContainers, err := createSimulatedContainers(stackName, filePath, allContainers)
			if err != nil {
				log.Printf("Error creating simulated containers for %s: %v", stackName, err)
				// Still add the stack but with empty containers
				runningStacks = append(runningStacks, map[string]interface{}{
					"name":       stackName,
					"containers": []interface{}{},
				})
			} else {
				runningStacks = append(runningStacks, map[string]interface{}{
					"name":       stackName,
					"containers": simulatedContainers,
				})
			}
		}
	}

	return runningStacks, nil
}


// HandleListStacks handles GET /api/stacks
// Returns a combined list of running stacks from Docker and available YAML files
func HandleListStacks(w http.ResponseWriter, r *http.Request) {
	stacks, err := getStacksList()
	if err != nil {
		log.Printf("Error getting stacks list: %v", err)
		http.Error(w, "Failed to get stacks", http.StatusInternalServerError)
		return
	}

	// Return the combined list
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(stacks)
}

// createSimulatedContainers creates simulated container objects from a docker-compose.yml file
func createSimulatedContainers(stackName, filePath string, allContainers []map[string]interface{}) ([]map[string]interface{}, error) {
	// Read the YAML file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse the YAML
	var compose ComposeFile
	if err := yaml.Unmarshal(content, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Create a map of containers by name for quick lookup
	containerMap := make(map[string]map[string]interface{})
	for _, container := range allContainers {
		if names, ok := container["Names"].(string); ok {
			// Docker ps returns names with leading slash, so we normalize
			normalizedName := strings.TrimPrefix(names, "/")
			containerMap[normalizedName] = container
		}
	}

	var containers []map[string]interface{}

	// Create a simulated container for each service
	for serviceName, service := range compose.Services {
		containerName := service.ContainerName
		if containerName == "" {
			containerName = serviceName
		}

		// Build labels map
		labels := make(map[string]interface{})
		labels["com.docker.compose.project"] = stackName
		labels["com.docker.compose.service"] = serviceName
		labels["com.docker.compose.oneoff"] = "False"

		// Add custom labels from the service definition
		if service.Labels != nil {
			switch v := service.Labels.(type) {
			case []interface{}:
				// Labels as array of strings
				for _, label := range v {
					if labelStr, ok := label.(string); ok {
						if parts := strings.SplitN(labelStr, "=", 2); len(parts) == 2 {
							labels[parts[0]] = parts[1]
						}
					}
				}
			case map[string]interface{}:
				// Labels as map
				for k, val := range v {
					labels[k] = val
				}
			}
		}

		// Build mounts from volumes
		var mounts []string
		for _, volume := range service.Volumes {
			// Extract volume name or path
			parts := strings.Split(volume, ":")
			if len(parts) > 0 {
				mounts = append(mounts, parts[0])
			}
		}
		mountsStr := strings.Join(mounts, ",")

		// Build ports string
		portsStr := strings.Join(service.Ports, ", ")

		// Build networks array/string
		var networksStr string
		switch v := service.Networks.(type) {
		case []interface{}:
			var nets []string
			for _, net := range v {
				if netStr, ok := net.(string); ok {
					nets = append(nets, netStr)
				}
			}
			networksStr = strings.Join(nets, ",")
		case map[string]interface{}:
			var nets []string
			for net := range v {
				nets = append(nets, net)
			}
			networksStr = strings.Join(nets, ",")
		}

		// Build command string
		var commandStr string
		switch v := service.Command.(type) {
		case string:
			commandStr = fmt.Sprintf("\"%s\"", v)
		case []interface{}:
			var cmdParts []string
			for _, part := range v {
				if partStr, ok := part.(string); ok {
					cmdParts = append(cmdParts, partStr)
				}
			}
			commandStr = fmt.Sprintf("\"%s\"", strings.Join(cmdParts, " "))
		}

		// Default state and status
		state := "created"
		status := "Created"

		// Check if this container actually exists in Docker
		if existingContainer, exists := containerMap[containerName]; exists {
			// Use the real State and Status from the existing container
			if s, ok := existingContainer["State"].(string); ok {
				state = s
			}
			if s, ok := existingContainer["Status"].(string); ok {
				status = s
			}
		}

		// Create the container object
		container := map[string]interface{}{
			"Names":        containerName,
			"Image":        service.Image,
			"Command":      commandStr,
			"State":        state,
			"Status":       status,
			"ID":           "",
			"CreatedAt":    "",
			"RunningFor":   "",
			"Size":         "0B",
			"LocalVolumes": fmt.Sprintf("%d", len(service.Volumes)),
			"Platform":     nil,
			"Networks":     networksStr,
			"Ports":        portsStr,
			"Mounts":       mountsStr,
			"Labels":       labels,
		}

		containers = append(containers, container)
	}

	return containers, nil
}

// getRunningStacks executes docker ps and returns stacks grouped by compose project
func getRunningStacks() ([]map[string]interface{}, error) {
	// Execute docker ps command
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

	// Group containers by com.docker.compose.project label
	stacksMap := make(map[string][]map[string]interface{})

	for _, container := range containers {
		projectName := "none"
		if labels, ok := container["Labels"].(map[string]interface{}); ok {
			if project, ok := labels["com.docker.compose.project"].(string); ok && project != "" {
				projectName = project
			}
		}

		stacksMap[projectName] = append(stacksMap[projectName], container)
	}

	// Convert map to array of stacks
	var stacks []map[string]interface{}
	for name, containers := range stacksMap {
		stacks = append(stacks, map[string]interface{}{
			"name":       name,
			"containers": containers,
		})
	}

	return stacks, nil
}

// HandleStopStack handles POST /api/stacks/{name}/stop
// Stops all containers in a Docker Compose stack
func HandleStopStack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract stack name from URL path
	// Expected format: /api/stacks/{name}/stop
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "stacks" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	stackName := pathParts[2]
	if stackName == "" {
		http.Error(w, "Stack name is required", http.StatusBadRequest)
		return
	}

	log.Printf("Stopping stack: %s", stackName)

	// Execute docker compose stop command
	cmd := exec.Command("docker", "compose", "-p", stackName, "stop")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error stopping stack %s: %v, output: %s", stackName, err, string(output))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to stop stack: %v", err),
			"output":  string(output),
		})
		return
	}

	log.Printf("Successfully stopped stack: %s", stackName)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"stackName": stackName,
		"message":   "Stack stopped successfully",
		"output":    string(output),
	})
}

// HandleStartStack handles POST /api/stacks/{name}/start
// Starts all containers in a Docker Compose stack
func HandleStartStack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract stack name from URL path
	// Expected format: /api/stacks/{name}/start
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "stacks" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	stackName := pathParts[2]
	if stackName == "" {
		http.Error(w, "Stack name is required", http.StatusBadRequest)
		return
	}

	log.Printf("Starting stack: %s", stackName)

	// Execute docker compose start command
	cmd := exec.Command("docker", "compose", "-p", stackName, "start")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error starting stack %s: %v, output: %s", stackName, err, string(output))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to start stack: %v", err),
			"output":  string(output),
		})
		return
	}

	log.Printf("Successfully deleted stack: %s", stackName)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"stackName": stackName,
		"message":   "Stack deleted successfully",
		"output":    string(output),
	})
}

// findContainersByProjectName finds all containers that match the given project name label
func findContainersByProjectName(projectName string) ([]string, error) {
	containers, err := getAllContainers()
	if err != nil {
		return nil, err
	}

	var containerIDs []string
	for _, container := range containers {
		if labels, ok := container["Labels"].(map[string]interface{}); ok {
			if project, ok := labels["com.docker.compose.project"].(string); ok && project == projectName {
				if id, ok := container["ID"].(string); ok {
					containerIDs = append(containerIDs, id)
				}
			}
		}
	}

	return containerIDs, nil
}

// inspectContainers runs docker inspect on the given container IDs and returns the parsed JSON
func inspectContainers(containerIDs []string) ([]map[string]interface{}, error) {
	if len(containerIDs) == 0 {
		return []map[string]interface{}{}, nil
	}

	args := append([]string{"inspect"}, containerIDs...)
	cmd := exec.Command("docker", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect containers: %w", err)
	}

	var inspectData []map[string]interface{}
	if err := json.Unmarshal(output, &inspectData); err != nil {
		return nil, fmt.Errorf("failed to parse inspect output: %w", err)
	}

	return inspectData, nil
}

// reconstructComposeFromContainers creates a docker-compose YAML from container inspection data
func reconstructComposeFromContainers(inspectData []map[string]interface{}) (string, error) {
	compose := ComposeFile{
		Services: make(map[string]ComposeService),
		Volumes:  make(map[string]ComposeVolume),
		Networks: make(map[string]ComposeNetwork),
	}

	for _, containerData := range inspectData {
		// Extract service name from labels
		config, ok := containerData["Config"].(map[string]interface{})
		if !ok {
			continue
		}

		labels, _ := config["Labels"].(map[string]interface{})
		serviceName, ok := labels["com.docker.compose.service"].(string)
		if !ok || serviceName == "" {
			// Fallback to container name without project prefix
			if name, ok := containerData["Name"].(string); ok {
				serviceName = strings.TrimPrefix(name, "/")
			} else {
				continue
			}
		}

		service := ComposeService{}

		// Image
		if image, ok := config["Image"].(string); ok {
			service.Image = image
		}

		// Container name
		if name, ok := containerData["Name"].(string); ok {
			service.ContainerName = strings.TrimPrefix(name, "/")
		}

		// Command
		if cmd, ok := config["Cmd"].([]interface{}); ok && len(cmd) > 0 {
			var cmdParts []string
			for _, part := range cmd {
				if str, ok := part.(string); ok {
					cmdParts = append(cmdParts, str)
				}
			}
			if len(cmdParts) > 0 {
				service.Command = cmdParts
			}
		}

		// Environment variables
		if env, ok := config["Env"].([]interface{}); ok && len(env) > 0 {
			var envVars []string
			for _, envVar := range env {
				if envStr, ok := envVar.(string); ok {
					// Filter out common system environment variables that Docker adds
					// Keep only user-defined environment variables
					if !strings.HasPrefix(envStr, "PATH=") &&
						!strings.HasPrefix(envStr, "HOSTNAME=") &&
						!strings.HasPrefix(envStr, "HOME=") {
						envVars = append(envVars, envStr)
					}
				}
			}
			if len(envVars) > 0 {
				service.Environment = envVars
			}
		}

		// Ports
		hostConfig, _ := containerData["HostConfig"].(map[string]interface{})
		if portBindings, ok := hostConfig["PortBindings"].(map[string]interface{}); ok {
			for containerPort, bindings := range portBindings {
				if bindingsList, ok := bindings.([]interface{}); ok {
					for _, binding := range bindingsList {
						if bindingMap, ok := binding.(map[string]interface{}); ok {
							hostPort, _ := bindingMap["HostPort"].(string)
							if hostPort != "" {
								service.Ports = append(service.Ports, fmt.Sprintf("%s:%s", hostPort, containerPort))
							}
						}
					}
				}
			}
		}

		// Volumes/Mounts
		if mounts, ok := containerData["Mounts"].([]interface{}); ok {
			for _, mount := range mounts {
				if mountMap, ok := mount.(map[string]interface{}); ok {
					mountType, _ := mountMap["Type"].(string)
					source, _ := mountMap["Source"].(string)
					destination, _ := mountMap["Destination"].(string)

					if mountType == "bind" {
						service.Volumes = append(service.Volumes, fmt.Sprintf("%s:%s", source, destination))
					} else if mountType == "volume" {
						volumeName, _ := mountMap["Name"].(string)
						if volumeName != "" {
							service.Volumes = append(service.Volumes, fmt.Sprintf("%s:%s", volumeName, destination))
							compose.Volumes[volumeName] = ComposeVolume{External: true} // Add to volumes section as external
						}
					}
				}
			}
		}

		// Networks
		if networkSettings, ok := containerData["NetworkSettings"].(map[string]interface{}); ok {
			if networks, ok := networkSettings["Networks"].(map[string]interface{}); ok {
				var networkNames []string
				for networkName := range networks {
					networkNames = append(networkNames, networkName)
					compose.Networks[networkName] = ComposeNetwork{External: true} // Add to networks section as external
				}
				if len(networkNames) > 0 {
					service.Networks = networkNames
				}
			}
		}

		// Labels (filter out compose-specific labels)
		serviceLabels := make(map[string]interface{})
		for key, value := range labels {
			if !strings.HasPrefix(key, "com.docker.compose.") {
				serviceLabels[key] = value
			}
		}
		if len(serviceLabels) > 0 {
			service.Labels = serviceLabels
		}

		compose.Services[serviceName] = service
	}

	// Marshal to YAML with 2-space indentation
	var buf strings.Builder

	// Add disclaimer comment at the top
	buf.WriteString("# This docker-compose.yml was automatically reconstructed from running and stopped containers.\n")
	buf.WriteString("# Some settings may be incomplete or differ from the original configuration.\n")
	buf.WriteString("# Please review and adjust as needed before using in production.\n")

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(compose); err != nil {
		return "", fmt.Errorf("failed to marshal compose file: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("failed to close yaml encoder: %w", err)
	}

	return buf.String(), nil
}

// HandleGetStack handles GET /api/stacks/{name}
// Serves the YAML file for the specified stack from the stacks directory
// If the file doesn't exist, reconstructs it from running containers
func HandleGetStack(w http.ResponseWriter, r *http.Request) {
	// Extract stack name from URL path
	// Expected format: /api/stacks/{name}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "stacks" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	stackName := pathParts[2]
	if stackName == "" {
		http.Error(w, "Stack name is required", http.StatusBadRequest)
		return
	}

	// Construct the file path in the stacks directory
	filePath := filepath.Join("stacks", stackName+".yml")

	// Check if file exists
	var content []byte
	var err error

	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		// File doesn't exist, try to reconstruct from running containers
		log.Printf("Stack file not found for %s, attempting to reconstruct from containers", stackName)

		// Find containers with matching project label
		containerIDs, err := findContainersByProjectName(stackName)
		if err != nil {
			log.Printf("Error finding containers for stack %s: %v", stackName, err)
			http.Error(w, "Stack not found and failed to find containers", http.StatusNotFound)
			return
		}

		if len(containerIDs) == 0 {
			log.Printf("No containers found for stack %s", stackName)
			http.Error(w, "Stack not found", http.StatusNotFound)
			return
		}

		// Inspect the containers
		inspectData, err := inspectContainers(containerIDs)
		if err != nil {
			log.Printf("Error inspecting containers for stack %s: %v", stackName, err)
			http.Error(w, "Failed to inspect containers", http.StatusInternalServerError)
			return
		}

		// Reconstruct docker-compose YAML
		yamlContent, err := reconstructComposeFromContainers(inspectData)
		if err != nil {
			log.Printf("Error reconstructing compose file for stack %s: %v", stackName, err)
			http.Error(w, "Failed to reconstruct compose file", http.StatusInternalServerError)
			return
		}

		content = []byte(yamlContent)
		log.Printf("Successfully reconstructed compose file for stack %s from %d container(s)", stackName, len(containerIDs))
	} else {
		// Read the file normally
		content, err = os.ReadFile(filePath)
		if err != nil {
			log.Printf("Error reading stack file %s: %v", filePath, err)
			http.Error(w, "Failed to read stack file", http.StatusInternalServerError)
			return
		}
	}

	// Serve the file content as text/plain
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// HandlePutStack handles PUT /api/stacks/{name}
// Updates the YAML file for the specified stack
func HandlePutStack(w http.ResponseWriter, r *http.Request) {
	// Extract stack name from URL path
	// Expected format: /api/stacks/{name}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "stacks" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	stackName := pathParts[2]
	if stackName == "" {
		http.Error(w, "Stack name is required", http.StatusBadRequest)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Construct the file path
	filePath := filepath.Join("stacks", stackName+".yml")

	// Ensure the stacks directory exists
	if err := os.MkdirAll("stacks", 0755); err != nil {
		log.Printf("Error creating stacks directory: %v", err)
		http.Error(w, "Failed to create stacks directory", http.StatusInternalServerError)
		return
	}

	// Write the file
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		log.Printf("Error writing stack file %s: %v", filePath, err)
		http.Error(w, "Failed to write stack file", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully updated stack: %s", stackName)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"stackName": stackName,
		"message":   "Stack updated successfully",
	})
}

func HandleDeleteStack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract stack name from URL path
	// Expected format: /api/stacks/{name}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "stacks" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	stackName := pathParts[2]
	if stackName == "" {
		http.Error(w, "Stack name is required", http.StatusBadRequest)
		return
	}

	log.Printf("Deleting stack: %s", stackName)

	// Execute docker compose down command to remove all containers
	cmd := exec.Command("docker", "compose", "-p", stackName, "down")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error deleting stack %s: %v, output: %s", stackName, err, string(output))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to delete stack: %v", err),
			"output":  string(output),
		})
		return
	}

	log.Printf("Successfully deleted stack: %s", stackName)

	// Delete the stack YAML file from stacks directory
	stackFilePath := filepath.Join("stacks", stackName+".yml")
	if err := os.Remove(stackFilePath); err != nil {
		// Log warning but don't fail the operation since containers are already removed
		log.Printf("Warning: Failed to delete stack file %s: %v", stackFilePath, err)
	} else {
		log.Printf("Deleted stack file: %s", stackFilePath)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"stackName": stackName,
		"message":   "Stack deleted successfully",
		"output":    string(output),
	})
}


// getStacksData returns the combined stacks data (same as GET /api/stacks)
// This is used to provide stacks data to Go templates
func getStacksData() ([]map[string]interface{}, error) {
	// Get all containers from Docker (running and stopped)
	allContainers, err := getAllContainers()
	if err != nil {
		return nil, fmt.Errorf("failed to get all containers: %w", err)
	}

	// Get running stacks from Docker
	runningStacks, err := getRunningStacks()
	if err != nil {
		return nil, fmt.Errorf("failed to get running stacks: %w", err)
	}

	// Get available YAML files from stacks directory
	stacksDir := "stacks"
	ymlStacks := make(map[string]string) // stackName -> filePath

	entries, err := os.ReadDir(stacksDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read stacks directory: %w", err)
	}

	if err == nil {
		// Collect YAML file stack names and paths
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yml") {
				stackName := strings.TrimSuffix(entry.Name(), ".yml")
				ymlStacks[stackName] = filepath.Join(stacksDir, entry.Name())
			}
		}
	}

	// Create a map to track which stacks are already running
	runningStackNames := make(map[string]bool)
	for _, stack := range runningStacks {
		if name, ok := stack["name"].(string); ok {
			runningStackNames[name] = true
		}
	}

	// Add YAML stacks that are not running (with simulated containers)
	for stackName, filePath := range ymlStacks {
		if !runningStackNames[stackName] {
			// Parse YAML file and create simulated containers
			simulatedContainers, err := createSimulatedContainers(stackName, filePath, allContainers)
			if err != nil {
				log.Printf("Error creating simulated containers for %s: %v", stackName, err)
				// Still add the stack but with empty containers
				runningStacks = append(runningStacks, map[string]interface{}{
					"name":       stackName,
					"containers": []interface{}{},
				})
			} else {
				runningStacks = append(runningStacks, map[string]interface{}{
					"name":       stackName,
					"containers": simulatedContainers,
				})
			}
		}
	}

	// Sort stacks alphabetically by name
	sort.Slice(runningStacks, func(i, j int) bool {
		nameI, _ := runningStacks[i]["name"].(string)
		nameJ, _ := runningStacks[j]["name"].(string)
		return nameI < nameJ
	})

	return runningStacks, nil
}
