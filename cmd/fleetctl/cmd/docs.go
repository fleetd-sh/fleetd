package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func newDocsCmd() *cobra.Command {
	var (
		outputDir string
		format    string
	)

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Generate documentation for fleetctl",
		Long: `Generate documentation for fleetctl commands.

This command generates documentation in various formats (markdown, man pages, etc.)
using Cobra's built-in documentation generator. The generated documentation
stays in sync with the actual CLI commands automatically.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Ensure output directory exists
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			printInfo("Generating %s documentation in %s", format, outputDir)

			switch format {
			case "markdown", "md":
				if err := doc.GenMarkdownTree(rootCmd, outputDir); err != nil {
					return fmt.Errorf("failed to generate markdown docs: %w", err)
				}
			case "man":
				if err := doc.GenManTree(rootCmd, &doc.GenManHeader{
					Title:   "fleetctl",
					Section: "1",
					Source:  "fleetd",
					Manual:  "fleetctl Manual",
				}, outputDir); err != nil {
					return fmt.Errorf("failed to generate man pages: %w", err)
				}
			case "yaml":
				if err := doc.GenYamlTree(rootCmd, outputDir); err != nil {
					return fmt.Errorf("failed to generate YAML docs: %w", err)
				}
			case "rest":
				if err := doc.GenReSTTree(rootCmd, outputDir); err != nil {
					return fmt.Errorf("failed to generate ReST docs: %w", err)
				}
			default:
				return fmt.Errorf("unsupported format: %s (supported: markdown, man, yaml, rest)", format)
			}

			printSuccess("Documentation generated successfully")
			printInfo("Files written to: %s", outputDir)

			// List generated files
			files, err := filepath.Glob(filepath.Join(outputDir, "*"))
			if err == nil && len(files) > 0 {
				printInfo("Generated files:")
				for _, file := range files {
					fmt.Printf("  - %s\n", filepath.Base(file))
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "gen/docs/cli", "Output directory for generated documentation")
	cmd.Flags().StringVarP(&format, "format", "f", "markdown", "Documentation format (markdown, man, yaml, rest)")

	return cmd
}