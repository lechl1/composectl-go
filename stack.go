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
	Configs  map[string]ComposeConfig  `yaml:"configs,omitempty"`
}

// ComposeVolume represents a volume configuration
type ComposeVolume struct {
	External   bool              `yaml:"external,omitempty"`
	Name       string            `yaml:"name,omitempty"`
	Driver     string            `yaml:"driver,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
}

// ComposeNetwork represents a network configuration
type ComposeNetwork struct {
	External bool `yaml:"external"`
}

// ComposeConfig represents a config configuration
type ComposeConfig struct {
	Content string `yaml:"content,omitempty"`
	File    string `yaml:"file,omitempty"`
}

// ComposeServiceConfig represents a config mount in a service
type ComposeServiceConfig struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// ComposeService represents a service in docker-compose.yml
type ComposeService struct {
	Image         string                 `yaml:"image"`
	ContainerName string                 `yaml:"container_name,omitempty"`
	User          string                 `yaml:"user,omitempty"`
	Restart       string                 `yaml:"restart,omitempty"`
	Volumes       []string               `yaml:"volumes,omitempty"`
	Ports         []string               `yaml:"ports,omitempty"`
	Environment   []string               `yaml:"environment,omitempty"`
	Networks      interface{}            `yaml:"networks,omitempty"` // Can be array or map
	Labels        interface{}            `yaml:"labels,omitempty"`   // Can be array or map
	Command       interface{}            `yaml:"command,omitempty"`  // Can be string or array
	Configs       []ComposeServiceConfig `yaml:"configs,omitempty"`
	CapAdd        []string               `yaml:"cap_add,omitempty"`
	Sysctls       interface{}            `yaml:"sysctls,omitempty"` // Can be array or map
}

// handleStackAPI routes stack API requests to appropriate handlers
func handleStackAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle listing all stacks at /api/stacks
	if path == "/api/stacks" || path == "/api/stacks/" {
		if r.Method == http.MethodGet {
			HandleListStacks(w, r)
		} else if r.Method == http.MethodPost {
			HandlePostStack(w, r)
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
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yml") && !strings.HasSuffix(entry.Name(), ".effective.yml") {
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

// getEffectiveComposeFile returns the path to the effective compose file for a stack
// If the effective file exists, it returns that path; otherwise, it returns the regular .yml path
func getEffectiveComposeFile(stackName string) string {
	effectivePath := filepath.Join("stacks", stackName+".effective.yml")
	regularPath := filepath.Join("stacks", stackName+".yml")

	// Check if effective file exists
	if _, err := os.Stat(effectivePath); err == nil {
		return effectivePath
	}

	// Fallback to regular file
	return regularPath
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

	// Get the effective compose file path
	composeFile := getEffectiveComposeFile(stackName)

	// Execute docker compose stop command with the effective compose file
	cmd := exec.Command("docker", "compose", "-f", composeFile, "-p", stackName, "stop")
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

	// Get the effective compose file path
	composeFile := getEffectiveComposeFile(stackName)

	// Execute docker compose start command with the effective compose file
	cmd := exec.Command("docker", "compose", "-f", composeFile, "-p", stackName, "start")
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
		Configs:  make(map[string]ComposeConfig),
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

		// Container name (only set if different from service name)
		if name, ok := containerData["Name"].(string); ok {
			containerName := strings.TrimPrefix(name, "/")
			if containerName != serviceName {
				service.ContainerName = containerName
			}
		}

		// Restart policy (only set if not "unless-stopped")
		hostConfig, _ := containerData["HostConfig"].(map[string]interface{})
		if restartPolicy, ok := hostConfig["RestartPolicy"].(map[string]interface{}); ok {
			if policyName, ok := restartPolicy["Name"].(string); ok && policyName != "" && policyName != "unless-stopped" {
				service.Restart = policyName
			}
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
				}
				if len(networkNames) > 0 {
					service.Networks = networkNames
				}
			}
		}

		// Check if standard HTTP/HTTPS ports are used before filtering labels
		standardHTTPPorts := []string{"80", "8000", "8080", "8081", "443", "8443", "3000", "3001", "5000", "5001"}
		httpsOnlyPorts := []string{"443", "8443"}
		usesHTTPPort := false
		detectedPort := ""
		isHTTPS := false

		// Check in port declarations
		for _, portMapping := range service.Ports {
			parts := strings.Split(portMapping, ":")
			var containerPort string
			if len(parts) > 1 {
				containerPort = strings.Split(parts[len(parts)-1], "/")[0] // Remove /tcp or /udp if present
			} else {
				containerPort = strings.Split(parts[0], "/")[0]
			}
			for _, httpPort := range standardHTTPPorts {
				if containerPort == httpPort {
					usesHTTPPort = true
					detectedPort = containerPort
					// Check if it's an HTTPS port
					for _, httpsPort := range httpsOnlyPorts {
						if containerPort == httpsPort {
							isHTTPS = true
							break
						}
					}
					break
				}
			}
			if usesHTTPPort {
				break
			}
		}

		// Check in environment variables if port not found yet
		if !usesHTTPPort && service.Environment != nil {
			for _, env := range service.Environment {
				envUpper := strings.ToUpper(env)
				for _, httpPort := range standardHTTPPorts {
					if strings.Contains(envUpper, "PORT="+httpPort) ||
						strings.Contains(envUpper, "PORT:"+httpPort) ||
						strings.Contains(envUpper, ":"+httpPort) {
						usesHTTPPort = true
						detectedPort = httpPort
						break
					}
				}
				if usesHTTPPort {
					break
				}
			}
		}

		// Check in original labels (before filtering) for port hints
		if !usesHTTPPort {
			for key, value := range labels {
				if strings.Contains(strings.ToLower(key), "port") {
					valueStr := fmt.Sprintf("%v", value)
					for _, httpPort := range standardHTTPPorts {
						if strings.Contains(valueStr, httpPort) {
							usesHTTPPort = true
							detectedPort = httpPort
							break
						}
					}
				}
				if usesHTTPPort {
					break
				}
			}
		}

		// Labels (filter out compose-specific labels, opencontainers labels, and traefik labels)
		serviceLabels := make(map[string]interface{})
		for key, value := range labels {
			if !strings.HasPrefix(key, "com.docker.compose.") &&
				!strings.HasPrefix(key, "org.opencontainers.image") &&
				!strings.HasPrefix(key, "traefik") {
				serviceLabels[key] = value
			}
		}

		// Add Traefik labels if HTTP port detected
		if usesHTTPPort {
			serviceLabels["traefik.enable"] = "true"
			serviceLabels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", serviceName)] = "https"
			serviceLabels[fmt.Sprintf("traefik.http.routers.%s.rule", serviceName)] = fmt.Sprintf("Host(`%s.localhost`) || Host(`%s.${HOMELAB_DOMAIN_NAME}`) || Host(`%s`)", serviceName, serviceName, serviceName)
			serviceLabels[fmt.Sprintf("traefik.http.routers.%s.service", serviceName)] = serviceName
			serviceLabels[fmt.Sprintf("traefik.http.routers.%s.tls", serviceName)] = "true"
			if detectedPort != "" {
				serviceLabels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName)] = detectedPort
			} else {
				serviceLabels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName)] = "80"
			}
			// Set scheme to https if HTTPS port (443 or 8443) is detected
			if isHTTPS {
				serviceLabels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.scheme", serviceName)] = "https"
			} else {
				serviceLabels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.scheme", serviceName)] = "http"
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

	// Parse and enrich the YAML with Traefik labels and auto-add undeclared networks/volumes
	enrichedYAML, err := enrichComposeWithTraefikLabels(body)
	if err != nil {
		log.Printf("Error enriching compose file: %v", err)
		http.Error(w, "Failed to process compose file", http.StatusBadRequest)
		return
	}

	// Construct the file paths
	originalFilePath := filepath.Join("stacks", stackName+".yml")
	effectiveFilePath := filepath.Join("stacks", stackName+".effective.yml")

	// Ensure the stacks directory exists
	if err := os.MkdirAll("stacks", 0755); err != nil {
		log.Printf("Error creating stacks directory: %v", err)
		http.Error(w, "Failed to create stacks directory", http.StatusInternalServerError)
		return
	}

	// Write the original file (user-provided content)
	if err := os.WriteFile(originalFilePath, body, 0644); err != nil {
		log.Printf("Error writing original stack file %s: %v", originalFilePath, err)
		http.Error(w, "Failed to write original stack file", http.StatusInternalServerError)
		return
	}

	// Write the effective file (enriched with Traefik labels and auto-added networks/volumes)
	if err := os.WriteFile(effectiveFilePath, enrichedYAML, 0644); err != nil {
		log.Printf("Error writing effective stack file %s: %v", effectiveFilePath, err)
		http.Error(w, "Failed to write effective stack file", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully updated stack: %s (original: %s, effective: %s)", stackName, originalFilePath, effectiveFilePath)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"stackName": stackName,
		"message":   "Stack updated successfully",
	})
}

// HandlePostStack handles POST /api/stacks
// Creates a new stack from the provided docker-compose YAML
func HandlePostStack(w http.ResponseWriter, r *http.Request) {
	// Parse the request to get stack name and YAML content
	var requestData struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		log.Printf("Error decoding request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if requestData.Name == "" {
		http.Error(w, "Stack name is required", http.StatusBadRequest)
		return
	}

	if requestData.Content == "" {
		http.Error(w, "Stack content is required", http.StatusBadRequest)
		return
	}

	// Parse and enrich the YAML with Traefik labels and auto-add undeclared networks/volumes
	enrichedYAML, err := enrichComposeWithTraefikLabels([]byte(requestData.Content))
	if err != nil {
		log.Printf("Error enriching compose file: %v", err)
		http.Error(w, "Failed to process compose file", http.StatusBadRequest)
		return
	}

	// Construct the file paths
	originalFilePath := filepath.Join("stacks", requestData.Name+".yml")
	effectiveFilePath := filepath.Join("stacks", requestData.Name+".effective.yml")

	// Check if files already exist
	if _, err := os.Stat(originalFilePath); err == nil {
		http.Error(w, "Stack already exists", http.StatusConflict)
		return
	}

	// Ensure the stacks directory exists
	if err := os.MkdirAll("stacks", 0755); err != nil {
		log.Printf("Error creating stacks directory: %v", err)
		http.Error(w, "Failed to create stacks directory", http.StatusInternalServerError)
		return
	}

	// Write the original file (user-provided content)
	if err := os.WriteFile(originalFilePath, []byte(requestData.Content), 0644); err != nil {
		log.Printf("Error writing original stack file %s: %v", originalFilePath, err)
		http.Error(w, "Failed to write original stack file", http.StatusInternalServerError)
		return
	}

	// Write the effective file (enriched with Traefik labels and auto-added networks/volumes)
	if err := os.WriteFile(effectiveFilePath, enrichedYAML, 0644); err != nil {
		log.Printf("Error writing effective stack file %s: %v", effectiveFilePath, err)
		http.Error(w, "Failed to write effective stack file", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully created stack: %s (original: %s, effective: %s)", requestData.Name, originalFilePath, effectiveFilePath)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"stackName": requestData.Name,
		"message":   "Stack created successfully",
	})
}

// addUndeclaredNetworksAndVolumes analyzes services and adds any undeclared networks and volumes
func addUndeclaredNetworksAndVolumes(compose *ComposeFile) {
	// Initialize maps if they don't exist
	if compose.Volumes == nil {
		compose.Volumes = make(map[string]ComposeVolume)
	}
	if compose.Networks == nil {
		compose.Networks = make(map[string]ComposeNetwork)
	}

	// Collect all networks and volumes referenced by services
	referencedNetworks := make(map[string]bool)
	referencedVolumes := make(map[string]bool)

	for _, service := range compose.Services {
		// Extract networks from service
		switch v := service.Networks.(type) {
		case []interface{}:
			for _, net := range v {
				if netStr, ok := net.(string); ok {
					referencedNetworks[netStr] = true
				}
			}
		case map[string]interface{}:
			for net := range v {
				referencedNetworks[net] = true
			}
		}

		// Extract volumes from service
		for _, volume := range service.Volumes {
			// Parse volume definition to extract volume name
			// Volume format can be:
			// - "volume_name:/path/in/container"
			// - "/host/path:/path/in/container"
			// - "volume_name:/path:ro"
			parts := strings.Split(volume, ":")
			if len(parts) > 0 {
				volumeName := parts[0]
				// Only consider named volumes (not host paths starting with / or ./)
				if !strings.HasPrefix(volumeName, "/") && !strings.HasPrefix(volumeName, "./") && !strings.HasPrefix(volumeName, "../") {
					referencedVolumes[volumeName] = true
				}
			}
		}
	}

	// Add missing networks as external
	for network := range referencedNetworks {
		if _, exists := compose.Networks[network]; !exists {
			compose.Networks[network] = ComposeNetwork{External: true}
			log.Printf("Auto-added undeclared network: %s (marked as external)", network)
		}
	}

	// Add missing volumes as external
	for volume := range referencedVolumes {
		if _, exists := compose.Volumes[volume]; !exists {
			compose.Volumes[volume] = ComposeVolume{External: true}
			log.Printf("Auto-added undeclared volume: %s (marked as external)", volume)
		}
	}
}

// enrichComposeWithTraefikLabels parses a docker-compose YAML and enriches services with Traefik labels
// extractPortNumber extracts the port number from various port formats
// Supports: "80", "0.0.0.0:80", "127.0.0.1:80:80", "80/tcp", "0.0.0.0:80/tcp", etc.
func extractPortNumber(portStr string) int {
	// Remove protocol suffix if present (/tcp, /udp)
	portStr = strings.Split(portStr, "/")[0]

	// Split by colon to handle bind addresses
	parts := strings.Split(portStr, ":")

	// The port is always the last part (or only part if no bind address)
	portPart := parts[len(parts)-1]

	// Try to parse as integer
	var port int
	fmt.Sscanf(portPart, "%d", &port)
	return port
}

// getLowestPrivilegedPort checks if any port below 1024 is used in the service
// and returns the lowest privileged port found, or 0 if none found
// Checks ports, environment variables, labels, and config content
func getLowestPrivilegedPort(service ComposeService, labelsMap map[string]string, configs map[string]ComposeConfig) int {
	lowestPort := 0

	// Check port declarations
	for _, portMapping := range service.Ports {
		// Check both host port and container port
		parts := strings.Split(portMapping, ":")
		for _, part := range parts {
			port := extractPortNumber(part)
			if port > 0 && port < 1024 {
				if lowestPort == 0 || port < lowestPort {
					lowestPort = port
				}
			}
		}
	}

	// Check environment variables for port values
	for _, env := range service.Environment {
		// Look for PORT=xxx or similar patterns
		if strings.Contains(strings.ToUpper(env), "PORT") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				port := extractPortNumber(parts[1])
				if port > 0 && port < 1024 {
					if lowestPort == 0 || port < lowestPort {
						lowestPort = port
					}
				}
			}
		}
	}

	// Check labels for port values (including Traefik labels)
	for key, value := range labelsMap {
		if strings.Contains(strings.ToLower(key), "port") {
			port := extractPortNumber(value)
			if port > 0 && port < 1024 {
				if lowestPort == 0 || port < lowestPort {
					lowestPort = port
				}
			}
		}
	}

	// Check config content for port declarations
	for _, configRef := range service.Configs {
		if configDef, exists := configs[configRef.Source]; exists {
			if configDef.Content != "" {
				// Parse config content looking for port values
				// Support JSON format: "port": 80 or "port":80
				// Support YAML format: port: 80
				// Support various port-related keys
				portKeys := []string{"port", "PORT", "Port", "listen_port", "bind_port", "server_port", "http_port", "https_port"}

				for _, key := range portKeys {
					// Simple pattern matching without regex for performance
					configLines := strings.Split(configDef.Content, "\n")
					for _, line := range configLines {
						// Look for "key": value or key: value
						if strings.Contains(line, key) && strings.Contains(line, ":") {
							// Extract the value after the colon
							parts := strings.Split(line, ":")
							if len(parts) >= 2 {
								// Get the part after the key
								for i, part := range parts {
									if strings.Contains(part, key) && i+1 < len(parts) {
										valuePart := strings.TrimSpace(parts[i+1])
										// Remove trailing comma, quotes, etc.
										valuePart = strings.Trim(valuePart, ` ,}"'`)
										port := extractPortNumber(valuePart)
										if port > 0 && port < 1024 {
											if lowestPort == 0 || port < lowestPort {
												lowestPort = port
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return lowestPort
}

func enrichComposeWithTraefikLabels(yamlContent []byte) ([]byte, error) {
	// Parse the YAML
	var compose ComposeFile
	if err := yaml.Unmarshal(yamlContent, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Enrich each service with Traefik labels
	for serviceName, service := range compose.Services {
		// Convert labels to map if needed
		labelsMap := make(map[string]string)

		if service.Labels != nil {
			switch v := service.Labels.(type) {
			case []interface{}:
				// Labels as array of strings
				for _, label := range v {
					if labelStr, ok := label.(string); ok {
						if parts := strings.SplitN(labelStr, "=", 2); len(parts) == 2 {
							labelsMap[parts[0]] = parts[1]
						}
					}
				}
			case map[string]interface{}:
				// Labels as map
				for k, val := range v {
					if valStr, ok := val.(string); ok {
						labelsMap[k] = valStr
					}
				}
			}
		}

		// Check if any Traefik labels are present
		hasTraefikLabels := false
		for key := range labelsMap {
			if strings.HasPrefix(key, "traefik.") {
				hasTraefikLabels = true
				break
			}
		}

		// If any Traefik label is used, automatically add traefik.enable=true
		if hasTraefikLabels {
			if _, exists := labelsMap["traefik.enable"]; !exists {
				labelsMap["traefik.enable"] = "true"
				log.Printf("Auto-added traefik.enable=true for service %s", serviceName)
			}
		}

		// Handle custom https.port and http.port labels
		var customPort string
		var customScheme string

		// Check for https.port=XXXX label
		if httpsPort, exists := labelsMap["https.port"]; exists {
			customPort = httpsPort
			customScheme = "https"
			delete(labelsMap, "https.port") // Remove from effective YAML
			log.Printf("Detected https.port=%s for service %s, adding Traefik labels", httpsPort, serviceName)
		} else if httpPort, exists := labelsMap["http.port"]; exists {
			// Check for http.port=XXXX label
			customPort = httpPort
			customScheme = "http"
			delete(labelsMap, "http.port") // Remove from effective YAML
			log.Printf("Detected http.port=%s for service %s, adding Traefik labels", httpPort, serviceName)
		}

		// If custom port labels were found, add Traefik labels
		if customPort != "" {
			// Add traefik.enable if not present
			if _, exists := labelsMap["traefik.enable"]; !exists {
				labelsMap["traefik.enable"] = "true"
			}

			// Add standard Traefik labels if not present
			traefikLabels := map[string]string{
				"traefik.http.routers." + serviceName + ".entrypoints":                 "https",
				"traefik.http.routers." + serviceName + ".rule":                        fmt.Sprintf("Host(`%s.localhost`) || Host(`%s.${HOMELAB_DOMAIN_NAME}`) || Host(`%s.leochl.ddns.net`) || Host(`%s`)", serviceName, serviceName, serviceName, serviceName),
				"traefik.http.routers." + serviceName + ".service":                     serviceName,
				"traefik.http.routers." + serviceName + ".tls":                         "true",
				"traefik.http.services." + serviceName + ".loadbalancer.server.port":   customPort,
				"traefik.http.services." + serviceName + ".loadbalancer.server.scheme": customScheme,
			}

			// Only add labels that don't already exist
			for key, value := range traefikLabels {
				if _, exists := labelsMap[key]; !exists {
					labelsMap[key] = value
				}
			}
		} else {
			// Existing logic: Detect if service uses HTTPS ports (443 or 8443)
			httpsOnlyPorts := []string{"443", "8443"}
			isHTTPS := false
			detectedPort := "80"

			// Check in port declarations
			for _, portMapping := range service.Ports {
				parts := strings.Split(portMapping, ":")
				var containerPort string
				if len(parts) > 1 {
					containerPort = strings.Split(parts[len(parts)-1], "/")[0] // Remove /tcp or /udp if present
				} else {
					containerPort = strings.Split(parts[0], "/")[0]
				}
				// Check if it's an HTTPS port
				for _, httpsPort := range httpsOnlyPorts {
					if containerPort == httpsPort {
						isHTTPS = true
						detectedPort = containerPort
						break
					}
				}
				if isHTTPS {
					break
				}
			}

			// Add Traefik labels if not already present
			scheme := "http"
			if isHTTPS {
				scheme = "https"
			}

			traefikLabels := map[string]string{
				"traefik.enable": "true",
				"traefik.http.routers." + serviceName + ".entrypoints":                 "https",
				"traefik.http.routers." + serviceName + ".rule":                        fmt.Sprintf("Host(`%s.localhost`) || Host(`%s.${HOMELAB_DOMAIN_NAME}`) || Host(`%s.leochl.ddns.net`) || Host(`%s`)", serviceName, serviceName, serviceName, serviceName),
				"traefik.http.routers." + serviceName + ".service":                     serviceName,
				"traefik.http.routers." + serviceName + ".tls":                         "true",
				"traefik.http.services." + serviceName + ".loadbalancer.server.port":   detectedPort,
				"traefik.http.services." + serviceName + ".loadbalancer.server.scheme": scheme,
			}

			// Only add labels that don't already exist
			for key, value := range traefikLabels {
				if _, exists := labelsMap[key]; !exists {
					labelsMap[key] = value
				}
			}
		}

		// Convert back to array format for consistency with docker-compose
		var labelsArray []string
		for key, value := range labelsMap {
			labelsArray = append(labelsArray, fmt.Sprintf("%s=%s", key, value))
		}

		// Sort labels for consistent output
		sort.Strings(labelsArray)

		service.Labels = labelsArray

		// Check for privileged ports and add necessary capabilities
		lowestPrivilegedPort := getLowestPrivilegedPort(service, labelsMap, compose.Configs)
		if lowestPrivilegedPort > 0 {
			// Add NET_BIND_SERVICE capability if not already present
			hasCapAdd := false
			for _, capability := range service.CapAdd {
				if capability == "NET_BIND_SERVICE" {
					hasCapAdd = true
					break
				}
			}

			if !hasCapAdd {
				service.CapAdd = append(service.CapAdd, "NET_BIND_SERVICE")
				log.Printf("Auto-added NET_BIND_SERVICE capability for service %s due to privileged port %d detection", serviceName, lowestPrivilegedPort)
			}

			// Add sysctls for unprivileged port start
			sysctlsMap := make(map[string]string)

			// Parse existing sysctls if present
			if service.Sysctls != nil {
				switch v := service.Sysctls.(type) {
				case []interface{}:
					// Sysctls as array of strings
					for _, sysctl := range v {
						if sysctlStr, ok := sysctl.(string); ok {
							if parts := strings.SplitN(sysctlStr, "=", 2); len(parts) == 2 {
								sysctlsMap[parts[0]] = parts[1]
							}
						}
					}
				case map[string]interface{}:
					// Sysctls as map
					for k, val := range v {
						if valStr, ok := val.(string); ok {
							sysctlsMap[k] = valStr
						}
					}
				}
			}

			// Add required sysctls if not present
			if _, exists := sysctlsMap["net.ipv4.ip_unprivileged_port_start"]; !exists {
				sysctlsMap["net.ipv4.ip_unprivileged_port_start"] = fmt.Sprintf("%d", lowestPrivilegedPort)
				log.Printf("Auto-added net.ipv4.ip_unprivileged_port_start=%d sysctl for service %s", lowestPrivilegedPort, serviceName)
			}

			// Convert sysctls to array format
			var sysctlsArray []string
			for key, value := range sysctlsMap {
				sysctlsArray = append(sysctlsArray, fmt.Sprintf("%s=%s", key, value))
			}
			sort.Strings(sysctlsArray)
			service.Sysctls = sysctlsArray
		}

		compose.Services[serviceName] = service
	}

	// Add undeclared networks and volumes
	addUndeclaredNetworksAndVolumes(&compose)

	// Marshal back to YAML
	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(compose); err != nil {
		return nil, fmt.Errorf("failed to marshal compose file: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("failed to close yaml encoder: %w", err)
	}

	return []byte(buf.String()), nil
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

	// Get the effective compose file path
	composeFile := getEffectiveComposeFile(stackName)

	// Execute docker compose down command to remove all containers using effective file
	cmd := exec.Command("docker", "compose", "-f", composeFile, "-p", stackName, "down")
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

	// Delete both the original and effective stack YAML files
	originalFilePath := filepath.Join("stacks", stackName+".yml")
	effectiveFilePath := filepath.Join("stacks", stackName+".effective.yml")

	if err := os.Remove(originalFilePath); err != nil {
		// Log warning but don't fail the operation since containers are already removed
		log.Printf("Warning: Failed to delete original stack file %s: %v", originalFilePath, err)
	} else {
		log.Printf("Deleted original stack file: %s", originalFilePath)
	}

	if err := os.Remove(effectiveFilePath); err != nil {
		// Log warning but don't fail the operation
		log.Printf("Warning: Failed to delete effective stack file %s: %v", effectiveFilePath, err)
	} else {
		log.Printf("Deleted effective stack file: %s", effectiveFilePath)
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
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yml") && !strings.HasSuffix(entry.Name(), ".effective.yml") {
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
