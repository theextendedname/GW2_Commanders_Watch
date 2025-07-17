package processor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	FightLogTemp = "FightLogTemp"
	LogArchive   = "Log_Archive"
)

// ProcessLog runs the Elite Insights CLI and returns the path to the temporary JSON file it creates.
// It no longer handles run creation or file archiving.
func ProcessLog(logPath string) (string, error) {

	// 1. Ensure FightLogTemp directory exists
	if err := os.MkdirAll(FightLogTemp, 0755); err != nil {
		return "", fmt.Errorf("failed to create %s directory: %w", FightLogTemp, err)
	}

	// 2. Run Elite Insights CLI
	cliPath := filepath.Join("GW2EICLI", "GuildWars2EliteInsights-CLI.exe")
	confPath := "ELI3.conf"
	cmd := exec.Command(cliPath, "-c", confPath, logPath)

	output, err := cmd.CombinedOutput()

	// Check for specific .NET error
	if strings.Contains(string(output), "You must install .NET to run this application") {
		return "", fmt.Errorf("EliteInsights-CLI required .NET runtime not found. Please install .NET 8.0.12 or a compatible version to continue")
	}

	// Check for other execution errors
	if err != nil {
		return "", fmt.Errorf("failed to execute Elite Insights CLI: %w\nOutput: %s", err, string(output))
	}

	// 3. Determine expected output file name and wait for it
	baseName := filepath.Base(logPath)
	ext := filepath.Ext(baseName)
	jsonBaseName := strings.TrimSuffix(baseName, ext) + "_detailed_wvw_kill.json"
	tempJSONPath := filepath.Join(FightLogTemp, jsonBaseName)

	unlockedJSONPath, err := waitForFile(tempJSONPath)
	if err != nil {
		return "", fmt.Errorf("error waiting for JSON file: %w", err)
	}

	return unlockedJSONPath, nil
}

// ArchiveLogFiles moves the generated .json and .html files from the temp folder to the final run archive directory.
func ArchiveLogFiles(tempJsonPath, finalRunPath string) (string, error) {
	if err := os.MkdirAll(finalRunPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create final run directory %s: %w", finalRunPath, err)
	}

	// Infer HTML path from JSON path
	jsonBaseName := filepath.Base(tempJsonPath)
	htmlBaseName := strings.TrimSuffix(jsonBaseName, ".json") + ".html"
	tempHTMLPath := filepath.Join(FightLogTemp, htmlBaseName)

	// Move JSON file
	archivedJSONPath := filepath.Join(finalRunPath, jsonBaseName)
	if err := moveFileWithRetry(tempJsonPath, archivedJSONPath, 3); err != nil {
		return "", fmt.Errorf("failed to move JSON file: %w", err)
	}

	// Move HTML file
	unlockedHTMLPath, err := waitForFile(tempHTMLPath)
	if err != nil {
		fmt.Printf("Warning: could not find matching HTML file to archive: %v\n", err)
	} else {
		archivedHTMLPath := filepath.Join(finalRunPath, htmlBaseName)
		if err := moveFileWithRetry(unlockedHTMLPath, archivedHTMLPath, 3); err != nil {
			// Don't return an error, just print a warning, as the JSON is the critical part
			fmt.Printf("Warning: failed to move HTML file: %v\n", err)
		}
	}

	return archivedJSONPath, nil
}

// moveFileWithRetry attempts to copy a file and then delete the source, with a given number of retries.
func moveFileWithRetry(src, dest string, retries int) error {
	var lastErr error
	for i := 0; i < retries; i++ {
		// Open the source file
		sourceFile, err := os.Open(src)
		if err != nil {
			lastErr = fmt.Errorf("could not open source file %s: %w", src, err)
			time.Sleep(250 * time.Millisecond)
			continue
		}

		// Create the destination file
		destFile, err := os.Create(dest)
		if err != nil {
			sourceFile.Close() // Close source since we're failing here
			lastErr = fmt.Errorf("could not create destination file %s: %w", dest, err)
			time.Sleep(250 * time.Millisecond)
			continue
		}

		// Copy data
		_, err = destFile.ReadFrom(sourceFile)

		// Explicitly close files right after use
		sourceFile.Close()
		destFile.Close()

		if err != nil {
			lastErr = fmt.Errorf("could not copy data from %s to %s: %w", src, dest, err)
			time.Sleep(250 * time.Millisecond)
			continue
		}

		// Verify the copy by checking file info
		srcInfo, err := os.Stat(src)
		if err != nil {
			lastErr = fmt.Errorf("could not stat source file %s: %w", src, err)
			time.Sleep(250 * time.Millisecond)
			continue
		}
		destInfo, err := os.Stat(dest)
		if err != nil {
			lastErr = fmt.Errorf("could not stat destination file %s: %w", dest, err)
			time.Sleep(250 * time.Millisecond)
			continue
		}

		if srcInfo.Size() != destInfo.Size() {
			lastErr = fmt.Errorf("file copy failed: size mismatch for %s", src)
			time.Sleep(250 * time.Millisecond)
			continue
		}

		// If copy is verified, delete the source file
		if err := os.Remove(src); err != nil {
			lastErr = fmt.Errorf("failed to remove source file %s after copy: %w", src, err)
			time.Sleep(250 * time.Millisecond)
			continue
		}

		// Success
		return nil
	}
	return fmt.Errorf("failed to move file %s after %d retries: %w", src, retries, lastErr)
}

// waitForFile polls for a file to exist and then for it to be unlocked.
func waitForFile(filePath string) (string, error) {
	// Wait for file to exist
	timeout := time.After(60 * time.Second) // 30-second timeout for file creation
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return "", fmt.Errorf("timed out waiting for file to exist: %s", filePath)
		case <-ticker.C:
			_, err := os.Stat(filePath)
			if err == nil {
				goto UNLOCK_CHECK
			}
			if !os.IsNotExist(err) {
				return "", fmt.Errorf("error checking file existence for %s: %w", filePath, err)
			}
		}
	}

UNLOCK_CHECK:
	// Wait for file to be unlocked
	timeout = time.After(60 * time.Second) // 30-second timeout for file unlock
	for {
		select {
		case <-timeout:
			return "", fmt.Errorf("timed out waiting for file to unlock: %s", filePath)
		case <-ticker.C:
			file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
			if err == nil {
				file.Close()
				return filePath, nil // success
			}
		}
	}
}
