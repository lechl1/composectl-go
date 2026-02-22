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

// getConfig retrieves a configuration value with the following priority:
// 1. Check program arguments for -key or --key flag
// 2. Check KEY_FILE env var (Docker secrets pattern)
// 3. Check KEY env var
// 4. Check prod.env file (case insensitive)
// 5. Check default Docker secrets location (/run/secrets/KEY - case insensitive)
// 6. Return provided default value
func getConfig(args []string, key string, defaultValue string) string {
	keyLower := strings.ToLower(key)
	keyUpper := strings.ToUpper(key)
	// Create title case manually (first char upper, rest lower)
	keyTitle := ""
	if len(keyLower) > 0 {
		keyTitle = strings.ToUpper(string(keyLower[0])) + keyLower[1:]
	}

	// Check program arguments first
	for i, arg := range args {
		argFlag := "-" + keyLower
		argFlagDouble := "--" + keyLower

		if (arg == argFlag || arg == argFlagDouble) && i+1 < len(args) {
			log.Printf("Loaded %s from program arguments: %s", keyUpper, args[i+1])
			return args[i+1]
		}
		// Handle --key=value format
		if strings.HasPrefix(arg, argFlagDouble+"=") {
			value := strings.TrimPrefix(arg, argFlagDouble+"=")
			log.Printf("Loaded %s from program arguments: %s", keyUpper, value)
			return value
		}
		if strings.HasPrefix(arg, argFlag+"=") {
			value := strings.TrimPrefix(arg, argFlag+"=")
			log.Printf("Loaded %s from program arguments: %s", keyUpper, value)
			return value
		}
	}

	// Try to read from file specified in KEY_FILE env var
	fileEnvVar := keyUpper + "_FILE"
	if configFile := os.Getenv(fileEnvVar); configFile != "" {
		if content, err := readSecretFile(configFile); err == nil {
			log.Printf("Loaded %s from file: %s", keyUpper, configFile)
			return content
		} else {
			log.Printf("Warning: Failed to read %s (%s): %v", fileEnvVar, configFile, err)
		}
	}

	// Check direct environment variable
	if value := os.Getenv(keyUpper); value != "" {
		return value
	}

	// Check prod.env (case insensitive)
	envVars, err := readProdEnv(ProdEnvPath)
	if err != nil {
		log.Printf("Warning: Failed to read prod.env for %s: %v", keyUpper, err)
	} else {
		// Try case-insensitive lookup
		for envKey, value := range envVars {
			if strings.ToLower(envKey) == keyLower {
				log.Printf("Loaded %s from prod.env: %s", keyUpper, envKey)
				return value
			}
		}
	}

	// Try default Docker secrets location (case insensitive)
	secretPaths := []string{
		"/run/secrets/" + keyUpper,
		"/run/secrets/" + keyLower,
		"/run/secrets/" + keyTitle,
	}
	for _, secretPath := range secretPaths {
		if content, err := readSecretFile(secretPath); err == nil {
			log.Printf("Loaded %s from Docker secrets: %s", keyUpper, secretPath)
			return content
		}
	}

	// Return default value
	return defaultValue
}

// GetPort retrieves the PORT configuration with the following priority:
// 1. Check program arguments for -port or --port flag
// 2. Check PORT_FILE env var (Docker secrets pattern)
// 3. Check PORT env var
// 4. Check prod.env file (case insensitive)
// 5. Check default Docker secrets location (/run/secrets/PORT - case insensitive)
// 6. Default to "8080"
func GetPort(args []string) string {
	return getConfig(args, "port", "8882")
}

// GetAddr retrieves the ADDR configuration with the following priority:
// 1. Check program arguments for -addr or --addr flag
// 2. Check ADDR_FILE env var (Docker secrets pattern)
// 3. Check ADDR env var
// 4. Check prod.env file (case insensitive)
// 5. Check default Docker secrets location (/run/secrets/ADDR - case insensitive)
// 6. Default to "0.0.0.0"
func GetAddr(args []string) string {
	return getConfig(args, "addr", "0.0.0.0")
}
