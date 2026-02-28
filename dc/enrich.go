package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func detectHTTPPort(service *ComposeService) (string, string, bool) {
	standardHTTPPorts := []string{"80", "443", "8000", "8080", "8081", "3000", "3001", "5000", "5001", "8443"}

	// Normalize labels into map[string]string for flexible handling
	labelsMap := make(map[string]string)
	if service.Labels != nil {
		switch v := service.Labels.(type) {
		case map[string]interface{}:
			for k, val := range v {
				labelsMap[k] = fmt.Sprintf("%v", val)
			}
		case map[string]string:
			for k, val := range v {
				labelsMap[k] = val
			}
		case map[interface{}]interface{}:
			for k, val := range v {
				if ks, ok := k.(string); ok {
					labelsMap[ks] = fmt.Sprintf("%v", val)
				}
			}
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					if parts := strings.SplitN(s, "=", 2); len(parts) == 2 {
						labelsMap[parts[0]] = parts[1]
					}
				}
			}
		case []string:
			for _, s := range v {
				if parts := strings.SplitN(s, "=", 2); len(parts) == 2 {
					labelsMap[parts[0]] = parts[1]
				}
			}
		}
	}

	for key, value := range labelsMap {
		if strings.Contains(strings.ToLower(key), "port") {
			valueStr := fmt.Sprintf("%v", value)
			for _, httpPort := range standardHTTPPorts {
				if strings.Contains(valueStr, httpPort) {
					if httpPort == "443" || httpPort == "8443" {
						return httpPort, "https", true
					}
					return httpPort, "http", true
				}
			}
		}
	}
	// Check explicit ports first
	for _, p := range service.Ports {
		// port formats: host:container, container, container/proto
		parts := strings.Split(p, ":")
		httpPort := strings.Split(parts[len(parts)-1], "/")[0]
		if httpPort != "" {
			if httpPort == "443" || httpPort == "8443" {
				return httpPort, "https", true
			}
			return httpPort, "http", true
		}
	}

	// Check environment variables for common port names
	envArr := normalizeEnvironment(service.Environment)
	for _, env := range envArr {
		if strings.Contains(strings.ToUpper(env), "PORT=") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				httpPort := extractPortNumber(parts[1])
				if httpPort > 0 {
					if httpPort == 443 || httpPort == 8443 {
						return strconv.FormatInt(int64(httpPort), 10), "https", true
					}
					return strconv.FormatInt(int64(httpPort), 10), "http", true
				}
			}
		}
	}

	return "", "", false
}

// labelsToStringMap normalizes any supported labels type into a flat map[string]string.
func labelsToStringMap(labels interface{}) map[string]string {
	m := make(map[string]string)
	switch v := labels.(type) {
	case map[string]string:
		for k, val := range v {
			m[k] = val
		}
	case map[string]interface{}:
		for k, val := range v {
			m[k] = fmt.Sprintf("%v", val)
		}
	case map[interface{}]interface{}:
		for k, val := range v {
			if ks, ok := k.(string); ok {
				m[ks] = fmt.Sprintf("%v", val)
			}
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				if parts := strings.SplitN(s, "=", 2); len(parts) == 2 {
					m[parts[0]] = parts[1]
				}
			}
		}
	case []string:
		for _, s := range v {
			if parts := strings.SplitN(s, "=", 2); len(parts) == 2 {
				m[parts[0]] = parts[1]
			}
		}
	}
	return m
}

// stringMapToLabels converts a flat map[string]string back to the same type as orig.
func stringMapToLabels(m map[string]string, orig interface{}) interface{} {
	switch orig.(type) {
	case map[string]string, nil:
		return m
	case map[string]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[interface{}]interface{}, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(m))
		for k, v := range m {
			out = append(out, fmt.Sprintf("%s=%s", k, v))
		}
		return out
	case []string:
		out := make([]string, 0, len(m))
		for k, v := range m {
			out = append(out, fmt.Sprintf("%s=%s", k, v))
		}
		return out
	default:
		fmt.Fprintf(os.Stderr, "unknown labels type %T, skipping Traefik label injection\n", orig)
		return orig
	}
}

// addTraefikLabelsInterface adds a minimal set of Traefik labels into a generic labels map
func addTraefikLabelsInterface(service *ComposeService, serviceName, port, scheme string) {
	fmt.Fprintf(os.Stderr, "Adding Traefik labels to service '%s' for port %s and scheme %s...\n", serviceName, port, scheme)

	entrypointVal := "http"
	if scheme == "https" {
		entrypointVal = "https"
	}

	flat := labelsToStringMap(service.Labels)
	flat[fmt.Sprintf("traefik.http.routers.%s.rule", serviceName)] = fmt.Sprintf("Host(`%s`)", serviceName)
	flat[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName)] = port
	flat[fmt.Sprintf("traefik.http.routers.%s.entrypoints", serviceName)] = entrypointVal
	service.Labels = stringMapToLabels(flat, service.Labels)
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

var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// expandStr replaces all ${VAR} and $VAR placeholders in s using the provided vars map.
// Unresolved placeholders are left unchanged.
func expandStr(s string, vars map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(match string) string {
		var name string
		if strings.HasPrefix(match, "${") {
			name = match[2 : len(match)-1]
		} else {
			name = match[1:]
		}
		if v, ok := vars[name]; ok {
			return v
		}
		return match
	})
}

// replacePlaceholders replaces all ${VAR} and $VAR placeholders in the compose file.
// Values are resolved from (lowest to highest priority): prod.env / /run/secrets, then OS environment variables.
func replacePlaceholders(compose *ComposeFile) {
	vars := make(map[string]string)

	// Seed implicit defaults (lowest priority)
	vars["DOCKER_SOCK"] = getDockerSocketPath()
	vars["DOCKER_SOCKET"] = getDockerSocketPath()
	vars["USER_ID"] = getCurrentUserID()
	vars["USER_GID"] = getCurrentGroupID()

	// Load prod.env and /run/secrets (lower priority)
	if ProdEnvPath != "" {
		if envVars, err := readProdEnv(ProdEnvPath); err == nil {
			for k, v := range envVars {
				vars[k] = v
			}
		}
	}

	// OS environment variables override (higher priority)
	for _, e := range os.Environ() {
		k, v, _ := strings.Cut(e, "=")
		vars[k] = v
	}

	for name, service := range compose.Services {
		for i, vol := range service.Volumes {
			service.Volumes[i] = expandStr(vol, vars)
		}

		if service.Environment != nil {
			switch v := service.Environment.(type) {
			case map[string]interface{}:
				for k, val := range v {
					if s, ok := val.(string); ok {
						v[k] = expandStr(s, vars)
					}
				}
				service.Environment = v
			case []interface{}:
				for i, item := range v {
					if s, ok := item.(string); ok {
						v[i] = expandStr(s, vars)
					}
				}
				service.Environment = v
			}
		}

		compose.Services[name] = service
	}
}

// ensureHomelabInServices makes sure every service references the "homelab" network.
// Handles common network representations (nil, []interface{}, []string, map[string]interface{}).
func ensureHomelabInServices(compose *ComposeFile) {
	if compose == nil || compose.Services == nil {
		return
	}

	for name, service := range compose.Services {
		added := false

		switch v := service.Networks.(type) {
		case nil:
			// No networks declared, set to sequence containing homelab
			service.Networks = []interface{}{"homelab"}
			added = true

		case string:
			// Single network as string
			if v != "homelab" {
				service.Networks = []interface{}{v, "homelab"}
				added = true
			}

		case []interface{}:
			found := false
			for _, item := range v {
				switch it := item.(type) {
				case string:
					if it == "homelab" {
						found = true
					}
				case map[string]interface{}:
					if _, ok := it["homelab"]; ok {
						found = true
					}
				case map[interface{}]interface{}:
					if _, ok := it["homelab"]; ok {
						found = true
					}
				}
				if found {
					break
				}
			}
			if !found {
				// Prefer to append a string entry for simplicity; some compose parsers also accept a map entry.
				v = append(v, "homelab")
				service.Networks = v
				added = true
			}

		case []string:
			found := false
			for _, s := range v {
				if s == "homelab" {
					found = true
					break
				}
			}
			if !found {
				v = append(v, "homelab")
				// convert to []interface{} to remain compatible with other code paths
				iface := make([]interface{}, len(v))
				for i := range v {
					iface[i] = v[i]
				}
				service.Networks = iface
				added = true
			}

		case map[string]interface{}:
			if _, ok := v["homelab"]; !ok {
				// Add an empty map as network config
				v["homelab"] = map[string]interface{}{}
				service.Networks = v
				added = true
			}

		case map[interface{}]interface{}:
			if _, ok := v["homelab"]; !ok {
				v["homelab"] = map[string]interface{}{}
				// convert map[interface{}]interface{} to map[string]interface{}
				out := make(map[string]interface{})
				for k, val := range v {
					if ks, ok := k.(string); ok {
						out[ks] = val
					}
				}
				service.Networks = out
				added = true
			}

		default:
			// Unknown type: try to stringify and append if possible
			if s, ok := v.(fmt.Stringer); ok {
				cur := s.String()
				if cur != "homelab" {
					service.Networks = []interface{}{cur, "homelab"}
					added = true
				}
			}
		}

		if added {
			compose.Services[name] = service
		}
	}
}

// New helper: ensureContainerNames sets ContainerName to the service key when it's not defined.
// This makes the effective compose file explicit about container names and ensures subsequent
// processing (like simulated container creation) uses predictable names.
func ensureContainerNames(compose *ComposeFile) {
	if compose == nil || compose.Services == nil {
		return
	}

	for serviceName, service := range compose.Services {
		if strings.TrimSpace(service.ContainerName) == "" {
			// Default container_name to the service key
			service.ContainerName = serviceName
			compose.Services[serviceName] = service
		}
	}
}

// New helper: ensureResourceDefaults sets MemLimit to "256m" and CPUs to 0.5 when they are not defined
func ensureResourceDefaults(compose *ComposeFile) {
	if compose == nil || compose.Services == nil {
		return
	}

	for serviceName, service := range compose.Services {
		// MemLimit: set default if empty or whitespace
		if strings.TrimSpace(service.MemLimit) == "" {
			service.MemLimit = "256m"
		}

		// CPUs: service.CPUs can be nil, string, or numeric. Only set default when not defined or empty string.
		switch v := service.CPUs.(type) {
		case nil:
			service.CPUs = 0.5
		case string:
			if strings.TrimSpace(v) == "" {
				service.CPUs = 0.5
			}
		default:
			// assume numeric or other defined value; leave as-is
		}

		compose.Services[serviceName] = service
	}
}

// enrichAndSanitizeCompose enriches and sanitizes a compose structure.
// NOTE: This function operates in-place on the provided ComposeFile and does NOT
// perform any YAML serialization or return any bytes. Serialization is the caller's
// responsibility so it can decide when to write or return YAML (for example only inside !dryRun).
func enrichAndSanitizeCompose(compose *ComposeFile) {
	// operate directly on the provided ComposeFile struct

	// Process secrets with or without side effects based on dryRun
	processSecrets(compose)

	// Ensure container_name is set for services that lack it
	ensureContainerNames(compose)

	// Ensure resource defaults for services
	ensureResourceDefaults(compose)

	// Ensure every service references the homelab network
	ensureHomelabInServices(compose)

	// Add undeclared networks/volumes
	addUndeclaredNetworksAndVolumes(compose)

	// Sanitize passwords with or without extraction based on dryRun
	sanitizeComposePasswords(compose)

	for serviceName, service := range compose.Services {
		fmt.Fprintf(os.Stderr, "Enriching proxy labels '%s'...\n", serviceName)
		enrichWithProxy(&service, serviceName)
		// write back the possibly modified service so changes persist in the compose struct
		compose.Services[serviceName] = service
	}
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

// sanitizeComposePasswords sanitizes environment variables in a ComposeFile
// by extracting plaintext passwords via `pw ins` and replacing them with variable references ${ENV_KEY}
func sanitizeComposePasswords(compose *ComposeFile) {
	for serviceName, service := range compose.Services {
		envArray := normalizeEnvironment(service.Environment)
		var sanitizedEnv []string
		for _, envVar := range envArray {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]
				if isSensitiveEnvironmentKey(key, value) && value != "" && !strings.HasPrefix(value, "${") && !strings.HasPrefix(value, "/run/secrets/") {
					normalizedKey := normalizeEnvKey(key)
					if err := pwIns(normalizedKey, value); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: Failed to store secret '%s' from service '%s': %v\n", normalizedKey, serviceName, err)
					}
				}
			}
			sanitizedEnv = append(sanitizedEnv, sanitizeEnvironmentVariable(envVar))
		}
		service.Environment = sanitizedEnv
		compose.Services[serviceName] = service
	}
}

func enrichWithProxy(service *ComposeService, serviceName string) {
	fmt.Fprintf(os.Stderr, "Enriching service '%s' with proxy labels if applicable...\n", serviceName)

	if detectedPort, scheme, usesHTTPPort := detectHTTPPort(service); usesHTTPPort {
		addTraefikLabelsInterface(service, serviceName, detectedPort, scheme)
	}
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
		case nil:
			// nothing to do
		case string:
			if v != "" {
				referencedNetworks[v] = true
			}
		case []interface{}:
			for _, net := range v {
				switch n := net.(type) {
				case string:
					referencedNetworks[n] = true
				case map[string]interface{}:
					for name := range n {
						referencedNetworks[name] = true
					}
				case map[interface{}]interface{}:
					for k := range n {
						if ks, ok := k.(string); ok {
							referencedNetworks[ks] = true
						}
					}
				}
			}
		case []string:
			for _, net := range v {
				referencedNetworks[net] = true
			}
		case map[string]interface{}:
			for net := range v {
				referencedNetworks[net] = true
			}
		case map[interface{}]interface{}:
			for k := range v {
				if ks, ok := k.(string); ok {
					referencedNetworks[ks] = true
				}
			}
		default:
			// Unknown type: ignore safely
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
			fmt.Fprintf(os.Stderr, "Auto-added undeclared network: %s (marked as external)\n", network)
		}
	}

	// Add missing volumes as external
	for volume := range referencedVolumes {
		if _, exists := compose.Volumes[volume]; !exists {
			compose.Volumes[volume] = ComposeVolume{External: true}
			fmt.Fprintf(os.Stderr, "Auto-added undeclared volume: %s (marked as external)\n", volume)
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
// and ensures the corresponding secrets are declared at both service and top level.
// Missing secrets are generated via `pw gen`.
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
					fmt.Fprintf(os.Stderr, "Auto-added secret '%s' to service '%s'\n", secretName, serviceName)
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
			fmt.Fprintf(os.Stderr, "Auto-added top-level secret declaration for '%s'\n", secretName)
		}
	}

	for secretName := range requiredSecrets {
		if err := pwGen(secretName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to generate secret '%s': %v\n", secretName, err)
		}
	}
}

// pwGen calls `<secrets_manager> gen KEY` to generate and store a new password.
// If the key already exists in the store, it silently succeeds.
func pwGen(secretName string) error {
	cmd := exec.Command(SecretsManager, "gen", secretName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "key already exists") {
			fmt.Fprintf(os.Stderr, "Secret '%s' already exists in %s store\n", secretName, SecretsManager)
			return nil
		}
		return fmt.Errorf("%s gen %s: %w: %s", SecretsManager, secretName, err, strings.TrimSpace(string(output)))
	}
	fmt.Fprintf(os.Stderr, "Generated new secret '%s' via %s\n", secretName, SecretsManager)
	return nil
}

// pwIns calls `<secrets_manager> ins KEY` with the given value on stdin.
// If the key already exists in the store, it silently succeeds.
func pwIns(secretName, value string) error {
	cmd := exec.Command(SecretsManager, "ins", secretName)
	cmd.Stdin = strings.NewReader(value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "key already exists") {
			fmt.Fprintf(os.Stderr, "Secret '%s' already exists in %s store\n", secretName, SecretsManager)
			return nil
		}
		return fmt.Errorf("%s ins %s: %w: %s", SecretsManager, secretName, err, strings.TrimSpace(string(output)))
	}
	fmt.Fprintf(os.Stderr, "Stored secret '%s' via %s\n", secretName, SecretsManager)
	return nil
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
				fmt.Fprintf(os.Stderr, "Duplicate key with different values in prod.env: '%s' and '%s'\n", existing, key)
				panic(fmt.Sprintf("Duplicate key with different values in prod.env: '%s' and '%s'", existing, key))
			}
			fmt.Fprintf(os.Stderr, "Warning: Duplicate key in prod.env (case variation): '%s' and '%s' with same value\n", existing, key)
		} else {
			envVars[key] = value
			caseMap[lowerKey] = key
		}
	}

	// Read /run/secrets directory
	secretsVars, secretsErr := readSecretsDir(secretsDir)
	if secretsErr != nil && !os.IsNotExist(secretsErr) {
		// Not a fatal error if secrets dir doesn't exist, just log
		fmt.Fprintf(os.Stderr, "Info: Could not read secrets directory %s: %v\n", secretsDir, secretsErr)
	}

	if secretsErr == nil {
		// Merge secrets with prod.env (case-insensitive validation)
		for secretKey, secretValue := range secretsVars {
			lowerKey := strings.ToLower(secretKey)
			if existing, found := caseMap[lowerKey]; found {
				// Key exists in prod.env (possibly with different case)
				if envVars[existing] == secretValue {
					fmt.Fprintf(os.Stderr, "Warning: Key '%s' exists in both prod.env (as '%s') and /run/secrets with the same value\n", secretKey, existing)
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
			fmt.Fprintf(os.Stderr, "Warning: Failed to read secret file %s: %v\n", secretPath, err)
			continue
		}

		// Use filename as key and trimmed content as value
		key := entry.Name()
		value := strings.TrimSpace(string(content))
		secrets[key] = value
		fmt.Fprintf(os.Stderr, "Loaded secret from %s: %s\n", secretsDir, key)
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

// replaceEnvVarsInCompose replaces ${VAR} and $VAR placeholders within a ComposeFile struct
// It modifies the struct in-place and returns the marshaled YAML string with replacements applied.
func replaceEnvVarsInCompose(compose *ComposeFile) error {
	// Read prod.env
	envVars, err := readProdEnv(ProdEnvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to read prod.env: %v\n", err)
		envVars = make(map[string]string)
	}

	// Built-in variables resolved at highest priority
	uid := os.Getuid()
	gid := os.Getgid()
	uidStr := strconv.Itoa(uid)
	gidStr := strconv.Itoa(gid)
	userDockerSock := fmt.Sprintf("/run/user/%d/docker.sock", uid)
	var dockerSock string
	if _, statErr := os.Stat(userDockerSock); statErr == nil {
		dockerSock = userDockerSock
	} else if _, statErr := os.Stat("/var/run/docker.sock"); statErr == nil {
		dockerSock = "/var/run/docker.sock"
	} else {
		panic("no docker socket found: neither " + userDockerSock + " nor /var/run/docker.sock exists")
	}
	builtinVars := map[string]string{
		"UID":         uidStr,
		"GID":         gidStr,
		"DOCKER_SOCK": dockerSock,
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
			if v, ok := builtinVars[varName]; ok {
				return v
			}
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
			if v, ok := builtinVars[varName]; ok {
				return v + trailing
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

	// Volumes - update keys and values
	if compose.Volumes != nil {
		newVolumes := make(map[string]ComposeVolume, len(compose.Volumes))
		for name, vol := range compose.Volumes {
			newName := replaceInString(name)
			vol.Name = replaceInString(vol.Name)
			vol.Driver = replaceInString(vol.Driver)
			if vol.DriverOpts != nil {
				newDriverOpts := make(map[string]string, len(vol.DriverOpts))
				for k, v := range vol.DriverOpts {
					newDriverOpts[replaceInString(k)] = replaceInString(v)
				}
				vol.DriverOpts = newDriverOpts
			}
			if _, exists := newVolumes[newName]; exists {
				fmt.Fprintf(os.Stderr, "Warning: volume key '%s' normalized to duplicate name '%s' - overwriting previous entry\n", name, newName)
			}
			if !strings.Contains(newName, "/") {
				newVolumes[newName] = vol
			}
		}
		compose.Volumes = newVolumes
	}

	// Networks
	for name, net := range compose.Networks {
		net.Driver = replaceInString(net.Driver)
		for k, v := range net.DriverOpts {
			net.DriverOpts[k] = replaceInString(v)
		}
		compose.Networks[name] = net
	}

	// Configs - update keys and values
	if compose.Configs != nil {
		newConfigs := make(map[string]ComposeConfig, len(compose.Configs))
		for name, cfg := range compose.Configs {
			newName := replaceInString(name)
			cfg.Content = replaceInString(cfg.Content)
			cfg.File = replaceInString(cfg.File)
			if _, exists := newConfigs[newName]; exists {
				fmt.Fprintf(os.Stderr, "Warning: config key '%s' normalized to duplicate name '%s' - overwriting previous entry\n", name, newName)
			}
			newConfigs[newName] = cfg
		}
		compose.Configs = newConfigs
	}

	// Secrets - update keys and values
	if compose.Secrets != nil {
		newSecrets := make(map[string]ComposeSecret, len(compose.Secrets))
		for name, s := range compose.Secrets {
			newName := replaceInString(name)
			s.Name = replaceInString(s.Name)
			s.Environment = replaceInString(s.Environment)
			s.File = replaceInString(s.File)
			if _, exists := newSecrets[newName]; exists {
				fmt.Fprintf(os.Stderr, "Warning: secret key '%s' normalized to duplicate name '%s' - overwriting previous entry\n", name, newName)
			}
			newSecrets[newName] = s
		}
		compose.Secrets = newSecrets
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
