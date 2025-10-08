package cmd

import (
	"time"

	"fleetd.sh/internal/tui"
	"github.com/spf13/cobra"
)

func newTUIDemoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "tui-demo",
		Short:  "Demo of the new TUI interface",
		Hidden: true,
		RunE:   runTUIDemo,
	}
	return cmd
}

func runTUIDemo(cmd *cobra.Command, args []string) error {
	status := tui.GetStatus()
	status.Start()
	defer status.Stop()

	// Simulate various tasks
	tasks := []struct {
		id      string
		name    string
		success bool
		delay   time.Duration
	}{
		{"docker", "Checking Docker", true, time.Second},
		{"compose", "Checking Docker Compose", true, time.Second},
		{"config", "Loading configuration", true, time.Millisecond * 500},
		{"pull-platform", "Pulling platform-api image", true, time.Second * 2},
		{"pull-device", "Pulling device-api image", true, time.Second * 2},
		{"pull-studio", "Pulling studio image", false, time.Second},
		{"postgres", "Starting PostgreSQL", true, time.Second * 2},
		{"valkey", "Starting Valkey", true, time.Second},
		{"platform", "Starting platform-api", true, time.Second * 2},
		{"device", "Starting device-api", true, time.Second},
		{"studio", "Starting studio", true, time.Second},
	}

	for _, task := range tasks {
		status.AddTask(task.id, task.name)
		status.UpdateTask(task.id, "running", "In progress...")

		time.Sleep(task.delay)

		if task.success {
			status.UpdateTask(task.id, "success", "Completed successfully")
		} else {
			status.UpdateTask(task.id, "error", "Failed to pull image")
		}
	}

	time.Sleep(time.Second * 2)
	return nil
}

func init() {
	rootCmd.AddCommand(newTUIDemoCmd())
}
