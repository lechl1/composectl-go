package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// matchResult contains the matched template path and extracted parameters
type matchResult struct {
	templatePath string
	params       map[string]string
}

// HandleRoot handles the main HTTP route
func HandleRoot(w http.ResponseWriter, r *http.Request) {
	// Convention: /X matches pages/X/X.html or pages/X/[param]/[param].html
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	// Try to match the route with dynamic segments
	match, err := matchRoute(r.URL.Path)
	if err != nil {
		log.Printf("Route match error: %v", err)
		http.NotFound(w, r)
		return
	}

	// Load and render the page template
	bodyTemplate, err := template.ParseFiles(match.templatePath)
	if err != nil {
		log.Printf("Template parse error: %v", err)
		http.NotFound(w, r)
		return
	}

	// Prepare page data with URL parameters
	templateData := map[string]interface{}{
		"Title": strings.ToTitle(path),
	}

	// Add all URL parameters to page data
	for key, value := range match.params {
		templateData[key] = value
	}

	// Find the deepest index.html for the path
	layoutPath := findDeepestIndexHTML(r.URL.Path)
	layoutTemplate := template.Must(template.ParseFiles(layoutPath))

	// Run scripts from all ancestor directories
	pageDir := filepath.Dir(match.templatePath)
	scriptData, err := runAncestorScripts(pageDir, match.params)
	if err != nil {
		log.Printf("Error running scripts: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add all URL parameters to layout data as well
	for key, value := range match.params {
		templateData[key] = value
	}

	// Add all script data to templateData as maps/slices for direct template access
	for name, data := range scriptData {
		templateData[name] = data
	}

	// Add stacks data to templateData
	stacksData, err := getStacksData()
	if err != nil {
		log.Printf("Error getting stacks data: %v", err)
		// Don't fail the whole request, just log the error
		templateData["stacks"] = []interface{}{}
	} else {
		templateData["stacks"] = stacksData
	}

	// Load all components
	components, err := loadComponents()
	if err != nil {
		log.Printf("Error loading components: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add all components to templateData
	for name, tpl := range components {
		var content bytes.Buffer
		if err := tpl.Execute(&content, templateData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		templateData[name] = template.HTML(content.String())
	}

	// Render the page template to get its content
	var bodyContent bytes.Buffer
	if err := bodyTemplate.Execute(&bodyContent, templateData); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templateData["Body"] = template.HTML(bodyContent.String())

	if err := layoutTemplate.Execute(w, templateData); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// matchRoute tries to match a URL path to a template, handling dynamic segments like [stack]
func matchRoute(urlPath string) (*matchResult, error) {
	// Remove leading slash and split path
	cleanPath := strings.TrimPrefix(urlPath, "/")
	if cleanPath == "" {
		return nil, fmt.Errorf("empty path")
	}

	urlSegments := strings.Split(cleanPath, "/")
	params := make(map[string]string)

	// Build the path progressively, matching dynamic segments
	currentPath := "pages"
	for _, segment := range urlSegments {
		// Try exact match first
		exactPath := filepath.Join(currentPath, segment)
		if info, err := os.Stat(exactPath); err == nil && info.IsDir() {
			currentPath = exactPath
			continue
		}

		// Try to find a dynamic segment directory [xxx]
		entries, err := os.ReadDir(currentPath)
		if err != nil {
			return nil, err
		}

		matched := false
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			name := entry.Name()
			if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
				// Extract parameter name from [paramName]
				paramName := name[1 : len(name)-1]
				params[paramName] = segment
				currentPath = filepath.Join(currentPath, name)
				matched = true
				break
			}
		}

		if !matched {
			return nil, fmt.Errorf("no match found for segment: %s", segment)
		}
	}

	// Determine the final template file name
	// For the last segment, use the last URL segment or dynamic param name
	lastSegment := urlSegments[len(urlSegments)-1]

	// Check if current path contains a dynamic segment directory
	dirName := filepath.Base(currentPath)
	var templateName string

	if strings.HasPrefix(dirName, "[") && strings.HasSuffix(dirName, "]") {
		templateName = dirName + ".html"
	} else {
		templateName = lastSegment + ".html"
	}

	templatePath := filepath.Join(currentPath, templateName)

	// Verify the template exists
	if _, err := os.Stat(templatePath); err != nil {
		return nil, fmt.Errorf("template not found: %s", templatePath)
	}

	return &matchResult{
		templatePath: templatePath,
		params:       params,
	}, nil
}

// loadComponents loads all component files matching "components/X/X.html" pattern
// and returns them as a map with the filename (without extension) as the key
func loadComponents() (map[string]*template.Template, error) {
	components := make(map[string]*template.Template)
	componentsDir := "components"
	if _, err := os.Stat(componentsDir); os.IsNotExist(err) {
		return components, nil
	}

	entries, err := os.ReadDir(componentsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		componentName := entry.Name()
		componentPath := filepath.Join(componentsDir, componentName, componentName+".html")

		if _, err := os.Stat(componentPath); err == nil {
			content, err := os.ReadFile(componentPath)
			if err != nil {
				log.Printf("Error reading component %s: %v", componentPath, err)
				continue
			}
			components[componentName], err = template.New(componentName).Parse(string(content))
			log.Printf("Loaded component: %s", componentName)
		}
	}

	return components, nil
}

// findDeepestIndexHTML returns the path to the deepest index.html for a given path
func findDeepestIndexHTML(requestPath string) string {
	// Start with the root index.html
	deepestIndex := "pages/index.html"

	// Split the path and check for index.html at each level
	parts := strings.Split(strings.TrimPrefix(requestPath, "/"), "/")
	for i := 0; i < len(parts); i++ {
		potentialIndex := filepath.Join("pages", filepath.Join(parts[:i+1]...), "index.html")
		if _, err := os.Stat(potentialIndex); err == nil {
			deepestIndex = potentialIndex
		}
	}

	return deepestIndex
}

// runScriptsInDirectory finds and executes all .sh scripts in a directory
// and returns their JSON output parsed into a map
func runScriptsInDirectory(dirPath string, params map[string]string) (map[string]interface{}, error) {
	scriptData := make(map[string]interface{})

	// Check if the directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return scriptData, nil
	}

	// Read directory entries
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return scriptData, err
	}

	// Find all .sh files
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
			continue
		}

		scriptPath := filepath.Join(dirPath, entry.Name())
		scriptName := strings.TrimSuffix(entry.Name(), ".sh")

		log.Printf("Executing script: %s", scriptPath)

		// Execute the script
		cmd := exec.Command("/bin/bash", scriptPath)

		// Set environment variables from URL parameters
		env := os.Environ()
		for key, value := range params {
			env = append(env, fmt.Sprintf("%s=%s", strings.ToUpper(key), value))
		}
		cmd.Env = env

		// Capture output
		output, err := cmd.Output()
		if err != nil {
			log.Printf("Error executing script %s: %v", scriptPath, err)
			// If script fails, store error info
			scriptData[scriptName] = map[string]interface{}{
				"error": err.Error(),
			}
			continue
		}

		// Parse JSON output
		var jsonData interface{}
		if err := json.Unmarshal(output, &jsonData); err != nil {
			log.Printf("Error parsing JSON from script %s: %v", scriptPath, err)
			// If JSON parsing fails, store the raw output as a string
			scriptData[scriptName] = string(output)
			continue
		}
		// Store parsed data with script name as key
		scriptData[scriptName] = jsonData
		log.Printf("Script %s executed successfully", scriptName)
	}

	return scriptData, nil
}

// runAncestorScripts executes scripts from all ancestor directories in the pages hierarchy
// starting from the root pages directory down to the specified directory
func runAncestorScripts(pageDir string, params map[string]string) (map[string]interface{}, error) {
	scriptData := make(map[string]interface{})

	// Get all ancestor directories from root to the page directory
	ancestors := getAncestorDirectories(pageDir)

	log.Printf("Running scripts from %d ancestor directories", len(ancestors))

	// Run scripts in each ancestor directory, from root to leaf
	for _, ancestorDir := range ancestors {
		dirScripts, err := runScriptsInDirectory(ancestorDir, params)
		if err != nil {
			return scriptData, err
		}

		// Merge scripts from this directory into the overall data
		// Later scripts override earlier ones if they have the same name
		for name, data := range dirScripts {
			scriptData[name] = data
		}
	}

	return scriptData, nil
}

// getAncestorDirectories returns all ancestor directories from pages root to the given directory
func getAncestorDirectories(targetDir string) []string {
	var ancestors []string

	// Clean and normalize the path
	targetDir = filepath.Clean(targetDir)

	// Start from the target and work up to the pages directory
	currentDir := targetDir
	pagesDir := filepath.Clean("pages")

	for {
		// Add current directory to the front of the list
		ancestors = append([]string{currentDir}, ancestors...)

		// If we've reached the pages directory, stop
		if currentDir == pagesDir {
			break
		}

		// Move to parent directory
		parentDir := filepath.Dir(currentDir)

		// If we can't go up anymore or went above pages, stop
		if parentDir == currentDir || !strings.HasPrefix(currentDir, pagesDir) {
			break
		}

		currentDir = parentDir
	}

	// Return in order from root (pages) to leaf (targetDir)
	return ancestors
}
