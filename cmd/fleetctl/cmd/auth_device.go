package cmd

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the FleetD platform",
	Long: `Authenticate with the FleetD platform using device flow authentication.
This will open your web browser to complete the authentication process.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		noBrowser, _ := cmd.Flags().GetBool("no-browser")
		// Use platform-api URL for auth
		apiURL := getPlatformAPIURL()

		// Step 1: Request device code
		deviceAuthResp, err := requestDeviceCode(apiURL)
		if err != nil {
			return fmt.Errorf("failed to request device code: %w", err)
		}

		// Display user code with nice formatting
		fmt.Println("\n🔐 FleetD Authentication")
		fmt.Println("========================")
		fmt.Printf("\nTo authenticate, visit:\n")
		fmt.Printf("  \033[1;36m%s\033[0m\n\n", deviceAuthResp.VerificationURL)
		fmt.Printf("Enter this code:\n")
		fmt.Printf("  \033[1;33m%s\033[0m\n\n", formatUserCode(deviceAuthResp.UserCode))

		// Open browser unless disabled
		if !noBrowser {
			browserURL := fmt.Sprintf("%s?code=%s", deviceAuthResp.VerificationURL, deviceAuthResp.UserCode)
			if err := browser.OpenURL(browserURL); err != nil {
				fmt.Printf("Could not open browser automatically. Please visit the URL above.\n")
			}
		}

		fmt.Println("⏳ Waiting for authentication...")

		// Step 2: Poll for token
		token, err := pollForToken(apiURL, deviceAuthResp.DeviceCode, deviceAuthResp.Interval)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		// Step 3: Save token
		if err := saveToken(token); err != nil {
			return fmt.Errorf("failed to save authentication token: %w", err)
		}

		printSuccess("✅ Authentication successful!")
		fmt.Printf("Token saved to %s\n", getTokenPath())

		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from the FleetD platform",
	RunE: func(cmd *cobra.Command, args []string) error {
		tokenPath := getTokenPath()

		// Check if token exists
		if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
			fmt.Println("You are not logged in.")
			return nil
		}

		// Load token to revoke it server-side
		token, err := loadToken()
		if err == nil && token.AccessToken != "" {
			// Attempt to revoke token server-side
			if err := revokeToken(getPlatformAPIURL(), token.AccessToken); err != nil {
				// Log error but continue with local cleanup
				fmt.Printf("Warning: Could not revoke token server-side: %v\n", err)
			}
		}

		// Remove token file
		if err := os.Remove(tokenPath); err != nil {
			return fmt.Errorf("failed to remove token file: %w", err)
		}

		printSuccess("✅ Logged out successfully")
		return nil
	},
}

// Device auth types
type DeviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type TokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int       `json:"expires_in"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func requestDeviceCode(apiURL string) (*DeviceAuthResponse, error) {
	// Call the actual API endpoint
	reqBody := map[string]string{
		"client_id": "fleetctl",
		"scope":     "api",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// Create HTTP client with TLS verification disabled for testing
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post(
		fmt.Sprintf("%s/api/v1/auth/device/code", apiURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to request device code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed: %s", body)
	}

	var authResp DeviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &authResp, nil
}

func pollForToken(apiURL, deviceCode string, interval int) (*TokenResponse, error) {
	reqBody := map[string]string{
		"device_code": deviceCode,
		"client_id":   "fleetctl",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// Create HTTP client with TLS verification disabled for testing
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	// Poll for up to 15 minutes
	timeout := time.After(15 * time.Minute)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("authentication timed out")
		case <-ticker.C:
			resp, err := client.Post(
				fmt.Sprintf("%s/api/v1/auth/device/token", apiURL),
				"application/json",
				bytes.NewBuffer(jsonData),
			)
			if err != nil {
				// Network error, keep trying
				continue
			}
			defer resp.Body.Close()

			body, _ := ioutil.ReadAll(resp.Body)

			if resp.StatusCode == http.StatusOK {
				var tokenResp TokenResponse
				if err := json.Unmarshal(body, &tokenResp); err != nil {
					return nil, fmt.Errorf("failed to decode token response: %w", err)
				}
				// Calculate expiration time
				tokenResp.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
				return &tokenResp, nil
			}

			// Check for specific error codes
			var errorResp map[string]string
			if err := json.Unmarshal(body, &errorResp); err == nil {
				if errorCode, ok := errorResp["error"]; ok {
					switch errorCode {
					case "authorization_pending":
						// Keep polling
						continue
					case "expired_token":
						return nil, fmt.Errorf("device code expired")
					default:
						return nil, fmt.Errorf("authentication failed: %s", errorCode)
					}
				}
			}
		}
	}
}

func revokeToken(apiURL, token string) error {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/auth/revoke", apiURL), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("failed to revoke token: %s", body)
	}

	return nil
}

func saveToken(token *TokenResponse) error {
	tokenPath := getTokenPath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(tokenPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Marshal token to JSON
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}

	// Write to file with restricted permissions
	return os.WriteFile(tokenPath, data, 0600)
}

func loadToken() (*TokenResponse, error) {
	tokenPath := getTokenPath()

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, err
	}

	var token TokenResponse
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

func getTokenPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".fleetd", "auth.json")
}

func getPlatformAPIURL() string {
	// Check environment variable first
	if url := os.Getenv("PLATFORM_API_URL"); url != "" {
		return url
	}
	// Default to localhost:8090 with HTTPS
	return "https://localhost:8090"
}

func formatUserCode(code string) string {
	// Format code as XXXX-XXXX for better readability
	if len(code) == 8 {
		return fmt.Sprintf("%s-%s", code[:4], code[4:])
	}
	return code
}

// GetAuthToken returns the current auth token for use in API calls
func GetAuthToken() (string, error) {
	token, err := loadToken()
	if err != nil {
		return "", fmt.Errorf("not authenticated: run 'fleetctl login'")
	}

	if token.ExpiresAt.Before(time.Now()) {
		return "", fmt.Errorf("authentication expired: run 'fleetctl login'")
	}

	return token.AccessToken, nil
}

// generateDeviceCode generates a secure random device code
func generateDeviceCode() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:43] // Remove padding
}

// generateUserCode generates a user-friendly code
func generateUserCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Avoid ambiguous characters
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[randInt(len(charset))]
	}
	return string(b)
}

func randInt(max int) int {
	b := make([]byte, 1)
	rand.Read(b)
	return int(b[0]) % max
}

func init() {
	loginCmd.Flags().Bool("no-browser", false, "Don't open browser automatically")

	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
}