package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AuthFlowE2ETest tests the complete authentication flow from CLI to API
type AuthFlowE2ETest struct {
	t              *testing.T
	platformAPIURL string
	webUIURL       string
	db             *sql.DB
	tempDir        string
	cleanup        []func()
}

func TestE2E_CompleteAuthFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	test := &AuthFlowE2ETest{
		t:              t,
		platformAPIURL: getEnvOrDefault("PLATFORM_API_URL", "https://localhost:8090"),
		webUIURL:       getEnvOrDefault("WEB_UI_URL", "https://localhost:3000"),
		cleanup:        []func(){},
	}
	defer test.Cleanup()

	// Setup test environment
	test.Setup()

	// Run test scenarios
	t.Run("SuccessfulAuthentication", test.TestSuccessfulAuthentication)
	t.Run("ExpiredCode", test.TestExpiredCode)
	t.Run("InvalidCode", test.TestInvalidCode)
	t.Run("TokenPersistence", test.TestTokenPersistence)
	t.Run("Logout", test.TestLogout)
}

func (test *AuthFlowE2ETest) Setup() {
	var err error

	// Create temp directory for test
	test.tempDir, err = ioutil.TempDir("", "fleetd-e2e-test-*")
	require.NoError(test.t, err)
	test.cleanup = append(test.cleanup, func() { os.RemoveAll(test.tempDir) })

	// Setup test database connection
	test.db, err = sql.Open("postgres", "postgresql://fleetd:fleetd_secret@localhost:5432/fleetd_test?sslmode=disable")
	if err != nil {
		test.t.Skip("Test database not available")
	}
	test.cleanup = append(test.cleanup, func() { test.db.Close() })

	// Clean test data
	test.cleanTestData()

	// Ensure platform-api is running
	test.ensurePlatformAPIRunning()
}

func (test *AuthFlowE2ETest) Cleanup() {
	for i := len(test.cleanup) - 1; i >= 0; i-- {
		test.cleanup[i]()
	}
}

func (test *AuthFlowE2ETest) TestSuccessfulAuthentication(t *testing.T) {
	// Step 1: Request device code via CLI simulation
	deviceAuthResp := test.requestDeviceCode(t)

	// Step 2: Verify the code is valid
	assert.True(t, test.verifyUserCode(t, deviceAuthResp.UserCode))

	// Step 3: Create a test user and approve the code
	userID := test.createTestUser(t, "test@example.com")
	test.approveDeviceAuth(t, deviceAuthResp.UserCode, userID)

	// Step 4: Poll for token (simulate CLI polling)
	tokenResp := test.pollForToken(t, deviceAuthResp.DeviceCode)
	assert.NotEmpty(t, tokenResp.AccessToken)
	assert.Equal(t, "Bearer", tokenResp.TokenType)

	// Step 5: Use the token to make an authenticated request
	test.testAuthenticatedRequest(t, tokenResp.AccessToken)
}

func (test *AuthFlowE2ETest) TestExpiredCode(t *testing.T) {
	// Create an expired device auth request directly in database
	deviceCode := "expired-device-code"
	userCode := "EXPIRED1"

	_, err := test.db.Exec(`
		INSERT INTO device_auth_request (id, device_code, user_code, verification_url, expires_at, client_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, "expired-id", deviceCode, userCode, test.webUIURL+"/auth/device",
		time.Now().Add(-1*time.Hour), "fleetctl")
	require.NoError(t, err)

	// Try to poll for token - should fail
	_, err = test.pollForTokenWithError(t, deviceCode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func (test *AuthFlowE2ETest) TestInvalidCode(t *testing.T) {
	// Try to verify an invalid code
	valid := test.verifyUserCode(t, "INVALID1")
	assert.False(t, valid)

	// Try to poll with invalid device code
	_, err := test.pollForTokenWithError(t, "invalid-device-code")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_grant")
}

func (test *AuthFlowE2ETest) TestTokenPersistence(t *testing.T) {
	// Complete authentication flow
	deviceAuthResp := test.requestDeviceCode(t)
	userID := test.createTestUser(t, "persist@example.com")
	test.approveDeviceAuth(t, deviceAuthResp.UserCode, userID)
	tokenResp := test.pollForToken(t, deviceAuthResp.DeviceCode)

	// Save token to file (simulate CLI behavior)
	authFile := filepath.Join(test.tempDir, "auth.json")
	tokenData, _ := json.Marshal(tokenResp)
	err := ioutil.WriteFile(authFile, tokenData, 0600)
	require.NoError(t, err)

	// Read token back
	data, err := ioutil.ReadFile(authFile)
	require.NoError(t, err)

	var loadedToken TokenResponse
	err = json.Unmarshal(data, &loadedToken)
	require.NoError(t, err)

	assert.Equal(t, tokenResp.AccessToken, loadedToken.AccessToken)
}

func (test *AuthFlowE2ETest) TestLogout(t *testing.T) {
	// Complete authentication flow
	deviceAuthResp := test.requestDeviceCode(t)
	userID := test.createTestUser(t, "logout@example.com")
	test.approveDeviceAuth(t, deviceAuthResp.UserCode, userID)
	tokenResp := test.pollForToken(t, deviceAuthResp.DeviceCode)

	// Use token to verify it works
	test.testAuthenticatedRequest(t, tokenResp.AccessToken)

	// Revoke token
	test.revokeToken(t, tokenResp.AccessToken)

	// Try to use revoked token - should fail
	err := test.testAuthenticatedRequestExpectError(t, tokenResp.AccessToken)
	assert.Error(t, err)
}

// Helper methods

func (test *AuthFlowE2ETest) requestDeviceCode(t *testing.T) *DeviceAuthResponse {
	payload := map[string]string{
		"client_id": "fleetctl",
		"scope":     "api",
	}

	body, _ := json.Marshal(payload)
	client := test.getHTTPClient()

	resp, err := client.Post(
		fmt.Sprintf("%s/api/v1/auth/device/code", test.platformAPIURL),
		"application/json",
		bytes.NewBuffer(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var authResp DeviceAuthResponse
	err = json.NewDecoder(resp.Body).Decode(&authResp)
	require.NoError(t, err)

	assert.NotEmpty(t, authResp.DeviceCode)
	assert.NotEmpty(t, authResp.UserCode)

	return &authResp
}

func (test *AuthFlowE2ETest) verifyUserCode(t *testing.T, userCode string) bool {
	client := test.getHTTPClient()

	resp, err := client.Get(
		fmt.Sprintf("%s/api/v1/auth/device/verify?code=%s", test.platformAPIURL, userCode),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var result map[string]bool
	json.NewDecoder(resp.Body).Decode(&result)
	return result["valid"]
}

func (test *AuthFlowE2ETest) approveDeviceAuth(t *testing.T, userCode, userID string) {
	payload := map[string]string{
		"code":    userCode,
		"user_id": userID,
	}

	body, _ := json.Marshal(payload)
	client := test.getHTTPClient()

	resp, err := client.Post(
		fmt.Sprintf("%s/api/v1/auth/device/approve", test.platformAPIURL),
		"application/json",
		bytes.NewBuffer(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func (test *AuthFlowE2ETest) pollForToken(t *testing.T, deviceCode string) *TokenResponse {
	tokenResp, err := test.pollForTokenWithError(t, deviceCode)
	require.NoError(t, err)
	return tokenResp
}

func (test *AuthFlowE2ETest) pollForTokenWithError(t *testing.T, deviceCode string) (*TokenResponse, error) {
	payload := map[string]string{
		"device_code": deviceCode,
		"client_id":   "fleetctl",
	}

	body, _ := json.Marshal(payload)
	client := test.getHTTPClient()

	// Poll for up to 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("polling timeout")
		case <-ticker.C:
			resp, err := client.Post(
				fmt.Sprintf("%s/api/v1/auth/device/token", test.platformAPIURL),
				"application/json",
				bytes.NewBuffer(body),
			)
			if err != nil {
				continue
			}

			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var tokenResp TokenResponse
				err := json.Unmarshal(bodyBytes, &tokenResp)
				if err != nil {
					return nil, err
				}
				return &tokenResp, nil
			}

			var errorResp map[string]string
			json.Unmarshal(bodyBytes, &errorResp)

			if errorCode := errorResp["error"]; errorCode != "authorization_pending" {
				return nil, fmt.Errorf("auth error: %s", errorCode)
			}
		}
	}
}

func (test *AuthFlowE2ETest) testAuthenticatedRequest(t *testing.T, token string) {
	client := test.getHTTPClient()

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/devices", test.platformAPIURL), nil)
	require.NoError(t, err)

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func (test *AuthFlowE2ETest) testAuthenticatedRequestExpectError(t *testing.T, token string) error {
	client := test.getHTTPClient()

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/devices", test.platformAPIURL), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	return nil
}

func (test *AuthFlowE2ETest) revokeToken(t *testing.T, token string) {
	client := test.getHTTPClient()

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/auth/revoke", test.platformAPIURL), nil)
	require.NoError(t, err)

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Contains(t, []int{http.StatusOK, http.StatusNoContent}, resp.StatusCode)
}

func (test *AuthFlowE2ETest) createTestUser(t *testing.T, email string) string {
	userID := fmt.Sprintf("user-%d", time.Now().Unix())

	_, err := test.db.Exec(`
		INSERT INTO user_account (id, email, password_hash, name, role)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (email) DO UPDATE SET id = EXCLUDED.id
		RETURNING id
	`, userID, email, "hash", "Test User", "admin")
	require.NoError(t, err)

	return userID
}

func (test *AuthFlowE2ETest) cleanTestData() {
	// Clean up test data
	test.db.Exec("DELETE FROM access_token WHERE client_id = 'fleetctl'")
	test.db.Exec("DELETE FROM device_auth_request WHERE client_id = 'fleetctl'")
	test.db.Exec("DELETE FROM user_account WHERE email LIKE '%@example.com'")
}

func (test *AuthFlowE2ETest) ensurePlatformAPIRunning() {
	client := test.getHTTPClient()

	// Try to reach the health endpoint
	resp, err := client.Get(fmt.Sprintf("%s/health", test.platformAPIURL))
	if err != nil || resp.StatusCode != http.StatusOK {
		test.t.Skip("Platform API is not running at " + test.platformAPIURL)
	}
	if resp != nil {
		resp.Body.Close()
	}
}

func (test *AuthFlowE2ETest) getHTTPClient() *http.Client {
	// Create client with TLS verification disabled for testing
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Response types

type DeviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// CLI Integration Test
func TestE2E_CLIAuthentication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if fleetctl binary exists
	fleetctlPath := "./bin/fleetctl"
	if _, err := os.Stat(fleetctlPath); os.IsNotExist(err) {
		t.Skip("fleetctl binary not found")
	}

	// Create temp home directory for test
	tempHome, err := ioutil.TempDir("", "fleetctl-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempHome)

	// Run fleetctl login with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, fleetctlPath, "login", "--no-browser")
	cmd.Env = append(os.Environ(),
		"HOME="+tempHome,
		"PLATFORM_API_URL=https://localhost:8090",
	)

	output, err := cmd.CombinedOutput()

	// We expect it to timeout waiting for authentication
	if ctx.Err() == context.DeadlineExceeded {
		// Check that it displayed the auth code
		outputStr := string(output)
		assert.Contains(t, outputStr, "FleetD Authentication")
		assert.Contains(t, outputStr, "Enter this code:")
		assert.Contains(t, outputStr, "https://localhost:3000/auth/device")

		// Extract the user code from output
		lines := strings.Split(outputStr, "\n")
		for i, line := range lines {
			if strings.Contains(line, "Enter this code:") && i+1 < len(lines) {
				// Next line should contain the code
				codeLine := lines[i+1]
				// Remove ANSI color codes
				codeLine = strings.ReplaceAll(codeLine, "\033[1;33m", "")
				codeLine = strings.ReplaceAll(codeLine, "\033[0m", "")
				code := strings.TrimSpace(codeLine)

				// Verify code format (XXXX-XXXX)
				assert.Regexp(t, `^[A-Z0-9]{4}-[A-Z0-9]{4}$`, code)
				break
			}
		}
	} else {
		t.Fatalf("Expected timeout waiting for auth, got: %v\nOutput: %s", err, output)
	}
}