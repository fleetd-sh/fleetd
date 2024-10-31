package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	sqlMeter     = otel.GetMeterProvider().Meter("fleetd/sqlite")
	influxMeter  = otel.GetMeterProvider().Meter("fleetd/influx")
	storageMeter = otel.GetMeterProvider().Meter("fleetd/disk")

	sqlDuration metric.Float64Histogram
	sqlErrors   metric.Int64Counter

	influxDuration metric.Float64Histogram
	influxErrors   metric.Int64Counter

	diskDuration metric.Float64Histogram
	diskErrors   metric.Int64Counter
)

func init() {
	var err error
	sqlDuration, err = sqlMeter.Float64Histogram("sqlite_query_duration_seconds")
	if err != nil {
		panic(err)
	}

	sqlErrors, err = sqlMeter.Int64Counter("sqlite_error_total")
	if err != nil {
		panic(err)
	}

	influxDuration, err = influxMeter.Float64Histogram("influx_query_duration_seconds")
	if err != nil {
		panic(err)
	}

	influxErrors, err = influxMeter.Int64Counter("influx_error_total")
	if err != nil {
		panic(err)
	}

	diskDuration, err = storageMeter.Float64Histogram("disk_operation_duration_seconds")
	if err != nil {
		panic(err)
	}

	diskErrors, err = storageMeter.Int64Counter("disk_error_total")
	if err != nil {
		panic(err)
	}
}

// TrackSQLOperation is a helper that can wrap any SQL operation
func TrackSQLOperation(ctx context.Context, op string) func(error) {
	start := time.Now()
	return func(err error) {
		duration := time.Since(start).Seconds()

		opAttr := metric.WithAttributes(attribute.String("operation", op))
		sqlDuration.Record(ctx, duration, opAttr)
		if err != nil {
			sqlErrors.Add(ctx, 1, opAttr)
		}
	}
}

func TrackInfluxOperation(ctx context.Context, op string) func(error) {
	start := time.Now()
	return func(err error) {
		duration := time.Since(start).Seconds()

		opAttr := metric.WithAttributes(attribute.String("operation", op))
		influxDuration.Record(ctx, duration, opAttr)
		if err != nil {
			influxErrors.Add(ctx, 1, opAttr)
		}
	}
}

func TrackDiskOperation(ctx context.Context, op string) func(error) {
	start := time.Now()
	return func(err error) {
		duration := time.Since(start).Seconds()

		opAttr := metric.WithAttributes(attribute.String("operation", op))
		diskDuration.Record(ctx, duration, opAttr)
		if err != nil {
			diskErrors.Add(ctx, 1, opAttr)
		}
	}
}
