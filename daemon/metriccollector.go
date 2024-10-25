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

	now := timestamppb.New(time.Now())

	metrics := []*metricspb.Metric{
		{
			Name:      "cpu_usage",
			Value:     cpuPercent[0],
			Timestamp: now,
			Tags:      map[string]string{"device_id": mc.config.DeviceID},
		},
		{
			Name:      "memory_usage",
			Value:     float64(vmStat.UsedPercent),
			Timestamp: now,
			Tags:      map[string]string{"device_id": mc.config.DeviceID},
		},
		{
			Name:      "disk_usage",
			Value:     float64(diskStat.UsedPercent),
			Timestamp: now,
			Tags:      map[string]string{"device_id": mc.config.DeviceID, "mount": "/"},
		},
		{
			Name:      "network_bytes_sent",
			Value:     float64(netStat[0].BytesSent),
			Timestamp: now,
			Tags:      map[string]string{"device_id": mc.config.DeviceID},
		},
		{
			Name:      "network_bytes_recv",
			Value:     float64(netStat[0].BytesRecv),
			Timestamp: now,
			Tags:      map[string]string{"device_id": mc.config.DeviceID},
		},
	}

	req := connect.NewRequest(&metricspb.SendMetricsRequest{
		DeviceId: mc.config.DeviceID,
		Metrics:  metrics,
	})

	resp, err := mc.client.SendMetrics(context.Background(), req)
	if err != nil {
		slog.Error("Error sending metrics", "error", err, "deviceID", mc.config.DeviceID)
		return
	}

	if !resp.Msg.Success {
		slog.Warn("Metrics not accepted by server",
			"message", resp.Msg.Message,
			"deviceID", mc.config.DeviceID)
	}
}
