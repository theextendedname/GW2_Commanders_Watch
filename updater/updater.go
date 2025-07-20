package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// IMPORTANT: Replace with the actual GitHub repository URL when available
	repoURL          = "theextendedname/GW2_Commanders_Watch"
	currentVersion   = "v0.1.1" // This should be updated with each new release and remember to change func (m *model) renderStatusBar in model.go line 365
	githubAPIRelease = "https://api.github.com/repos/"
)

// UpdateInfo holds the URL for the latest release.
type UpdateInfo struct {
	URL string
}

// CheckForUpdates compares the current app version with the latest release on GitHub.
func CheckForUpdates() (*UpdateInfo, error) {
	if repoURL == "YOUR_USERNAME/YOUR_REPONAME" {
		// Don't check if the placeholder URL is still there
		return nil, nil
	}

	apiURL := fmt.Sprintf("%s%s/releases/latest", githubAPIRelease, repoURL)
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status from GitHub API: %s", resp.Status)
	}

	var release struct {
		TagName    string `json:"tag_name"`
		HTMLURL    string `json:"html_url"`
		PreRelease bool   `json:"prerelease"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	// Simple version comparison (e.g., "v0.2.0" > "v0.1.0")
	if !release.PreRelease && release.TagName > currentVersion {
		return &UpdateInfo{URL: release.HTMLURL}, nil
	}

	return nil, nil // No update available or it's a pre-release
}
