package cmd

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/spf13/cobra"
	"net/http"
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Display device metrics with charts",
	Long:  `Display real-time device metrics using terminal charts and graphs`,
}

var metricsChartCmd = &cobra.Command{
	Use:   "chart [device-id]",
	Short: "Display metrics as terminal charts",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID := ""
		if len(args) > 0 {
			deviceID = args[0]
		}

		refresh, _ := cmd.Flags().GetInt("refresh")
		duration, _ := cmd.Flags().GetString("duration")

		if err := ui.Init(); err != nil {
			return fmt.Errorf("failed to initialize termui: %v", err)
		}
		defer ui.Close()

		// Create widgets
		cpuGauge := widgets.NewGauge()
		cpuGauge.Title = "CPU Usage"
		cpuGauge.SetRect(0, 0, 50, 3)
		cpuGauge.BarColor = ui.ColorGreen

		memGauge := widgets.NewGauge()
		memGauge.Title = "Memory Usage"
		memGauge.SetRect(0, 3, 50, 6)
		memGauge.BarColor = ui.ColorYellow

		diskGauge := widgets.NewGauge()
		diskGauge.Title = "Disk Usage"
		diskGauge.SetRect(0, 6, 50, 9)
		diskGauge.BarColor = ui.ColorMagenta

		cpuSparkline := widgets.NewSparkline()
		cpuSparkline.Title = "CPU"
		cpuSparkline.LineColor = ui.ColorCyan
		cpuSparkline.Data = []float64{}

		memSparkline := widgets.NewSparkline()
		memSparkline.Title = "Memory"
		memSparkline.LineColor = ui.ColorYellow
		memSparkline.Data = []float64{}

		sparklineGroup := widgets.NewSparklineGroup(cpuSparkline, memSparkline)
		sparklineGroup.Title = fmt.Sprintf("Metrics History - %s", duration)
		sparklineGroup.SetRect(0, 9, 50, 20)

		tempPlot := widgets.NewPlot()
		tempPlot.Title = "Temperature (°C)"
		tempPlot.SetRect(51, 0, 100, 10)
		tempPlot.Data = [][]float64{{}}
		tempPlot.AxesColor = ui.ColorWhite
		tempPlot.LineColors[0] = ui.ColorRed

		netSparkline := widgets.NewSparkline()
		netSparkline.Title = "Network (MB/s)"
		netSparkline.LineColor = ui.ColorBlue
		netSparkline.Data = []float64{}

		netGroup := widgets.NewSparklineGroup(netSparkline)
		netGroup.Title = "Network Usage"
		netGroup.SetRect(51, 10, 100, 20)

		statusPar := widgets.NewParagraph()
		statusPar.Title = "Device Status"
		statusPar.SetRect(0, 20, 100, 25)
		statusPar.Text = fmt.Sprintf("Device: %s\nRefresh: %ds\nPress 'q' to quit", deviceID, refresh)
		statusPar.BorderStyle.Fg = ui.ColorWhite

		// Render function
		render := func() {
			ui.Render(cpuGauge, memGauge, diskGauge, sparklineGroup, tempPlot, netGroup, statusPar)
		}

		// Update function
		update := func() {
			client := fleetpbconnect.NewTelemetryServiceClient(
				http.DefaultClient,
				getAPIURL(),
			)

			req := &fleetpb.GetTelemetryRequest{
				DeviceId: deviceID,
				Limit:    50,
			}

			resp, err := client.GetTelemetry(context.Background(), connect.NewRequest(req))
			if err != nil {
				statusPar.Text = fmt.Sprintf("Error: %v", err)
				render()
				return
			}

			if len(resp.Msg.Data) > 0 {
				// Get latest data point
				latest := resp.Msg.Data[len(resp.Msg.Data)-1]

				// Update gauges
				cpuGauge.Percent = int(latest.CpuUsage)
				cpuGauge.Label = fmt.Sprintf("%.1f%%", latest.CpuUsage)
				if latest.CpuUsage > 80 {
					cpuGauge.BarColor = ui.ColorRed
				} else if latest.CpuUsage > 60 {
					cpuGauge.BarColor = ui.ColorYellow
				} else {
					cpuGauge.BarColor = ui.ColorGreen
				}

				memGauge.Percent = int(latest.MemoryUsage)
				memGauge.Label = fmt.Sprintf("%.1f%%", latest.MemoryUsage)
				if latest.MemoryUsage > 80 {
					memGauge.BarColor = ui.ColorRed
				} else if latest.MemoryUsage > 60 {
					memGauge.BarColor = ui.ColorYellow
				} else {
					memGauge.BarColor = ui.ColorGreen
				}

				diskGauge.Percent = int(latest.DiskUsage)
				diskGauge.Label = fmt.Sprintf("%.1f%%", latest.DiskUsage)

				// Update sparklines with historical data
				cpuData := []float64{}
				memData := []float64{}
				netData := []float64{}
				tempData := []float64{}

				for _, data := range resp.Msg.Data {
					cpuData = append(cpuData, data.CpuUsage)
					memData = append(memData, data.MemoryUsage)
					netData = append(netData, data.NetworkUsage)
					tempData = append(tempData, data.Temperature)
				}

				cpuSparkline.Data = cpuData
				memSparkline.Data = memData
				netSparkline.Data = netData
				tempPlot.Data = [][]float64{tempData}

				// Update status
				statusPar.Text = fmt.Sprintf(
					"Device: %s\nLast Update: %s\nCPU: %.1f%% | Memory: %.1f%% | Disk: %.1f%% | Network: %.1f MB/s | Temp: %.1f°C\nPress 'q' to quit",
					deviceID,
					latest.Timestamp.AsTime().Format("15:04:05"),
					latest.CpuUsage,
					latest.MemoryUsage,
					latest.DiskUsage,
					latest.NetworkUsage,
					latest.Temperature,
				)
			}

			render()
		}

		// Initial render
		update()

		// Set up event loop
		uiEvents := ui.PollEvents()
		ticker := time.NewTicker(time.Duration(refresh) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case e := <-uiEvents:
				switch e.ID {
				case "q", "<C-c>":
					return nil
				case "<Resize>":
					payload := e.Payload.(ui.Resize)
					cpuGauge.SetRect(0, 0, payload.Width/2, 3)
					memGauge.SetRect(0, 3, payload.Width/2, 6)
					diskGauge.SetRect(0, 6, payload.Width/2, 9)
					sparklineGroup.SetRect(0, 9, payload.Width/2, 20)
					tempPlot.SetRect(payload.Width/2, 0, payload.Width, 10)
					netGroup.SetRect(payload.Width/2, 10, payload.Width, 20)
					statusPar.SetRect(0, 20, payload.Width, 25)
					render()
				}
			case <-ticker.C:
				update()
			}
		}
	},
}

var metricsDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Display a full metrics dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		refresh, _ := cmd.Flags().GetInt("refresh")

		if err := ui.Init(); err != nil {
			return fmt.Errorf("failed to initialize termui: %v", err)
		}
		defer ui.Close()

		// Create table for device list
		deviceTable := widgets.NewTable()
		deviceTable.Title = "Fleet Devices"
		deviceTable.SetRect(0, 0, 50, 15)
		deviceTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		deviceTable.RowSeparator = true
		deviceTable.Rows = [][]string{
			{"Device ID", "Status", "CPU%", "Memory%", "Disk%"},
		}

		// Create pie chart for fleet overview
		pieChart := widgets.NewPieChart()
		pieChart.Title = "Fleet Status"
		pieChart.SetRect(51, 0, 100, 15)
		pieChart.Data = []float64{0, 0, 0}
		pieChart.LabelFormatter = func(i int, v float64) string {
			labels := []string{"Online", "Offline", "Warning"}
			return fmt.Sprintf("%s: %.0f", labels[i], v)
		}
		pieChart.Colors = []ui.Color{ui.ColorGreen, ui.ColorRed, ui.ColorYellow}

		// Create bar chart for top CPU usage
		barChart := widgets.NewBarChart()
		barChart.Title = "Top CPU Usage by Device"
		barChart.SetRect(0, 15, 50, 25)
		barChart.Data = []float64{}
		barChart.Labels = []string{}
		barChart.BarWidth = 5
		barChart.BarColors = []ui.Color{ui.ColorCyan}
		barChart.LabelStyles = []ui.Style{ui.NewStyle(ui.ColorWhite)}
		barChart.NumStyles = []ui.Style{ui.NewStyle(ui.ColorWhite)}

		// Create list for recent alerts
		alertList := widgets.NewList()
		alertList.Title = "Recent Alerts"
		alertList.SetRect(51, 15, 100, 25)
		alertList.Rows = []string{"No alerts"}
		alertList.TextStyle = ui.NewStyle(ui.ColorYellow)
		alertList.WrapText = true

		// Status bar
		statusBar := widgets.NewParagraph()
		statusBar.Title = "Dashboard Controls"
		statusBar.SetRect(0, 25, 100, 28)
		statusBar.Text = fmt.Sprintf("Refresh: %ds | Press 'q' to quit | 'r' to refresh now", refresh)
		statusBar.BorderStyle.Fg = ui.ColorWhite

		render := func() {
			ui.Render(deviceTable, pieChart, barChart, alertList, statusBar)
		}

		update := func() {
			client := fleetpbconnect.NewDeviceServiceClient(
				http.DefaultClient,
				getAPIURL(),
			)

			// Get device list
			devResp, err := client.ListDevices(context.Background(), connect.NewRequest(&fleetpb.ListDevicesRequest{}))
			if err != nil {
				statusBar.Text = fmt.Sprintf("Error: %v", err)
				render()
				return
			}

			// Update device table
			rows := [][]string{{"Device ID", "Status", "CPU%", "Memory%", "Disk%"}}
			online := 0.0
			offline := 0.0
			warning := 0.0
			cpuData := []float64{}
			cpuLabels := []string{}

			telemetryClient := fleetpbconnect.NewTelemetryServiceClient(
				http.DefaultClient,
				getAPIURL(),
			)

			for i, device := range devResp.Msg.Devices {
				if i >= 10 { // Limit to 10 devices in table
					break
				}

				// Get telemetry for each device
				telResp, _ := telemetryClient.GetTelemetry(
					context.Background(),
					connect.NewRequest(&fleetpb.GetTelemetryRequest{
						DeviceId: device.Id,
						Limit:    1,
					}),
				)

				status := "Offline"
				cpu := "-"
				mem := "-"
				disk := "-"

				// Check if device is online based on LastSeen
				isOnline := false
				if device.LastSeen != nil {
					// Consider online if seen in last 5 minutes
					if time.Since(device.LastSeen.AsTime()) < 5*time.Minute {
						isOnline = true
					}
				}

				if isOnline {
					online++
					status = "Online"

					if telResp != nil && len(telResp.Msg.Data) > 0 {
						latest := telResp.Msg.Data[0]
						cpu = fmt.Sprintf("%.1f", latest.CpuUsage)
						mem = fmt.Sprintf("%.1f", latest.MemoryUsage)
						disk = fmt.Sprintf("%.1f", latest.DiskUsage)

						if latest.CpuUsage > 80 {
							warning++
							online--
						}

						// Add to CPU chart data
						if i < 5 { // Top 5 for bar chart
							cpuData = append(cpuData, latest.CpuUsage)
							cpuLabels = append(cpuLabels, device.Name)
						}
					}
				} else {
					offline++
				}

				rows = append(rows, []string{device.Id, status, cpu, mem, disk})
			}

			deviceTable.Rows = rows
			pieChart.Data = []float64{online, offline, warning}
			barChart.Data = cpuData
			barChart.Labels = cpuLabels

			// Update status
			statusBar.Text = fmt.Sprintf(
				"Total Devices: %d | Online: %.0f | Offline: %.0f | Warning: %.0f | Last Update: %s | 'q' quit | 'r' refresh",
				len(devResp.Msg.Devices),
				online,
				offline,
				warning,
				time.Now().Format("15:04:05"),
			)

			render()
		}

		// Initial render
		update()

		// Event loop
		uiEvents := ui.PollEvents()
		ticker := time.NewTicker(time.Duration(refresh) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case e := <-uiEvents:
				switch e.ID {
				case "q", "<C-c>":
					return nil
				case "r":
					update()
				case "<Resize>":
					payload := e.Payload.(ui.Resize)
					deviceTable.SetRect(0, 0, payload.Width/2, 15)
					pieChart.SetRect(payload.Width/2, 0, payload.Width, 15)
					barChart.SetRect(0, 15, payload.Width/2, 25)
					alertList.SetRect(payload.Width/2, 15, payload.Width, 25)
					statusBar.SetRect(0, 25, payload.Width, 28)
					render()
				}
			case <-ticker.C:
				update()
			}
		}
	},
}

func init() {
	metricsChartCmd.Flags().Int("refresh", 5, "Refresh interval in seconds")
	metricsChartCmd.Flags().String("duration", "5m", "Duration of historical data to show")

	metricsDashboardCmd.Flags().Int("refresh", 10, "Refresh interval in seconds")

	metricsCmd.AddCommand(metricsChartCmd)
	metricsCmd.AddCommand(metricsDashboardCmd)

	rootCmd.AddCommand(metricsCmd)
}