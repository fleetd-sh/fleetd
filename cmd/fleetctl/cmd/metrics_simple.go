package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var metricsGraphCmd = &cobra.Command{
	Use:   "graph [device-id]",
	Short: "Display metrics as ASCII graphs",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID := ""
		if len(args) > 0 {
			deviceID = args[0]
		}

		limit, _ := cmd.Flags().GetInt32("points")
		width, _ := cmd.Flags().GetInt("width")
		height, _ := cmd.Flags().GetInt("height")
		follow, _ := cmd.Flags().GetBool("follow")
		interval, _ := cmd.Flags().GetInt("interval")

		client := fleetpbconnect.NewTelemetryServiceClient(
			http.DefaultClient,
			getAPIURL(),
		)

		displayMetrics := func() error {
			req := &fleetpb.GetTelemetryRequest{
				DeviceId: deviceID,
				Limit:    limit,
			}

			resp, err := client.GetTelemetry(context.Background(), connect.NewRequest(req))
			if err != nil {
				return fmt.Errorf("failed to get telemetry: %w", err)
			}

			if len(resp.Msg.Data) == 0 {
				fmt.Println("No telemetry data available")
				return nil
			}

			// Clear screen
			fmt.Print("\033[2J\033[H")

			// Display header
			fmt.Printf("=== Device Metrics: %s ===\n", deviceID)
			fmt.Printf("Time: %s\n\n", time.Now().Format("15:04:05"))

			// Extract data for graphs
			cpuData := make([]float64, len(resp.Msg.Data))
			memData := make([]float64, len(resp.Msg.Data))
			diskData := make([]float64, len(resp.Msg.Data))
			netData := make([]float64, len(resp.Msg.Data))
			tempData := make([]float64, len(resp.Msg.Data))

			for i, data := range resp.Msg.Data {
				cpuData[i] = data.CpuUsage
				memData[i] = data.MemoryUsage
				diskData[i] = data.DiskUsage
				netData[i] = data.NetworkUsage
				tempData[i] = data.Temperature
			}

			// Display CPU graph
			fmt.Println("CPU Usage (%):")
			drawASCIIGraph(cpuData, width, height, 0, 100)
			fmt.Println()

			// Display Memory graph
			fmt.Println("Memory Usage (%):")
			drawASCIIGraph(memData, width, height, 0, 100)
			fmt.Println()

			// Display bar charts for latest values
			latest := resp.Msg.Data[len(resp.Msg.Data)-1]

			fmt.Println("Current Status:")
			drawHorizontalBar("CPU", latest.CpuUsage, 100, 40)
			drawHorizontalBar("Memory", latest.MemoryUsage, 100, 40)
			drawHorizontalBar("Disk", latest.DiskUsage, 100, 40)
			fmt.Printf("Network: %.2f MB/s\n", latest.NetworkUsage)
			fmt.Printf("Temperature: %.1f°C\n", latest.Temperature)

			if follow {
				fmt.Printf("\nRefreshing every %d seconds. Press Ctrl+C to exit.\n", interval)
			}

			return nil
		}

		if follow {
			ticker := time.NewTicker(time.Duration(interval) * time.Second)
			defer ticker.Stop()

			// Initial display
			if err := displayMetrics(); err != nil {
				return err
			}

			// Continue updating
			for range ticker.C {
				if err := displayMetrics(); err != nil {
					return err
				}
			}
		} else {
			return displayMetrics()
		}

		return nil
	},
}

// drawASCIIGraph creates a simple ASCII line graph
func drawASCIIGraph(data []float64, width, height int, min, max float64) {
	if len(data) == 0 {
		return
	}

	// Normalize data to fit the height
	normalized := make([]int, len(data))
	for i, val := range data {
		normalized[i] = int((val - min) / (max - min) * float64(height-1))
	}

	// Create the graph
	graph := make([][]rune, height)
	for i := range graph {
		graph[i] = make([]rune, width)
		for j := range graph[i] {
			graph[i][j] = ' '
		}
	}

	// Plot the data
	step := float64(width) / float64(len(data)-1)
	for i := 0; i < len(data)-1; i++ {
		x1 := int(float64(i) * step)
		x2 := int(float64(i+1) * step)
		y1 := height - 1 - normalized[i]
		y2 := height - 1 - normalized[i+1]

		// Draw line between points
		if x1 < width && y1 >= 0 && y1 < height {
			graph[y1][x1] = '●'
		}

		// Simple line interpolation
		if y1 == y2 {
			for x := x1; x <= x2 && x < width; x++ {
				if y1 >= 0 && y1 < height {
					if graph[y1][x] == ' ' {
						graph[y1][x] = '─'
					}
				}
			}
		} else if y1 < y2 {
			for y := y1; y <= y2 && y < height; y++ {
				if x1 < width && y >= 0 {
					if graph[y][x1] == ' ' {
						graph[y][x1] = '│'
					}
				}
			}
		} else {
			for y := y2; y <= y1 && y < height; y++ {
				if x2 < width && y >= 0 {
					if graph[y][x2] == ' ' {
						graph[y][x2] = '│'
					}
				}
			}
		}
	}

	// Add Y-axis labels
	for i := 0; i < height; i++ {
		value := max - (float64(i)/float64(height-1))*(max-min)
		fmt.Printf("%6.1f │", value)
		for j := 0; j < width; j++ {
			fmt.Printf("%c", graph[i][j])
		}
		fmt.Println()
	}

	// Add X-axis
	fmt.Printf("       └")
	for i := 0; i < width; i++ {
		fmt.Print("─")
	}
	fmt.Println()
}

// drawHorizontalBar creates a horizontal bar chart
func drawHorizontalBar(label string, value, max float64, width int) {
	barWidth := int(value / max * float64(width))
	bar := strings.Repeat("█", barWidth) + strings.Repeat("░", width-barWidth)

	// Color based on value
	c := color.New(color.FgGreen)
	if value > 80 {
		c = color.New(color.FgRed)
	} else if value > 60 {
		c = color.New(color.FgYellow)
	}

	fmt.Printf("%-8s [", label)
	c.Printf("%s", bar)
	fmt.Printf("] %.1f%%\n", value)
}

var metricsSparklineCmd = &cobra.Command{
	Use:   "sparkline [device-id]",
	Short: "Display metrics as sparklines",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID := ""
		if len(args) > 0 {
			deviceID = args[0]
		}

		limit, _ := cmd.Flags().GetInt32("points")

		client := fleetpbconnect.NewTelemetryServiceClient(
			http.DefaultClient,
			getAPIURL(),
		)

		req := &fleetpb.GetTelemetryRequest{
			DeviceId: deviceID,
			Limit:    limit,
		}

		resp, err := client.GetTelemetry(context.Background(), connect.NewRequest(req))
		if err != nil {
			return fmt.Errorf("failed to get telemetry: %w", err)
		}

		if len(resp.Msg.Data) == 0 {
			fmt.Println("No telemetry data available")
			return nil
		}

		// Extract data
		cpuData := make([]float64, len(resp.Msg.Data))
		memData := make([]float64, len(resp.Msg.Data))
		diskData := make([]float64, len(resp.Msg.Data))
		netData := make([]float64, len(resp.Msg.Data))

		for i, data := range resp.Msg.Data {
			cpuData[i] = data.CpuUsage
			memData[i] = data.MemoryUsage
			diskData[i] = data.DiskUsage
			netData[i] = data.NetworkUsage
		}

		// Display sparklines
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "Metric\tSparkline\tMin\tMax\tCurrent\n")
		fmt.Fprintf(w, "------\t---------\t---\t---\t-------\n")

		fmt.Fprintf(w, "CPU\t%s\t%.1f%%\t%.1f%%\t%.1f%%\n",
			sparkline(cpuData, 30),
			min(cpuData), max(cpuData), cpuData[len(cpuData)-1])

		fmt.Fprintf(w, "Memory\t%s\t%.1f%%\t%.1f%%\t%.1f%%\n",
			sparkline(memData, 30),
			min(memData), max(memData), memData[len(memData)-1])

		fmt.Fprintf(w, "Disk\t%s\t%.1f%%\t%.1f%%\t%.1f%%\n",
			sparkline(diskData, 30),
			min(diskData), max(diskData), diskData[len(diskData)-1])

		fmt.Fprintf(w, "Network\t%s\t%.1f\t%.1f\t%.1f MB/s\n",
			sparkline(netData, 30),
			min(netData), max(netData), netData[len(netData)-1])

		w.Flush()

		return nil
	},
}

// sparkline creates a Unicode sparkline graph
func sparkline(data []float64, width int) string {
	if len(data) == 0 {
		return ""
	}

	// Unicode sparkline characters
	sparks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	// Resample data if needed
	resampled := data
	if len(data) > width {
		resampled = make([]float64, width)
		step := float64(len(data)) / float64(width)
		for i := 0; i < width; i++ {
			idx := int(float64(i) * step)
			if idx >= len(data) {
				idx = len(data) - 1
			}
			resampled[i] = data[idx]
		}
	}

	// Find min and max
	minVal := min(resampled)
	maxVal := max(resampled)
	if maxVal == minVal {
		maxVal = minVal + 1
	}

	// Create sparkline
	result := ""
	for _, val := range resampled {
		idx := int((val - minVal) / (maxVal - minVal) * 7)
		if idx < 0 {
			idx = 0
		}
		if idx > 7 {
			idx = 7
		}
		result += string(sparks[idx])
	}

	return result
}

func min(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := data[0]
	for _, v := range data {
		if v < m {
			m = v
		}
	}
	return m
}

func max(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := data[0]
	for _, v := range data {
		if v > m {
			m = v
		}
	}
	return m
}

func init() {
	metricsGraphCmd.Flags().Int32("points", 50, "Number of data points to display")
	metricsGraphCmd.Flags().Int("width", 60, "Width of the graph")
	metricsGraphCmd.Flags().Int("height", 10, "Height of the graph")
	metricsGraphCmd.Flags().Bool("follow", false, "Continuously update the display")
	metricsGraphCmd.Flags().Int("interval", 5, "Refresh interval in seconds")

	metricsSparklineCmd.Flags().Int32("points", 100, "Number of data points to display")

	metricsCmd.AddCommand(metricsGraphCmd)
	metricsCmd.AddCommand(metricsSparklineCmd)
}
