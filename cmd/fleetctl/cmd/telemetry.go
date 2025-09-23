package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"github.com/spf13/cobra"
)

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Manage telemetry and monitoring",
	Long:  `View and manage telemetry data, metrics, and logs from devices`,
}

var telemetryGetCmd = &cobra.Command{
	Use:   "get [device-id]",
	Short: "Get telemetry data",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID := ""
		if len(args) > 0 {
			deviceID = args[0]
		}

		limit, _ := cmd.Flags().GetInt32("limit")
		format, _ := cmd.Flags().GetString("format")

		client := fleetpbconnect.NewTelemetryServiceClient(
			http.DefaultClient,
			fmt.Sprintf("http://localhost:%d", getAPIPort()),
		)

		req := &fleetpb.GetTelemetryRequest{
			DeviceId: deviceID,
			Limit:    limit,
		}

		resp, err := client.GetTelemetry(context.Background(), connect.NewRequest(req))
		if err != nil {
			return fmt.Errorf("failed to get telemetry: %w", err)
		}

		if format == "json" {
			return outputJSON(resp.Msg.Data)
		}

		// Table format
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DEVICE\tTIME\tCPU%\tMEM%\tDISK%\tNET MB/s\tTEMP°C")
		for _, data := range resp.Msg.Data {
			fmt.Fprintf(w, "%s\t%s\t%.1f\t%.1f\t%.1f\t%.1f\t%.1f\n",
				data.DeviceId,
				data.Timestamp.AsTime().Format("15:04:05"),
				data.CpuUsage,
				data.MemoryUsage,
				data.DiskUsage,
				data.NetworkUsage,
				data.Temperature,
			)
		}
		w.Flush()

		return nil
	},
}

var telemetryLogsCmd = &cobra.Command{
	Use:   "logs [device-id]",
	Short: "Get device logs",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceIDs := []string{}
		if len(args) > 0 {
			deviceIDs = append(deviceIDs, args[0])
		}

		limit, _ := cmd.Flags().GetInt32("limit")
		filter, _ := cmd.Flags().GetString("filter")
		level, _ := cmd.Flags().GetString("level")
		follow, _ := cmd.Flags().GetBool("follow")

		client := fleetpbconnect.NewTelemetryServiceClient(
			http.DefaultClient,
			fmt.Sprintf("http://localhost:%d", getAPIPort()),
		)

		if follow {
			// Stream logs in real-time
			streamReq := &fleetpb.StreamLogsRequest{
				DeviceIds: deviceIDs,
				Filter:    filter,
			}

			if level != "" {
				lvl := parseLogLevel(level)
				if lvl != fleetpb.LogLevel_LOG_LEVEL_UNSPECIFIED {
					streamReq.Levels = []fleetpb.LogLevel{lvl}
				}
			}

			stream, err := client.StreamLogs(context.Background(), connect.NewRequest(streamReq))
			if err != nil {
				return fmt.Errorf("failed to stream logs: %w", err)
			}

			for stream.Receive() {
				log := stream.Msg()
				fmt.Printf("%s [%s] [%s] %s\n",
					log.Timestamp.AsTime().Format("15:04:05"),
					log.Level.String(),
					log.DeviceId,
					log.Message,
				)
			}

			if err := stream.Err(); err != nil {
				return fmt.Errorf("stream error: %w", err)
			}
		} else {
			// Get logs snapshot
			req := &fleetpb.GetLogsRequest{
				DeviceIds: deviceIDs,
				Filter:    filter,
				Limit:     limit,
			}

			if level != "" {
				lvl := parseLogLevel(level)
				if lvl != fleetpb.LogLevel_LOG_LEVEL_UNSPECIFIED {
					req.Levels = []fleetpb.LogLevel{lvl}
				}
			}

			resp, err := client.GetLogs(context.Background(), connect.NewRequest(req))
			if err != nil {
				return fmt.Errorf("failed to get logs: %w", err)
			}

			for _, log := range resp.Msg.Logs {
				fmt.Printf("%s [%s] [%s] %s\n",
					log.Timestamp.AsTime().Format("15:04:05"),
					log.Level.String(),
					log.DeviceId,
					log.Message,
				)
			}
		}

		return nil
	},
}

var telemetryAlertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Manage telemetry alerts",
}

var telemetryAlertsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured alerts",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := fleetpbconnect.NewTelemetryServiceClient(
			http.DefaultClient,
			fmt.Sprintf("http://localhost:%d", getAPIPort()),
		)

		req := &fleetpb.ListAlertsRequest{
			EnabledOnly: false,
		}

		resp, err := client.ListAlerts(context.Background(), connect.NewRequest(req))
		if err != nil {
			return fmt.Errorf("failed to list alerts: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tTYPE\tTHRESHOLD\tENABLED")
		for _, alert := range resp.Msg.Alerts {
			fmt.Fprintf(w, "%s\t%s\t%s\t%.1f\t%t\n",
				alert.Id,
				alert.Name,
				alert.Type.String(),
				alert.Threshold,
				alert.Enabled,
			)
		}
		w.Flush()

		return nil
	},
}

var telemetryAlertsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new alert",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		alertType, _ := cmd.Flags().GetString("type")
		threshold, _ := cmd.Flags().GetFloat64("threshold")
		description, _ := cmd.Flags().GetString("description")

		if name == "" || alertType == "" {
			return fmt.Errorf("name and type are required")
		}

		client := fleetpbconnect.NewTelemetryServiceClient(
			http.DefaultClient,
			fmt.Sprintf("http://localhost:%d", getAPIPort()),
		)

		alert := &fleetpb.Alert{
			Name:        name,
			Description: description,
			Type:        parseAlertType(alertType),
			Threshold:   threshold,
			Condition:   fleetpb.AlertCondition_ALERT_CONDITION_GREATER_THAN,
			Enabled:     true,
		}

		req := &fleetpb.ConfigureAlertRequest{
			Alert: alert,
		}

		resp, err := client.ConfigureAlert(context.Background(), connect.NewRequest(req))
		if err != nil {
			return fmt.Errorf("failed to create alert: %w", err)
		}

		printSuccess("Alert created: %s", resp.Msg.Alert.Id)
		return nil
	},
}

func parseLogLevel(level string) fleetpb.LogLevel {
	switch level {
	case "debug":
		return fleetpb.LogLevel_LOG_LEVEL_DEBUG
	case "info":
		return fleetpb.LogLevel_LOG_LEVEL_INFO
	case "warn":
		return fleetpb.LogLevel_LOG_LEVEL_WARN
	case "error":
		return fleetpb.LogLevel_LOG_LEVEL_ERROR
	case "fatal":
		return fleetpb.LogLevel_LOG_LEVEL_FATAL
	default:
		return fleetpb.LogLevel_LOG_LEVEL_UNSPECIFIED
	}
}

func parseAlertType(alertType string) fleetpb.AlertType {
	switch alertType {
	case "cpu":
		return fleetpb.AlertType_ALERT_TYPE_CPU
	case "memory":
		return fleetpb.AlertType_ALERT_TYPE_MEMORY
	case "disk":
		return fleetpb.AlertType_ALERT_TYPE_DISK
	case "network":
		return fleetpb.AlertType_ALERT_TYPE_NETWORK
	case "temperature":
		return fleetpb.AlertType_ALERT_TYPE_TEMPERATURE
	case "offline":
		return fleetpb.AlertType_ALERT_TYPE_DEVICE_OFFLINE
	default:
		return fleetpb.AlertType_ALERT_TYPE_CUSTOM
	}
}

func init() {
	telemetryGetCmd.Flags().Int32("limit", 100, "Maximum number of data points to retrieve")
	telemetryGetCmd.Flags().String("format", "table", "Output format (table or json)")

	telemetryLogsCmd.Flags().Int32("limit", 100, "Maximum number of logs to retrieve")
	telemetryLogsCmd.Flags().String("filter", "", "Filter logs by text")
	telemetryLogsCmd.Flags().String("level", "", "Filter by log level (debug, info, warn, error, fatal)")
	telemetryLogsCmd.Flags().Bool("follow", false, "Stream logs in real-time")

	telemetryAlertsCreateCmd.Flags().String("name", "", "Alert name")
	telemetryAlertsCreateCmd.Flags().String("type", "", "Alert type (cpu, memory, disk, network, temperature, offline)")
	telemetryAlertsCreateCmd.Flags().Float64("threshold", 0, "Alert threshold")
	telemetryAlertsCreateCmd.Flags().String("description", "", "Alert description")

	telemetryAlertsCmd.AddCommand(telemetryAlertsListCmd)
	telemetryAlertsCmd.AddCommand(telemetryAlertsCreateCmd)

	telemetryCmd.AddCommand(telemetryGetCmd)
	telemetryCmd.AddCommand(telemetryLogsCmd)
	telemetryCmd.AddCommand(telemetryAlertsCmd)

	rootCmd.AddCommand(telemetryCmd)
}
