package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HandleThumbnail serves thumbnails for Docker Hub images
// GET /thumbnail/{image} returns the thumbnail for the specified Docker Hub image
func HandleThumbnail(w http.ResponseWriter, r *http.Request) {
	// Extract image name from URL path
	imageName := strings.TrimPrefix(r.URL.Path, "/thumbnail/")
	if imageName == "" {
		http.Error(w, "Image name is required", http.StatusBadRequest)
		return
	}

	// Create thumbnails directory if it doesn't exist
	thumbnailsDir := "thumbnails"
	if err := os.MkdirAll(thumbnailsDir, 0755); err != nil {
		log.Printf("Error creating thumbnails directory: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Generate safe filename from image name
	safeFilename := generateSafeFilename(imageName)
	thumbnailPath := filepath.Join(thumbnailsDir, safeFilename+".jpg")

	// Check if thumbnail already exists
	if _, err := os.Stat(thumbnailPath); err == nil {
		// Thumbnail exists, serve it
		http.ServeFile(w, r, thumbnailPath)
		return
	}

	// Thumbnail doesn't exist, scrape Docker Hub
	log.Printf("Thumbnail not found for %s, scraping Docker Hub...", imageName)
	gravatarURL, err := scrapeDockerHubGravatar(imageName)
	if err != nil {
		log.Printf("Error scraping Docker Hub for %s: %v", imageName, err)
		http.Error(w, "Failed to fetch thumbnail", http.StatusNotFound)
		return
	}

	// Download the gravatar image
	if err := downloadImage(gravatarURL, thumbnailPath); err != nil {
		log.Printf("Error downloading gravatar for %s: %v", imageName, err)
		http.Error(w, "Failed to download thumbnail", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully downloaded thumbnail for %s", imageName)
	// Serve the newly downloaded thumbnail
	http.ServeFile(w, r, thumbnailPath)
}

// generateSafeFilename creates a safe filename from an image name using MD5 hash
func generateSafeFilename(imageName string) string {
	hash := md5.Sum([]byte(imageName))
	return hex.EncodeToString(hash[:])
}

// scrapeDockerHubGravatar fetches the Docker Hub page and extracts the gravatar URL
func scrapeDockerHubGravatar(imageName string) (string, error) {
	// Remove docker.io/ prefix if present
	cleanImageName := strings.TrimPrefix(imageName, "docker.io/")

	// Remove version tag if present (everything after and including ":")
	if colonIndex := strings.Index(cleanImageName, ":"); colonIndex != -1 {
		cleanImageName = cleanImageName[:colonIndex]
	}

	// Try official Docker Hub URL format first (for library images like nginx, postgres, etc.)
	dockerHubURLs := []string{
		fmt.Sprintf("https://hub.docker.com/_/%s", cleanImageName),
		fmt.Sprintf("https://hub.docker.com/r/%s", cleanImageName),
	}

	var lastErr error
	for _, dockerHubURL := range dockerHubURLs {
		gravatarURL, err := tryFetchGravatar(dockerHubURL)
		if err == nil {
			return gravatarURL, nil
		}
		lastErr = err
	}

	return "", lastErr
}

// tryFetchGravatar attempts to fetch and extract gravatar URL from a Docker Hub page
func tryFetchGravatar(dockerHubURL string) (string, error) {
	// Fetch the page
	fmt.Printf("Fetching Docker Hub page: %s\n", dockerHubURL)
	resp, err := http.Get(dockerHubURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch Docker Hub page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Docker Hub returned status %d for %s", resp.StatusCode, dockerHubURL)
	}

	// Read the HTML content
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Search for gravatar.com URLs using regex
	// Looking for patterns like: https://www.gravatar.com/avatar/{hexadecimal}
	gravatarRegex := regexp.MustCompile(`https://(?:www\.)?gravatar\.com/avatar/([a-fA-F0-9]+)`)
	matches := gravatarRegex.FindStringSubmatch(string(body))

	if len(matches) < 1 {
		return "", fmt.Errorf("no gravatar URL found in Docker Hub page %s", dockerHubURL)
	}

	// Return the full gravatar URL
	return matches[0], nil
}

// downloadImage downloads an image from a URL and saves it to the specified path
func downloadImage(url string, destPath string) error {
	// Fetch the image
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("image download returned status %d", resp.StatusCode)
	}

	// Create the destination file
	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer outFile.Close()

	// Copy the image data to the file
	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	return nil
}
