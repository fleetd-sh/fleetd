package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fleetd.sh/internal/version"
	"github.com/spf13/cobra"
)

var (
	updateYes bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update fleetctl to the latest version",
	Long:  "Check for updates and automatically install the latest version of fleetctl",
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().BoolVarP(&updateYes, "yes", "y", false, "Skip confirmation prompt")
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

	printWarning("New version available: %s (current: %s)", latestVersion, currentVersion)
	fmt.Println()

	// Ask for confirmation unless --yes flag is used
	if !updateYes {
		fmt.Print("Update to the latest version? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			printInfo("Update cancelled")
			return nil
		}
	}

	fmt.Println()
	printInfo("Downloading fleetctl %s...", latestVersion)

	// Download and install
	if err := downloadAndInstall(latestVersion); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	printSuccess("Successfully updated to version %s", latestVersion)
	printInfo("Run 'fleetctl version' to verify")

	return nil
}

func downloadAndInstall(version string) error {
	// Detect platform
	platform := runtime.GOOS
	arch := runtime.GOARCH

	// Build download URL
	filename := fmt.Sprintf("fleetctl_%s_%s_%s.tar.gz", version, platform, arch)
	url := fmt.Sprintf("https://github.com/fleetd-sh/fleetd/releases/download/v%s/%s", version, filename)

	// Download to temp file
	tmpDir, err := os.MkdirTemp("", "fleetctl-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, filename)
	if err := downloadFile(tmpFile, url); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	// Extract binary
	binaryPath := filepath.Join(tmpDir, "fleetctl")
	if err := extractTarGz(tmpFile, tmpDir); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	// Get current executable path
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return fmt.Errorf("failed to resolve symlink: %w", err)
	}

	// Check if we need sudo
	needsSudo := !isWritable(currentExe)

	// Replace binary
	if needsSudo {
		printInfo("Updating requires elevated privileges...")
		if err := replaceBinaryWithSudo(binaryPath, currentExe); err != nil {
			return err
		}
	} else {
		if err := replaceBinary(binaryPath, currentExe); err != nil {
			return err
		}
	}

	return nil
}

func downloadFile(filepath, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractTarGz(tarGzPath, destDir string) error {
	file, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeReg:
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

func isWritable(path string) bool {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	file.Close()
	return true
}

func replaceBinary(src, dest string) error {
	// Backup current binary
	backup := dest + ".old"
	if err := os.Rename(dest, backup); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Copy new binary
	if err := copyFile(src, dest); err != nil {
		// Restore backup on failure
		os.Rename(backup, dest)
		return fmt.Errorf("failed to copy new binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(dest, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Remove backup
	os.Remove(backup)

	return nil
}

func replaceBinaryWithSudo(src, dest string) error {
	// Use sudo to replace the binary
	cmd := exec.Command("sudo", "cp", src, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy with sudo: %w", err)
	}

	// Set permissions
	cmd = exec.Command("sudo", "chmod", "755", dest)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
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
