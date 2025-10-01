package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var deviceCmd = &cobra.Command{
	Use:   "device",
	Short: "Manage devices",
	Long:  `Register, monitor, and manage devices in device fleets.`,
}

var deviceRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a new device",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		name, _ := cmd.Flags().GetString("name")
		deviceType, _ := cmd.Flags().GetString("type")
		labels, _ := cmd.Flags().GetStringSlice("label")
		metadata, _ := cmd.Flags().GetString("metadata")

		// Build device object
		device := map[string]interface{}{
			"name": name,
			"type": deviceType,
		}

		// Add labels
		if len(labels) > 0 {
			labelMap := make(map[string]string)
			for _, label := range labels {
				parts := strings.Split(label, "=")
				if len(parts) == 2 {
					labelMap[parts[0]] = parts[1]
				}
			}
			device["labels"] = labelMap
		}

		// Add metadata
		if metadata != "" {
			var meta map[string]interface{}
			if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
				return fmt.Errorf("invalid metadata JSON: %w", err)
			}
			device["metadata"] = meta
		}

		// Create request
		body, _ := json.Marshal(device)
		url := fmt.Sprintf("%s/api/v1/devices/register", getServerURL())
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to register device: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			DeviceID     string `json:"device_id"`
			Token        string `json:"token"`
			Message      string `json:"message"`
			EnrollmentID string `json:"enrollment_id"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		fmt.Printf("Device registered successfully:\n")
		fmt.Printf("  Device ID:     %s\n", result.DeviceID)
		fmt.Printf("  Enrollment ID: %s\n", result.EnrollmentID)

		// Save token if requested
		tokenFile, _ := cmd.Flags().GetString("save-token")
		if tokenFile != "" {
			if err := os.WriteFile(tokenFile, []byte(result.Token), 0600); err != nil {
				return fmt.Errorf("failed to save token: %w", err)
			}
			fmt.Printf("  Token saved to: %s\n", tokenFile)
		} else if result.Token != "" {
			fmt.Printf("\nDevice Token (save this securely):\n%s\n", result.Token)
		}

		return nil
	},
}

var deviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List devices",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get filters
		status, _ := cmd.Flags().GetString("status")
		labels, _ := cmd.Flags().GetStringSlice("label")
		limit, _ := cmd.Flags().GetInt("limit")
		format, _ := cmd.Flags().GetString("format")

		// Build query parameters
		params := fmt.Sprintf("?limit=%d", limit)
		if status != "" {
			params += fmt.Sprintf("&status=%s", status)
		}
		for _, label := range labels {
			params += fmt.Sprintf("&label=%s", label)
		}

		// Create request
		url := fmt.Sprintf("%s/api/v1/devices%s", getServerURL(), params)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to list devices: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			Devices []struct {
				ID            string            `json:"id"`
				Name          string            `json:"name"`
				Type          string            `json:"type"`
				Status        string            `json:"status"`
				Version       string            `json:"version"`
				Labels        map[string]string `json:"labels"`
				LastSeen      time.Time         `json:"last_seen"`
				RegisteredAt  time.Time         `json:"registered_at"`
				HealthStatus  string            `json:"health_status"`
			} `json:"devices"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Display devices
		switch format {
		case "json":
			output, _ := json.MarshalIndent(result.Devices, "", "  ")
			fmt.Println(string(output))

		case "wide":
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tName\tType\tStatus\tHealth\tVersion\tLabels\tLast Seen\n")
			for _, d := range result.Devices {
				labelStr := ""
				for k, v := range d.Labels {
					if labelStr != "" {
						labelStr += ","
					}
					labelStr += fmt.Sprintf("%s=%s", k, v)
				}
				lastSeen := "Never"
				if !d.LastSeen.IsZero() {
					lastSeen = time.Since(d.LastSeen).Round(time.Second).String() + " ago"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					d.ID[:8], d.Name, d.Type, d.Status, d.HealthStatus,
					d.Version, labelStr, lastSeen)
			}
			w.Flush()

		default: // table
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tName\tStatus\tHealth\tVersion\tLast Seen\n")
			for _, d := range result.Devices {
				lastSeen := "Never"
				if !d.LastSeen.IsZero() {
					if time.Since(d.LastSeen) < time.Hour {
						lastSeen = time.Since(d.LastSeen).Round(time.Second).String() + " ago"
					} else {
						lastSeen = d.LastSeen.Format("2006-01-02 15:04")
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					d.ID[:8], d.Name, d.Status, d.HealthStatus, d.Version, lastSeen)
			}
			w.Flush()
		}

		fmt.Printf("\nTotal: %d devices\n", len(result.Devices))

		return nil
	},
}

var deviceStatusCmd = &cobra.Command{
	Use:   "status [device-id]",
	Short: "Get device status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID := args[0]

		// Get flags
		detailed, _ := cmd.Flags().GetBool("detailed")

		// Create request
		url := fmt.Sprintf("%s/api/v1/devices/%s", getServerURL(), deviceID)
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
			return fmt.Errorf("failed to get device status: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var device map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&device); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Display device status
		fmt.Printf("Device Status:\n")
		fmt.Printf("  ID:           %v\n", device["id"])
		fmt.Printf("  Name:         %v\n", device["name"])
		fmt.Printf("  Type:         %v\n", device["type"])
		fmt.Printf("  Status:       %v\n", device["status"])
		fmt.Printf("  Version:      %v\n", device["version"])
		fmt.Printf("  Health:       %v\n", device["health_status"])
		fmt.Printf("  Last Seen:    %v\n", device["last_seen"])
		fmt.Printf("  Registered:   %v\n", device["registered_at"])

		if labels, ok := device["labels"].(map[string]interface{}); ok && len(labels) > 0 {
			fmt.Printf("\nLabels:\n")
			for k, v := range labels {
				fmt.Printf("  %s: %v\n", k, v)
			}
		}

		if detailed {
			if system, ok := device["system_info"].(map[string]interface{}); ok {
				fmt.Printf("\nSystem Info:\n")
				fmt.Printf("  OS:           %v\n", system["os"])
				fmt.Printf("  Architecture: %v\n", system["arch"])
				fmt.Printf("  CPU Cores:    %v\n", system["cpu_cores"])
				fmt.Printf("  Memory:       %v\n", system["memory"])
				fmt.Printf("  Disk:         %v\n", system["disk"])
			}

			if metrics, ok := device["metrics"].(map[string]interface{}); ok {
				fmt.Printf("\nCurrent Metrics:\n")
				fmt.Printf("  CPU Usage:    %.2f%%\n", metrics["cpu_usage"])
				fmt.Printf("  Memory Usage: %.2f%%\n", metrics["memory_usage"])
				fmt.Printf("  Disk Usage:   %.2f%%\n", metrics["disk_usage"])
				if temp, ok := metrics["temperature"].(float64); ok {
					fmt.Printf("  Temperature:  %.1f°C\n", temp)
				}
			}

			if deployments, ok := device["recent_deployments"].([]interface{}); ok && len(deployments) > 0 {
				fmt.Printf("\nRecent Deployments:\n")
				for _, dep := range deployments {
					if d, ok := dep.(map[string]interface{}); ok {
						fmt.Printf("  - %v: %v (at %v)\n",
							d["deployment_id"], d["status"], d["updated_at"])
					}
				}
			}
		}

		return nil
	},
}

var deviceUpdateCmd = &cobra.Command{
	Use:   "update [device-id]",
	Short: "Update device properties",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID := args[0]

		// Get flags
		name, _ := cmd.Flags().GetString("name")
		labels, _ := cmd.Flags().GetStringSlice("label")
		removeLabels, _ := cmd.Flags().GetStringSlice("remove-label")
		metadata, _ := cmd.Flags().GetString("metadata")

		// Build update object
		update := make(map[string]interface{})

		if name != "" {
			update["name"] = name
		}

		// Process labels
		if len(labels) > 0 || len(removeLabels) > 0 {
			labelOps := make(map[string]interface{})

			if len(labels) > 0 {
				addLabels := make(map[string]string)
				for _, label := range labels {
					parts := strings.Split(label, "=")
					if len(parts) == 2 {
						addLabels[parts[0]] = parts[1]
					}
				}
				labelOps["add"] = addLabels
			}

			if len(removeLabels) > 0 {
				labelOps["remove"] = removeLabels
			}

			update["labels"] = labelOps
		}

		// Add metadata
		if metadata != "" {
			var meta map[string]interface{}
			if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
				return fmt.Errorf("invalid metadata JSON: %w", err)
			}
			update["metadata"] = meta
		}

		if len(update) == 0 {
			return fmt.Errorf("no updates specified")
		}

		// Create request
		body, _ := json.Marshal(update)
		url := fmt.Sprintf("%s/api/v1/devices/%s", getServerURL(), deviceID)
		req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to update device: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("update failed with status %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Device %s updated successfully\n", deviceID)
		return nil
	},
}

var deviceDeleteCmd = &cobra.Command{
	Use:   "delete [device-id]",
	Short: "Delete a device",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID := args[0]

		// Confirm deletion
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Are you sure you want to delete device %s? (y/N): ", deviceID)
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		// Create request
		url := fmt.Sprintf("%s/api/v1/devices/%s", getServerURL(), deviceID)
		req, err := http.NewRequest("DELETE", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to delete device: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("deletion failed with status %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Device %s deleted successfully\n", deviceID)
		return nil
	},
}

var deviceRebootCmd = &cobra.Command{
	Use:   "reboot [device-id]",
	Short: "Reboot a device",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID := args[0]

		// Get flags
		force, _ := cmd.Flags().GetBool("force")
		wait, _ := cmd.Flags().GetBool("wait")

		// Create request
		payload := map[string]interface{}{
			"force": force,
		}

		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("%s/api/v1/devices/%s/reboot", getServerURL(), deviceID)
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to reboot device: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("reboot failed with status %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Reboot command sent to device %s\n", deviceID)

		if wait {
			fmt.Println("Waiting for device to come back online...")
			if err := waitForDevice(deviceID, 5*time.Minute); err != nil {
				return fmt.Errorf("device did not come back online: %w", err)
			}
			fmt.Println("Device is back online")
		}

		return nil
	},
}

func waitForDevice(deviceID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for device")
			}

			// Check device status
			url := fmt.Sprintf("%s/api/v1/devices/%s", getServerURL(), deviceID)
			req, _ := http.NewRequest("GET", url, nil)
			addAuthHeaders(req)

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

func init() {
	// Register command flags
	deviceRegisterCmd.Flags().String("name", "", "Device name")
	deviceRegisterCmd.Flags().String("type", "generic", "Device type")
	deviceRegisterCmd.Flags().StringSlice("label", []string{}, "Labels (key=value)")
	deviceRegisterCmd.Flags().String("metadata", "{}", "Device metadata (JSON)")
	deviceRegisterCmd.Flags().String("save-token", "", "Save device token to file")
	deviceRegisterCmd.MarkFlagRequired("name")

	// List command flags
	deviceListCmd.Flags().String("status", "", "Filter by status")
	deviceListCmd.Flags().StringSlice("label", []string{}, "Filter by labels")
	deviceListCmd.Flags().Int("limit", 100, "Limit results")
	deviceListCmd.Flags().String("format", "table", "Output format (table, wide, json)")

	// Status command flags
	deviceStatusCmd.Flags().Bool("detailed", false, "Show detailed information")

	// Update command flags
	deviceUpdateCmd.Flags().String("name", "", "New device name")
	deviceUpdateCmd.Flags().StringSlice("label", []string{}, "Add/update labels (key=value)")
	deviceUpdateCmd.Flags().StringSlice("remove-label", []string{}, "Remove labels")
	deviceUpdateCmd.Flags().String("metadata", "", "Update metadata (JSON)")

	// Delete command flags
	deviceDeleteCmd.Flags().Bool("force", false, "Force deletion without confirmation")

	// Reboot command flags
	deviceRebootCmd.Flags().Bool("force", false, "Force immediate reboot")
	deviceRebootCmd.Flags().Bool("wait", false, "Wait for device to come back online")

	// Add subcommands
	deviceCmd.AddCommand(deviceRegisterCmd)
	deviceCmd.AddCommand(deviceListCmd)
	deviceCmd.AddCommand(deviceStatusCmd)
	deviceCmd.AddCommand(deviceUpdateCmd)
	deviceCmd.AddCommand(deviceDeleteCmd)
	deviceCmd.AddCommand(deviceRebootCmd)

	rootCmd.AddCommand(deviceCmd)
}