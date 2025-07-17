package eicli

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	githubAPIURL = "https://api.github.com/repos/baaron4/GW2-Elite-Insights-Parser/releases/latest"
	cliDir       = "GW2EICLI"
	tempDir      = "FightLogTemp" // Using the same temp dir as the processor
)

// CheckCLIExists verifies if the Elite Insights CLI executable is present.
func CheckCLIExists() bool {
	cliPath := filepath.Join(cliDir, "GuildWars2EliteInsights-CLI.exe")
	_, err := os.Stat(cliPath)
	return err == nil
}

// InstallCLI downloads and unzips the latest Elite Insights CLI if it's not already present.
// It sends status updates via the provided channel.
func InstallCLI(statusChan chan<- string) {
	if CheckCLIExists() {
		statusChan <- "Elite Insights CLI found."
		return
	}

	statusChan <- "Elite Insights CLI not found. Downloading..."

	// 1. Get latest release info from GitHub
	resp, err := http.Get(githubAPIURL)
	if err != nil {
		statusChan <- fmt.Sprintf("Error getting release info: %v", err)
		return
	}
	defer resp.Body.Close()

	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		statusChan <- fmt.Sprintf("Error parsing release info: %v", err)
		return
	}

	// 2. Find the correct download URL for "GW2EICLI.zip"
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == "GW2EICLI.zip" {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		statusChan <- "Error: Could not find GW2EICLI.zip in the latest release."
		return
	}

	// 3. Download the zip file to the temp directory
	statusChan <- "Downloading GW2EICLI.zip..."
	zipPath := filepath.Join(tempDir, "GW2EICLI.zip")
	if err := downloadFile(zipPath, downloadURL); err != nil {
		statusChan <- fmt.Sprintf("Error downloading zip: %v", err)
		return
	}
	defer os.Remove(zipPath) // Clean up the zip file afterwards

	// 4. Unzip the archive to the target directory
	statusChan <- "Extracting CLI..."
	if err := unzip(zipPath, cliDir); err != nil {
		statusChan <- fmt.Sprintf("Error extracting zip: %v", err)
		return
	}

	statusChan <- "Elite Insights CLI installed successfully."
}

func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func unzip(src, dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}
