package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	once    sync.Once
	initErr error

	sqlMeter     metric.Meter
	influxMeter  metric.Meter
	storageMeter metric.Meter

	sqlDuration metric.Float64Histogram
	sqlErrors   metric.Int64Counter

	influxDuration metric.Float64Histogram
	influxErrors   metric.Int64Counter

	diskDuration metric.Float64Histogram
	diskErrors   metric.Int64Counter
)

// Initialize sets up telemetry metrics. Must be called before using any metrics.
func Initialize() error {
	once.Do(func() {
		initErr = initializeMetrics()
	})
	return initErr
}

func initializeMetrics() error {
	sqlMeter = otel.GetMeterProvider().Meter("fleetd/sqlite")
	influxMeter = otel.GetMeterProvider().Meter("fleetd/influx")
	storageMeter = otel.GetMeterProvider().Meter("fleetd/disk")

	var err error
	sqlDuration, err = sqlMeter.Float64Histogram("sqlite_query_duration_seconds")
	if err != nil {
		return fmt.Errorf("failed to create SQL duration histogram: %w", err)
	}

	sqlErrors, err = sqlMeter.Int64Counter("sqlite_error_total")
	if err != nil {
		return fmt.Errorf("failed to create SQL error counter: %w", err)
	}

	influxDuration, err = influxMeter.Float64Histogram("influx_query_duration_seconds")
	if err != nil {
		return fmt.Errorf("failed to create InfluxDB duration histogram: %w", err)
	}

	influxErrors, err = influxMeter.Int64Counter("influx_error_total")
	if err != nil {
		return fmt.Errorf("failed to create InfluxDB error counter: %w", err)
	}

	diskDuration, err = storageMeter.Float64Histogram("disk_operation_duration_seconds")
	if err != nil {
		return fmt.Errorf("failed to create disk duration histogram: %w", err)
	}

	diskErrors, err = storageMeter.Int64Counter("disk_error_total")
	if err != nil {
		return fmt.Errorf("failed to create disk error counter: %w", err)
	}

	return nil
}

// TrackSQLOperation is a helper that can wrap any SQL operation
func TrackSQLOperation(ctx context.Context, operation string, fn func() error) error {
	if sqlDuration == nil || sqlErrors == nil {
		slog.Debug("telemetry not initialized, skipping metrics")
		return fn()
	}

	start := time.Now()
	err := fn()
	duration := time.Since(start).Seconds()

	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
		attribute.Bool("error", err != nil),
	}

	sqlDuration.Record(ctx, duration, metric.WithAttributes(attrs...))

	if err != nil {
		sqlErrors.Add(ctx, 1, metric.WithAttributes(attrs...))
	}

	return err
}

// TrackInfluxOperation is a helper that can wrap any InfluxDB operation
func TrackInfluxOperation(ctx context.Context, operation string, fn func() error) error {
	if influxDuration == nil || influxErrors == nil {
		slog.Debug("telemetry not initialized, skipping metrics")
		return fn()
	}

	start := time.Now()
	err := fn()
	duration := time.Since(start).Seconds()

	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
		attribute.Bool("error", err != nil),
	}

	influxDuration.Record(ctx, duration, metric.WithAttributes(attrs...))

	if err != nil {
		influxErrors.Add(ctx, 1, metric.WithAttributes(attrs...))
	}

	return err
}

// TrackDiskOperation is a helper that can wrap any disk operation
func TrackDiskOperation(ctx context.Context, operation string, fn func() error) error {
	if diskDuration == nil || diskErrors == nil {
		slog.Debug("telemetry not initialized, skipping metrics")
		return fn()
	}

	start := time.Now()
	err := fn()
	duration := time.Since(start).Seconds()

	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
		attribute.Bool("error", err != nil),
	}

	diskDuration.Record(ctx, duration, metric.WithAttributes(attrs...))

	if err != nil {
		diskErrors.Add(ctx, 1, metric.WithAttributes(attrs...))
	}

	return err
}
