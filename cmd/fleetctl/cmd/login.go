package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	publicv1 "fleetd.sh/gen/public/v1"
	"fleetd.sh/gen/public/v1/publicv1connect"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

type authConfig struct {
	Email        string    `json:"email"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func runLogin(cmd *cobra.Command, args []string) error {
	printHeader("fleetd Platform Login")
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

	// Get platform API URL from config
	baseURL := viper.GetString("platform_api.url")
	if baseURL == "" {
		host := viper.GetString("platform_api.host")
		port := viper.GetInt("platform_api.port")
		if host == "" {
			host = "localhost"
		}
		if port == 0 {
			port = 8090
		}
		baseURL = fmt.Sprintf("http://%s:%d", host, port)
	}

	// Create auth client
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	authClient := publicv1connect.NewAuthServiceClient(httpClient, baseURL)

	// Create login request
	ctx := context.Background()
	loginReq := connect.NewRequest(&publicv1.LoginRequest{
		Credential: &publicv1.LoginRequest_Password{
			Password: &publicv1.PasswordCredential{
				Email:    email,
				Password: password,
			},
		},
	})

	// Perform login
	loginResp, err := authClient.Login(ctx, loginReq)
	if err != nil {
		// Check if it's a connection error
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "no such host") {
			printError("Failed to connect to platform API at %s", baseURL)
			printInfo("Make sure the platform API is running with: just platform-api-dev")
			return fmt.Errorf("connection failed")
		}
		// Check for authentication errors
		if strings.Contains(err.Error(), "unauthorized") || strings.Contains(err.Error(), "invalid credentials") {
			printError("Invalid email or password")
			return fmt.Errorf("authentication failed")
		}
		printError("Login failed: %v", err)
		return err
	}

	// Calculate expiry time
	expiresAt := time.Now().Add(time.Duration(loginResp.Msg.ExpiresIn) * time.Second)

	// Save credentials
	configDir := os.ExpandEnv("$HOME/.fleetctl")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		printError("Failed to create config directory: %v", err)
		return err
	}

	configPath := filepath.Join(configDir, "auth.json")
	authCfg := authConfig{
		Email:        email,
		AccessToken:  loginResp.Msg.AccessToken,
		RefreshToken: loginResp.Msg.RefreshToken,
		ExpiresAt:    expiresAt,
	}

	configData, err := json.MarshalIndent(authCfg, "", "  ")
	if err != nil {
		printError("Failed to marshal auth config: %v", err)
		return err
	}

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		printError("Failed to save credentials: %v", err)
		return err
	}

	// Also save to viper for current session
	viper.Set("auth.token", loginResp.Msg.AccessToken)
	viper.Set("auth.refresh_token", loginResp.Msg.RefreshToken)
	viper.Set("auth.email", email)

	if loginResp.Msg.User != nil {
		printSuccess("Logged in as %s (%s)", loginResp.Msg.User.Name, loginResp.Msg.User.Email)
		if loginResp.Msg.User.Role > 0 {
			roleStr := "User"
			switch loginResp.Msg.User.Role {
			case publicv1.UserRole_USER_ROLE_VIEWER:
				roleStr = "Viewer"
			case publicv1.UserRole_USER_ROLE_OPERATOR:
				roleStr = "Operator"
			case publicv1.UserRole_USER_ROLE_ADMIN:
				roleStr = "Admin"
			case publicv1.UserRole_USER_ROLE_OWNER:
				roleStr = "Owner"
			}
			printInfo("Role: %s", roleStr)
		}
	} else {
		printSuccess("Logged in as %s", email)
	}
	printInfo("Credentials saved to ~/.fleetctl/auth.json")

	return nil
}
