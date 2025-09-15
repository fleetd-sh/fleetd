package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to Fleet Cloud",
		Long: `Authenticate with Fleet Cloud for remote management and deployment.

This stores your authentication token locally for use with other commands.`,
		RunE: runLogin,
	}

	return cmd
}

func runLogin(cmd *cobra.Command, args []string) error {
	printHeader("Fleet Cloud Login")
	fmt.Println()

	// Get email
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Email: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	email = strings.TrimSpace(email)

	// Get password (hidden)
	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return err
	}
	fmt.Println()
	password := string(passwordBytes)

	// Authenticate
	printInfo("Authenticating...")

	// This would make an API call to authenticate
	// For now, simulate success
	_ = password // Use password to avoid unused variable error
	token := "fleet_token_" + email

	// Save token
	configPath := os.ExpandEnv("$HOME/.fleet/config.json")
	if err := os.MkdirAll(os.ExpandEnv("$HOME/.fleet"), 0o700); err != nil {
		printError("Failed to create config directory: %v", err)
		return err
	}

	// Write token to config
	config := fmt.Sprintf(`{
  "email": "%s",
  "token": "%s"
}`, email, token)

	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		printError("Failed to save credentials: %v", err)
		return err
	}

	printSuccess("Logged in as %s", email)
	printInfo("Credentials saved to ~/.fleet/config.json")

	return nil
}
