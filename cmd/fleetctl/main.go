package main

import (
	"os"

	"fleetd.sh/cmd/fleetctl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
