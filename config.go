package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ContainersDir is the base directory for all composectl data
	ContainersDir string

	// StacksDir is the directory containing stack YAML files
	StacksDir string

	// ProdEnvPath is the path to the prod.env file
	ProdEnvPath string
)

func init() {
	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}

	// Set base directory to $HOME/.local/containers
	ContainersDir = filepath.Join(homeDir, ".local", "containers")

	// Set subdirectories and files
	StacksDir = filepath.Join(ContainersDir, "stacks")
	ProdEnvPath = filepath.Join(ContainersDir, "prod.env")

	// Ensure directories exist
	ensureDirectories()
}

// ensureDirectories creates necessary directories if they don't exist
func ensureDirectories() {
	dirs := []string{
		ContainersDir,
		StacksDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Warning: Failed to create directory %s: %v", dir, err)
		}
	}

	log.Printf("Using containers directory: %s", ContainersDir)
	log.Printf("Stacks directory: %s", StacksDir)
	log.Printf("Prod.env path: %s", ProdEnvPath)
}

// GetStackPath returns the full path to a stack file
func GetStackPath(stackName string, effective bool) string {
	suffix := ".yml"
	if effective {
		suffix = ".effective.yml"
	}
	return filepath.Join(StacksDir, stackName+suffix)
}

// GetPort retrieves the PORT configuration with the following priority:
// 1. Check PORT_FILE env var (Docker secrets pattern)
// 2. Check default Docker secrets location (/run/secrets/PORT or /run/secrets/port - case insensitive)
// 3. Check PORT env var
// 4. Check prod.env file (case insensitive)
// 5. Default to "8080"
func GetPort() string {
	// Try to read from file specified in PORT_FILE env var
	if portFile := os.Getenv("PORT_FILE"); portFile != "" {
		if content, err := readSecretFile(portFile); err == nil {
			log.Printf("Loaded PORT from file: %s", portFile)
			return content
		} else {
			log.Printf("Warning: Failed to read PORT_FILE (%s): %v", portFile, err)
		}
	}

	// Try default Docker secrets location (case insensitive)
	// Try both uppercase and lowercase variants
	secretPaths := []string{
		"/run/secrets/PORT",
		"/run/secrets/port",
		"/run/secrets/Port",
	}
	for _, secretPath := range secretPaths {
		if content, err := readSecretFile(secretPath); err == nil {
			log.Printf("Loaded PORT from Docker secrets: %s", secretPath)
			return content
		}
	}

	// Fall back to direct environment variable
	if port := os.Getenv("PORT"); port != "" {
		return port
	}

	// If PORT is still not set, load from prod.env (case insensitive)
	envVars, err := readProdEnv(ProdEnvPath)
	if err != nil {
		log.Printf("Warning: Failed to read prod.env for PORT: %v", err)
	} else {
		// Try case-insensitive lookup
		for key, value := range envVars {
			if strings.ToLower(key) == "port" {
				log.Printf("Loaded PORT from prod.env: %s", key)
				return value
			}
		}
	}

	// Default to 8080
	return "8080"
}
