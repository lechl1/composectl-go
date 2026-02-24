package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	// StacksDir is the directory containing stack YAML files and all dc data
	StacksDir string

	// ProdEnvPath is the path to the prod.env file
	ProdEnvPath string

	// initialized tracks whether paths have been initialized
	initialized bool
)

// getDefaultStacksDir returns the default stacks directory with priority:
// 1. /containers if it exists
// 2. /stacks if it exists
// 3. Default to $HOME/.local/containers
func getDefaultStacksDir() string {
	if info, err := os.Stat("/containers"); err == nil && info.IsDir() {
		return "/containers"
	}
	if info, err := os.Stat("/stacks"); err == nil && info.IsDir() {
		return "/stacks"
	}

	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}
	return filepath.Join(homeDir, ".local", "containers")
}

// InitPaths initializes StacksDir and ProdEnvPath using command-line arguments
func InitPaths(args []string) {
	if initialized {
		return
	}

	// Get stacks directory from config (respects --stacks-dir argument)
	StacksDir = getConfig("stacks_dir", getDefaultStacksDir())

	// Get prod.env path from config (respects --env-path argument)
	// Default to StacksDir/prod.env if not specified
	defaultEnvPath := filepath.Join(StacksDir, "prod.env")
	ProdEnvPath = getConfig("env_path", defaultEnvPath)

	// Ensure directories exist
	if err := os.MkdirAll(StacksDir, 0755); err != nil {
		log.Printf("Warning: Failed to create directory %s: %v", StacksDir, err)
	}

	log.Printf("Using stacks directory: %s", StacksDir)
	log.Printf("Prod.env path: %s", ProdEnvPath)

	initialized = true
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
// 4. Check prod.env file (case insensitive) - only if ProdEnvPath is initialized
// 5. Check default Docker secrets location (/run/secrets/KEY - case insensitive)
// 6. Return provided default value
func getConfig(key string, defaultValue string) string {
	keyLower := strings.ToLower(key)
	keyUpper := strings.ToUpper(key)
	// Create title case manually (first char upper, rest lower)
	//keyTitle := ""
	//if len(keyLower) > 0 {
	//	keyTitle = strings.ToUpper(string(keyLower[0])) + keyLower[1:]
	//}

	// Check program arguments first
	args := os.Args[1:] // Skip program name
	for i, arg := range args {
		// Replace underscores with dashes for command-line flag names
		keyFlag := strings.ReplaceAll(keyLower, "_", "-")
		argFlag := "-" + keyFlag
		argFlagDouble := "--" + keyFlag

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
	// fileEnvVar := keyUpper + "_FILE"
	// if configFile := os.Getenv(fileEnvVar); configFile != "" {
	// 	if content, err :=  readSecretFile(configFile); err == nil {
	// 		log.Printf("Loaded %s from file: %s", keyUpper, configFile)
	// 		return content
	// 	} else {
	// 		log.Printf("Warning: Failed to read %s (%s): %v", fileEnvVar, configFile, err)
	// 	}
	// }

	// Check direct environment variable
	if value := os.Getenv(keyUpper); value != "" {
		return value
	}

	// Check prod.env (case insensitive) - only if ProdEnvPath is initialized
	// This avoids circular dependency during path initialization
	if ProdEnvPath != "" {
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
	}

	// Try default Docker secrets location (case insensitive)
	// secretPaths := []string{
	// 	"/run/secrets/" + keyUpper,
	// 	"/run/secrets/" + keyLower,
	// 	"/run/secrets/" + keyTitle,
	// }
	// for _, secretPath := range secretPaths {
	// 	if content, err := readSecretFile(secretPath); err == nil {
	// 		log.Printf("Loaded %s from Docker secrets: %s", keyUpper, secretPath)
	// 		return content
	// 	}
	// }

	// Return default value
	return defaultValue
}
