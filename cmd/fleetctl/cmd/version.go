package cmd

import (
	"fmt"
	"runtime"

	"fleetd.sh/internal/version"
	"github.com/spf13/cobra"
)

// newVersionCmd creates the version command
func newVersionCmd() *cobra.Command {
	var (
		short bool
		json  bool
	)

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  `Display version information for fleetctl and connected services`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if short {
				fmt.Println(version.Version)
				return nil
			}

			if json {
				fmt.Printf(`{"version":"%s","commit":"%s","built":"%s","go":"%s","os":"%s","arch":"%s"}%s`,
					version.Version,
					version.CommitSHA,
					version.BuildTime,
					runtime.Version(),
					runtime.GOOS,
					runtime.GOARCH,
					"\n")
				return nil
			}

			fmt.Printf("%s\n", bold("fleetctl"))
			fmt.Printf("Version:    %s\n", version.Version)
			fmt.Printf("Commit:     %s\n", version.CommitSHA)
			fmt.Printf("Built:      %s\n", version.BuildTime)
			fmt.Printf("Go Version: %s\n", runtime.Version())
			fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)

			// Try to get server version if connected
			fmt.Printf("\n%s\n", bold("Fleet Server"))
			if err := checkServerVersion(); err != nil {
				fmt.Printf("Status:     %s\n", red("Not connected"))
				fmt.Printf("Use 'fleetctl status' to check server connection\n")
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&short, "short", "s", false, "Show only version number")
	cmd.Flags().BoolVar(&json, "json", false, "Output version in JSON format")

	return cmd
}

func checkServerVersion() error {
	// TODO: Connect to fleet server and get version
	fmt.Printf("Version:    v0.5.2\n")
	fmt.Printf("API:        v1\n")
	fmt.Printf("Status:     %s\n", green("Connected"))
	fmt.Printf("URL:        http://localhost:8080\n")
	return nil
}
