package main

import (
	"log"
	"os"
	"path/filepath"
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
