package integration

import (
	"os"
	"testing"
)

// ExampleIntegrationTest demonstrates the pattern for integration tests
// that skip when INTEGRATION environment variable is not set.
// This follows best practices from Peter Bourgon's article:
// https://peter.bourgon.org/blog/2021/04/02/dont-use-build-tags-for-integration-tests.html
func TestExampleIntegration(t *testing.T) {
	// Check for integration test environment variable
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run integration tests")
	}

	// Your integration test code here
	t.Log("Running integration test...")

	// Example: would connect to database, external services, etc.
	// db := setupTestDatabase(t)
	// defer db.Close()
}

// TestExampleIntegrationWithDatabase shows database integration pattern
func TestExampleIntegrationWithDatabase(t *testing.T) {
	// Skip if not running integration tests
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run integration tests")
	}

	// Also check if database is available
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("set DATABASE_URL to run database integration tests")
	}

	// Integration test code here
	t.Log("Running database integration test...")
}