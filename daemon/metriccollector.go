package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"fleetd.sh/pkg/metricsclient"
)

type MetricCollector struct {
	config *Config
	client *metricsclient.Client
	stopCh chan struct{}
}

func NewMetricCollector(cfg *Config, client *metricsclient.Client) (*MetricCollector, error) {
	return &MetricCollector{
		config: cfg,
		client: client,
		stopCh: make(chan struct{}),
	}, nil
}

func (mc *MetricCollector) Start(ctx context.Context) {
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
		case <-ctx.Done():
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

	metrics := []*metricsclient.Metric{
		{
			DeviceID:    mc.config.DeviceID,
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
			Timestamp: time.Now(),
		},
	}

	err := mc.client.SendMetrics(context.Background(), metrics, "s")
	if err != nil {
		slog.With("error", err, "device_id", mc.config.DeviceID).Error("Error sending metrics")
		return
	}
}
