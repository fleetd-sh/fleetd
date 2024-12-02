package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSQLiteMetrics(t *testing.T) (MetricsStorage, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	factory := &SQLiteMetricsFactory{}
	storage, err := factory.Create(MetricsStorageConfig{
		Type:    "sqlite",
		Options: map[string]interface{}{"path": dbPath},
	})
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return storage, cleanup
}

func TestSQLiteMetrics_Store(t *testing.T) {
	storage, cleanup := setupSQLiteMetrics(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Test storing numeric value
	err := storage.Store(ctx, "test_metric", MetricValue{
		Value:     float64(42),
		Timestamp: now,
		Labels:    map[string]string{"host": "test-host"},
	})
	require.NoError(t, err)

	// Test storing string value
	err = storage.Store(ctx, "test_metric_str", MetricValue{
		Value:     "test-value",
		Timestamp: now,
		Labels:    map[string]string{"type": "test"},
	})
	require.NoError(t, err)

	// Query metrics
	result, err := storage.Query(ctx, MetricQuery{
		Names: []string{"test_metric", "test_metric_str"},
		Filter: MetricFilter{
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(time.Hour),
		},
	})
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Verify numeric metric
	numericSeries := result[0]
	assert.Equal(t, "test_metric", numericSeries.Name)
	assert.Len(t, numericSeries.Values, 1)
	assert.Equal(t, float64(42), numericSeries.Values[0].Value)
	assert.Equal(t, now, numericSeries.Values[0].Timestamp)
	assert.Equal(t, "test-host", numericSeries.Values[0].Labels["host"])

	// Verify string metric
	stringSeries := result[1]
	assert.Equal(t, "test_metric_str", stringSeries.Name)
	assert.Len(t, stringSeries.Values, 1)
	assert.Equal(t, "test-value", stringSeries.Values[0].Value)
	assert.Equal(t, now, stringSeries.Values[0].Timestamp)
	assert.Equal(t, "test", stringSeries.Values[0].Labels["type"])
}

func TestSQLiteMetrics_StoreBatch(t *testing.T) {
	storage, cleanup := setupSQLiteMetrics(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	metrics := map[string][]MetricValue{
		"cpu": {
			{Value: float64(50), Timestamp: now, Labels: map[string]string{"host": "host1"}},
			{Value: float64(60), Timestamp: now.Add(time.Minute), Labels: map[string]string{"host": "host1"}},
		},
		"memory": {
			{Value: float64(1024), Timestamp: now, Labels: map[string]string{"host": "host1"}},
			{Value: float64(2048), Timestamp: now.Add(time.Minute), Labels: map[string]string{"host": "host1"}},
		},
	}

	err := storage.StoreBatch(ctx, metrics)
	require.NoError(t, err)

	// Query metrics
	result, err := storage.Query(ctx, MetricQuery{
		Names: []string{"cpu", "memory"},
		Filter: MetricFilter{
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(time.Hour),
		},
	})
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Verify CPU metrics
	cpuSeries := result[0]
	assert.Equal(t, "cpu", cpuSeries.Name)
	assert.Len(t, cpuSeries.Values, 2)
	assert.Equal(t, float64(50), cpuSeries.Values[0].Value)
	assert.Equal(t, float64(60), cpuSeries.Values[1].Value)

	// Verify memory metrics
	memorySeries := result[1]
	assert.Equal(t, "memory", memorySeries.Name)
	assert.Len(t, memorySeries.Values, 2)
	assert.Equal(t, float64(1024), memorySeries.Values[0].Value)
	assert.Equal(t, float64(2048), memorySeries.Values[1].Value)
}

func TestSQLiteMetrics_Query(t *testing.T) {
	storage, cleanup := setupSQLiteMetrics(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Store test data
	metrics := map[string][]MetricValue{
		"cpu": {
			{Value: float64(50), Timestamp: now, Labels: map[string]string{"host": "host1", "env": "prod"}},
			{Value: float64(60), Timestamp: now.Add(time.Minute), Labels: map[string]string{"host": "host1", "env": "prod"}},
			{Value: float64(40), Timestamp: now, Labels: map[string]string{"host": "host2", "env": "dev"}},
		},
	}

	err := storage.StoreBatch(ctx, metrics)
	require.NoError(t, err)

	// Test querying with label filter
	result, err := storage.Query(ctx, MetricQuery{
		Names: []string{"cpu"},
		Filter: MetricFilter{
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(time.Hour),
			Labels:    map[string]string{"env": "prod"},
		},
	})
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Len(t, result[0].Values, 2)

	// Test querying with aggregation
	result, err = storage.Query(ctx, MetricQuery{
		Names: []string{"cpu"},
		Filter: MetricFilter{
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(time.Hour),
		},
		Aggregation: AggregationAvg,
		Interval:    time.Hour,
	})
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Len(t, result[0].Values, 1)
	assert.InDelta(t, 50, result[0].Values[0].Value, 0.1) // (50 + 60 + 40) / 3
}

func TestSQLiteMetrics_Delete(t *testing.T) {
	storage, cleanup := setupSQLiteMetrics(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Store test data
	metrics := map[string][]MetricValue{
		"cpu": {
			{Value: float64(50), Timestamp: now, Labels: map[string]string{"host": "host1", "env": "prod"}},
			{Value: float64(60), Timestamp: now.Add(time.Hour), Labels: map[string]string{"host": "host1", "env": "prod"}},
			{Value: float64(40), Timestamp: now, Labels: map[string]string{"host": "host2", "env": "dev"}},
		},
	}

	err := storage.StoreBatch(ctx, metrics)
	require.NoError(t, err)

	// Delete metrics with label filter
	err = storage.Delete(ctx, MetricFilter{
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Labels:    map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	// Verify deletion
	result, err := storage.Query(ctx, MetricQuery{
		Names: []string{"cpu"},
		Filter: MetricFilter{
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(2 * time.Hour),
		},
	})
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Len(t, result[0].Values, 1)
	assert.Equal(t, float64(40), result[0].Values[0].Value)
}

func TestSQLiteMetrics_ListMetrics(t *testing.T) {
	storage, cleanup := setupSQLiteMetrics(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Store test data
	metrics := map[string][]MetricValue{
		"cpu":    {{Value: float64(50), Timestamp: now}},
		"memory": {{Value: float64(1024), Timestamp: now}},
		"disk":   {{Value: float64(500), Timestamp: now}},
	}

	err := storage.StoreBatch(ctx, metrics)
	require.NoError(t, err)

	// List metrics
	names, err := storage.ListMetrics(ctx)
	require.NoError(t, err)
	assert.Len(t, names, 3)
	assert.Contains(t, names, "cpu")
	assert.Contains(t, names, "memory")
	assert.Contains(t, names, "disk")
}

func TestSQLiteMetrics_GetMetricInfo(t *testing.T) {
	storage, cleanup := setupSQLiteMetrics(t)
	defer cleanup()

	ctx := context.Background()

	// Store metric info
	_, err := storage.(*SQLiteMetricsStorage).db.ExecContext(ctx,
		`INSERT INTO metric_info (name, description, type, unit, labels)
		 VALUES (?, ?, ?, ?, ?)`,
		"cpu", "CPU usage", string(MetricTypeGauge), "%", `["host", "env"]`)
	require.NoError(t, err)

	// Get metric info
	info, err := storage.GetMetricInfo(ctx, "cpu")
	require.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, "cpu", info.Name)
	assert.Equal(t, "CPU usage", info.Description)
	assert.Equal(t, MetricTypeGauge, info.Type)
	assert.Equal(t, "%", info.Unit)
	assert.Equal(t, []string{"host", "env"}, info.Labels)

	// Get non-existent metric info
	info, err = storage.GetMetricInfo(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, info)
}
