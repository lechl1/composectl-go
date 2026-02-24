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
	"regexp"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Stack represents a Docker Compose stack with its containers
type Stack struct {
	Name       string          `json:"name"`
	Containers []DockerInspect `json:"containers"`
}

// detectHTTPPort inspects a service and tries to find a reasonable HTTP/HTTPS port
// Returns (portString, isHTTPS, usesHTTPPort)
func detectHTTPPort(service ComposeService) (string, bool, bool) {
	// Check explicit ports first
	for _, p := range service.Ports {
		// port formats: host:container, container, container/proto
		parts := strings.Split(p, ":")
		cand := parts[len(parts)-1]
		cand = strings.Split(cand, "/")[0]
		if cand != "" {
			isHTTPS := (cand == "443" || cand == "8443")
			return cand, isHTTPS, true
		}
	}

	// Check environment variables for common port names
	envArr := normalizeEnvironment(service.Environment)
	for _, env := range envArr {
		if strings.Contains(strings.ToUpper(env), "PORT=") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				cand := extractPortNumber(parts[1])
				if cand > 0 {
					portStr := fmt.Sprintf("%d", cand)
					isHTTPS := (portStr == "443" || portStr == "8443")
					return portStr, isHTTPS, true
				}
			}
		}
	}

	return "", false, false
}

// addTraefikLabelsInterface adds a minimal set of Traefik labels into a generic labels map
func addTraefikLabelsInterface(labels map[string]interface{}, serviceName, port, scheme string) {
	if labels == nil {
		return
	}
	// Add simple router rule and service port label
	routerKey := fmt.Sprintf("traefik.http.routers.%s.rule", serviceName)
	labels[routerKey] = fmt.Sprintf("Host(`%s`)", serviceName)
	servicePortKey := fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName)
	labels[servicePortKey] = port
	// Add entrypoint based on scheme
	entrypointKey := fmt.Sprintf("traefik.http.routers.%s.entrypoints", serviceName)
	if scheme == "https" {
		labels[entrypointKey] = "https"
	} else {
		labels[entrypointKey] = "http"
	}
}

// getDockerSocketPath returns a sensible docker socket path
func getDockerSocketPath() string {
	if v := os.Getenv("DOCKER_SOCK"); v != "" {
		return v
	}
	return "/var/run/docker.sock"
}

// getCurrentUserID returns current user id as string
func getCurrentUserID() string {
	return fmt.Sprintf("%d", os.Geteuid())
}

// getCurrentGroupID returns current group id as string
func getCurrentGroupID() string {
	return fmt.Sprintf("%d", os.Getegid())
}

// replacePlaceholders replaces placeholders like ${DOCKER_SOCK}, ${USER_ID}, ${USER_GID}
func replacePlaceholders(compose *ComposeFile) {
	dockerSocket := getDockerSocketPath()
	userID := getCurrentUserID()
	groupID := getCurrentGroupID()

	for name, service := range compose.Services {
		// Volumes
		for i, vol := range service.Volumes {
			vol = strings.ReplaceAll(vol, "${DOCKER_SOCK}", dockerSocket)
			vol = strings.ReplaceAll(vol, "${DOCKER_SOCKET}", dockerSocket)
			vol = strings.ReplaceAll(vol, "$DOCKER_SOCK", dockerSocket)
			vol = strings.ReplaceAll(vol, "$DOCKER_SOCKET", dockerSocket)
			vol = strings.ReplaceAll(vol, "${USER_ID}", userID)
			vol = strings.ReplaceAll(vol, "$USER_ID", userID)
			vol = strings.ReplaceAll(vol, "${USER_GID}", groupID)
			vol = strings.ReplaceAll(vol, "$USER_GID", groupID)
			service.Volumes[i] = vol
		}

		// Environment map/array
		if service.Environment != nil {
			switch v := service.Environment.(type) {
			case map[string]interface{}:
				for k, val := range v {
					if s, ok := val.(string); ok {
						v[k] = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "${DOCKER_SOCK}", dockerSocket), "${USER_ID}", userID), "${USER_GID}", groupID)
					}
				}
				service.Environment = v
			case []interface{}:
				for i, item := range v {
					if s, ok := item.(string); ok {
						v[i] = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "${DOCKER_SOCK}", dockerSocket), "${USER_ID}", userID), "${USER_GID}", groupID)
					}
				}
				service.Environment = v
			}
		}

		compose.Services[name] = service
	}
}

// enrichAndSanitizeCompose enriches and sanitizes a compose structure
// If dryRun is true, it will not write to prod.env or files
// NOTE: This function now operates in-place on the provided ComposeFile and does NOT
// perform any YAML serialization or return any bytes. Serialization is the caller's
// responsibility so it can decide when to write or return YAML (for example only inside !dryRun).
func enrichAndSanitizeCompose(compose *ComposeFile, dryRun bool) {
	// operate directly on the provided ComposeFile struct

	// Process secrets with or without side effects based on dryRun
	processSecrets(compose, dryRun)

	// Replace placeholders (DOCKER_SOCK, DOCKER_SOCKET, etc.)
	replacePlaceholders(compose)

	// Add undeclared networks/volumes
	addUndeclaredNetworksAndVolumes(compose)

	// Sanitize passwords with or without extraction based on dryRun
	sanitizeComposePasswords(compose, dryRun)
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
	External   bool              `yaml:"external,omitempty"`
	Driver     string            `yaml:"driver,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
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

// ComposeAction represents the action to perform on a compose stack
type ComposeAction int

const (
	ComposeActionNone ComposeAction = iota
	ComposeActionUp   ComposeAction = iota
	ComposeActionDown ComposeAction = iota
	ComposeActionStop ComposeAction = iota
)

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

// getStacksList returns a combined list of running stacks from Docker and available YAML files
func getStacksList() ([]Stack, error) {
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
		runningStackNames[stack.Name] = true
	}

	// Add YAML stacks that are not running (with simulated containers)
	for stackName, filePath := range ymlStacks {
		if !runningStackNames[stackName] {
			// Parse YAML file and create simulated containers
			simulatedContainers, err := createSimulatedContainers(stackName, filePath, allContainers)
			if err != nil {
				log.Printf("Error creating simulated containers for %s: %v", stackName, err)
				// Still add the stack but with empty containers
				runningStacks = append(runningStacks, Stack{
					Name:       stackName,
					Containers: []DockerInspect{},
				})
			} else {
				runningStacks = append(runningStacks, Stack{
					Name:       stackName,
					Containers: simulatedContainers,
				})
			}
		}
	}

	return runningStacks, nil
}

// streamCommandOutput executes a command and streams its stdout and stderr to the HTTP response
// using chunked transfer encoding. Returns error if command execution fails.
// Note: Headers should be set by the caller before calling this function if multiple commands are streamed.
func streamCommandOutput(w http.ResponseWriter, cmd *exec.Cmd) error {

	// Get pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Use WaitGroup to wait for both streams to complete
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(w, "[STDOUT] %s\n", line)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}()

	// Stream stderr
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(w, "[STDERR] %s\n", line)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}()

	// Wait for both streams to complete
	wg.Wait()

	// Wait for command to finish and get exit status
	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(w, "[ERROR] Command failed: %v\n", err)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return err
	}

	fmt.Fprintf(w, "[DONE] Command completed successfully\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
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
// Uses raw docker inspect JSON format with lowercase keys
func createSimulatedContainers(stackName, filePath string, allContainers []map[string]interface{}) ([]DockerInspect, error) {
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

	// Get existing container IDs for inspection
	var existingContainerIDs []string
	containerNameToID := make(map[string]string)
	for _, container := range allContainers {
		if names, ok := container["Names"].(string); ok {
			normalizedName := strings.TrimPrefix(names, "/")
			if id, ok := container["ID"].(string); ok {
				containerNameToID[normalizedName] = id
				existingContainerIDs = append(existingContainerIDs, id)
			}
		}
	}

	// Inspect existing containers to get full details
	var inspectedContainers []DockerInspect
	if len(existingContainerIDs) > 0 {
		inspectData, err := inspectContainers(existingContainerIDs)
		if err != nil {
			log.Printf("Warning: failed to inspect containers: %v", err)
		} else {
			inspectedContainers = inspectData
		}
	}

	// Create a map of inspected containers by name for quick lookup
	inspectedMap := make(map[string]DockerInspect)
	for _, inspected := range inspectedContainers {
		normalizedName := strings.TrimPrefix(inspected.Name, "/")
		inspectedMap[normalizedName] = inspected
	}

	var containers []DockerInspect

	// Create a simulated container for each service
	for serviceName, service := range compose.Services {
		containerName := service.ContainerName
		if containerName == "" {
			containerName = serviceName
		}

		// Check if this container actually exists in Docker
		if inspectedData, exists := inspectedMap[containerName]; exists {
			// Use the real docker inspect data
			containers = append(containers, inspectedData)
		} else {
			// Create a simulated docker inspect format container
			// Build labels map
			labels := make(map[string]string)
			labels["com.docker.compose.project"] = stackName
			labels["com.docker.compose.service"] = serviceName
			labels["com.docker.compose.oneoff"] = "False"

			// Add custom labels from the service definition
			if service.Labels != nil {
				switch v := service.Labels.(type) {
				case []interface{}:
					for _, label := range v {
						if labelStr, ok := label.(string); ok {
							if parts := strings.SplitN(labelStr, "=", 2); len(parts) == 2 {
								labels[parts[0]] = parts[1]
							}
						}
					}
				case map[string]interface{}:
					for k, val := range v {
						labels[k] = fmt.Sprintf("%v", val)
					}
				}
			}

			// Build command array
			var cmd []string
			switch v := service.Command.(type) {
			case string:
				cmd = []string{v}
			case []interface{}:
				for _, c := range v {
					if s, ok := c.(string); ok {
						cmd = append(cmd, s)
					}
				}
			}

			// Build environment array
			var env []string
			switch v := service.Environment.(type) {
			case []interface{}:
				for _, e := range v {
					if s, ok := e.(string); ok {
						env = append(env, s)
					}
				}
			case map[string]interface{}:
				for k, val := range v {
					env = append(env, fmt.Sprintf("%s=%v", k, val))
				}
			}

			// Build mounts from volumes
			var mounts []Mount
			for _, volume := range service.Volumes {
				parts := strings.Split(volume, ":")
				mountType := "volume"
				source := ""
				destination := ""

				if len(parts) >= 2 {
					source = parts[0]
					destination = parts[1]
					// If source starts with / or ./, it's a bind mount
					if strings.HasPrefix(source, "/") || strings.HasPrefix(source, "./") {
						mountType = "bind"
					}
				}

				mounts = append(mounts, Mount{
					Type:        mountType,
					Source:      source,
					Destination: destination,
					Mode:        "",
					RW:          true,
					Propagation: "rprivate",
				})
			}

			// Build networks
			networks := make(map[string]EndpointSettings)
			switch v := service.Networks.(type) {
			case []interface{}:
				for _, net := range v {
					if netStr, ok := net.(string); ok {
						networks[netStr] = EndpointSettings{}
					}
				}
			case map[string]interface{}:
				for net := range v {
					networks[net] = EndpointSettings{}
				}
			}

			// Build exposed ports and port bindings
			exposedPorts := make(map[string]interface{})
			portBindings := make(map[string][]PortBinding)
			for _, portStr := range service.Ports {
				// Parse port format: "host:container" or "container"
				parts := strings.Split(portStr, ":")
				containerPort := ""
				hostPort := ""

				if len(parts) == 2 {
					hostPort = parts[0]
					containerPort = parts[1]
				} else if len(parts) == 1 {
					containerPort = parts[0]
				}

				// Add protocol if not present
				if !strings.Contains(containerPort, "/") {
					containerPort = containerPort + "/tcp"
				}

				exposedPorts[containerPort] = struct{}{}

				if hostPort != "" {
					portBindings[containerPort] = []PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: hostPort,
						},
					}
				}
			}

			// Create simulated docker inspect format
			container := DockerInspect{
				ID:      "",
				Created: "",
				Path:    "",
				Args:    []string{},
				State: ContainerState{
					Status:     "created",
					Running:    false,
					Paused:     false,
					Restarting: false,
					OOMKilled:  false,
					Dead:       false,
					Pid:        0,
					ExitCode:   0,
					Error:      "",
					StartedAt:  "",
					FinishedAt: "",
				},
				Image:           service.Image,
				ResolvConfPath:  "",
				HostnamePath:    "",
				HostsPath:       "",
				LogPath:         "",
				Name:            "/" + containerName,
				RestartCount:    0,
				Driver:          "overlay2",
				Platform:        "linux",
				MountLabel:      "",
				ProcessLabel:    "",
				AppArmorProfile: "",
				ExecIDs:         nil,
				HostConfig: HostConfig{
					Binds:           service.Volumes,
					ContainerIDFile: "",
					LogConfig: LogConfig{
						Type:   "json-file",
						Config: map[string]string{},
					},
					NetworkMode:  "default",
					PortBindings: portBindings,
					RestartPolicy: RestartPolicy{
						Name:              "no",
						MaximumRetryCount: 0,
					},
					AutoRemove:           false,
					VolumeDriver:         "",
					VolumesFrom:          nil,
					CapabilityAdd:        nil,
					CapabilityDrop:       nil,
					DNS:                  []string{},
					DNSOptions:           []string{},
					DNSSearch:            []string{},
					ExtraHosts:           nil,
					GroupAdd:             nil,
					IpcMode:              "private",
					Cgroup:               "",
					Links:                nil,
					OomScoreAdj:          0,
					PidMode:              "",
					Privileged:           false,
					PublishAllPorts:      false,
					ReadonlyRootfs:       false,
					SecurityOpt:          nil,
					UTSMode:              "",
					UsernsMode:           "",
					ShmSize:              67108864,
					Runtime:              "runc",
					ConsoleSize:          []int{0, 0},
					Isolation:            "",
					CPUShares:            0,
					Memory:               0,
					NanoCPUs:             0,
					CgroupParent:         "",
					BlkioWeight:          0,
					BlkioWeightDevice:    nil,
					BlkioDeviceReadBps:   nil,
					BlkioDeviceWriteBps:  nil,
					BlkioDeviceReadIOps:  nil,
					BlkioDeviceWriteIOps: nil,
					CPUPeriod:            0,
					CPUQuota:             0,
					CPURealtimePeriod:    0,
					CPURealtimeRuntime:   0,
					CpusetCpus:           "",
					CpusetMems:           "",
					Devices:              nil,
					DeviceCgroupRules:    nil,
					DiskQuota:            0,
					KernelMemory:         0,
					MemoryReservation:    0,
					MemorySwap:           0,
					MemorySwappiness:     nil,
					OomKillDisable:       nil,
					PidsLimit:            nil,
					Ulimits:              nil,
					CPUCount:             0,
					CPUPercent:           0,
					IOMaximumIOps:        0,
					IOMaximumBandwidth:   0,
				},
				GraphDriver: GraphDriver{
					Name: "overlay2",
					Data: map[string]string{
						"lowerdir":  "",
						"mergeddir": "",
						"upperdir":  "",
						"workdir":   "",
					},
				},
				Mounts: mounts,
				Config: ContainerConfig{
					Hostname:     containerName,
					Domainname:   "",
					User:         "",
					AttachStdin:  false,
					AttachStdout: false,
					AttachStderr: false,
					ExposedPorts: exposedPorts,
					Tty:          false,
					OpenStdin:    false,
					StdinOnce:    false,
					Env:          env,
					Cmd:          cmd,
					Image:        service.Image,
					Volumes:      nil,
					WorkingDir:   "",
					Entrypoint:   nil,
					OnBuild:      nil,
					Labels:       labels,
				},
				NetworkSettings: NetworkSettings{
					Bridge:                 "",
					SandboxID:              "",
					HairpinMode:            false,
					LinkLocalIPv6Address:   "",
					LinkLocalIPv6PrefixLen: 0,
					Ports:                  portBindings,
					SandboxKey:             "",
					SecondaryIPAddresses:   nil,
					SecondaryIPv6Addresses: nil,
					EndpointID:             "",
					Gateway:                "",
					GlobalIPv6Address:      "",
					GlobalIPv6PrefixLen:    0,
					IPAddress:              "",
					IPPrefixLen:            0,
					IPv6Gateway:            "",
					MacAddress:             "",
					Networks:               networks,
				},
			}

			containers = append(containers, container)
		}
	}

	return containers, nil
}

// getRunningStacks executes docker ps and returns stacks grouped by compose project
func getRunningStacks() ([]Stack, error) {
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
	stacksMap := make(map[string][]string) // projectName -> []containerIDs

	for _, container := range containers {
		projectName := "none"
		if labels, ok := container["Labels"].(map[string]interface{}); ok {
			if project, ok := labels["com.docker.compose.project"].(string); ok && project != "" {
				projectName = project
			}
		}

		if id, ok := container["ID"].(string); ok {
			stacksMap[projectName] = append(stacksMap[projectName], id)
		}
	}

	// Inspect all containers and group by stack
	var stacks []Stack
	for projectName, containerIDs := range stacksMap {
		// Inspect containers to get full details
		inspectedContainers, err := inspectContainers(containerIDs)
		if err != nil {
			log.Printf("Warning: failed to inspect containers for stack %s: %v", projectName, err)
			// Add stack with empty containers on error
			stacks = append(stacks, Stack{
				Name:       projectName,
				Containers: []DockerInspect{},
			})
			continue
		}

		stacks = append(stacks, Stack{
			Name:       projectName,
			Containers: inspectedContainers,
		})
	}

	return stacks, nil
}

// getEffectiveComposeFile returns the path to the effective compose file for a stack
// If the effective file exists, it returns that path; otherwise, it returns the regular .yml path
func getEffectiveComposeFile(stackName string) string {
	effectivePath := GetStackPath(stackName, true)
	regularPath := GetStackPath(stackName, false)

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
	if r.Method != http.MethodPut {
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

	HandleDockerComposeFile(w, r, stackName, false, ComposeActionStop)
}

// HandleStartStack handles POST /api/stacks/{name}/start
// Starts all containers in a Docker Compose stack
func HandleStartStack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
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
	HandleDockerComposeFile(w, r, stackName, false, ComposeActionUp)
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
func inspectContainers(containerIDs []string) ([]DockerInspect, error) {
	if len(containerIDs) == 0 {
		return []DockerInspect{}, nil
	}

	args := append([]string{"inspect"}, containerIDs...)
	cmd := exec.Command("docker", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect containers: %w", err)
	}

	var inspectData []DockerInspect
	if err := json.Unmarshal(output, &inspectData); err != nil {
		return nil, fmt.Errorf("failed to parse inspect output: %w", err)
	}

	return inspectData, nil
}

// sanitizeEnvironmentVariable checks if an environment variable contains sensitive information
// and replaces its value with a variable reference in the format ${ENV_KEY}
// isSensitiveEnvironmentKey checks if an environment variable key is considered sensitive
// based on common password/secret keywords. Excludes variables with "_FILE" suffix and
// values that reference /run/secrets (Docker secrets path).
func isSensitiveEnvironmentKey(key, value string) bool {
	upperKey := strings.ToUpper(key)

	// Exclude variables with "_FILE" suffix as they are file references, not actual passwords
	if strings.Contains(upperKey, "_FILE") {
		return false
	}

	// Do not treat as sensitive if the value starts with /run/secrets (Docker secrets path)
	if strings.HasPrefix(value, "/run/secrets") {
		return false
	}

	// Check for sensitive keywords
	sensitiveKeywords := []string{"PASSWD", "PASSWORD", "SECRET", "KEY", "TOKEN", "API_KEY", "APIKEY", "PRIVATE"}
	for _, keyword := range sensitiveKeywords {
		if strings.Contains(upperKey, keyword) {
			return true
		}
	}

	return false
}

func sanitizeEnvironmentVariable(envStr string) string {
	// Split the environment variable into key and value
	parts := strings.SplitN(envStr, "=", 2)
	if len(parts) != 2 {
		return envStr
	}

	key := parts[0]
	value := parts[1]

	// Check if the key is sensitive
	if !isSensitiveEnvironmentKey(key, value) {
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
// If dryRun is true, it will skip writing to prod.env
func sanitizeComposePasswords(compose *ComposeFile, dryRun bool) {
	// Read existing prod.env
	envVars, err := readProdEnv(ProdEnvPath)
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

				// Check if this is a sensitive variable using shared helper
				isSensitive := isSensitiveEnvironmentKey(key, value)

				// If sensitive and has a value, save to prod.env
				if isSensitive && value != "" && !strings.HasPrefix(value, "${") && !strings.HasPrefix(value, "/run/secrets/") {
					normalizedKey := normalizeEnvKey(key)
					// Passwords should not be fetched from runtime environment - only save to prod.env
					if _, exists := envVars[normalizedKey]; !exists {
						// Only save if not already in prod.env
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
						// Check if variable is available in runtime environment
						if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
							log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
						} else if _, exists := envVars[normalizedVarName]; !exists {
							// Only add if not already in prod.env and not in runtime
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
								// Check if variable is available in runtime environment
								if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
									log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
								} else if _, exists := envVars[normalizedVarName]; !exists {
									// Only add if not already in prod.env and not in runtime
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
							// Check if variable is available in runtime environment
							if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
								log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
							} else if _, exists := envVars[normalizedVarName]; !exists {
								// Only add if not already in prod.env and not in runtime
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
					// Check if variable is available in runtime environment
					if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
						log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
					} else if _, exists := envVars[normalizedVarName]; !exists {
						// Only add if not already in prod.env and not in runtime
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
					// Check if variable is available in runtime environment
					if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
						log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
					} else if _, exists := envVars[normalizedVarName]; !exists {
						// Only add if not already in prod.env and not in runtime
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
					// Check if variable is available in runtime environment
					if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
						log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
					} else if _, exists := envVars[normalizedVarName]; !exists {
						// Only add if not already in prod.env and not in runtime
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
						// Check if variable is available in runtime environment
						if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
							log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
						} else if _, exists := envVars[normalizedVarName]; !exists {
							// Only add if not already in prod.env and not in runtime
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
				// Check if variable is available in runtime environment
				if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
					log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
				} else if _, exists := envVars[normalizedVarName]; !exists {
					// Only add if not already in prod.env and not in runtime
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
					// Check if variable is available in runtime environment
					if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
						log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
					} else if _, exists := envVars[normalizedVarName]; !exists {
						// Only add if not already in prod.env and not in runtime
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
				// Check if variable is available in runtime environment
				if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
					log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
				} else if _, exists := envVars[normalizedVarName]; !exists {
					// Only add if not already in prod.env and not in runtime
					envVars[normalizedVarName] = ""
					modified = true
					log.Printf("Added environment variable '%s' to prod.env from service '%s' image", normalizedVarName, serviceName)
				}
			}
		}
	}

	// Write back to prod.env if modified (skip if dry run)
	if modified && !dryRun {
		if err := writeProdEnv(ProdEnvPath, envVars); err != nil {
			log.Printf("Warning: Failed to write prod.env: %v", err)
		} else {
			log.Printf("Updated prod.env with extracted passwords and environment variables")
		}
	}
}

// reconstructComposeFromContainers creates a docker-compose YAML from container inspection data
func reconstructComposeFromContainers(inspectData []DockerInspect) (string, error) {
	compose := ComposeFile{
		Services: make(map[string]ComposeService),
		Volumes:  make(map[string]ComposeVolume),
		Networks: make(map[string]ComposeNetwork),
		Configs:  make(map[string]ComposeConfig),
		Secrets:  make(map[string]ComposeSecret),
	}

	for _, containerData := range inspectData {
		// Extract service name from labels
		labels := containerData.Config.Labels
		serviceName, ok := labels["com.docker.compose.service"]
		if !ok || serviceName == "" {
			// Fallback to container name without project prefix
			serviceName = strings.TrimPrefix(containerData.Name, "/")
			if serviceName == "" {
				continue
			}
		}

		service := ComposeService{}

		// Image
		service.Image = containerData.Config.Image

		// Container name (only set if different from service name)
		containerName := strings.TrimPrefix(containerData.Name, "/")
		if containerName != serviceName {
			service.ContainerName = containerName
		}

		// Restart policy (only set if not "unless-stopped")
		if containerData.HostConfig.RestartPolicy.Name != "" && containerData.HostConfig.RestartPolicy.Name != "unless-stopped" {
			service.Restart = containerData.HostConfig.RestartPolicy.Name
		}

		// Command
		if len(containerData.Config.Cmd) > 0 {
			service.Command = containerData.Config.Cmd
		}

		// Environment variables
		if len(containerData.Config.Env) > 0 {
			var envVars []string
			for _, envStr := range containerData.Config.Env {
				// Filter out common system environment variables that Docker adds
				// Keep only user-defined environment variables
				if !strings.HasPrefix(envStr, "PATH=") &&
					!strings.HasPrefix(envStr, "HOSTNAME=") &&
					!strings.HasPrefix(envStr, "HOME=") {
					envVars = append(envVars, sanitizeEnvironmentVariable(envStr))
				}
			}
			if len(envVars) > 0 {
				service.Environment = envVars
			}
		}

		// Ports
		for containerPort, bindings := range containerData.HostConfig.PortBindings {
			for _, binding := range bindings {
				hostPort := binding.HostPort
				if hostPort != "" {
					service.Ports = append(service.Ports, fmt.Sprintf("%s:%s", hostPort, containerPort))
				}
			}
		}

		// Volumes/Mounts
		for _, mount := range containerData.Mounts {
			mountType := mount.Type
			source := mount.Source
			destination := mount.Destination

			if mountType == "bind" {
				service.Volumes = append(service.Volumes, fmt.Sprintf("%s:%s", source, destination))
			} else if mountType == "volume" {
				volumeName := mount.Name
				if volumeName != "" {
					service.Volumes = append(service.Volumes, fmt.Sprintf("%s:%s", volumeName, destination))
				}
			}
		}

		// Networks
		var networkNames []string
		for networkName := range containerData.NetworkSettings.Networks {
			networkNames = append(networkNames, networkName)
		}
		if len(networkNames) > 0 {
			service.Networks = networkNames
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
	processSecrets(&compose, false)

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
	filePath := GetStackPath(stackName, false)

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

	HandleDockerComposeFile(w, r, stackName, false, ComposeActionNone)
}

func HandleDockerComposeFile(w http.ResponseWriter, r *http.Request, stackName string, dryRun bool, action ComposeAction) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()

	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// First, sanitize passwords and extract them to prod.env
	// This must be done BEFORE enrichment to capture plaintext passwords
	var modifiedComposeFile ComposeFile
	if err := yaml.Unmarshal(body, &modifiedComposeFile); err != nil {
		log.Printf("Error parsing YAML for sanitization: %v", err)
		http.Error(w, fmt.Sprintf("Failed to parse YAML: %v", err), http.StatusBadRequest)
		return
	}
	sanitizeComposePasswords(&modifiedComposeFile, dryRun)

	// Marshal the sanitized original version back to YAML for .yml file
	var originalComposeYamlBuffer strings.Builder
	if err := encodeYAMLWithMultiline(&originalComposeYamlBuffer, modifiedComposeFile); err != nil {
		log.Printf("Failed to serialize original YAML: %v", err)
		http.Error(w, fmt.Sprintf("Failed to serialize original YAML: %v", err), http.StatusInternalServerError)
		return
	}

	enrichAndSanitizeCompose(&modifiedComposeFile, dryRun)

	// Marshal the sanitized original version back to YAML for .yml file
	var modifiedComposeYamlBuffer strings.Builder
	if err := encodeYAMLWithMultiline(&modifiedComposeYamlBuffer, modifiedComposeFile); err != nil {
		log.Printf("Failed to serialize modified YAML: %v", err)
		http.Error(w, fmt.Sprintf("Failed to serialize modified YAML: %v", err), http.StatusInternalServerError)
		return
	}

	var cmd *exec.Cmd
	var actionName string
	// Set up streaming headers before docker modifiedComposeFile up/down
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	if dryRun {
		return
	}

	switch action {
	case ComposeActionUp:
		actionName = "up"
		// Create missing networks and volumes before docker modifiedComposeFile up/down
		if err = ensureNetworksExist(&modifiedComposeFile, w); err != nil {
			log.Printf("Error ensuring networks exist for stack %s: %v", stackName, err)
			fmt.Fprintf(w, "[ERROR] Failed to ensure networks exist: %v\n", err)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if err = ensureVolumesExist(&modifiedComposeFile, w); err != nil {
			log.Printf("Error ensuring volumes exist for stack %s: %v", stackName, err)
			fmt.Fprintf(w, "[ERROR] Failed to ensure volumes exist: %v\n", err)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}

		if modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, done := serializeYamlWithPlainTextSecrets(w, modifiedComposeFile); !done {
			cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, "up", "-d", "--wait", "--remove-orphans")
			cmd.Stdin = strings.NewReader(modifiedComposeYamlWithPlainTextSecrets)
			modifiedComposeYamlWithPlainTextSecretsBuffer.Reset()
		}
	case ComposeActionDown:
		actionName = "down"
		if modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, done := serializeYamlWithPlainTextSecrets(w, modifiedComposeFile); !done {
			cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, "down", "--wait", "--remove-orphans")
			cmd.Stdin = strings.NewReader(modifiedComposeYamlWithPlainTextSecrets)
			modifiedComposeYamlWithPlainTextSecretsBuffer.Reset()
		}
	case ComposeActionStop:
		actionName = "stop"
		if modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, done := serializeYamlWithPlainTextSecrets(w, modifiedComposeFile); !done {
			cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, "stop")
			cmd.Stdin = strings.NewReader(modifiedComposeYamlWithPlainTextSecrets)
			modifiedComposeYamlWithPlainTextSecretsBuffer.Reset()
		}
	}

	if cmd != nil {
		log.Printf("Executing docker modifiedComposeFile %s for stack: %s", actionName, stackName)

		// Stream the output (headers already set above)
		if err := streamCommandOutput(w, cmd); err != nil {
			log.Printf("Error executing docker modifiedComposeFile %s for stack %s: %v", actionName, stackName, err)
			// Error already written to response stream
			return
		}
		log.Printf("Successfully executed docker modifiedComposeFile %s for stack %s", actionName, stackName)
	}

	if action == ComposeActionNone || action == ComposeActionUp {
		// Ensure the stacks directory exists
		if err := os.MkdirAll(StacksDir, 0755); err != nil {
			log.Printf("Error creating stacks directory: %v", err)
			http.Error(w, "Failed to create stacks directory", http.StatusInternalServerError)
			return
		}

		// Construct the file paths
		originalFilePath := GetStackPath(stackName, false)
		effectiveFilePath := GetStackPath(stackName, true)

		// Write the original file (sanitized user-provided content without plaintext passwords)
		if err := os.WriteFile(originalFilePath, []byte(originalComposeYamlBuffer.String()), 0644); err != nil {
			log.Printf("Error writing original stack file %s: %v", originalFilePath, err)
			http.Error(w, "Failed to write original stack file", http.StatusInternalServerError)
			return
		}

		// Write the effective file (enriched and sanitized - no plaintext passwords)
		if err := os.WriteFile(effectiveFilePath, []byte(modifiedComposeYamlBuffer.String()), 0644); err != nil {
			log.Printf("Error writing effective stack file %s: %v", effectiveFilePath, err)
			http.Error(w, "Failed to write effective stack file", http.StatusInternalServerError)
			return
		}
		log.Printf("Successfully persisted stack: %s (original: %s, effective: %s)", stackName, originalFilePath, effectiveFilePath)
	}
}

func serializeYamlWithPlainTextSecrets(w http.ResponseWriter, modifiedComposeFile ComposeFile) (strings.Builder, string, bool) {
	// Replace environment variables in the effective YAML content
	if err := replaceEnvVarsInCompose(&modifiedComposeFile); err != nil {
		log.Printf("Error replacing environment variables in modifiedComposeFile file: %v", err)
		fmt.Fprintf(w, "[ERROR] Failed to process modifiedComposeFile file: %v\n", err)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return strings.Builder{}, "", true
	}
	var modifiedComposeYamlWithPlainTextSecretsBuffer strings.Builder
	if err := encodeYAMLWithMultiline(&modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeFile); err != nil {
		log.Printf("Failed to serialize modified YAML with secrets: %v", err)
		http.Error(w, fmt.Sprintf("Failed to serialize modified YAML with secrets: %v", err), http.StatusInternalServerError)
		return strings.Builder{}, "", true
	}
	var modifiedComposeYamlWithPlainTextSecrets = modifiedComposeYamlWithPlainTextSecretsBuffer.String()
	return modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, false
}

// ensureNetworksExist checks all networks defined in the compose file and creates missing ones
// Networks are created in bridge mode if no driver is specified and external is false
// If w is not nil, output is streamed to the HTTP response
func ensureNetworksExist(compose *ComposeFile, w http.ResponseWriter) error {
	if compose.Networks == nil {
		return nil
	}

	for networkName, networkConfig := range compose.Networks {
		// Skip external networks as they should already exist
		if networkConfig.External {
			log.Printf("Skipping external network: %s", networkName)
			if w != nil {
				fmt.Fprintf(w, "[INFO] Skipping external network: %s\n", networkName)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
			continue
		}

		// Check if network exists
		checkCmd := exec.Command("docker", "network", "inspect", networkName)
		if err := checkCmd.Run(); err == nil {
			log.Printf("Network already exists: %s", networkName)
			if w != nil {
				fmt.Fprintf(w, "[INFO] Network already exists: %s\n", networkName)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
			continue
		}

		// Network doesn't exist, create it
		// Use the driver specified in config, or default to "bridge"
		driver := "bridge"
		if networkConfig.Driver != "" {
			driver = networkConfig.Driver
		}

		createArgs := []string{"network", "create", "--driver", driver}

		// Add driver options if specified
		if networkConfig.DriverOpts != nil {
			for key, value := range networkConfig.DriverOpts {
				createArgs = append(createArgs, "-o", fmt.Sprintf("%s=%s", key, value))
			}
		}

		createArgs = append(createArgs, networkName)

		createCmd := exec.Command("docker", createArgs...)

		// Stream output if ResponseWriter is provided
		if w != nil {
			log.Printf("Creating network: %s with driver: %s", networkName, driver)
			fmt.Fprintf(w, "[INFO] Creating network: %s with driver: %s\n", networkName, driver)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

			if err := streamCommandOutput(w, createCmd); err != nil {
				return fmt.Errorf("failed to create network %s: %v", networkName, err)
			}
		} else {
			// Fall back to non-streaming for backward compatibility
			output, err := createCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to create network %s: %v, output: %s", networkName, err, string(output))
			}
		}

		log.Printf("Successfully created network: %s with driver: %s", networkName, driver)
	}

	return nil
}

// ensureVolumesExist checks all volumes defined in the compose file and creates missing ones
// Volumes are created with driver "local" if no driver is specified and external is false
// If w is not nil, output is streamed to the HTTP response
func ensureVolumesExist(compose *ComposeFile, w http.ResponseWriter) error {
	if compose.Volumes == nil {
		return nil
	}

	for volumeName, volumeConfig := range compose.Volumes {
		// Skip external volumes as they should already exist
		if volumeConfig.External {
			log.Printf("Skipping external volume: %s", volumeName)
			if w != nil {
				fmt.Fprintf(w, "[INFO] Skipping external volume: %s\n", volumeName)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
			continue
		}

		// Check if volume exists
		checkCmd := exec.Command("docker", "volume", "inspect", volumeName)
		if err := checkCmd.Run(); err == nil {
			log.Printf("Volume already exists: %s", volumeName)
			if w != nil {
				fmt.Fprintf(w, "[INFO] Volume already exists: %s\n", volumeName)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
			continue
		}

		// Volume doesn't exist, create it
		// Use the driver specified in config, or default to "local"
		driver := "local"
		if volumeConfig.Driver != "" {
			driver = volumeConfig.Driver
		}

		createArgs := []string{"volume", "create", "--driver", driver}

		// Add driver options if specified
		if volumeConfig.DriverOpts != nil {
			for key, value := range volumeConfig.DriverOpts {
				createArgs = append(createArgs, "-o", fmt.Sprintf("%s=%s", key, value))
			}
		}

		// Add volume name or use the custom name if specified
		targetName := volumeName
		if volumeConfig.Name != "" {
			targetName = volumeConfig.Name
		}

		createArgs = append(createArgs, targetName)

		createCmd := exec.Command("docker", createArgs...)

		// Stream output if ResponseWriter is provided
		if w != nil {
			log.Printf("Creating volume: %s with driver: %s", targetName, driver)
			fmt.Fprintf(w, "[INFO] Creating volume: %s with driver: %s\n", targetName, driver)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

			if err := streamCommandOutput(w, createCmd); err != nil {
				return fmt.Errorf("failed to create volume %s: %v", targetName, err)
			}
		} else {
			// Fall back to non-streaming for backward compatibility
			output, err := createCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to create volume %s: %v, output: %s", targetName, err, string(output))
			}
		}

		log.Printf("Successfully created volume: %s with driver: %s", targetName, driver)
	}

	return nil
}

// HandleEnrichStack handles POST /api/enrich/{name}
// Enriches the provided docker-compose YAML without modifying files or creating secrets
func HandleEnrichStack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract stack name from URL path
	// Expected format: /api/enrich/{name}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "enrich" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	stackName := pathParts[2]
	if stackName == "" {
		http.Error(w, "Stack name is required", http.StatusBadRequest)
		return
	}

	HandleDockerComposeFile(w, r, stackName, true, ComposeActionNone)
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

// enrichAndSanitizeCompose parses a docker-compose YAML and enriches services with Traefik labels
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
// processSecrets scans environment variables for /run/secrets/ references
// and ensures the corresponding secrets are declared at both service and top level
// If dryRun is true, it will not write to prod.env (no file system modifications)
func processSecrets(compose *ComposeFile, dryRun bool) {
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
					// Normalize the secret name by extracting the variable name from ${XXX} if present
					normalizedSecretName := secretName
					if strings.HasPrefix(secretName, "${") && strings.HasSuffix(secretName, "}") {
						normalizedSecretName = secretName[2 : len(secretName)-1]
					}
					serviceSecrets[normalizedSecretName] = true
					requiredSecrets[normalizedSecretName] = true
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
					if dryRun {
						log.Printf("Auto-added secret '%s' to service '%s' (dry run)", secretName, serviceName)
					} else {
						log.Printf("Auto-added secret '%s' to service '%s'", secretName, serviceName)
					}
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
			if dryRun {
				log.Printf("Auto-added top-level secret declaration for '%s' (dry run)", secretName)
			} else {
				log.Printf("Auto-added top-level secret declaration for '%s'", secretName)
			}
		}
	}

	// Ensure all secrets exist in prod.env file (only if not in dry run mode)
	if !dryRun && len(requiredSecrets) > 0 {
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
	return readProdEnvWithSecrets(filePath, "/run/secrets")
}

// readProdEnvWithSecrets reads environment variables from both prod.env and /run/secrets directory
// It performs case-insensitive matching and validates that duplicate keys have the same value
func readProdEnvWithSecrets(prodEnvPath string, secretsDir string) (map[string]string, error) {
	envVars := make(map[string]string)
	// Track original case keys for case-insensitive comparison
	caseMap := make(map[string]string) // lowercase -> original case

	// Read prod.env file
	prodEnvVars, err := readEnvFile(prodEnvPath)
	if err != nil {
		return nil, err
	}

	// Add prod.env variables to the result (case-insensitive)
	for key, value := range prodEnvVars {
		lowerKey := strings.ToLower(key)
		if existing, found := caseMap[lowerKey]; found {
			// Should not happen within the same file, but handle it
			if envVars[existing] != value {
				log.Panicf("Duplicate key with different values in prod.env: '%s' and '%s'", existing, key)
			}
			log.Printf("Warning: Duplicate key in prod.env (case variation): '%s' and '%s' with same value", existing, key)
		} else {
			envVars[key] = value
			caseMap[lowerKey] = key
		}
	}

	// Read /run/secrets directory
	secretsVars, secretsErr := readSecretsDir(secretsDir)
	if secretsErr != nil && !os.IsNotExist(secretsErr) {
		// Not a fatal error if secrets dir doesn't exist, just log
		log.Printf("Info: Could not read secrets directory %s: %v", secretsDir, secretsErr)
	}

	if secretsErr == nil {
		// Merge secrets with prod.env (case-insensitive validation)
		for secretKey, secretValue := range secretsVars {
			lowerKey := strings.ToLower(secretKey)
			if existing, found := caseMap[lowerKey]; found {
				// Key exists in prod.env (possibly with different case)
				if envVars[existing] == secretValue {
					log.Printf("Warning: Key '%s' exists in both prod.env (as '%s') and /run/secrets with the same value", secretKey, existing)
				} else {
					log.Panicf("FATAL: Key '%s' exists in both prod.env (as '%s') and /run/secrets with DIFFERENT values. prod.env='%s', secrets='%s'",
						secretKey, existing, sanitizeForLog(envVars[existing]), sanitizeForLog(secretValue))
				}
			} else {
				// New key from secrets
				envVars[secretKey] = secretValue
				caseMap[lowerKey] = secretKey
			}
		}
	}

	return envVars, nil
}

// readEnvFile reads a single .env file and returns the key-value pairs
func readEnvFile(filePath string) (map[string]string, error) {
	envVars := make(map[string]string)

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return empty map
			return envVars, nil
		}
		return nil, fmt.Errorf("failed to open %s: %w", filePath, err)
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
		return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	return envVars, nil
}

// readSecretsDir reads all files from /run/secrets directory
// Each file name becomes the key, and the file content becomes the value
func readSecretsDir(secretsDir string) (map[string]string, error) {
	secrets := make(map[string]string)

	// Check if directory exists
	info, err := os.Stat(secretsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist, return empty map
			return secrets, nil
		}
		return nil, fmt.Errorf("failed to stat secrets directory: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", secretsDir)
	}

	// Read directory entries
	entries, err := os.ReadDir(secretsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read secrets directory: %w", err)
	}

	// Process each file
	for _, entry := range entries {
		// Skip directories and hidden files
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		secretPath := filepath.Join(secretsDir, entry.Name())
		content, err := os.ReadFile(secretPath)
		if err != nil {
			log.Printf("Warning: Failed to read secret file %s: %v", secretPath, err)
			continue
		}

		// Use filename as key and trimmed content as value
		key := entry.Name()
		value := strings.TrimSpace(string(content))
		secrets[key] = value
		log.Printf("Loaded secret from %s: %s", secretsDir, key)
	}

	return secrets, nil
}

// sanitizeForLog sanitizes sensitive values for logging (shows first 3 chars only)
func sanitizeForLog(value string) string {
	if len(value) <= 3 {
		return "***"
	}
	return value[:3] + "***"
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
	const passwordLength = 24

	// Read existing prod.env
	envVars, err := readProdEnv(ProdEnvPath)
	if err != nil {
		return err
	}

	modified := false

	// Check each secret
	for _, secretName := range secretNames {
		// Secrets should not be fetched from runtime environment - only from prod.env
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
		if err := writeProdEnv(ProdEnvPath, envVars); err != nil {
			return err
		}
		log.Printf("Updated prod.env with %d new secret(s)", len(secretNames))
	}

	return nil
}

// replaceEnvVarsInCompose replaces ${VAR} and $VAR placeholders within a ComposeFile struct
// It modifies the struct in-place and returns the marshaled YAML string with replacements applied.
func replaceEnvVarsInCompose(compose *ComposeFile) error {
	// Read prod.env
	envVars, err := readProdEnv(ProdEnvPath)
	if err != nil {
		log.Printf("Warning: Failed to read prod.env: %v", err)
		envVars = make(map[string]string)
	}

	undefinedVars := make(map[string]bool)

	// Helper to replace variables in a single string
	replaceInString := func(s string) string {
		if s == "" {
			return s
		}

		// Handle ${VAR}
		re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
		s = re.ReplaceAllStringFunc(s, func(match string) string {
			varName := match[2 : len(match)-1]
			if isSensitiveEnvironmentKey(varName, "") {
				if v, ok := envVars[varName]; ok {
					return v
				}
				undefinedVars[varName] = true
				return ""
			}
			if runtimeValue := os.Getenv(varName); runtimeValue != "" {
				return runtimeValue
			}
			if v, ok := envVars[varName]; ok {
				return v
			}
			undefinedVars[varName] = true
			return ""
		})

		// Handle $VAR (simple form)
		re2 := regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)(?:[^A-Za-z0-9_]|$)`)
		s = re2.ReplaceAllStringFunc(s, func(match string) string {
			// Extract variable name and trailing char if present
			varName := match[1:]
			trailing := ""
			if len(varName) > 0 && !regexp.MustCompile(`[A-Za-z0-9_]`).MatchString(string(varName[len(varName)-1])) {
				trailing = string(varName[len(varName)-1])
				varName = varName[:len(varName)-1]
			}
			if isSensitiveEnvironmentKey(varName, "") {
				if v, ok := envVars[varName]; ok {
					return v + trailing
				}
				undefinedVars[varName] = true
				return trailing
			}
			if runtimeValue := os.Getenv(varName); runtimeValue != "" {
				return runtimeValue + trailing
			}
			if v, ok := envVars[varName]; ok {
				return v + trailing
			}
			undefinedVars[varName] = true
			return trailing
		})

		return s
	}

	// Process services
	for _, service := range compose.Services {
		// Simple string fields
		service.Image = replaceInString(service.Image)
		service.ContainerName = replaceInString(service.ContainerName)
		service.User = replaceInString(service.User)
		service.Restart = replaceInString(service.Restart)

		// Volumes
		for i, vol := range service.Volumes {
			service.Volumes[i] = replaceInString(vol)
		}

		// Ports
		for i, p := range service.Ports {
			service.Ports[i] = replaceInString(p)
		}

		// Environment: map or array
		if service.Environment != nil {
			if envMap, ok := service.Environment.(map[string]interface{}); ok {
				for k, v := range envMap {
					if strValue, ok := v.(string); ok {
						envMap[k] = replaceInString(strValue)
					}
				}
				service.Environment = envMap
			} else if envArr, ok := service.Environment.([]interface{}); ok {
				for i, item := range envArr {
					if s, ok := item.(string); ok {
						// If it's KEY=VALUE, only replace VALUE portion
						if eq := strings.Index(s, "="); eq != -1 {
							key := s[:eq]
							val := s[eq+1:]
							envArr[i] = fmt.Sprintf("%s=%s", key, replaceInString(val))
						} else {
							envArr[i] = replaceInString(s)
						}
					}
				}
				service.Environment = envArr
			}
		}

		// Networks (array form)
		if service.Networks != nil {
			if netArr, ok := service.Networks.([]interface{}); ok {
				for i, item := range netArr {
					if s, ok := item.(string); ok {
						netArr[i] = replaceInString(s)
					}
				}
				service.Networks = netArr
			}
		}

		// Labels map or array
		if service.Labels != nil {
			if labMap, ok := service.Labels.(map[string]interface{}); ok {
				for k, v := range labMap {
					if str, ok := v.(string); ok {
						labMap[k] = replaceInString(str)
					}
				}
				service.Labels = labMap
			} else if labArr, ok := service.Labels.([]interface{}); ok {
				for i, item := range labArr {
					if s, ok := item.(string); ok {
						labArr[i] = replaceInString(s)
					}
				}
				service.Labels = labArr
			}
		}

		// Command
		if service.Command != nil {
			if cmdStr, ok := service.Command.(string); ok {
				service.Command = replaceInString(cmdStr)
			} else if cmdArr, ok := service.Command.([]interface{}); ok {
				for i, item := range cmdArr {
					if s, ok := item.(string); ok {
						cmdArr[i] = replaceInString(s)
					}
				}
				service.Command = cmdArr
			}
		}

		// Configs
		for i := range service.Configs {
			service.Configs[i].Source = replaceInString(service.Configs[i].Source)
			service.Configs[i].Target = replaceInString(service.Configs[i].Target)
		}

		// Sysctls
		if service.Sysctls != nil {
			if sMap, ok := service.Sysctls.(map[string]interface{}); ok {
				for k, v := range sMap {
					if str, ok := v.(string); ok {
						sMap[k] = replaceInString(str)
					}
				}
				service.Sysctls = sMap
			} else if sArr, ok := service.Sysctls.([]interface{}); ok {
				for i, item := range sArr {
					if s, ok := item.(string); ok {
						sArr[i] = replaceInString(s)
					}
				}
				service.Sysctls = sArr
			}
		}

		// Secrets
		for i, s := range service.Secrets {
			service.Secrets[i] = replaceInString(s)
		}

		// Logging options
		if service.Logging != nil && service.Logging.Options != nil {
			for k, v := range service.Logging.Options {
				service.Logging.Options[k] = replaceInString(v)
			}
		}
	}

	// Volumes
	for name, vol := range compose.Volumes {
		vol.Name = replaceInString(vol.Name)
		vol.Driver = replaceInString(vol.Driver)
		for k, v := range vol.DriverOpts {
			vol.DriverOpts[k] = replaceInString(v)
		}
		compose.Volumes[name] = vol
	}

	// Networks
	for name, net := range compose.Networks {
		net.Driver = replaceInString(net.Driver)
		for k, v := range net.DriverOpts {
			net.DriverOpts[k] = replaceInString(v)
		}
		compose.Networks[name] = net
	}

	// Configs
	for name, cfg := range compose.Configs {
		cfg.Content = replaceInString(cfg.Content)
		cfg.File = replaceInString(cfg.File)
		compose.Configs[name] = cfg
	}

	// Secrets
	for name, s := range compose.Secrets {
		s.Name = replaceInString(s.Name)
		s.Environment = replaceInString(s.Environment)
		s.File = replaceInString(s.File)
		compose.Secrets[name] = s
	}

	if len(undefinedVars) > 0 {
		varList := make([]string, 0, len(undefinedVars))
		for varName := range undefinedVars {
			varList = append(varList, varName)
		}
		sort.Strings(varList)
		return fmt.Errorf("undefined variables: %s", strings.Join(varList, ", "))
	}

	return nil
}

// HandleEnrichYAML handles PUT /api/enrich/{stackname} - enriches YAML without saving to prod.env or files
func HandleEnrichYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "api" || pathParts[1] != "enrich" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	stackName := pathParts[2]
	if stackName == "" {
		http.Error(w, "Stack name is required", http.StatusBadRequest)
		return
	}

	HandleDockerComposeFile(w, r, stackName, false, ComposeActionNone)
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

	HandleDockerComposeFile(w, r, stackName, false, ComposeActionDown)
}

// getStacksData returns the combined stacks data (same as GET /api/stacks)
// This is used to provide stacks data to Go templates
func getStacksData() ([]Stack, error) {
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
	ymlStacks := make(map[string]string) // stackName -> filePath

	entries, err := os.ReadDir(StacksDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read stacks directory: %w", err)
	}

	if err == nil {
		// Collect YAML file stack names and paths
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yml") && !strings.HasSuffix(entry.Name(), ".effective.yml") {
				stackName := strings.TrimSuffix(entry.Name(), ".yml")
				ymlStacks[stackName] = filepath.Join(StacksDir, entry.Name())
			}
		}
	}

	// Create a map to track which stacks are already running
	runningStackNames := make(map[string]bool)
	for _, stack := range runningStacks {
		runningStackNames[stack.Name] = true
	}

	// Add YAML stacks that are not running (with simulated containers)
	for stackName, filePath := range ymlStacks {
		if !runningStackNames[stackName] {
			// Parse YAML file and create simulated containers
			simulatedContainers, err := createSimulatedContainers(stackName, filePath, allContainers)
			if err != nil {
				log.Printf("Error creating simulated containers for %s: %v", stackName, err)
				// Still add the stack but with empty containers
				runningStacks = append(runningStacks, Stack{
					Name:       stackName,
					Containers: []DockerInspect{},
				})
			} else {
				runningStacks = append(runningStacks, Stack{
					Name:       stackName,
					Containers: simulatedContainers,
				})
			}
		}
	}

	// Sort stacks alphabetically by name
	sort.Slice(runningStacks, func(i, j int) bool {
		return runningStacks[i].Name < runningStacks[j].Name
	})

	return runningStacks, nil
}
