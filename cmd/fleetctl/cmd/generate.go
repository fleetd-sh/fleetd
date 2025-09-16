package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate [types]",
		Short: "Generate code from protobuf definitions",
		Long: `Generate code from protobuf definitions.

Available generators:
  types    - Generate TypeScript types from protobuf
  go       - Generate Go code from protobuf
  connect  - Generate Connect RPC client/server code`,
		RunE: runGenerate,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{"types", "go", "connect"}, cobra.ShellCompDirectiveNoFileComp
		},
	}

	return cmd
}

func runGenerate(cmd *cobra.Command, args []string) error {
	target := "all"
	if len(args) > 0 {
		target = args[0]
	}

	printHeader(fmt.Sprintf("Generating %s", target))

	switch target {
	case "types":
		return generateTypes()
	case "go":
		return generateGo()
	case "connect":
		return generateConnect()
	case "all":
		if err := generateGo(); err != nil {
			return err
		}
		if err := generateConnect(); err != nil {
			return err
		}
		return generateTypes()
	default:
		printError("Unknown target: %s", target)
		return fmt.Errorf("unknown generation target: %s", target)
	}
}

func generateTypes() error {
	printInfo("Generating TypeScript types...")

	cmd := exec.Command("buf", "generate", "--template", "buf.gen.yaml", "--path", "proto")
	cmd.Dir = getProjectRoot()

	if output, err := cmd.CombinedOutput(); err != nil {
		printError("Failed to generate types: %v", err)
		fmt.Println(string(output))
		return err
	}

	printSuccess("TypeScript types generated")
	return nil
}

func generateGo() error {
	printInfo("Generating Go code...")

	cmd := exec.Command("buf", "generate", "--template", "buf.gen.yaml", "--path", "proto")
	cmd.Dir = getProjectRoot()

	if output, err := cmd.CombinedOutput(); err != nil {
		printError("Failed to generate Go code: %v", err)
		fmt.Println(string(output))
		return err
	}

	printSuccess("Go code generated")
	return nil
}

func generateConnect() error {
	printInfo("Generating Connect RPC code...")

	cmd := exec.Command("buf", "generate", "--template", "buf.gen.yaml")
	cmd.Dir = getProjectRoot()

	if output, err := cmd.CombinedOutput(); err != nil {
		printError("Failed to generate Connect code: %v", err)
		fmt.Println(string(output))
		return err
	}

	printSuccess("Connect RPC code generated")
	return nil
}
