package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"fleetd.sh/internal/version"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for fleetctl updates",
	Long:  "Check if a newer version of fleetctl is available",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

type GitHubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	HTMLURL    string `json:"html_url"`
}

func runUpdate(cmd *cobra.Command, args []string) error {
	currentVersion := strings.TrimPrefix(version.Version, "v")

	if currentVersion == "development" || currentVersion == "" {
		printInfo("Running development version, update check skipped")
		return nil
	}

	printInfo("Checking for updates...")

	// Fetch latest release from GitHub
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/fleetd-sh/fleetd/releases/latest")
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch latest release: HTTP %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to parse release info: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	if currentVersion == latestVersion {
		printSuccess("You're running the latest version: %s", currentVersion)
		return nil
	}

	printWarning("A newer version is available: %s (current: %s)", latestVersion, currentVersion)
	printInfo("Download: %s", release.HTMLURL)
	printInfo("Update with: curl -sSL https://get.fleetd.sh | sh")

	return nil
}

// CheckForUpdates performs a silent update check and shows a message if update is available
func CheckForUpdates() {
	currentVersion := strings.TrimPrefix(version.Version, "v")

	if currentVersion == "development" || currentVersion == "" {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/fleetd-sh/fleetd/releases/latest")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	if currentVersion != latestVersion {
		fmt.Println()
		printWarning("New version available: %s (current: %s)", latestVersion, currentVersion)
		printInfo("Run 'fleetctl update' for more information")
		fmt.Println()
	}
}
