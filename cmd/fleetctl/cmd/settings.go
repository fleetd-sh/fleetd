package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"github.com/spf13/cobra"
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage system settings",
	Long:  `View and update system settings including organization, security, and API configuration`,
}

var settingsGetCmd = &cobra.Command{
	Use:   "get [category]",
	Short: "Get system settings",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		category := "all"
		if len(args) > 0 {
			category = args[0]
		}

		client := fleetpbconnect.NewSettingsServiceClient(
			http.DefaultClient,
			fmt.Sprintf("http://localhost:%d", getAPIPort()),
		)

		ctx := context.Background()

		switch category {
		case "org", "organization":
			resp, err := client.GetOrganizationSettings(ctx, connect.NewRequest(&fleetpb.GetOrganizationSettingsRequest{}))
			if err != nil {
				return fmt.Errorf("failed to get organization settings: %w", err)
			}
			printOrganizationSettings(resp.Msg.Settings)

		case "security":
			resp, err := client.GetSecuritySettings(ctx, connect.NewRequest(&fleetpb.GetSecuritySettingsRequest{}))
			if err != nil {
				return fmt.Errorf("failed to get security settings: %w", err)
			}
			printSecuritySettings(resp.Msg.Settings)

		case "notifications":
			resp, err := client.GetNotificationSettings(ctx, connect.NewRequest(&fleetpb.GetNotificationSettingsRequest{}))
			if err != nil {
				return fmt.Errorf("failed to get notification settings: %w", err)
			}
			printNotificationSettings(resp.Msg.Settings)

		case "api":
			resp, err := client.GetAPISettings(ctx, connect.NewRequest(&fleetpb.GetAPISettingsRequest{}))
			if err != nil {
				return fmt.Errorf("failed to get API settings: %w", err)
			}
			printAPISettings(resp.Msg.Settings)

		case "advanced":
			resp, err := client.GetAdvancedSettings(ctx, connect.NewRequest(&fleetpb.GetAdvancedSettingsRequest{}))
			if err != nil {
				return fmt.Errorf("failed to get advanced settings: %w", err)
			}
			printAdvancedSettings(resp.Msg.Settings)

		case "all":
			// Get all settings
			orgResp, _ := client.GetOrganizationSettings(ctx, connect.NewRequest(&fleetpb.GetOrganizationSettingsRequest{}))
			if orgResp != nil {
				fmt.Println("\n=== Organization Settings ===")
				printOrganizationSettings(orgResp.Msg.Settings)
			}

			secResp, _ := client.GetSecuritySettings(ctx, connect.NewRequest(&fleetpb.GetSecuritySettingsRequest{}))
			if secResp != nil {
				fmt.Println("\n=== Security Settings ===")
				printSecuritySettings(secResp.Msg.Settings)
			}

			notifResp, _ := client.GetNotificationSettings(ctx, connect.NewRequest(&fleetpb.GetNotificationSettingsRequest{}))
			if notifResp != nil {
				fmt.Println("\n=== Notification Settings ===")
				printNotificationSettings(notifResp.Msg.Settings)
			}

			apiResp, _ := client.GetAPISettings(ctx, connect.NewRequest(&fleetpb.GetAPISettingsRequest{}))
			if apiResp != nil {
				fmt.Println("\n=== API Settings ===")
				printAPISettings(apiResp.Msg.Settings)
			}

		default:
			return fmt.Errorf("unknown category: %s (valid: org, security, notifications, api, advanced)", category)
		}

		return nil
	},
}

var settingsUpdateCmd = &cobra.Command{
	Use:   "update [category]",
	Short: "Update system settings",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		category := args[0]

		client := fleetpbconnect.NewSettingsServiceClient(
			http.DefaultClient,
			fmt.Sprintf("http://localhost:%d", getAPIPort()),
		)

		ctx := context.Background()

		switch category {
		case "org", "organization":
			name, _ := cmd.Flags().GetString("name")
			email, _ := cmd.Flags().GetString("email")
			timezone, _ := cmd.Flags().GetString("timezone")
			language, _ := cmd.Flags().GetString("language")

			// Get current settings
			currentResp, err := client.GetOrganizationSettings(ctx, connect.NewRequest(&fleetpb.GetOrganizationSettingsRequest{}))
			if err != nil {
				return fmt.Errorf("failed to get current settings: %w", err)
			}

			settings := currentResp.Msg.Settings
			if name != "" {
				settings.Name = name
			}
			if email != "" {
				settings.ContactEmail = email
			}
			if timezone != "" {
				settings.Timezone = timezone
			}
			if language != "" {
				settings.Language = language
			}

			_, err = client.UpdateOrganizationSettings(ctx, connect.NewRequest(&fleetpb.UpdateOrganizationSettingsRequest{
				Settings: settings,
			}))
			if err != nil {
				return fmt.Errorf("failed to update organization settings: %w", err)
			}
			printSuccess("Organization settings updated")

		case "security":
			twoFactor, _ := cmd.Flags().GetBool("2fa")
			sessionTimeout, _ := cmd.Flags().GetInt32("session-timeout")
			auditLogging, _ := cmd.Flags().GetBool("audit-logging")

			// Get current settings
			currentResp, err := client.GetSecuritySettings(ctx, connect.NewRequest(&fleetpb.GetSecuritySettingsRequest{}))
			if err != nil {
				return fmt.Errorf("failed to get current settings: %w", err)
			}

			settings := currentResp.Msg.Settings
			if cmd.Flags().Changed("2fa") {
				settings.TwoFactorRequired = twoFactor
			}
			if cmd.Flags().Changed("session-timeout") {
				settings.SessionTimeoutMinutes = sessionTimeout
			}
			if cmd.Flags().Changed("audit-logging") {
				settings.AuditLoggingEnabled = auditLogging
			}

			_, err = client.UpdateSecuritySettings(ctx, connect.NewRequest(&fleetpb.UpdateSecuritySettingsRequest{
				Settings: settings,
			}))
			if err != nil {
				return fmt.Errorf("failed to update security settings: %w", err)
			}
			printSuccess("Security settings updated")

		default:
			return fmt.Errorf("unknown category: %s (valid: org, security)", category)
		}

		return nil
	},
}

var settingsAPIKeyCmd = &cobra.Command{
	Use:   "api-key",
	Short: "Manage API keys",
}

var settingsAPIKeyShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := fleetpbconnect.NewSettingsServiceClient(
			http.DefaultClient,
			fmt.Sprintf("http://localhost:%d", getAPIPort()),
		)

		resp, err := client.GetAPISettings(context.Background(), connect.NewRequest(&fleetpb.GetAPISettingsRequest{}))
		if err != nil {
			return fmt.Errorf("failed to get API settings: %w", err)
		}

		fmt.Printf("API Key: %s\n", resp.Msg.Settings.ApiKey)
		fmt.Printf("Created: %s\n", resp.Msg.Settings.ApiKeyCreatedAt.AsTime().Format("2006-01-02 15:04:05"))

		return nil
	},
}

var settingsAPIKeyRegenerateCmd = &cobra.Command{
	Use:   "regenerate",
	Short: "Regenerate API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		if !force {
			fmt.Println("Warning: Regenerating the API key will invalidate the current key.")
			fmt.Print("Continue? (y/n): ")
			var response string
			fmt.Scanln(&response)
			if !strings.HasPrefix(strings.ToLower(response), "y") {
				return fmt.Errorf("operation cancelled")
			}
		}

		client := fleetpbconnect.NewSettingsServiceClient(
			http.DefaultClient,
			fmt.Sprintf("http://localhost:%d", getAPIPort()),
		)

		resp, err := client.RegenerateAPIKey(context.Background(), connect.NewRequest(&fleetpb.RegenerateAPIKeyRequest{}))
		if err != nil {
			return fmt.Errorf("failed to regenerate API key: %w", err)
		}

		printSuccess("New API key generated: %s", resp.Msg.NewApiKey)
		fmt.Println("Make sure to update your applications with the new key.")

		return nil
	},
}

var settingsExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export system data",
	RunE: func(cmd *cobra.Command, args []string) error {
		dataTypes, _ := cmd.Flags().GetStringSlice("types")
		format, _ := cmd.Flags().GetString("format")

		client := fleetpbconnect.NewSettingsServiceClient(
			http.DefaultClient,
			fmt.Sprintf("http://localhost:%d", getAPIPort()),
		)

		req := &fleetpb.ExportDataRequest{
			DataTypes: dataTypes,
			Format:    format,
		}

		resp, err := client.ExportData(context.Background(), connect.NewRequest(req))
		if err != nil {
			return fmt.Errorf("failed to export data: %w", err)
		}

		printSuccess("Data export initiated")
		fmt.Printf("Download URL: %s\n", resp.Msg.DownloadUrl)
		fmt.Printf("Size: %.2f MB\n", float64(resp.Msg.SizeBytes)/(1024*1024))
		fmt.Printf("Expires: %s\n", resp.Msg.ExpiresAt.AsTime().Format("2006-01-02 15:04:05"))

		return nil
	},
}

func printOrganizationSettings(settings *fleetpb.OrganizationSettings) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SETTING\tVALUE")
	fmt.Fprintf(w, "Name\t%s\n", settings.Name)
	fmt.Fprintf(w, "Contact Email\t%s\n", settings.ContactEmail)
	fmt.Fprintf(w, "Timezone\t%s\n", settings.Timezone)
	fmt.Fprintf(w, "Language\t%s\n", settings.Language)
	w.Flush()
}

func printSecuritySettings(settings *fleetpb.SecuritySettings) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SETTING\tVALUE")
	fmt.Fprintf(w, "Two-Factor Required\t%t\n", settings.TwoFactorRequired)
	fmt.Fprintf(w, "Session Timeout\t%d minutes\n", settings.SessionTimeoutMinutes)
	fmt.Fprintf(w, "IP Whitelist Enabled\t%t\n", settings.IpWhitelistEnabled)
	fmt.Fprintf(w, "Audit Logging\t%t\n", settings.AuditLoggingEnabled)
	if settings.PasswordPolicy != nil {
		fmt.Fprintf(w, "Password Min Length\t%d\n", settings.PasswordPolicy.MinLength)
		fmt.Fprintf(w, "Password Expiry\t%d days\n", settings.PasswordPolicy.ExpiryDays)
	}
	w.Flush()
}

func printNotificationSettings(settings *fleetpb.NotificationSettings) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SETTING\tVALUE")
	if settings.EmailNotifications != nil {
		fmt.Fprintf(w, "Device Offline Alerts\t%t\n", settings.EmailNotifications.DeviceOfflineAlerts)
		fmt.Fprintf(w, "Deployment Updates\t%t\n", settings.EmailNotifications.DeploymentStatusUpdates)
		fmt.Fprintf(w, "Security Alerts\t%t\n", settings.EmailNotifications.SecurityAlerts)
	}
	if settings.AlertThresholds != nil {
		fmt.Fprintf(w, "CPU Alert Threshold\t%.0f%%\n", settings.AlertThresholds.CpuUsagePercent)
		fmt.Fprintf(w, "Memory Alert Threshold\t%.0f%%\n", settings.AlertThresholds.MemoryUsagePercent)
		fmt.Fprintf(w, "Disk Alert Threshold\t%.0f%%\n", settings.AlertThresholds.DiskUsagePercent)
	}
	w.Flush()
}

func printAPISettings(settings *fleetpb.APISettings) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SETTING\tVALUE")
	fmt.Fprintf(w, "API Key\t%s...\n", settings.ApiKey[:16])
	fmt.Fprintf(w, "Rate Limit (per min)\t%d\n", settings.RateLimitPerMinute)
	fmt.Fprintf(w, "Rate Limit (per hour)\t%d\n", settings.RateLimitPerHour)
	if settings.CorsSettings != nil {
		fmt.Fprintf(w, "CORS Origins\t%s\n", strings.Join(settings.CorsSettings.AllowedOrigins, ", "))
	}
	w.Flush()
}

func printAdvancedSettings(settings *fleetpb.AdvancedSettings) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SETTING\tVALUE")
	if settings.DataRetention != nil {
		fmt.Fprintf(w, "Telemetry Retention\t%d days\n", settings.DataRetention.TelemetryDays)
		fmt.Fprintf(w, "Logs Retention\t%d days\n", settings.DataRetention.LogsDays)
		fmt.Fprintf(w, "Audit Logs Retention\t%d days\n", settings.DataRetention.AuditLogsDays)
	}
	if settings.ExperimentalFeatures != nil {
		fmt.Fprintf(w, "Beta Features\t%t\n", settings.ExperimentalFeatures.BetaFeaturesEnabled)
		fmt.Fprintf(w, "Debug Mode\t%t\n", settings.ExperimentalFeatures.DebugModeEnabled)
	}
	w.Flush()
}

func init() {
	// Update command flags
	settingsUpdateCmd.Flags().String("name", "", "Organization name")
	settingsUpdateCmd.Flags().String("email", "", "Contact email")
	settingsUpdateCmd.Flags().String("timezone", "", "Timezone")
	settingsUpdateCmd.Flags().String("language", "", "Language")
	settingsUpdateCmd.Flags().Bool("2fa", false, "Two-factor authentication")
	settingsUpdateCmd.Flags().Int32("session-timeout", 30, "Session timeout in minutes")
	settingsUpdateCmd.Flags().Bool("audit-logging", true, "Enable audit logging")

	// API key regenerate flags
	settingsAPIKeyRegenerateCmd.Flags().Bool("force", false, "Skip confirmation prompt")

	// Export flags
	settingsExportCmd.Flags().StringSlice("types", []string{}, "Data types to export")
	settingsExportCmd.Flags().String("format", "json", "Export format (json, csv)")

	// Build command tree
	settingsAPIKeyCmd.AddCommand(settingsAPIKeyShowCmd)
	settingsAPIKeyCmd.AddCommand(settingsAPIKeyRegenerateCmd)

	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsUpdateCmd)
	settingsCmd.AddCommand(settingsAPIKeyCmd)
	settingsCmd.AddCommand(settingsExportCmd)

	rootCmd.AddCommand(settingsCmd)
}
