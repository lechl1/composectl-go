package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
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

	return runningStacks, nil
}

// streamCommandOutput executes a command and streams its stdout and stderr to the HTTP response
// using chunked transfer encoding. Returns error if command execution fails.
// Note: Headers should be set by the caller before calling this function if multiple commands are streamed.
func streamCommandOutput(cmd *exec.Cmd) error {

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
			fmt.Fprintf(os.Stderr, "[STDOUT] %s\n", line)
		}
	}()

	// Stream stderr
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(os.Stderr, "[STDERR] %s\n", line)
		}
	}()

	// Wait for both streams to complete
	wg.Wait()

	// Wait for command to finish and get exit status
	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Command failed: %v\n", err)
		return err
	}

	fmt.Fprintf(os.Stderr, "[DONE] Command completed successfully\n")

	return nil
}

// HandleListStacks handles GET /api/stacks
// Returns a combined list of running stacks from Docker and available YAML files
func HandleListStacks() {
	stacks, err := getStacksList()
	if err != nil {
		log.Printf("Error getting stacks list: %v", err)
		return
	}
	json.NewEncoder(os.Stdout).Encode(stacks)
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

		enrichWithProxy(&service, serviceName)
		// Labels (filter out compose-specific labels, opencontainers labels, and traefik labels)
		serviceLabels := make(map[string]interface{})
		for key, value := range labels {
			if !strings.HasPrefix(key, "com.docker.compose.") &&
				!strings.HasPrefix(key, "org.opencontainers.image") &&
				!strings.HasPrefix(key, "traefik") {
				serviceLabels[key] = value
			}
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

func HandleDockerComposeFile(body []byte, stackName string, dryRun bool, action ComposeAction) {
	// First, sanitize passwords and extract them to prod.env
	// This must be done BEFORE enrichment to capture plaintext passwords
	var modifiedComposeFile ComposeFile
	if err := yaml.Unmarshal(body, &modifiedComposeFile); err != nil {
		log.Printf("Error parsing YAML for sanitization: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to parse YAML: %v\n", err)
		return
	}
	sanitizeComposePasswords(&modifiedComposeFile)

	// Marshal the sanitized original version back to YAML for .yml file
	var originalComposeYamlBuffer strings.Builder
	if err := encodeYAMLWithMultiline(&originalComposeYamlBuffer, modifiedComposeFile); err != nil {
		log.Printf("Failed to serialize original YAML: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to serialize original YAML: %v\n", err)
		return
	}

	enrichAndSanitizeCompose(&modifiedComposeFile)

	// Marshal the sanitized original version back to YAML for .yml file
	var modifiedComposeYamlBuffer strings.Builder
	if err := encodeYAMLWithMultiline(&modifiedComposeYamlBuffer, modifiedComposeFile); err != nil {
		log.Printf("Failed to serialize modified YAML: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to serialize modified YAML: %v\n", err)
		return
	}

	var cmd *exec.Cmd
	var actionName string

	if dryRun {
		return
	}

	switch action {
	case ComposeActionUp:
		actionName = "up"
		// Create missing networks and volumes before docker modifiedComposeFile up/down
		if err := ensureNetworksExist(&modifiedComposeFile); err != nil {
			log.Printf("Error ensuring networks exist for stack %s: %v", stackName, err)
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to ensure networks exist: %v\n", err)
		}
		if err := ensureVolumesExist(&modifiedComposeFile); err != nil {
			log.Printf("Error ensuring volumes exist for stack %s: %v", stackName, err)
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to ensure volumes exist: %v\n", err)
		}

		if modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, done := serializeYamlWithPlainTextSecrets(modifiedComposeFile); !done {
			cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, "up", "-d", "--wait", "--remove-orphans")
			cmd.Stdin = strings.NewReader(modifiedComposeYamlWithPlainTextSecrets)
			modifiedComposeYamlWithPlainTextSecretsBuffer.Reset()
		}
	case ComposeActionDown:
		actionName = "down"
		if modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, done := serializeYamlWithPlainTextSecrets(modifiedComposeFile); !done {
			cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, actionName, "--wait", "--remove-orphans")
			cmd.Stdin = strings.NewReader(modifiedComposeYamlWithPlainTextSecrets)
			modifiedComposeYamlWithPlainTextSecretsBuffer.Reset()
		}
	case ComposeActionStop:
		actionName = "stop"
		if modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, done := serializeYamlWithPlainTextSecrets(modifiedComposeFile); !done {
			cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, actionName)
			cmd.Stdin = strings.NewReader(modifiedComposeYamlWithPlainTextSecrets)
			modifiedComposeYamlWithPlainTextSecretsBuffer.Reset()
		}
	case ComposeActionRemove:
		actionName = "rm"
		if modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, done := serializeYamlWithPlainTextSecrets(modifiedComposeFile); !done {
			cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, actionName, "-f")
			cmd.Stdin = strings.NewReader(modifiedComposeYamlWithPlainTextSecrets)
			modifiedComposeYamlWithPlainTextSecretsBuffer.Reset()
		}
	case ComposeActionStart:
		actionName = "start"
		if modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, done := serializeYamlWithPlainTextSecrets(modifiedComposeFile); !done {
			cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, actionName)
			cmd.Stdin = strings.NewReader(modifiedComposeYamlWithPlainTextSecrets)
			modifiedComposeYamlWithPlainTextSecretsBuffer.Reset()
		}
	case ComposeActionCreate:
		actionName = "create"
		if modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, done := serializeYamlWithPlainTextSecrets(modifiedComposeFile); !done {
			cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, actionName)
			cmd.Stdin = strings.NewReader(modifiedComposeYamlWithPlainTextSecrets)
			modifiedComposeYamlWithPlainTextSecretsBuffer.Reset()
		}
	}

	if cmd != nil {
		log.Printf("Executing docker modifiedComposeFile %s for stack: %s", actionName, stackName)

		// Stream the output (headers already set above)
		if err := streamCommandOutput(cmd); err != nil {
			log.Printf("Error executing docker modifiedComposeFile %s for stack %s: %v", actionName, stackName, err)
			// Error already written to response stream
			return
		}
		log.Printf("Successfully executed docker modifiedComposeFile %s for stack %s", actionName, stackName)
	}

	if action == ComposeActionNone || action == ComposeActionUp || action == ComposeActionCreate {
		// Ensure the stacks directory exists
		if err := os.MkdirAll(StacksDir, 0755); err != nil {
			log.Printf("Error creating stacks directory: %v", err)
			fmt.Fprintf(os.Stderr, "Failed to create stacks directory\n")
			return
		}

		// Construct the file paths
		originalFilePath := GetStackPath(stackName, false)
		effectiveFilePath := GetStackPath(stackName, true)

		// Write the original file (sanitized user-provided content without plaintext passwords)
		if err := os.WriteFile(originalFilePath, []byte(originalComposeYamlBuffer.String()), 0644); err != nil {
			log.Printf("Error writing original stack file %s: %v", originalFilePath, err)
			fmt.Fprintf(os.Stderr, "Failed to write original stack file\n")
			return
		}

		// Write the effective file (enriched and sanitized - no plaintext passwords)
		if err := os.WriteFile(effectiveFilePath, []byte(modifiedComposeYamlBuffer.String()), 0644); err != nil {
			log.Printf("Error writing effective stack file %s: %v", effectiveFilePath, err)
			fmt.Fprintf(os.Stderr, "Failed to write effective stack file\n")
			return
		}
		log.Printf("Successfully persisted stack: %s (original: %s, effective: %s)", stackName, originalFilePath, effectiveFilePath)
	}
}

func serializeYamlWithPlainTextSecrets(modifiedComposeFile ComposeFile) (strings.Builder, string, bool) {
	// Replace environment variables in the effective YAML content
	if err := replaceEnvVarsInCompose(&modifiedComposeFile); err != nil {
		log.Printf("Error replacing environment variables in modifiedComposeFile file: %v", err)
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to process modifiedComposeFile file: %v\n", err)
		return strings.Builder{}, "", true
	}
	var modifiedComposeYamlWithPlainTextSecretsBuffer strings.Builder
	if err := encodeYAMLWithMultiline(&modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeFile); err != nil {
		log.Printf("Failed to serialize modified YAML with secrets: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to serialize modified YAML with secrets: %v\n", err)
		return strings.Builder{}, "", true
	}
	var modifiedComposeYamlWithPlainTextSecrets = modifiedComposeYamlWithPlainTextSecretsBuffer.String()
	return modifiedComposeYamlWithPlainTextSecretsBuffer, modifiedComposeYamlWithPlainTextSecrets, false
}

// ensureNetworksExist checks all networks defined in the compose file and creates missing ones
// Networks are created in bridge mode if no driver is specified and external is false
// If w is not nil, output is streamed to the HTTP response
func ensureNetworksExist(compose *ComposeFile) error {
	if compose.Networks == nil {
		return nil
	}

	for networkName, networkConfig := range compose.Networks {
		// Skip external networks as they should already exist
		if networkConfig.External {
			log.Printf("Skipping external network: %s", networkName)
			fmt.Fprintf(os.Stderr, "[INFO] Skipping external network: %s\n", networkName)
			continue
		}

		// Check if network exists
		checkCmd := exec.Command("docker", "network", "inspect", networkName)
		if err := checkCmd.Run(); err == nil {
			log.Printf("Network already exists: %s", networkName)
			fmt.Fprintf(os.Stderr, "[INFO] Network already exists: %s\n", networkName)
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
		log.Printf("Creating network: %s with driver: %s", networkName, driver)
		fmt.Fprintf(os.Stderr, "[INFO] Creating network: %s with driver: %s\n", networkName, driver)

		if err := streamCommandOutput(createCmd); err != nil {
			return fmt.Errorf("failed to create network %s: %v", networkName, err)
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
func ensureVolumesExist(compose *ComposeFile) error {
	if compose.Volumes == nil {
		return nil
	}

	for volumeName, volumeConfig := range compose.Volumes {
		// Skip external volumes as they should already exist
		if volumeConfig.External {
			log.Printf("Skipping external volume: %s", volumeName)
			fmt.Fprintf(os.Stderr, "[INFO] Skipping external volume: %s\n", volumeName)
			continue
		}

		// Check if volume exists
		checkCmd := exec.Command("docker", "volume", "inspect", volumeName)
		if err := checkCmd.Run(); err == nil {
			log.Printf("Volume already exists: %s", volumeName)
			fmt.Fprintf(os.Stderr, "[INFO] Volume already exists: %s\n", volumeName)
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
		log.Printf("Creating volume: %s with driver: %s", targetName, driver)
		fmt.Fprintf(os.Stderr, "[INFO] Creating volume: %s with driver: %s\n", targetName, driver)

		if err := streamCommandOutput(createCmd); err != nil {
			return fmt.Errorf("failed to create volume %s: %v", targetName, err)
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

func HandleStreamStackLogs(body []byte, path string) {
	// Extract stack name from URL path
	// Expected format: /api/stacks/{name}/logs
	pathParts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(pathParts) < 4 || pathParts[0] != "api" || pathParts[1] != "stacks" || pathParts[3] != "logs" {
		fmt.Fprintf(os.Stderr, "Invalid URL format\n")
		return
	}

	stackName := pathParts[2]
	if stackName == "" {
		fmt.Fprintf(os.Stderr, "Stack name is required\n")
		return
	}

	log.Printf("Streaming logs for stack: %s", stackName)

	// Command to stream logs
	cmd := exec.Command("docker-compose", "-f", GetStackPath(stackName, true), "logs", "-f")

	// Stream logs to the response
	err := streamCommandOutput(cmd)
	if err != nil {
		log.Printf("Error streaming logs for stack %s: %v", stackName, err)
		fmt.Fprintf(os.Stderr, "Failed to stream logs\n")
	}
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
