package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"fleetd.sh/internal/artifact"
)

var artifactCmd = &cobra.Command{
	Use:   "artifact",
	Short: "Manage deployment artifacts",
	Long:  `Upload, list, verify, and manage deployment artifacts.`,
}

var artifactUploadCmd = &cobra.Command{
	Use:   "upload [file]",
	Short: "Upload a new artifact",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		// Get flags
		name, _ := cmd.Flags().GetString("name")
		version, _ := cmd.Flags().GetString("version")
		artifactType, _ := cmd.Flags().GetString("type")
		metadata, _ := cmd.Flags().GetString("metadata")
		sign, _ := cmd.Flags().GetBool("sign")

		if name == "" {
			name = filepath.Base(filePath)
		}

		// Open file
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()

		// Get file info
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat file: %w", err)
		}

		// Create multipart form
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)

		// Add file field
		part, err := writer.CreateFormFile("artifact", filepath.Base(filePath))
		if err != nil {
			return fmt.Errorf("failed to create form file: %w", err)
		}

		if _, err := io.Copy(part, file); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}

		// Add metadata fields
		writer.WriteField("name", name)
		writer.WriteField("version", version)
		writer.WriteField("type", artifactType)
		writer.WriteField("metadata", metadata)
		writer.WriteField("sign", fmt.Sprintf("%v", sign))

		if err := writer.Close(); err != nil {
			return fmt.Errorf("failed to close writer: %w", err)
		}

		// Create request
		url := fmt.Sprintf("%s/api/v1/artifacts", getServerURL())
		req, err := http.NewRequest("POST", url, &buf)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", writer.FormDataContentType())
		addAuthHeaders(req)

		// Send request
		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to upload artifact: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			Artifact *artifact.Artifact `json:"artifact"`
			Message  string              `json:"message"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		fmt.Printf("Artifact uploaded successfully:\n")
		fmt.Printf("  ID:       %s\n", result.Artifact.ID)
		fmt.Printf("  Name:     %s\n", result.Artifact.Name)
		fmt.Printf("  Version:  %s\n", result.Artifact.Version)
		fmt.Printf("  Size:     %d bytes\n", info.Size())
		fmt.Printf("  Checksum: %s\n", result.Artifact.Checksum)
		fmt.Printf("  URL:      %s\n", result.Artifact.URL)

		return nil
	},
}

var artifactListCmd = &cobra.Command{
	Use:   "list",
	Short: "List artifacts",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get filters
		name, _ := cmd.Flags().GetString("name")
		artifactType, _ := cmd.Flags().GetString("type")
		limit, _ := cmd.Flags().GetInt("limit")

		// Build query parameters
		params := fmt.Sprintf("?limit=%d", limit)
		if name != "" {
			params += fmt.Sprintf("&name=%s", name)
		}
		if artifactType != "" {
			params += fmt.Sprintf("&type=%s", artifactType)
		}

		// Create request
		url := fmt.Sprintf("%s/api/v1/artifacts%s", getServerURL(), params)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		// Send request
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to list artifacts: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			Artifacts []*artifact.Artifact `json:"artifacts"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Display artifacts
		fmt.Printf("%-36s %-20s %-10s %-10s %s\n", "ID", "Name", "Version", "Type", "Uploaded")
		fmt.Println(string(make([]byte, 100)))

		for _, a := range result.Artifacts {
			fmt.Printf("%-36s %-20s %-10s %-10s %s\n",
				a.ID, a.Name, a.Version, a.Type,
				a.UploadedAt.Format("2006-01-02 15:04"))
		}

		fmt.Printf("\nTotal: %d artifacts\n", len(result.Artifacts))

		return nil
	},
}

var artifactVerifyCmd = &cobra.Command{
	Use:   "verify [artifact-id]",
	Short: "Verify artifact integrity",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		artifactID := args[0]

		// Create request
		url := fmt.Sprintf("%s/api/v1/artifacts/%s/verify", getServerURL(), artifactID)
		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		// Send request
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to verify artifact: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("verification failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			Valid   bool   `json:"valid"`
			Message string `json:"message"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if result.Valid {
			fmt.Println("✓ Artifact verification successful")
		} else {
			fmt.Printf("✗ Artifact verification failed: %s\n", result.Message)
			return fmt.Errorf("verification failed")
		}

		return nil
	},
}

var artifactDeleteCmd = &cobra.Command{
	Use:   "delete [artifact-id]",
	Short: "Delete an artifact",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		artifactID := args[0]

		// Confirm deletion
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Are you sure you want to delete artifact %s? (y/N): ", artifactID)
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		// Create request
		url := fmt.Sprintf("%s/api/v1/artifacts/%s", getServerURL(), artifactID)
		req, err := http.NewRequest("DELETE", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		// Send request
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to delete artifact: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("deletion failed with status %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Artifact %s deleted successfully\n", artifactID)

		return nil
	},
}

var artifactCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up old artifacts",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		days, _ := cmd.Flags().GetInt("days")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Create request
		url := fmt.Sprintf("%s/api/v1/artifacts/cleanup", getServerURL())

		payload := map[string]interface{}{
			"max_age_days": days,
			"dry_run":      dryRun,
		}

		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		addAuthHeaders(req)

		// Send request
		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to cleanup artifacts: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("cleanup failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			Deleted int    `json:"deleted"`
			Freed   int64  `json:"freed_bytes"`
			DryRun  bool   `json:"dry_run"`
			Message string `json:"message"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if result.DryRun {
			fmt.Println("DRY RUN - No artifacts were actually deleted")
		}

		fmt.Printf("Cleanup completed:\n")
		fmt.Printf("  Artifacts deleted: %d\n", result.Deleted)
		fmt.Printf("  Space freed: %.2f MB\n", float64(result.Freed)/(1<<20))

		return nil
	},
}

func init() {
	// Upload command flags
	artifactUploadCmd.Flags().StringP("name", "n", "", "Artifact name")
	artifactUploadCmd.Flags().StringP("version", "v", "", "Artifact version")
	artifactUploadCmd.Flags().StringP("type", "t", "binary", "Artifact type (binary, config, script)")
	artifactUploadCmd.Flags().StringP("metadata", "m", "{}", "Additional metadata (JSON)")
	artifactUploadCmd.Flags().BoolP("sign", "s", false, "Sign the artifact")

	// List command flags
	artifactListCmd.Flags().StringP("name", "n", "", "Filter by name")
	artifactListCmd.Flags().StringP("type", "t", "", "Filter by type")
	artifactListCmd.Flags().IntP("limit", "l", 100, "Limit results")

	// Delete command flags
	artifactDeleteCmd.Flags().BoolP("force", "f", false, "Force deletion without confirmation")

	// Cleanup command flags
	artifactCleanupCmd.Flags().IntP("days", "d", 90, "Delete artifacts older than this many days")
	artifactCleanupCmd.Flags().BoolP("dry-run", "n", false, "Perform dry run (don't actually delete)")

	// Add subcommands
	artifactCmd.AddCommand(artifactUploadCmd)
	artifactCmd.AddCommand(artifactListCmd)
	artifactCmd.AddCommand(artifactVerifyCmd)
	artifactCmd.AddCommand(artifactDeleteCmd)
	artifactCmd.AddCommand(artifactCleanupCmd)

	rootCmd.AddCommand(artifactCmd)
}