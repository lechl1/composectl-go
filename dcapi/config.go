package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
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
	StacksDir = getConfig(args, "stacks_dir", getDefaultStacksDir())

	// Get prod.env path from config (respects --env-path argument)
	// Default to StacksDir/prod.env if not specified
	defaultEnvPath := filepath.Join(StacksDir, "prod.env")
	ProdEnvPath = getConfig(args, "env_path", defaultEnvPath)

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

// GetAdminUsername retrieves the ADMIN_USERNAME configuration with the following priority:
// 1. Check program arguments for -admin_username or --admin_username flag
// 2. Check ADMIN_USERNAME_FILE env var (Docker secrets pattern)
// 3. Check ADMIN_USERNAME env var
// 4. Check prod.env file (case insensitive)
// 5. Check default Docker secrets location (/run/secrets/ADMIN_USERNAME - case insensitive)
// 6. Default to ""
func GetAdminUsername(args []string) string {
	return getConfig(args, "admin_username", "")
}

// GetAdminPassword retrieves the ADMIN_PASSWORD configuration with the following priority:
// 1. Check program arguments for -admin_password or --admin_password flag
// 2. Check ADMIN_PASSWORD_FILE env var (Docker secrets pattern)
// 3. Check ADMIN_PASSWORD env var
// 4. Check prod.env file (case insensitive)
// 5. Check default Docker secrets location (/run/secrets/ADMIN_PASSWORD - case insensitive)
// 6. Default to ""
func GetAdminPassword(args []string) string {
	return getConfig(args, "admin_password", "")
}

// GetSecretKey retrieves the SECRET_KEY configuration with the following priority:
// 1. Check program arguments for -secret-key or --secret-key flag
// 2. Check SECRET_KEY_FILE env var (Docker secrets pattern)
// 3. Check SECRET_KEY env var
// 4. Check prod.env file (case insensitive)
// 5. Check default Docker secrets location (/run/secrets/SECRET_KEY - case insensitive)
// 6. Generate and save a new random secret key to prod.env
func GetSecretKey(args []string) string {
	secretKey := getConfig(args, "secret_key", "")

	// If no secret key found, generate one and save it to prod.env
	if secretKey == "" {
		var err error
		secretKey, err = generateAndSaveSecretKey()
		if err != nil {
			log.Fatalf("Failed to generate secret key: %v", err)
		}
	}

	return secretKey
}

// generateAndSaveSecretKey generates a new random secret key and saves it to prod.env
func generateAndSaveSecretKey() (string, error) {
	// Generate a 64-character random secret key
	secretKey, err := generateURLSafePassword(64)
	if err != nil {
		return "", fmt.Errorf("failed to generate secret key: %w", err)
	}

	// Append to prod.env
	file, err := os.OpenFile(ProdEnvPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to open prod.env for appending: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString(fmt.Sprintf("SECRET_KEY=%s\n", secretKey))

	if err := writer.Flush(); err != nil {
		return "", fmt.Errorf("failed to write secret key to prod.env: %w", err)
	}

	log.Printf("Generated and saved SECRET_KEY to %s", ProdEnvPath)
	return secretKey, nil
}

// generateURLSafePassword generates a cryptographically secure URL-safe random password
func generateURLSafePassword(length int) (string, error) {
	// Generate random bytes (we need more bytes than the final length due to base64 encoding)
	numBytes := (length * 6) / 8
	if numBytes < length {
		numBytes = length
	}

	randomBytes := make([]byte, numBytes)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64 URL-safe format and remove padding
	password := base64.URLEncoding.EncodeToString(randomBytes)
	password = strings.TrimRight(password, "=")

	// Truncate to desired length
	if len(password) > length {
		password = password[:length]
	}

	return password, nil
}

// ensureProdEnvCredentials ensures that prod.env exists with required credentials
func ensureProdEnvCredentials() error {
	// Check if prod.env exists
	_, err := os.Stat(ProdEnvPath)
	fileExists := err == nil

	// Read existing prod.env if it exists
	existingVars := make(map[string]string)
	if fileExists {
		existingVars, err = readProdEnv(ProdEnvPath)
		if err != nil {
			return fmt.Errorf("failed to read existing prod.env: %w", err)
		}
	}

	// Check if credentials are missing (case-insensitive check)
	hasUsername := false
	hasPassword := false

	for key := range existingVars {
		keyLower := strings.ToLower(key)
		if keyLower == "admin_username" {
			hasUsername = true
		}
		if keyLower == "admin_password" {
			hasPassword = true
		}
	}

	// If both exist, nothing to do
	if hasUsername && hasPassword {
		return nil
	}

	// Generate random password
	randomPassword, err := generateURLSafePassword(32)
	if err != nil {
		return fmt.Errorf("failed to generate random password: %w", err)
	}

	// Prepare credentials to add
	needsUsername := !hasUsername
	needsPassword := !hasPassword

	// If file doesn't exist, create it with header
	if !fileExists {
		file, err := os.OpenFile(ProdEnvPath, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to create prod.env: %w", err)
		}
		defer file.Close()

		writer := bufio.NewWriter(file)
		writer.WriteString("# Auto-generated secrets for Docker Compose\n")
		writer.WriteString("# This file is managed automatically by dc\n")
		writer.WriteString("# Do not edit manually unless you know what you are doing\n")
		writer.WriteString("\n")

		if needsUsername {
			writer.WriteString("ADMIN_USERNAME=admin\n")
		}
		if needsPassword {
			writer.WriteString(fmt.Sprintf("ADMIN_PASSWORD=%s\n", randomPassword))
		}

		if err := writer.Flush(); err != nil {
			return fmt.Errorf("failed to write prod.env: %w", err)
		}

		log.Printf("Created %s with auto-generated credentials", ProdEnvPath)
		log.Printf("Admin credentials:")
		log.Printf("  Username: admin")
		log.Printf("  Password: %s", randomPassword)
		log.Printf("")
		log.Printf("⚠️  IMPORTANT: Save this password securely! It won't be displayed again.")
		log.Printf("")

		return nil
	}

	// File exists but missing some credentials - append them
	file, err := os.OpenFile(ProdEnvPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open prod.env for appending: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	if needsUsername {
		writer.WriteString("ADMIN_USERNAME=admin\n")
		log.Printf("Added ADMIN_USERNAME to %s", ProdEnvPath)
	}
	if needsPassword {
		writer.WriteString(fmt.Sprintf("ADMIN_PASSWORD=%s\n", randomPassword))
		log.Printf("Added ADMIN_PASSWORD to %s", ProdEnvPath)
		log.Printf("Generated password: %s", randomPassword)
		log.Printf("⚠️  IMPORTANT: Save this password securely!")
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to write to prod.env: %w", err)
	}

	return nil
}

// GetAdminCredentials retrieves admin credentials, ensuring they exist in prod.env
// If credentials are missing, it generates them and adds them to prod.env
func GetAdminCredentials(args []string) (username, password string, err error) {
	// Ensure credentials exist in prod.env
	if err := ensureProdEnvCredentials(); err != nil {
		return "", "", fmt.Errorf("failed to ensure credentials: %w", err)
	}

	// Now retrieve the credentials using the standard config system
	username = GetAdminUsername(args)
	password = GetAdminPassword(args)

	// Final validation
	if username == "" || password == "" {
		return "", "", fmt.Errorf("failed to retrieve credentials after ensuring they exist")
	}

	return username, password, nil
}
