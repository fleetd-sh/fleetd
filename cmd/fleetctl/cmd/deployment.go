package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var deploymentCmd = &cobra.Command{
	Use:   "deployment",
	Short: "Manage deployments",
	Long:  `Create, monitor, and manage device fleet deployments.`,
}

var deploymentCreateCmd = &cobra.Command{
	Use:   "create [manifest-file]",
	Short: "Create a new deployment",
	Long:  `Create a new deployment from a manifest file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifestFile := args[0]

		// Read manifest file
		data, err := os.ReadFile(manifestFile)
		if err != nil {
			return fmt.Errorf("failed to read manifest file: %w", err)
		}

		// Parse manifest (try YAML first, then JSON)
		var manifest map[string]interface{}
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			if err := json.Unmarshal(data, &manifest); err != nil {
				return fmt.Errorf("failed to parse manifest: %w", err)
			}
		}

		// Get flags
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		autoApprove, _ := cmd.Flags().GetBool("auto-approve")
		wait, _ := cmd.Flags().GetBool("wait")
		timeout, _ := cmd.Flags().GetDuration("timeout")

		if dryRun {
			fmt.Println("DRY RUN MODE - No deployment will be created")
			fmt.Printf("Manifest:\n%s\n", string(data))
			return nil
		}

		// Add auto-approve flag to manifest if specified
		if autoApprove {
			if strategy, ok := manifest["strategy"].(map[string]interface{}); ok {
				strategy["auto_approve"] = true
			}
		}

		// Create deployment
		body, _ := json.Marshal(manifest)
		url := fmt.Sprintf("%s/api/v1/deployments", getServerURL())
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to create deployment: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("deployment creation failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			DeploymentID string                 `json:"deployment_id"`
			Status       string                 `json:"status"`
			Message      string                 `json:"message"`
			Deployment   map[string]interface{} `json:"deployment"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		fmt.Printf("Deployment created successfully:\n")
		fmt.Printf("  ID:     %s\n", result.DeploymentID)
		fmt.Printf("  Status: %s\n", result.Status)

		// Wait for completion if requested
		if wait {
			fmt.Printf("\nWaiting for deployment to complete...\n")
			return waitForDeployment(result.DeploymentID, timeout)
		}

		return nil
	},
}

var deploymentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List deployments",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get filters
		status, _ := cmd.Flags().GetString("status")
		limit, _ := cmd.Flags().GetInt("limit")
		all, _ := cmd.Flags().GetBool("all")

		// Build query parameters
		params := fmt.Sprintf("?limit=%d", limit)
		if status != "" {
			params += fmt.Sprintf("&status=%s", status)
		}
		if all {
			params += "&all=true"
		}

		// Create request
		url := fmt.Sprintf("%s/api/v1/deployments%s", getServerURL(), params)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to list deployments: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			Deployments []struct {
				ID          string    `json:"id"`
				Name        string    `json:"name"`
				Status      string    `json:"status"`
				Strategy    string    `json:"strategy"`
				TargetCount int       `json:"target_count"`
				Progress    int       `json:"progress"`
				CreatedAt   time.Time `json:"created_at"`
				UpdatedAt   time.Time `json:"updated_at"`
			} `json:"deployments"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Display deployments
		fmt.Printf("%-36s %-20s %-12s %-10s %8s %s\n",
			"ID", "Name", "Status", "Strategy", "Progress", "Created")
		fmt.Println(strings.Repeat("-", 110))

		for _, d := range result.Deployments {
			progress := fmt.Sprintf("%d%%", d.Progress)
			if d.TargetCount > 0 {
				progress = fmt.Sprintf("%d%%", (d.Progress*100)/d.TargetCount)
			}

			fmt.Printf("%-36s %-20s %-12s %-10s %8s %s\n",
				d.ID, d.Name, d.Status, d.Strategy, progress,
				d.CreatedAt.Format("2006-01-02 15:04"))
		}

		fmt.Printf("\nTotal: %d deployments\n", len(result.Deployments))

		return nil
	},
}

var deploymentStatusCmd = &cobra.Command{
	Use:   "status [deployment-id]",
	Short: "Get deployment status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deploymentID := args[0]

		// Get flags
		detailed, _ := cmd.Flags().GetBool("detailed")
		follow, _ := cmd.Flags().GetBool("follow")

		if follow {
			return followDeployment(deploymentID)
		}

		// Get deployment status
		url := fmt.Sprintf("%s/api/v1/deployments/%s", getServerURL(), deploymentID)
		if detailed {
			url += "?detailed=true"
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to get deployment status: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var deployment map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&deployment); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Display deployment status
		fmt.Printf("Deployment Status:\n")
		fmt.Printf("  ID:          %v\n", deployment["id"])
		fmt.Printf("  Name:        %v\n", deployment["name"])
		fmt.Printf("  Status:      %v\n", deployment["status"])
		fmt.Printf("  Strategy:    %v\n", deployment["strategy"])
		fmt.Printf("  Created:     %v\n", deployment["created_at"])
		fmt.Printf("  Updated:     %v\n", deployment["updated_at"])

		if progress, ok := deployment["progress"].(map[string]interface{}); ok {
			fmt.Printf("\nProgress:\n")
			fmt.Printf("  Total:       %v\n", progress["total"])
			fmt.Printf("  Completed:   %v\n", progress["completed"])
			fmt.Printf("  Failed:      %v\n", progress["failed"])
			fmt.Printf("  In Progress: %v\n", progress["in_progress"])
		}

		if detailed {
			if devices, ok := deployment["devices"].([]interface{}); ok {
				fmt.Printf("\nDevice Status:\n")
				for _, device := range devices {
					if d, ok := device.(map[string]interface{}); ok {
						fmt.Printf("  - %v: %v\n", d["device_id"], d["status"])
					}
				}
			}
		}

		return nil
	},
}

var deploymentPauseCmd = &cobra.Command{
	Use:   "pause [deployment-id]",
	Short: "Pause a deployment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deploymentID := args[0]

		// Create request
		url := fmt.Sprintf("%s/api/v1/deployments/%s/pause", getServerURL(), deploymentID)
		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to pause deployment: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Deployment %s paused successfully\n", deploymentID)
		return nil
	},
}

var deploymentResumeCmd = &cobra.Command{
	Use:   "resume [deployment-id]",
	Short: "Resume a paused deployment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deploymentID := args[0]

		// Create request
		url := fmt.Sprintf("%s/api/v1/deployments/%s/resume", getServerURL(), deploymentID)
		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to resume deployment: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Deployment %s resumed successfully\n", deploymentID)
		return nil
	},
}

var deploymentRollbackCmd = &cobra.Command{
	Use:   "rollback [deployment-id]",
	Short: "Rollback a deployment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deploymentID := args[0]

		// Get flags
		force, _ := cmd.Flags().GetBool("force")
		reason, _ := cmd.Flags().GetString("reason")

		// Confirm rollback
		if !force {
			fmt.Printf("Are you sure you want to rollback deployment %s? (y/N): ", deploymentID)
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Println("Rollback cancelled")
				return nil
			}
		}

		// Create request
		payload := map[string]interface{}{
			"reason": reason,
		}

		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("%s/api/v1/deployments/%s/rollback", getServerURL(), deploymentID)
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		addAuthHeaders(req)

		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to rollback deployment: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("rollback failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			Success    bool     `json:"success"`
			RolledBack int      `json:"rolled_back"`
			Failed     int      `json:"failed"`
			Message    string   `json:"message"`
			Errors     []string `json:"errors"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if result.Success {
			fmt.Printf("Rollback completed successfully:\n")
			fmt.Printf("  Rolled back: %d devices\n", result.RolledBack)
			if result.Failed > 0 {
				fmt.Printf("  Failed:      %d devices\n", result.Failed)
			}
		} else {
			fmt.Printf("Rollback failed: %s\n", result.Message)
			for _, err := range result.Errors {
				fmt.Printf("  - %s\n", err)
			}
			return fmt.Errorf("rollback failed")
		}

		return nil
	},
}

var deploymentApproveCmd = &cobra.Command{
	Use:   "approve [deployment-id]",
	Short: "Approve a deployment waiting for approval",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deploymentID := args[0]

		// Create request
		url := fmt.Sprintf("%s/api/v1/deployments/%s/approve", getServerURL(), deploymentID)
		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to approve deployment: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Deployment %s approved successfully\n", deploymentID)
		return nil
	},
}

// Helper functions

func waitForDeployment(deploymentID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("deployment timed out after %v", timeout)
		case <-ticker.C:
			status, err := getDeploymentStatus(deploymentID)
			if err != nil {
				return err
			}

			fmt.Printf("Status: %s\n", status["status"])

			switch status["status"] {
			case "completed":
				fmt.Println("Deployment completed successfully")
				return nil
			case "failed", "rolled_back":
				return fmt.Errorf("deployment %s", status["status"])
			}
		}
	}
}

func followDeployment(deploymentID string) error {
	fmt.Printf("Following deployment %s (press Ctrl+C to stop)...\n\n", deploymentID)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastStatus := ""
	for {
		select {
		case <-ticker.C:
			status, err := getDeploymentStatus(deploymentID)
			if err != nil {
				return err
			}

			currentStatus := fmt.Sprintf("%v", status["status"])
			if currentStatus != lastStatus {
				fmt.Printf("[%s] Status: %s\n",
					time.Now().Format("15:04:05"), currentStatus)
				lastStatus = currentStatus
			}

			if progress, ok := status["progress"].(map[string]interface{}); ok {
				total := int(progress["total"].(float64))
				completed := int(progress["completed"].(float64))
				failed := int(progress["failed"].(float64))

				if total > 0 {
					percent := (completed * 100) / total
					fmt.Printf("  Progress: %d/%d (%d%%), Failed: %d\n",
						completed, total, percent, failed)
				}
			}

			if currentStatus == "completed" || currentStatus == "failed" ||
				currentStatus == "rolled_back" {
				fmt.Printf("\nDeployment %s\n", currentStatus)
				return nil
			}
		}
	}
}

func getDeploymentStatus(deploymentID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/deployments/%s", getServerURL(), deploymentID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	addAuthHeaders(req)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed: %s", string(body))
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	return status, nil
}

func init() {
	// Create command flags
	deploymentCreateCmd.Flags().Bool("dry-run", false, "Perform a dry run")
	deploymentCreateCmd.Flags().Bool("auto-approve", false, "Auto-approve canary deployments")
	deploymentCreateCmd.Flags().Bool("wait", false, "Wait for deployment to complete")
	deploymentCreateCmd.Flags().Duration("timeout", 30*time.Minute, "Timeout for waiting")

	// List command flags
	deploymentListCmd.Flags().String("status", "", "Filter by status")
	deploymentListCmd.Flags().Int("limit", 50, "Limit results")
	deploymentListCmd.Flags().Bool("all", false, "Show all deployments including completed")

	// Status command flags
	deploymentStatusCmd.Flags().Bool("detailed", false, "Show detailed device status")
	deploymentStatusCmd.Flags().Bool("follow", false, "Follow deployment progress")

	// Rollback command flags
	deploymentRollbackCmd.Flags().Bool("force", false, "Force rollback without confirmation")
	deploymentRollbackCmd.Flags().String("reason", "Manual rollback", "Reason for rollback")

	// Add subcommands
	deploymentCmd.AddCommand(deploymentCreateCmd)
	deploymentCmd.AddCommand(deploymentListCmd)
	deploymentCmd.AddCommand(deploymentStatusCmd)
	deploymentCmd.AddCommand(deploymentPauseCmd)
	deploymentCmd.AddCommand(deploymentResumeCmd)
	deploymentCmd.AddCommand(deploymentRollbackCmd)
	deploymentCmd.AddCommand(deploymentApproveCmd)

	rootCmd.AddCommand(deploymentCmd)
}