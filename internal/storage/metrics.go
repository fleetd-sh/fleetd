package storage

import (
	"context"
	"time"
)

// MetricValue represents a single metric value
type MetricValue struct {
	DeviceID  string
	Value     any
	Timestamp time.Time
	Labels    map[string]string
}

// MetricSeries represents a series of metric values
type MetricSeries struct {
	Name   string
	Values []MetricValue
}

// MetricFilter represents filtering options for querying metrics
type MetricFilter struct {
	DeviceID  string
	StartTime time.Time
	EndTime   time.Time
	Labels    map[string]string
}

// MetricQuery represents a query for metrics
type MetricQuery struct {
	DeviceID string
	Names    []string
	Filter   MetricFilter
	Interval time.Duration // For time-based aggregation
}

// MetricsStorage defines the interface for metrics storage backends
type MetricsStorage interface {
	// Store stores a metric value
	Store(ctx context.Context, name string, value MetricValue) error

	// StoreBatch stores multiple metric values in a batch
	StoreBatch(ctx context.Context, metrics map[string][]MetricValue) error

	// Query retrieves metrics based on the provided query
	Query(ctx context.Context, query MetricQuery) ([]MetricSeries, error)

	// Delete deletes metrics matching the filter
	Delete(ctx context.Context, name string) error

	// ListMetrics lists available metric names
	ListMetrics(ctx context.Context) ([]string, error)

	// GetMetricInfo gets information about a metric
	GetMetricInfo(ctx context.Context, name string) (*MetricInfo, error)
}

// MetricInfo represents information about a metric
type MetricInfo struct {
	Name        string
	Description string
	Type        MetricType
	Unit        string
	Labels      []string
}

// MetricType represents the type of a metric
type MetricType string

const (
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeCounter   MetricType = "counter"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// MetricsStorageConfig represents configuration for a metrics storage backend
type MetricsStorageConfig struct {
	Type    string         // Type of storage backend
	Options map[string]any // Backend-specific options
}

// MetricsStorageFactory creates a new metrics storage backend
type MetricsStorageFactory interface {
	// Create creates a new metrics storage backend with the given configuration
	Create(config MetricsStorageConfig) (MetricsStorage, error)
}

// RegisterMetricsStorageFactory registers a metrics storage factory
func RegisterMetricsStorageFactory(name string, factory MetricsStorageFactory) {
	metricsStorageFactories[name] = factory
}

// GetMetricsStorageFactory gets a registered metrics storage factory
func GetMetricsStorageFactory(name string) (MetricsStorageFactory, bool) {
	factory, ok := metricsStorageFactories[name]
	return factory, ok
}

var metricsStorageFactories = make(map[string]MetricsStorageFactory)
