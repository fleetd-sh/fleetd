package build

import "context"

// ServiceMetrics represents the runtime metrics of a service
type ServiceMetrics struct {
	CPU      CPUMetrics
	Memory   MemoryMetrics
	IO       IOMetrics
	Network  NetworkMetrics
	Restarts int64
	UptimeNs int64
}

type CPUMetrics struct {
	Usage       float64
	Throttled   bool
	ThrottledNs int64
}

type MemoryMetrics struct {
	Current int64
	Peak    int64
	Limit   int64
	Swapped int64
}

type IOMetrics struct {
	ReadBytes  int64
	WriteBytes int64
	ReadOps    int64
	WriteOps   int64
}

type NetworkMetrics struct {
	RxBytes   int64
	TxBytes   int64
	RxPackets int64
	TxPackets int64
}

type MetricsExporter interface {
	Export(ctx context.Context, metrics *ServiceMetrics) error
	Close() error
}
