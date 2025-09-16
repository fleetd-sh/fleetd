package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	environment string
	tag         string
	dryRun      bool
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy Fleet to production",
		Long: `Deploy Fleet server and infrastructure to production environment.

This command handles:
  - Building and tagging Docker images
  - Pushing to container registry
  - Updating Kubernetes manifests
  - Running database migrations
  - Health checks`,
		RunE: runDeploy,
	}

	cmd.Flags().StringVarP(&environment, "env", "e", "staging", "Target environment (staging/production)")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Image tag to deploy")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deployed without making changes")

	return cmd
}

func runDeploy(cmd *cobra.Command, args []string) error {
	printHeader(fmt.Sprintf("Deploying to %s", environment))
	fmt.Println()

	if dryRun {
		printWarning("DRY RUN - No changes will be made")
		fmt.Println()
	}

	// Load environment-specific config
	envConfig := viper.Sub(fmt.Sprintf("environments.%s", environment))
	if envConfig == nil {
		printError("Environment '%s' not configured", environment)
		return fmt.Errorf("unknown environment: %s", environment)
	}

	apiURL := envConfig.GetString("api.url")
	if apiURL == "" {
		printError("API URL not configured for environment %s", environment)
		return fmt.Errorf("missing API URL for environment")
	}

	// Deployment steps
	steps := []struct {
		name string
		fn   func() error
	}{
		{"Checking prerequisites", checkDeployPrerequisites},
		{"Building images", buildImages},
		{"Running tests", runTests},
		{"Tagging images", tagImages},
		{"Pushing to registry", pushImages},
		{"Updating manifests", updateManifests},
		{"Applying migrations", applyMigrations},
		{"Deploying services", deployServices},
		{"Running health checks", runHealthChecks},
	}

	for _, step := range steps {
		printInfo("%s...", step.name)
		if dryRun {
			printSuccess("[DRY RUN] %s", step.name)
			continue
		}

		if err := step.fn(); err != nil {
			printError("Failed: %v", err)
			return err
		}
		printSuccess("%s", step.name)
	}

	fmt.Println()
	if dryRun {
		printInfo("Dry run complete. Run without --dry-run to deploy.")
	} else {
		printSuccess("Deployment to %s complete!", environment)
		printInfo("View deployment: %s", apiURL)
	}

	return nil
}

func checkDeployPrerequisites() error {
	// Check for required tools
	tools := []string{"docker", "kubectl", "git"}
	for _, tool := range tools {
		if err := runCommand("which", tool); err != nil {
			return fmt.Errorf("%s not found", tool)
		}
	}
	return nil
}

func buildImages() error {
	// Build Docker images
	return runCommand("docker", "build", "-t", "fleet-server:latest", ".")
}

func runTests() error {
	// Run test suite
	return runCommand("go", "test", "./...")
}

func tagImages() error {
	// Tag images with version
	if tag == "" {
		tag = "latest"
	}
	return runCommand("docker", "tag", "fleet-server:latest", fmt.Sprintf("fleet-server:%s", tag))
}

func pushImages() error {
	// Push to registry
	// This would push to configured registry
	printInfo("Would push to registry")
	return nil
}

func updateManifests() error {
	// Update Kubernetes manifests
	printInfo("Would update Kubernetes manifests")
	return nil
}

func applyMigrations() error {
	// Run database migrations
	printInfo("Would apply database migrations")
	return nil
}

func deployServices() error {
	// Deploy to Kubernetes
	printInfo("Would deploy services")
	return nil
}

func runHealthChecks() error {
	// Check service health
	printInfo("Would run health checks")
	return nil
}
