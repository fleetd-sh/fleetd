package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"fleetd.sh/internal/client"
	"github.com/spf13/cobra"
)

// newSecurityCmd creates the security command
func newSecurityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "security",
		Short: "Check security status of the platform API",
		Long: `Check the authentication and security configuration of the platform API.

This command helps identify potential security misconfigurations before deployment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create API client to get proper base URL
			apiClient, err := client.NewClient(nil)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			// Use the client's base URL for the security endpoint
			url := fmt.Sprintf("%s/security", apiClient.BaseURL())

			// Make request
			resp, err := http.Get(url)
			if err != nil {
				printError("Failed to connect to platform API")
				printInfo("Make sure platform-api is running")
				return err
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			// Parse response
			var status struct {
				AuthenticationEnabled bool     `json:"authentication_enabled"`
				Mode                  string   `json:"mode"`
				Warnings              []string `json:"warnings"`
			}

			if err := json.Unmarshal(body, &status); err != nil {
				return err
			}

			// Display status
			printHeader("Security Status")
			fmt.Println()

			if status.AuthenticationEnabled {
				printSuccess("Authentication: ENABLED")
				printSuccess("Mode: %s", status.Mode)
				fmt.Println()
				printInfo("Platform API is configured securely")
			} else {
				printError("Authentication: DISABLED")
				printWarning("Mode: %s", status.Mode)
				fmt.Println()

				if len(status.Warnings) > 0 {
					printError("SECURITY WARNINGS:")
					for _, warning := range status.Warnings {
						printError("  • %s", warning)
					}
					fmt.Println()
				}

				printWarning("To secure your deployment:")
				printInfo("  1. Set FLEETD_AUTH_MODE=production")
				printInfo("  2. Remove FLEETD_INSECURE=true if set")
				printInfo("  3. Configure JWT secret key")
				printInfo("  4. Restart platform-api")
			}

			return nil
		},
	}

	return cmd
}
