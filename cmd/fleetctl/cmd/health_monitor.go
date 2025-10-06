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

var healthMonitorCmd = &cobra.Command{
	Use:   "health",
	Short: "Health monitoring and alerts",
	Long:  `Check system health, view alerts, and manage health monitoring.`,
}

var healthCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Run health checks",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		verbose, _ := cmd.Flags().GetBool("verbose")
		format, _ := cmd.Flags().GetString("format")
		checkType, _ := cmd.Flags().GetString("type")

		// Build query parameters
		params := ""
		if checkType != "" {
			params = fmt.Sprintf("?type=%s", checkType)
		}

		// Create request
		url := fmt.Sprintf("%s/api/v1/health/check%s", getServerURL(), params)
		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to run health check: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("health check failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			Status string `json:"status"` // healthy, degraded, unhealthy
			Checks []struct {
				Name     string        `json:"name"`
				Status   string        `json:"status"`
				Message  string        `json:"message"`
				Duration time.Duration `json:"duration_ms"`
				Error    string        `json:"error,omitempty"`
			} `json:"checks"`
			Timestamp time.Time `json:"timestamp"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Display results
		switch format {
		case "json":
			output, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(output))

		default:
			// Summary
			statusIcon := "✓"
			if result.Status == "unhealthy" {
				statusIcon = "✗"
			} else if result.Status == "degraded" {
				statusIcon = "⚠"
			}

			fmt.Printf("%s System Health: %s\n", statusIcon, strings.ToUpper(result.Status))
			fmt.Printf("Timestamp: %s\n\n", result.Timestamp.Format("2006-01-02 15:04:05"))

			// Individual checks
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "Check\tStatus\tDuration\tMessage\n")
			fmt.Fprintf(w, "-----\t------\t--------\t-------\n")

			for _, check := range result.Checks {
				statusStr := check.Status
				if check.Status == "healthy" {
					statusStr = "✓ " + check.Status
				} else if check.Status == "unhealthy" {
					statusStr = "✗ " + check.Status
				} else {
					statusStr = "⚠ " + check.Status
				}

				message := check.Message
				if verbose && check.Error != "" {
					message = fmt.Sprintf("%s (Error: %s)", message, check.Error)
				}

				fmt.Fprintf(w, "%s\t%s\t%dms\t%s\n",
					check.Name, statusStr, check.Duration/time.Millisecond, message)
			}
			w.Flush()
		}

		return nil
	},
}

var healthMonitorStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get overall health status",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		deviceID, _ := cmd.Flags().GetString("device")
		deploymentID, _ := cmd.Flags().GetString("deployment")
		history, _ := cmd.Flags().GetBool("history")
		days, _ := cmd.Flags().GetInt("days")

		// Build URL based on scope
		var url string
		if deviceID != "" {
			url = fmt.Sprintf("%s/api/v1/devices/%s/health", getServerURL(), deviceID)
		} else if deploymentID != "" {
			url = fmt.Sprintf("%s/api/v1/deployments/%s/health", getServerURL(), deploymentID)
		} else {
			url = fmt.Sprintf("%s/api/v1/health/status", getServerURL())
		}

		if history {
			url += fmt.Sprintf("?history=true&days=%d", days)
		}

		// Create request
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to get health status: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Display status
		fmt.Printf("Health Status:\n")
		fmt.Printf("  Status:     %v\n", result["status"])
		fmt.Printf("  Updated:    %v\n", result["updated_at"])

		if metrics, ok := result["metrics"].(map[string]interface{}); ok {
			fmt.Printf("\nMetrics:\n")
			if val, ok := metrics["success_rate"]; ok {
				fmt.Printf("  Success Rate:    %.2f%%\n", val)
			}
			if val, ok := metrics["error_rate"]; ok {
				fmt.Printf("  Error Rate:      %.2f%%\n", val)
			}
			if val, ok := metrics["avg_response_time"]; ok {
				fmt.Printf("  Avg Response:    %vms\n", val)
			}
			if val, ok := metrics["healthy_devices"]; ok {
				fmt.Printf("  Healthy Devices: %v\n", val)
			}
			if val, ok := metrics["degraded_devices"]; ok {
				fmt.Printf("  Degraded:        %v\n", val)
			}
			if val, ok := metrics["failed_devices"]; ok {
				fmt.Printf("  Failed:          %v\n", val)
			}
		}

		if history {
			if historyData, ok := result["history"].([]interface{}); ok && len(historyData) > 0 {
				fmt.Printf("\nHistory (last %d days):\n", days)
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintf(w, "Time\tStatus\tSuccess Rate\tError Rate\n")
				for _, entry := range historyData {
					if e, ok := entry.(map[string]interface{}); ok {
						fmt.Fprintf(w, "%v\t%v\t%.2f%%\t%.2f%%\n",
							e["timestamp"], e["status"],
							e["success_rate"], e["error_rate"])
					}
				}
				w.Flush()
			}
		}

		return nil
	},
}

var healthAlertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Manage health alerts",
}

var healthAlertsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List health alerts",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		severity, _ := cmd.Flags().GetString("severity")
		unresolved, _ := cmd.Flags().GetBool("unresolved")
		limit, _ := cmd.Flags().GetInt("limit")
		format, _ := cmd.Flags().GetString("format")

		// Build query parameters
		params := fmt.Sprintf("?limit=%d", limit)
		if severity != "" {
			params += fmt.Sprintf("&severity=%s", severity)
		}
		if unresolved {
			params += "&unresolved=true"
		}

		// Create request
		url := fmt.Sprintf("%s/api/v1/health/alerts%s", getServerURL(), params)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to list alerts: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result struct {
			Alerts []struct {
				ID           string     `json:"id"`
				Type         string     `json:"type"`
				Severity     string     `json:"severity"`
				Source       string     `json:"source"`
				SourceID     string     `json:"source_id"`
				Message      string     `json:"message"`
				Acknowledged bool       `json:"acknowledged"`
				Resolved     bool       `json:"resolved"`
				CreatedAt    time.Time  `json:"created_at"`
				ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
			} `json:"alerts"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Display alerts
		switch format {
		case "json":
			output, _ := json.MarshalIndent(result.Alerts, "", "  ")
			fmt.Println(string(output))

		default:
			if len(result.Alerts) == 0 {
				fmt.Println("No alerts found")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tSeverity\tType\tSource\tMessage\tStatus\tCreated\n")
			fmt.Fprintf(w, "--\t--------\t----\t------\t-------\t------\t-------\n")

			for _, alert := range result.Alerts {
				sevIcon := ""
				switch alert.Severity {
				case "critical":
					sevIcon = "🔴"
				case "warning":
					sevIcon = "🟡"
				case "info":
					sevIcon = "🔵"
				}

				status := "Active"
				if alert.Resolved {
					status = "Resolved"
				} else if alert.Acknowledged {
					status = "Ack'd"
				}

				message := alert.Message
				if len(message) > 40 {
					message = message[:37] + "..."
				}

				fmt.Fprintf(w, "%s\t%s %s\t%s\t%s\t%s\t%s\t%s\n",
					alert.ID[:8],
					sevIcon, alert.Severity,
					alert.Type,
					alert.Source,
					message,
					status,
					alert.CreatedAt.Format("2006-01-02 15:04"))
			}
			w.Flush()

			fmt.Printf("\nTotal: %d alerts\n", len(result.Alerts))
		}

		return nil
	},
}

var healthAlertsAckCmd = &cobra.Command{
	Use:   "acknowledge [alert-id]",
	Short: "Acknowledge an alert",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alertID := args[0]

		// Get flags
		message, _ := cmd.Flags().GetString("message")

		// Create request
		payload := map[string]interface{}{
			"message": message,
		}

		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("%s/api/v1/health/alerts/%s/acknowledge", getServerURL(), alertID)
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to acknowledge alert: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Alert %s acknowledged successfully\n", alertID)
		return nil
	},
}

var healthAlertsResolveCmd = &cobra.Command{
	Use:   "resolve [alert-id]",
	Short: "Resolve an alert",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alertID := args[0]

		// Get flags
		message, _ := cmd.Flags().GetString("message")

		// Create request
		payload := map[string]interface{}{
			"message": message,
		}

		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("%s/api/v1/health/alerts/%s/resolve", getServerURL(), alertID)
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		addAuthHeaders(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to resolve alert: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Alert %s resolved successfully\n", alertID)
		return nil
	},
}

func init() {
	// Check command flags
	healthCheckCmd.Flags().Bool("verbose", false, "Show detailed output")
	healthCheckCmd.Flags().String("format", "table", "Output format (table, json)")
	healthCheckCmd.Flags().String("type", "", "Check type to run")

	// Status command flags
	healthMonitorStatusCmd.Flags().String("device", "", "Get health for specific device")
	healthMonitorStatusCmd.Flags().String("deployment", "", "Get health for specific deployment")
	healthMonitorStatusCmd.Flags().Bool("history", false, "Show historical data")
	healthMonitorStatusCmd.Flags().Int("days", 7, "Number of days for history")

	// Alerts list command flags
	healthAlertsListCmd.Flags().String("severity", "", "Filter by severity (critical, warning, info)")
	healthAlertsListCmd.Flags().Bool("unresolved", false, "Show only unresolved alerts")
	healthAlertsListCmd.Flags().Int("limit", 50, "Limit results")
	healthAlertsListCmd.Flags().String("format", "table", "Output format (table, json)")

	// Alert acknowledge/resolve flags
	healthAlertsAckCmd.Flags().String("message", "", "Acknowledgment message")
	healthAlertsResolveCmd.Flags().String("message", "", "Resolution message")

	// Add subcommands
	healthAlertsCmd.AddCommand(healthAlertsListCmd)
	healthAlertsCmd.AddCommand(healthAlertsAckCmd)
	healthAlertsCmd.AddCommand(healthAlertsResolveCmd)

	healthMonitorCmd.AddCommand(healthCheckCmd)
	healthMonitorCmd.AddCommand(healthMonitorStatusCmd)
	healthMonitorCmd.AddCommand(healthAlertsCmd)

	rootCmd.AddCommand(healthMonitorCmd)
}
