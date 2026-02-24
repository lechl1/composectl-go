package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// WatchFiles monitors the pages and components directories for changes
func WatchFiles() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Error creating file watcher:", err)
	}
	defer watcher.Close()

	// Watch pages directory
	err = addWatchRecursive(watcher, "pages")
	if err != nil {
		log.Println("Error watching pages directory:", err)
	}

	// Watch components directory (if it exists)
	if _, err := os.Stat("components"); err == nil {
		err = addWatchRecursive(watcher, "components")
		if err != nil {
			log.Println("Error watching components directory:", err)
		}
	}

	log.Println("File watcher started for pages and components directories")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
				log.Printf("File change detected: %s [%s]", event.Name, event.Op)
				broadcast <- FileChangeMessage{
					Type: event.Op.String(),
					Path: event.Name,
				}

				// If a new directory was created, watch it too
				if event.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						addWatchRecursive(watcher, event.Name)
					}
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("File watcher error:", err)
		}
	}
}

// addWatchRecursive adds a directory and all its subdirectories to the watcher
func addWatchRecursive(watcher *fsnotify.Watcher, path string) error {
	return filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = watcher.Add(walkPath)
			if err != nil {
				log.Printf("Error watching %s: %v", walkPath, err)
				return err
			}
			log.Printf("Watching: %s", walkPath)
		}
		return nil
	})
}
