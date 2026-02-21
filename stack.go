package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MultilineString is a custom type that forces multiline YAML formatting for strings with line breaks
type MultilineString string

// MarshalYAML implements yaml.Marshaler to force literal style for multiline strings
func (ms MultilineString) MarshalYAML() (interface{}, error) {
	s := string(ms)
	if strings.Contains(s, "\n") {
		// Create a node with literal style (|) for multiline strings
		node := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Style: yaml.LiteralStyle, // Use | style for multiline
			Value: s,
		}
		return node, nil
	}
	// Return as regular string if no line breaks
	return s, nil
}

// forceMultilineInYAML recursively processes YAML nodes and sets literal style for strings with line breaks
func forceMultilineInYAML(node *yaml.Node) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		// Process document content
		for _, child := range node.Content {
			forceMultilineInYAML(child)
		}
	case yaml.MappingNode:
		// Process all key-value pairs in the mapping
		for _, child := range node.Content {
			forceMultilineInYAML(child)
		}
	case yaml.SequenceNode:
		// Process all items in the sequence
		for _, child := range node.Content {
			forceMultilineInYAML(child)
		}
	case yaml.ScalarNode:
		// If this is a string scalar with line breaks, use literal style
		if node.Tag == "!!str" && strings.Contains(node.Value, "\n") {
			node.Style = yaml.LiteralStyle
		}
	}
}

// encodeYAMLWithMultiline encodes a value to YAML with multiline strings properly formatted
func encodeYAMLWithMultiline(buf *strings.Builder, value interface{}) error {
	// First, marshal to a YAML node
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return fmt.Errorf("failed to encode to node: %w", err)
	}

	// Process the node to set multiline style
	forceMultilineInYAML(&node)

	// Now encode the processed node
	encoder := yaml.NewEncoder(buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(&node); err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return fmt.Errorf("failed to close encoder: %w", err)
	}

	return nil
}

// ComposeFile represents a docker-compose.yml structure
type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
	Volumes  map[string]ComposeVolume  `yaml:"volumes,omitempty"`
	Networks map[string]ComposeNetwork `yaml:"networks,omitempty"`
	Configs  map[string]ComposeConfig  `yaml:"configs,omitempty"`
	Secrets  map[string]ComposeSecret  `yaml:"secrets,omitempty"`
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

// ComposeSecret represents a secret configuration
type ComposeSecret struct {
	Name        string `yaml:"name,omitempty"`
	Environment string `yaml:"environment,omitempty"`
	File        string `yaml:"file,omitempty"`
	External    bool   `yaml:"external,omitempty"`
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
	Environment   interface{}            `yaml:"environment,omitempty"` // Can be array or map
	Networks      interface{}            `yaml:"networks,omitempty"`    // Can be array or map
	Labels        interface{}            `yaml:"labels,omitempty"`      // Can be array or map
	Command       interface{}            `yaml:"command,omitempty"`     // Can be string or array
	Configs       []ComposeServiceConfig `yaml:"configs,omitempty"`
	CapAdd        []string               `yaml:"cap_add,omitempty"`
	Sysctls       interface{}            `yaml:"sysctls,omitempty"` // Can be array or map
	Secrets       []string               `yaml:"secrets,omitempty"`
	MemLimit      string                 `yaml:"mem_limit,omitempty"`
	MemswapLimit  int64                  `yaml:"memswap_limit,omitempty"`
	CPUs          interface{}            `yaml:"cpus,omitempty"` // Can be string or number
	Logging       *LoggingConfig         `yaml:"logging,omitempty"`
}

// LoggingConfig represents the logging configuration for a service
type LoggingConfig struct {
	Driver  string            `yaml:"driver"`
	Options map[string]string `yaml:"options,omitempty"`
}

// normalizeEnvironment converts environment variables from map or array format to array format
// Returns an array of strings in "KEY=VALUE" format
func normalizeEnvironment(env interface{}) []string {
	if env == nil {
		return nil
	}

	// If it's already an array
	if envArray, ok := env.([]interface{}); ok {
		result := make([]string, 0, len(envArray))
		for _, item := range envArray {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}

	// If it's a map (from YAML)
	if envMap, ok := env.(map[string]interface{}); ok {
		result := make([]string, 0, len(envMap))
		// Sort keys for consistent output
		keys := make([]string, 0, len(envMap))
		for k := range envMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			value := envMap[key]
			// Convert value to string
			var valueStr string
			switch v := value.(type) {
			case string:
				valueStr = v
			case int, int64, float64, bool:
				valueStr = fmt.Sprintf("%v", v)
			default:
				valueStr = fmt.Sprintf("%v", v)
			}
			result = append(result, fmt.Sprintf("%s=%s", key, valueStr))
		}
		return result
	}

	// If it's already a []string (shouldn't happen after unmarshal, but just in case)
	if envStrings, ok := env.([]string); ok {
		return envStrings
	}

	return nil
}

// setEnvironmentAsArray converts environment to array format and updates the service
func setEnvironmentAsArray(service *ComposeService, envArray []string) {
	if len(envArray) == 0 {
		service.Environment = nil
	} else {
		service.Environment = envArray
	}
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
	} else if strings.HasSuffix(path, "/enrich") {
		HandleEnrichStack(w, r)
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

// sanitizeEnvironmentVariable checks if an environment variable contains sensitive information
// and replaces its value with a variable reference in the format ${ENV_KEY}
func sanitizeEnvironmentVariable(envStr string) string {
	// Split the environment variable into key and value
	parts := strings.SplitN(envStr, "=", 2)
	if len(parts) != 2 {
		return envStr
	}

	key := parts[0]

	// Check if the key contains sensitive keywords
	upperKey := strings.ToUpper(key)
	sensitiveKeywords := []string{"PASSWD", "PASSWORD", "SECRET", "KEY", "TOKEN", "API_KEY", "APIKEY", "PRIVATE"}

	isSensitive := false
	// Exclude variables with "_FILE" suffix as they are file references, not actual passwords
	if !strings.Contains(upperKey, "_FILE") {
		for _, keyword := range sensitiveKeywords {
			if strings.Contains(upperKey, keyword) {
				isSensitive = true
				break
			}
		}
	}

	if !isSensitive {
		return envStr
	}

	value := parts[1]

	// Do not replace if the value starts with /run/secrets (Docker secrets path)
	if strings.HasPrefix(value, "/run/secrets") {
		return envStr
	}

	// Normalize the key to the ENV_KEY format
	// Replace multiple consecutive non-alphanumeric characters with a single underscore
	normalizedKey := normalizeEnvKey(key)

	// Return the environment variable with the value replaced
	return fmt.Sprintf("%s=${%s}", key, normalizedKey)
}

// normalizeEnvKey normalizes an environment key to uppercase with underscores
// Multiple consecutive non-alphanumeric characters are replaced with a single underscore
func normalizeEnvKey(key string) string {
	// Convert to uppercase
	normalized := strings.ToUpper(key)

	// Replace non-alphanumeric characters with underscores
	var result strings.Builder
	lastWasUnderscore := false

	for _, ch := range normalized {
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			result.WriteRune(ch)
			lastWasUnderscore = false
		} else {
			// Replace any non-alphanumeric character with underscore
			if !lastWasUnderscore {
				result.WriteRune('_')
				lastWasUnderscore = true
			}
		}
	}

	// Trim leading and trailing underscores
	return strings.Trim(result.String(), "_")
}

// extractVariableReferences extracts variable names from strings containing ${XXX} or $XXX patterns
func extractVariableReferences(value string) []string {
	var variables []string

	// Pattern 1: ${VAR_NAME}
	i := 0
	for i < len(value) {
		if i+1 < len(value) && value[i] == '$' && value[i+1] == '{' {
			// Found ${, now find the closing }
			start := i + 2
			end := start
			for end < len(value) && value[end] != '}' {
				end++
			}
			if end < len(value) {
				varName := value[start:end]
				if varName != "" {
					variables = append(variables, varName)
				}
				i = end + 1
				continue
			}
		}
		// Pattern 2: $VAR_NAME (where VAR_NAME is uppercase letters, numbers, and underscores)
		if value[i] == '$' && i+1 < len(value) {
			start := i + 1
			end := start
			// Variable name must start with a letter or underscore
			if (value[end] >= 'A' && value[end] <= 'Z') || (value[end] >= 'a' && value[end] <= 'z') || value[end] == '_' {
				end++
				// Continue with alphanumeric and underscore
				for end < len(value) && ((value[end] >= 'A' && value[end] <= 'Z') ||
					(value[end] >= 'a' && value[end] <= 'z') ||
					(value[end] >= '0' && value[end] <= '9') ||
					value[end] == '_') {
					end++
				}
				varName := value[start:end]
				if varName != "" {
					variables = append(variables, varName)
				}
				i = end
				continue
			}
		}
		i++
	}

	return variables
}

// sanitizeComposePasswords sanitizes environment variables in a ComposeFile
// by extracting plaintext passwords to prod.env and replacing them with variable references ${ENV_KEY}
func sanitizeComposePasswords(compose *ComposeFile) {
	const prodEnvPath = "prod.env"

	// Read existing prod.env
	envVars, err := readProdEnv(prodEnvPath)
	if err != nil {
		log.Printf("Warning: Failed to read prod.env: %v", err)
		envVars = make(map[string]string)
	}

	modified := false

	// Process each service
	for serviceName, service := range compose.Services {
		// Process environment variables
		envArray := normalizeEnvironment(service.Environment)
		var sanitizedEnv []string
		for _, envVar := range envArray {
			// Split the environment variable into key and value
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]

				// Check if this is a sensitive variable
				upperKey := strings.ToUpper(key)
				sensitiveKeywords := []string{"PASSWD", "PASSWORD", "SECRET", "KEY", "TOKEN", "API_KEY", "APIKEY", "PRIVATE"}

				isSensitive := false
				// Exclude variables with "_FILE" suffix as they are file references, not actual passwords
				if !strings.Contains(upperKey, "_FILE") {
					for _, keyword := range sensitiveKeywords {
						if strings.Contains(upperKey, keyword) {
							isSensitive = true
							break
						}
					}
				}

				// If sensitive and has a value, save to prod.env
				if isSensitive && value != "" && !strings.HasPrefix(value, "${") && !strings.HasPrefix(value, "/run/secrets/") {
					normalizedKey := normalizeEnvKey(key)
					// Only save if not already in prod.env
					if _, exists := envVars[normalizedKey]; !exists {
						envVars[normalizedKey] = value
						modified = true
						log.Printf("Extracted password '%s' to prod.env from service '%s'", normalizedKey, serviceName)
					}
				}

				// Check if value contains variable references (${XXX} or $XXX) and is not sensitive
				if !isSensitive && value != "" {
					extractedVars := extractVariableReferences(value)
					for _, varName := range extractedVars {
						// Normalize the variable name before saving
						normalizedVarName := normalizeEnvKey(varName)
						// Only add if not already in prod.env
						if _, exists := envVars[normalizedVarName]; !exists {
							envVars[normalizedVarName] = "" // Add with empty value as placeholder
							modified = true
							log.Printf("Added environment variable '%s' to prod.env from service '%s'", normalizedVarName, serviceName)
						}
					}
				}
			}

			// Sanitize the environment variable
			sanitizedEnv = append(sanitizedEnv, sanitizeEnvironmentVariable(envVar))
		}
		service.Environment = sanitizedEnv
		compose.Services[serviceName] = service
	}

	// Also process labels for variable references
	for serviceName, service := range compose.Services {
		// Process labels if they exist
		if service.Labels != nil {
			if labelArray, ok := service.Labels.([]interface{}); ok {
				for _, label := range labelArray {
					if labelStr, ok := label.(string); ok {
						// Extract variable references from label values
						parts := strings.SplitN(labelStr, "=", 2)
						if len(parts) == 2 {
							value := parts[1]
							extractedVars := extractVariableReferences(value)
							for _, varName := range extractedVars {
								// Normalize the variable name before saving
								normalizedVarName := normalizeEnvKey(varName)
								// Only add if not already in prod.env
								if _, exists := envVars[normalizedVarName]; !exists {
									envVars[normalizedVarName] = "" // Add with empty value as placeholder
									modified = true
									log.Printf("Added environment variable '%s' to prod.env from service '%s' labels", normalizedVarName, serviceName)
								}
							}
						}
					}
				}
			} else if labelMap, ok := service.Labels.(map[string]interface{}); ok {
				for _, value := range labelMap {
					if valueStr, ok := value.(string); ok {
						extractedVars := extractVariableReferences(valueStr)
						for _, varName := range extractedVars {
							// Normalize the variable name before saving
							normalizedVarName := normalizeEnvKey(varName)
							// Only add if not already in prod.env
							if _, exists := envVars[normalizedVarName]; !exists {
								envVars[normalizedVarName] = "" // Add with empty value as placeholder
								modified = true
								log.Printf("Added environment variable '%s' to prod.env from service '%s' labels", normalizedVarName, serviceName)
							}
						}
					}
				}
			}
		}
	}

	// Also process configs for variable references
	if compose.Configs != nil {
		for configName, config := range compose.Configs {
			// Extract variable references from config content
			if config.Content != "" {
				extractedVars := extractVariableReferences(config.Content)
				for _, varName := range extractedVars {
					// Normalize the variable name before saving
					normalizedVarName := normalizeEnvKey(varName)
					// Only add if not already in prod.env
					if _, exists := envVars[normalizedVarName]; !exists {
						envVars[normalizedVarName] = "" // Add with empty value as placeholder
						modified = true
						log.Printf("Added environment variable '%s' to prod.env from config '%s'", normalizedVarName, configName)
					}
				}
			}
			// Also extract from file path if it exists
			if config.File != "" {
				extractedVars := extractVariableReferences(config.File)
				for _, varName := range extractedVars {
					normalizedVarName := normalizeEnvKey(varName)
					if _, exists := envVars[normalizedVarName]; !exists {
						envVars[normalizedVarName] = ""
						modified = true
						log.Printf("Added environment variable '%s' to prod.env from config '%s' file path", normalizedVarName, configName)
					}
				}
			}
		}
	}

	// Process volumes for variable references
	if compose.Volumes != nil {
		for volumeName, volume := range compose.Volumes {
			if volume.Name != "" {
				extractedVars := extractVariableReferences(volume.Name)
				for _, varName := range extractedVars {
					normalizedVarName := normalizeEnvKey(varName)
					if _, exists := envVars[normalizedVarName]; !exists {
						envVars[normalizedVarName] = ""
						modified = true
						log.Printf("Added environment variable '%s' to prod.env from volume '%s'", normalizedVarName, volumeName)
					}
				}
			}
			if volume.DriverOpts != nil {
				for _, optValue := range volume.DriverOpts {
					extractedVars := extractVariableReferences(optValue)
					for _, varName := range extractedVars {
						normalizedVarName := normalizeEnvKey(varName)
						if _, exists := envVars[normalizedVarName]; !exists {
							envVars[normalizedVarName] = ""
							modified = true
							log.Printf("Added environment variable '%s' to prod.env from volume '%s' driver opts", normalizedVarName, volumeName)
						}
					}
				}
			}
		}
	}

	// Process service-level fields for variable references
	for serviceName, service := range compose.Services {
		// Process volumes mount paths
		for _, volumeMount := range service.Volumes {
			extractedVars := extractVariableReferences(volumeMount)
			for _, varName := range extractedVars {
				normalizedVarName := normalizeEnvKey(varName)
				if _, exists := envVars[normalizedVarName]; !exists {
					envVars[normalizedVarName] = ""
					modified = true
					log.Printf("Added environment variable '%s' to prod.env from service '%s' volume mounts", normalizedVarName, serviceName)
				}
			}
		}

		// Process command field
		if service.Command != nil {
			var commandStrings []string
			switch cmd := service.Command.(type) {
			case string:
				commandStrings = []string{cmd}
			case []interface{}:
				for _, c := range cmd {
					if cmdStr, ok := c.(string); ok {
						commandStrings = append(commandStrings, cmdStr)
					}
				}
			}
			for _, cmdStr := range commandStrings {
				extractedVars := extractVariableReferences(cmdStr)
				for _, varName := range extractedVars {
					normalizedVarName := normalizeEnvKey(varName)
					if _, exists := envVars[normalizedVarName]; !exists {
						envVars[normalizedVarName] = ""
						modified = true
						log.Printf("Added environment variable '%s' to prod.env from service '%s' command", normalizedVarName, serviceName)
					}
				}
			}
		}

		// Process image field
		if service.Image != "" {
			extractedVars := extractVariableReferences(service.Image)
			for _, varName := range extractedVars {
				normalizedVarName := normalizeEnvKey(varName)
				if _, exists := envVars[normalizedVarName]; !exists {
					envVars[normalizedVarName] = ""
					modified = true
					log.Printf("Added environment variable '%s' to prod.env from service '%s' image", normalizedVarName, serviceName)
				}
			}
		}
	}

	// Write back to prod.env if modified
	if modified {
		if err := writeProdEnv(prodEnvPath, envVars); err != nil {
			log.Printf("Warning: Failed to write prod.env: %v", err)
		} else {
			log.Printf("Updated prod.env with extracted passwords and environment variables")
		}
	}
}

// sanitizeComposePasswordsWithoutExtraction sanitizes environment variables in a ComposeFile
// by replacing plaintext passwords with variable references ${ENV_KEY}
// WITHOUT extracting them to prod.env (assumes they're already there)
func sanitizeComposePasswordsWithoutExtraction(compose *ComposeFile) {
	// Process each service
	for serviceName, service := range compose.Services {
		// Process environment variables
		envArray := normalizeEnvironment(service.Environment)
		var sanitizedEnv []string
		for _, envVar := range envArray {
			// Sanitize the environment variable
			sanitizedEnv = append(sanitizedEnv, sanitizeEnvironmentVariable(envVar))
		}
		service.Environment = sanitizedEnv
		compose.Services[serviceName] = service
	}
}

// reconstructComposeFromContainers creates a docker-compose YAML from container inspection data
func reconstructComposeFromContainers(inspectData []map[string]interface{}) (string, error) {
	compose := ComposeFile{
		Services: make(map[string]ComposeService),
		Volumes:  make(map[string]ComposeVolume),
		Networks: make(map[string]ComposeNetwork),
		Configs:  make(map[string]ComposeConfig),
		Secrets:  make(map[string]ComposeSecret),
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
						envVars = append(envVars, sanitizeEnvironmentVariable(envStr))
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
		detectedPort, isHTTPS, usesHTTPPort := detectHTTPPort(service)

		// If port not detected from service config, check in original labels for port hints
		if !usesHTTPPort {
			standardHTTPPorts := []string{"80", "8000", "8080", "8081", "443", "8443", "3000", "3001", "5000", "5001"}
			for key, value := range labels {
				if strings.Contains(strings.ToLower(key), "port") {
					valueStr := fmt.Sprintf("%v", value)
					for _, httpPort := range standardHTTPPorts {
						if strings.Contains(valueStr, httpPort) {
							usesHTTPPort = true
							detectedPort = httpPort
							// Check if it's HTTPS port
							if httpPort == "443" || httpPort == "8443" {
								isHTTPS = true
							}
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
			scheme := "http"
			if isHTTPS {
				scheme = "https"
			}
			if detectedPort == "" {
				detectedPort = "80"
			}
			addTraefikLabelsInterface(serviceLabels, serviceName, detectedPort, scheme)
		}

		if len(serviceLabels) > 0 {
			service.Labels = serviceLabels
		}

		compose.Services[serviceName] = service
	}

	// Process secrets to ensure proper declaration
	processSecrets(&compose)

	// Marshal to YAML with 2-space indentation and multiline string support
	var buf strings.Builder

	// Add disclaimer comment at the top
	buf.WriteString("# This docker-compose.yml was automatically reconstructed from running and stopped containers.\n")
	buf.WriteString("# Some settings may be incomplete or differ from the original configuration.\n")
	buf.WriteString("# Please review and adjust as needed before using in production.\n")

	if err := encodeYAMLWithMultiline(&buf, compose); err != nil {
		return "", err
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

	// Validate YAML syntax before processing
	var testCompose ComposeFile
	if err := yaml.Unmarshal(body, &testCompose); err != nil {
		log.Printf("Invalid YAML syntax for stack %s: %v", stackName, err)
		http.Error(w, fmt.Sprintf("Invalid YAML syntax: %v", err), http.StatusBadRequest)
		return
	}

	// Validate that services section exists
	if testCompose.Services == nil || len(testCompose.Services) == 0 {
		log.Printf("No services defined in stack %s", stackName)
		http.Error(w, "Invalid compose file: no services defined", http.StatusBadRequest)
		return
	}

	// First, sanitize passwords and extract them to prod.env
	// This must be done BEFORE enrichment to capture plaintext passwords
	var originalCompose ComposeFile
	if err := yaml.Unmarshal(body, &originalCompose); err != nil {
		log.Printf("Error parsing YAML for sanitization: %v", err)
		http.Error(w, fmt.Sprintf("Failed to parse YAML: %v", err), http.StatusBadRequest)
		return
	}
	sanitizeComposePasswords(&originalCompose)

	// Marshal the sanitized original version back to YAML for .yml file
	var sanitizedBuf strings.Builder
	if err := encodeYAMLWithMultiline(&sanitizedBuf, originalCompose); err != nil {
		log.Printf("Error encoding sanitized YAML: %v", err)
		http.Error(w, fmt.Sprintf("Failed to encode YAML: %v", err), http.StatusInternalServerError)
		return
	}
	sanitizedYAML := []byte(sanitizedBuf.String())

	// Now enrich the YAML with Traefik labels and auto-add undeclared networks/volumes
	// Note: enrichComposeWithTraefikLabels works on the original body which has plaintext passwords,
	// so we need to sanitize the enriched output as well
	enrichedYAML, err := enrichComposeWithTraefikLabels(body)
	if err != nil {
		log.Printf("Error enriching compose file: %v", err)
		http.Error(w, fmt.Sprintf("Failed to process compose file: %v", err), http.StatusBadRequest)
		return
	}

	// Sanitize the enriched YAML as well (passwords were already extracted to prod.env above)
	var enrichedCompose ComposeFile
	if err := yaml.Unmarshal(enrichedYAML, &enrichedCompose); err != nil {
		log.Printf("Error parsing enriched YAML for sanitization: %v", err)
		http.Error(w, fmt.Sprintf("Failed to parse enriched YAML: %v", err), http.StatusBadRequest)
		return
	}
	// Sanitize without extracting again (passwords already in prod.env)
	sanitizeComposePasswordsWithoutExtraction(&enrichedCompose)

	// Marshal the sanitized enriched version back to YAML for .effective.yml file
	var enrichedSanitizedBuf strings.Builder
	if err := encodeYAMLWithMultiline(&enrichedSanitizedBuf, enrichedCompose); err != nil {
		log.Printf("Error encoding sanitized enriched YAML: %v", err)
		http.Error(w, fmt.Sprintf("Failed to encode YAML: %v", err), http.StatusInternalServerError)
		return
	}
	enrichedSanitizedYAML := []byte(enrichedSanitizedBuf.String())

	// Construct the file paths
	originalFilePath := filepath.Join("stacks", stackName+".yml")
	effectiveFilePath := filepath.Join("stacks", stackName+".effective.yml")

	// Ensure the stacks directory exists
	if err := os.MkdirAll("stacks", 0755); err != nil {
		log.Printf("Error creating stacks directory: %v", err)
		http.Error(w, "Failed to create stacks directory", http.StatusInternalServerError)
		return
	}

	// Write the original file (sanitized user-provided content without plaintext passwords)
	if err := os.WriteFile(originalFilePath, sanitizedYAML, 0644); err != nil {
		log.Printf("Error writing original stack file %s: %v", originalFilePath, err)
		http.Error(w, "Failed to write original stack file", http.StatusInternalServerError)
		return
	}

	// Write the effective file (enriched and sanitized - no plaintext passwords)
	if err := os.WriteFile(effectiveFilePath, enrichedSanitizedYAML, 0644); err != nil {
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

// HandleEnrichStack handles POST /api/enrich/{name}
// Enriches the provided docker-compose YAML without modifying files or creating secrets
func HandleEnrichStack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract stack name from URL path
	// Expected format: /api/stacks/{name}/enrich
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

	// Validate YAML syntax before processing
	var testCompose ComposeFile
	if err := yaml.Unmarshal(body, &testCompose); err != nil {
		log.Printf("Invalid YAML syntax for stack %s: %v", stackName, err)
		http.Error(w, fmt.Sprintf("Invalid YAML syntax: %v", err), http.StatusBadRequest)
		return
	}

	// Validate that services section exists
	if testCompose.Services == nil || len(testCompose.Services) == 0 {
		log.Printf("No services defined in stack %s", stackName)
		http.Error(w, "Invalid compose file: no services defined", http.StatusBadRequest)
		return
	}

	// Enrich the YAML without modifying files or creating secrets
	enrichedYAML, err := enrichComposeWithoutSideEffects(body)
	if err != nil {
		log.Printf("Error enriching compose file: %v", err)
		http.Error(w, fmt.Sprintf("Failed to process compose file: %v", err), http.StatusBadRequest)
		return
	}

	log.Printf("Successfully enriched stack YAML for: %s (no files modified)", stackName)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(enrichedYAML)
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

	// Validate YAML syntax before processing
	var testCompose ComposeFile
	if err := yaml.Unmarshal([]byte(requestData.Content), &testCompose); err != nil {
		log.Printf("Invalid YAML syntax for stack %s: %v", requestData.Name, err)
		http.Error(w, fmt.Sprintf("Invalid YAML syntax: %v", err), http.StatusBadRequest)
		return
	}

	// Validate that services section exists
	if testCompose.Services == nil || len(testCompose.Services) == 0 {
		log.Printf("No services defined in stack %s", requestData.Name)
		http.Error(w, "Invalid compose file: no services defined", http.StatusBadRequest)
		return
	}

	// Parse and enrich the YAML with Traefik labels and auto-add undeclared networks/volumes
	enrichedYAML, err := enrichComposeWithTraefikLabels([]byte(requestData.Content))
	if err != nil {
		log.Printf("Error enriching compose file: %v", err)
		http.Error(w, fmt.Sprintf("Failed to process compose file: %v", err), http.StatusBadRequest)
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
	envArray := normalizeEnvironment(service.Environment)
	for _, env := range envArray {
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

// processSecrets scans environment variables for /run/secrets/ references
// and ensures the corresponding secrets are declared at both service and top level
func processSecrets(compose *ComposeFile) {
	// Track all secrets that need to be declared at top level
	requiredSecrets := make(map[string]bool)

	// Process each service
	for serviceName, service := range compose.Services {
		// Track secrets needed by this service
		serviceSecrets := make(map[string]bool)

		// Scan environment variables for /run/secrets/ references
		envArray := normalizeEnvironment(service.Environment)
		for _, envVar := range envArray {
			// Parse the environment variable
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) != 2 {
				continue
			}

			value := parts[1]

			// Check if the value matches /run/secrets/XXXX pattern
			if strings.HasPrefix(value, "/run/secrets/") {
				secretName := strings.TrimPrefix(value, "/run/secrets/")
				if secretName != "" {
					serviceSecrets[secretName] = true
					requiredSecrets[secretName] = true
				}
			}
		}

		// Add secrets to service if needed
		if len(serviceSecrets) > 0 {
			// Get existing service secrets
			existingSecrets := make(map[string]bool)
			for _, secret := range service.Secrets {
				existingSecrets[secret] = true
			}

			// Add missing secrets to service
			for secretName := range serviceSecrets {
				if !existingSecrets[secretName] {
					service.Secrets = append(service.Secrets, secretName)
					log.Printf("Auto-added secret '%s' to service '%s'", secretName, serviceName)
				}
			}

			// Update the service in the compose file
			compose.Services[serviceName] = service
		}
	}

	// Initialize top-level secrets map if needed
	if compose.Secrets == nil {
		compose.Secrets = make(map[string]ComposeSecret)
	}

	// Add missing secrets at top level
	for secretName := range requiredSecrets {
		if _, exists := compose.Secrets[secretName]; !exists {
			compose.Secrets[secretName] = ComposeSecret{
				Name:        secretName,
				Environment: secretName,
			}
			log.Printf("Auto-added top-level secret declaration for '%s'", secretName)
		}
	}

	// Ensure all secrets exist in prod.env file
	if len(requiredSecrets) > 0 {
		secretNames := make([]string, 0, len(requiredSecrets))
		for secretName := range requiredSecrets {
			secretNames = append(secretNames, secretName)
		}
		if err := ensureSecretsInProdEnv(secretNames); err != nil {
			log.Printf("Warning: Failed to ensure secrets in prod.env: %v", err)
		}
	}
}

// generateRandomPassword generates a secure random password using safe characters
// Characters: A-Z, a-z, 0-9, ._+-
// Length: 24 characters
func generateRandomPassword(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._+-"
	password := make([]byte, length)

	for i := range password {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		password[i] = charset[num.Int64()]
	}

	return string(password), nil
}

// readProdEnv reads the prod.env file and returns a map of environment variables
func readProdEnv(filePath string) (map[string]string, error) {
	envVars := make(map[string]string)

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return empty map
			return envVars, nil
		}
		return nil, fmt.Errorf("failed to open prod.env: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			envVars[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read prod.env: %w", err)
	}

	return envVars, nil
}

// writeProdEnv writes environment variables to the prod.env file
func writeProdEnv(filePath string, envVars map[string]string) error {
	// Create a sorted list of keys for consistent output
	keys := make([]string, 0, len(envVars))
	for key := range envVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Create or truncate the file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create prod.env: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// Write header comment
	fmt.Fprintln(writer, "# Auto-generated secrets for Docker Compose")
	fmt.Fprintln(writer, "# This file is managed automatically by composectl")
	fmt.Fprintln(writer, "# Do not edit manually unless you know what you are doing")
	fmt.Fprintln(writer, "")

	// Write all environment variables
	for _, key := range keys {
		fmt.Fprintf(writer, "%s=%s\n", key, envVars[key])
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to write prod.env: %w", err)
	}

	return nil
}

// ensureSecretsInProdEnv ensures all required secrets exist in prod.env file
// Creates missing secrets with randomly generated passwords
func ensureSecretsInProdEnv(secretNames []string) error {
	const prodEnvPath = "prod.env"
	const passwordLength = 24

	// Read existing prod.env
	envVars, err := readProdEnv(prodEnvPath)
	if err != nil {
		return err
	}

	modified := false

	// Check each secret
	for _, secretName := range secretNames {
		if _, exists := envVars[secretName]; !exists {
			// Generate a new password
			password, err := generateRandomPassword(passwordLength)
			if err != nil {
				return fmt.Errorf("failed to generate password for %s: %w", secretName, err)
			}

			envVars[secretName] = password
			modified = true
			log.Printf("Generated new secret '%s' in prod.env", secretName)
		} else {
			log.Printf("Secret '%s' already exists in prod.env", secretName)
		}
	}

	// Write back to file if modified
	if modified {
		if err := writeProdEnv(prodEnvPath, envVars); err != nil {
			return err
		}
		log.Printf("Updated prod.env with %d new secret(s)", len(secretNames))
	}

	return nil
}

// enrichComposeWithoutSideEffects enriches a docker-compose YAML with Traefik labels,
// networks, volumes, secrets, etc. but does NOT modify files on disk or create secrets in prod.env
func enrichComposeWithoutSideEffects(yamlContent []byte) ([]byte, error) {
	// Parse the YAML
	var compose ComposeFile
	if err := yaml.Unmarshal(yamlContent, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Process secrets without side effects (no prod.env modification)
	processSecretsWithoutSideEffects(&compose)

	// Enrich each service with Traefik labels and other auto-additions
	enrichServices(&compose)

	// Marshal back to YAML with multiline string support
	var buf strings.Builder
	if err := encodeYAMLWithMultiline(&buf, compose); err != nil {
		return nil, err
	}

	return []byte(buf.String()), nil
}

// processSecretsWithoutSideEffects scans environment variables for /run/secrets/ references
// and ensures the corresponding secrets are declared at both service and top level
// WITHOUT creating entries in prod.env
func processSecretsWithoutSideEffects(compose *ComposeFile) {
	// Track all secrets that need to be declared at top level
	requiredSecrets := make(map[string]bool)

	// Process each service
	for serviceName, service := range compose.Services {
		// Track secrets needed by this service
		serviceSecrets := make(map[string]bool)

		// Scan environment variables for /run/secrets/ references
		envArray := normalizeEnvironment(service.Environment)
		for _, envVar := range envArray {
			// Parse the environment variable
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) != 2 {
				continue
			}

			value := parts[1]

			// Check if the value matches /run/secrets/XXXX pattern
			if strings.HasPrefix(value, "/run/secrets/") {
				secretName := strings.TrimPrefix(value, "/run/secrets/")
				if secretName != "" {
					serviceSecrets[secretName] = true
					requiredSecrets[secretName] = true
				}
			}
		}

		// Add secrets to service if needed
		if len(serviceSecrets) > 0 {
			// Get existing service secrets
			existingSecrets := make(map[string]bool)
			for _, secret := range service.Secrets {
				existingSecrets[secret] = true
			}

			// Add missing secrets to service
			for secretName := range serviceSecrets {
				if !existingSecrets[secretName] {
					service.Secrets = append(service.Secrets, secretName)
					log.Printf("Auto-added secret '%s' to service '%s' (enrichment only)", secretName, serviceName)
				}
			}

			// Update the service in the compose file
			compose.Services[serviceName] = service
		}
	}

	// Initialize top-level secrets map if needed
	if compose.Secrets == nil {
		compose.Secrets = make(map[string]ComposeSecret)
	}

	// Add missing secrets at top level
	for secretName := range requiredSecrets {
		if _, exists := compose.Secrets[secretName]; !exists {
			compose.Secrets[secretName] = ComposeSecret{
				Name:        secretName,
				Environment: secretName,
			}
			log.Printf("Auto-added top-level secret declaration for '%s' (enrichment only)", secretName)
		}
	}

	// NOTE: We do NOT call ensureSecretsInProdEnv here - that's the key difference!
}

// detectHTTPPort detects HTTP/HTTPS ports from service configuration
// Returns: detectedPort, isHTTPS, found
func detectHTTPPort(service ComposeService) (string, bool, bool) {
	httpsOnlyPorts := []string{"443", "8443"}
	httpPorts := []string{"80", "8080", "3000", "4200", "5000", "8000", "8081", "8888", "9000"}
	standardHTTPPorts := append(httpPorts, httpsOnlyPorts...)

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

		// Check if it's an HTTPS port
		for _, httpsPort := range httpsOnlyPorts {
			if containerPort == httpsPort {
				isHTTPS = true
				detectedPort = containerPort
				return detectedPort, isHTTPS, true
			}
		}

		// Check if it's a common HTTP port
		for _, httpPort := range httpPorts {
			if containerPort == httpPort {
				detectedPort = containerPort
				return detectedPort, isHTTPS, true
			}
		}
	}

	// Check in environment variables if port not found yet
	if service.Environment != nil {
		envArray := normalizeEnvironment(service.Environment)
		for _, env := range envArray {
			envUpper := strings.ToUpper(env)
			for _, httpPort := range standardHTTPPorts {
				if strings.Contains(envUpper, "PORT="+httpPort) ||
					strings.Contains(envUpper, "PORT:"+httpPort) ||
					strings.Contains(envUpper, ":"+httpPort) {
					detectedPort = httpPort
					// Check if it's HTTPS
					for _, httpsPort := range httpsOnlyPorts {
						if httpPort == httpsPort {
							isHTTPS = true
							break
						}
					}
					return detectedPort, isHTTPS, true
				}
			}
		}
	}

	return "", false, false
}

// generateTraefikLabels generates the standard Traefik labels for a service
func generateTraefikLabels(serviceName string, port string, scheme string) map[string]string {
	return map[string]string{
		"traefik.enable": "true",
		"traefik.http.routers." + serviceName + ".entrypoints":                 "https",
		"traefik.http.routers." + serviceName + ".rule":                        fmt.Sprintf("Host(`%s.localhost`) || Host(`%s.${PUBLIC_DOMAIN_NAME}`) || Host(`%s`)", serviceName, serviceName, serviceName),
		"traefik.http.routers." + serviceName + ".service":                     serviceName,
		"traefik.http.routers." + serviceName + ".tls":                         "true",
		"traefik.http.services." + serviceName + ".loadbalancer.server.port":   port,
		"traefik.http.services." + serviceName + ".loadbalancer.server.scheme": scheme,
	}
}

// addTraefikLabels adds Traefik labels to a labels map if they don't already exist
// Returns true if any labels were added
func addTraefikLabels(labelsMap map[string]string, serviceName string, port string, scheme string) bool {
	traefikLabels := generateTraefikLabels(serviceName, port, scheme)

	added := false
	// Only add labels that don't already exist
	for key, value := range traefikLabels {
		if _, exists := labelsMap[key]; !exists {
			labelsMap[key] = value
			added = true
		}
	}

	return added
}

// addTraefikLabelsInterface adds Traefik labels to a labels map[string]interface{} if they don't already exist
// Returns true if any labels were added
func addTraefikLabelsInterface(labelsMap map[string]interface{}, serviceName string, port string, scheme string) bool {
	traefikLabels := generateTraefikLabels(serviceName, port, scheme)

	added := false
	// Only add labels that don't already exist
	for key, value := range traefikLabels {
		if _, exists := labelsMap[key]; !exists {
			labelsMap[key] = value
			added = true
		}
	}

	return added
}

// enrichServices enriches all services in a compose file with Traefik labels,
// networks, volumes, sysctls, capabilities, timezone mounts, etc.
func enrichServices(compose *ComposeFile) {
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
			addTraefikLabels(labelsMap, serviceName, customPort, customScheme)
		} else if hasTraefikLabels {
			// Only auto-add Traefik configuration if the service already has Traefik labels
			// Detect port from service configuration
			detectedPort, isHTTPS, found := detectHTTPPort(service)

			// Only add full Traefik configuration if we detected an HTTP/HTTPS port
			if found && detectedPort != "" {
				scheme := "http"
				if isHTTPS {
					scheme = "https"
				}
				addTraefikLabels(labelsMap, serviceName, detectedPort, scheme)
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

		// Automatically add homelab network if not present
		hasHomelabNetwork := false
		networksList := []string{}

		// Parse existing networks
		if service.Networks != nil {
			switch v := service.Networks.(type) {
			case []interface{}:
				// Networks as array
				for _, net := range v {
					if netStr, ok := net.(string); ok {
						networksList = append(networksList, netStr)
						if netStr == "homelab" {
							hasHomelabNetwork = true
						}
					}
				}
			case []string:
				// Networks as string array
				for _, netStr := range v {
					networksList = append(networksList, netStr)
					if netStr == "homelab" {
						hasHomelabNetwork = true
					}
				}
			case map[string]interface{}:
				// Networks as map
				for netName := range v {
					networksList = append(networksList, netName)
					if netName == "homelab" {
						hasHomelabNetwork = true
					}
				}
			case string:
				// Single network as string
				networksList = append(networksList, v)
				if v == "homelab" {
					hasHomelabNetwork = true
				}
			}
		}

		// Add homelab network if not present
		if !hasHomelabNetwork {
			networksList = append(networksList, "homelab")
			service.Networks = networksList
			log.Printf("Auto-added homelab network to service %s", serviceName)
		}

		// Automatically add timezone mounts if files exist on host and not already mounted
		timezoneMounts := []string{
			"/etc/localtime:/etc/localtime:ro",
			"/etc/timezone:/etc/timezone:ro",
		}

		for _, mount := range timezoneMounts {
			// Extract the host path (first part before colon)
			mountParts := strings.Split(mount, ":")
			if len(mountParts) < 2 {
				continue
			}
			hostPath := mountParts[0]
			containerPath := mountParts[1]

			// Check if file exists on host
			if _, err := os.Stat(hostPath); err == nil {
				// File exists, check if it's already in volumes
				alreadyMounted := false

				for _, existingVolume := range service.Volumes {
					// Check if this source or target path is already mounted
					volumeParts := strings.Split(existingVolume, ":")
					if len(volumeParts) >= 2 {
						existingSource := volumeParts[0]
						existingTarget := volumeParts[1]

						// Check if either the source or target matches
						if existingSource == hostPath || existingTarget == containerPath {
							alreadyMounted = true
							break
						}
					}
				}

				if !alreadyMounted {
					service.Volumes = append(service.Volumes, mount)
					log.Printf("Auto-added timezone mount %s to service %s", mount, serviceName)
				}
			}
		}

		// Automatically add TZ environment variable if not already set
		hasTZ := false
		envArray := normalizeEnvironment(service.Environment)
		for _, env := range envArray {
			// Check if TZ is already defined
			if strings.HasPrefix(env, "TZ=") {
				hasTZ = true
				break
			}
		}

		if !hasTZ {
			envArray = append(envArray, "TZ=${TZ}")
			setEnvironmentAsArray(&service, envArray)
			log.Printf("Auto-added TZ=${TZ} environment variable to service %s", serviceName)
		}

		// Automatically set container_name based on image if not already defined
		if service.ContainerName == "" && service.Image != "" {
			// Extract container name from image
			// Examples:
			// "nginx:latest" -> "nginx"
			// "docker.io/library/nginx:latest" -> "nginx"
			// "ghcr.io/owner/repo:tag" -> "repo"
			// "registry.example.com:5000/path/to/image:version" -> "image"

			imageName := service.Image

			// Remove registry prefix (anything before the last /)
			parts := strings.Split(imageName, "/")
			imageName = parts[len(parts)-1]

			// Remove version/tag (anything after :)
			imageName = strings.Split(imageName, ":")[0]

			// Remove @sha256 digest if present
			imageName = strings.Split(imageName, "@")[0]

			if imageName != "" {
				service.ContainerName = imageName
				log.Printf("Auto-set container_name=%s for service %s based on image %s", imageName, serviceName, service.Image)
			}
		}

		// Add default resource limits if not specified
		if service.MemLimit == "" {
			service.MemLimit = "256m"
			log.Printf("Auto-set mem_limit=256m for service %s", serviceName)
		}

		if service.MemswapLimit == 0 {
			service.MemswapLimit = 0
			log.Printf("Auto-set memswap_limit=0 for service %s", serviceName)
		}

		if service.CPUs == nil {
			service.CPUs = "0.5"
			log.Printf("Auto-set cpus=0.5 for service %s", serviceName)
		}

		// Add default logging configuration if not specified
		if service.Logging == nil {
			service.Logging = &LoggingConfig{
				Driver: "json-file",
				Options: map[string]string{
					"max-size": "10m",
					"max-file": "3",
				},
			}
			log.Printf("Auto-set logging driver=json-file with max-size=10m and max-file=3 for service %s", serviceName)
		} else if service.Logging.Driver == "json-file" {
			// If driver is json-file but options are missing, fill in the defaults
			if service.Logging.Options == nil {
				service.Logging.Options = make(map[string]string)
			}
			if _, exists := service.Logging.Options["max-size"]; !exists {
				service.Logging.Options["max-size"] = "10m"
				log.Printf("Auto-set logging max-size=10m for service %s", serviceName)
			}
			if _, exists := service.Logging.Options["max-file"]; !exists {
				service.Logging.Options["max-file"] = "3"
				log.Printf("Auto-set logging max-file=3 for service %s", serviceName)
			}
		}

		compose.Services[serviceName] = service
	}

	// Add undeclared networks and volumes
	addUndeclaredNetworksAndVolumes(compose)
}

// getDockerSocketPath returns the appropriate Docker socket path
// Checks if /run/user/<UID>/docker.sock exists, otherwise returns /var/run/docker.sock
func getDockerSocketPath() string {
	// Get the current user's UID
	uid := os.Getuid()
	userSocket := fmt.Sprintf("/run/user/%d/docker.sock", uid)

	// Check if user-specific socket exists
	if _, err := os.Stat(userSocket); err == nil {
		return userSocket
	}

	// Fallback to system socket
	return "/var/run/docker.sock"
}

// getCurrentUserID returns the current user's UID as a string
func getCurrentUserID() string {
	return fmt.Sprintf("%d", os.Getuid())
}

// getCurrentGroupID returns the current user's GID as a string
func getCurrentGroupID() string {
	return fmt.Sprintf("%d", os.Getgid())
}

// replacePlaceholders replaces DOCKER_SOCK, DOCKER_SOCKET, USER_ID, and USER_GID placeholders with the appropriate values
// Handles both literal strings and environment variable syntax (${VAR}, $VAR)
func replacePlaceholders(compose *ComposeFile) {
	dockerSocket := getDockerSocketPath()
	userID := getCurrentUserID()
	groupID := getCurrentGroupID()

	// Process each service
	for serviceName, service := range compose.Services {
		// Replace in volumes
		if len(service.Volumes) > 0 {
			for i, volume := range service.Volumes {
				// Replace both literal and environment variable syntax
				volume = strings.ReplaceAll(volume, "${DOCKER_SOCK}", dockerSocket)
				volume = strings.ReplaceAll(volume, "${DOCKER_SOCKET}", dockerSocket)
				volume = strings.ReplaceAll(volume, "$DOCKER_SOCK", dockerSocket)
				volume = strings.ReplaceAll(volume, "$DOCKER_SOCKET", dockerSocket)
				volume = strings.ReplaceAll(volume, "${USER_ID}", userID)
				volume = strings.ReplaceAll(volume, "$USER_ID", userID)
				volume = strings.ReplaceAll(volume, "${USERID}", userID)
				volume = strings.ReplaceAll(volume, "$USERID", userID)
				volume = strings.ReplaceAll(volume, "${USER_GID}", groupID)
				volume = strings.ReplaceAll(volume, "$USER_GID", groupID)
				volume = strings.ReplaceAll(volume, "${USERGID}", groupID)
				volume = strings.ReplaceAll(volume, "$USERGID", groupID)
				volume = strings.ReplaceAll(volume, "${UID}", groupID)
				volume = strings.ReplaceAll(volume, "$UID", groupID)
				volume = strings.ReplaceAll(volume, "${GID}", groupID)
				volume = strings.ReplaceAll(volume, "$GID", groupID)
				service.Volumes[i] = volume
			}
		}

		// Replace in environment variables
		if service.Environment != nil {
			// Handle map format
			if envMap, ok := service.Environment.(map[string]interface{}); ok {
				for key, value := range envMap {
					if strValue, ok := value.(string); ok {
						strValue = strings.ReplaceAll(strValue, "${DOCKER_SOCK}", dockerSocket)
						strValue = strings.ReplaceAll(strValue, "${DOCKER_SOCKET}", dockerSocket)
						strValue = strings.ReplaceAll(strValue, "$DOCKER_SOCK", dockerSocket)
						strValue = strings.ReplaceAll(strValue, "$DOCKER_SOCKET", dockerSocket)
						strValue = strings.ReplaceAll(strValue, "${USER_ID}", userID)
						strValue = strings.ReplaceAll(strValue, "$USER_ID", userID)
						strValue = strings.ReplaceAll(strValue, "${USER_GID}", groupID)
						strValue = strings.ReplaceAll(strValue, "$USER_GID", groupID)
						envMap[key] = strValue
					}
				}
			}
			// Handle array format
			if envArray, ok := service.Environment.([]interface{}); ok {
				for i, item := range envArray {
					if strValue, ok := item.(string); ok {
						strValue = strings.ReplaceAll(strValue, "${DOCKER_SOCK}", dockerSocket)
						strValue = strings.ReplaceAll(strValue, "${DOCKER_SOCKET}", dockerSocket)
						strValue = strings.ReplaceAll(strValue, "$DOCKER_SOCK", dockerSocket)
						strValue = strings.ReplaceAll(strValue, "$DOCKER_SOCKET", dockerSocket)
						strValue = strings.ReplaceAll(strValue, "${USER_ID}", userID)
						strValue = strings.ReplaceAll(strValue, "$USER_ID", userID)
						strValue = strings.ReplaceAll(strValue, "${USER_GID}", groupID)
						strValue = strings.ReplaceAll(strValue, "$USER_GID", groupID)
						envArray[i] = strValue
					}
				}
			}
		}

		// Update the service back to the map
		compose.Services[serviceName] = service
	}

	log.Printf("Replaced placeholders - DOCKER_SOCK/DOCKER_SOCKET: %s, USER_ID: %s, USER_GID: %s", dockerSocket, userID, groupID)
}

func enrichComposeWithTraefikLabels(yamlContent []byte) ([]byte, error) {
	// Parse the YAML
	var compose ComposeFile
	if err := yaml.Unmarshal(yamlContent, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Process secrets first
	processSecrets(&compose)

	// Replace placeholders (DOCKER_SOCK, DOCKER_SOCKET, etc.)
	replacePlaceholders(&compose)

	// Enrich services
	enrichServices(&compose)

	// Marshal back to YAML with multiline string support
	var buf strings.Builder
	if err := encodeYAMLWithMultiline(&buf, compose); err != nil {
		return nil, err
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
