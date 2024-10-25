package daemon

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"google.golang.org/protobuf/types/known/timestamppb"

	metricspb "fleetd.sh/gen/metrics/v1"
	metricsrpc "fleetd.sh/gen/metrics/v1/metricsv1connect"
)

type MetricCollector struct {
	config *Config
	client metricsrpc.MetricsServiceClient
	stopCh chan struct{}
}

func NewMetricCollector(cfg *Config) (*MetricCollector, error) {
	client := metricsrpc.NewMetricsServiceClient(
		http.DefaultClient,
		cfg.MetricsServerURL,
	)

	return &MetricCollector{
		config: cfg,
		client: client,
		stopCh: make(chan struct{}),
	}, nil
}

func (mc *MetricCollector) Start() {
	interval, _ := time.ParseDuration(mc.config.MetricCollectionInterval)
	if interval == 0 {
		interval = time.Minute // Default to 1 minute if parsing fails
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.collectAndSendMetrics()
		case <-mc.stopCh:
			return
		}
	}
}

func (mc *MetricCollector) Stop() {
	close(mc.stopCh)
}

func (mc *MetricCollector) collectAndSendMetrics() {
	cpuPercent, _ := cpu.Percent(time.Second, false)
	vmStat, _ := mem.VirtualMemory()
	diskStat, _ := disk.Usage("/")
	netStat, _ := net.IOCounters(false)

	now := timestamppb.Now()

	metrics := []*metricspb.Metric{
		{
			Measurement: "system_metrics",
			Tags: map[string]string{
				"device_id": mc.config.DeviceID,
			},
			Fields: map[string]float64{
				"cpu_usage":          cpuPercent[0],
				"memory_usage":       float64(vmStat.UsedPercent),
				"disk_usage":         float64(diskStat.UsedPercent),
				"network_bytes_sent": float64(netStat[0].BytesSent),
				"network_bytes_recv": float64(netStat[0].BytesRecv),
			},
			Timestamp: now,
		},
	}

	req := connect.NewRequest(&metricspb.SendMetricsRequest{
		DeviceId:  mc.config.DeviceID,
		Metrics:   metrics,
		Precision: "s", // Assuming second precision
	})

	resp, err := mc.client.SendMetrics(context.Background(), req)
	if err != nil {
		slog.With("error", err, "device_id", mc.config.DeviceID).Error("Error sending metrics")
		return
	}

	if !resp.Msg.Success {
		slog.With("message", resp.Msg.Message, "device_id", mc.config.DeviceID).Warn("Metrics not accepted by server")
	}
}
