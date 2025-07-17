package watcher

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Start initializes and runs the file system watcher.
func Start(watchPath string, eventChan chan<- string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Add all subdirectories to the watcher
	err = filepath.Walk(watchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// We only care about new files being created.
				if event.Op&fsnotify.Create == fsnotify.Create {
					// Check if it's a directory or a file
					info, err := os.Stat(event.Name)
					if err != nil {
						// File might be gone again, ignore
						continue
					}

					if info.IsDir() {
						// New directory created, add it to the watcher
						if err := watcher.Add(event.Name); err != nil {
							log.Printf("Error adding new directory to watcher: %v", err)
						}
						continue
					}

					// We are only interested in .zevtc files
					if strings.HasSuffix(strings.ToLower(event.Name), ".zevtc") {
						go func(filePath string) {
							// Poll the file until it's no longer locked
							for {
								file, err := os.OpenFile(filePath, os.O_RDONLY, 0644)
								if err == nil {
									// Success, file is not locked
									file.Close()
									absPath, _ := filepath.Abs(filePath)
									eventChan <- absPath
									break
								}
								// Wait a bit before trying again
								time.Sleep(250 * time.Millisecond)
							}
						}(event.Name)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Watcher error: %v", err)
			}
		}
	}()

	// Block forever
	<-make(chan struct{})
	return nil
}
