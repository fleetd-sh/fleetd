package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"fleetd.sh/internal/migrations"
)

func setupSQLiteMetrics(t *testing.T) (MetricsStorage, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	factory := &SQLiteMetricsFactory{}
	storage, err := factory.Create(MetricsStorageConfig{
		Type:    "sqlite",
		Options: map[string]any{"path": dbPath},
	})
	require.NoError(t, err)

	// Run migrations
	db := storage.(*SQLiteMetricsStorage).db
	version, dirty, err := migrations.MigrateUp(db)
	require.NoError(t, err)
	require.False(t, dirty)
	require.GreaterOrEqual(t, version, 1)

	// Add device registration before testing metrics
	result, err := db.Exec(
		"INSERT INTO device (id, name, type, version, api_key) VALUES (?, ?, ?, ?, ?)",
		"test-device", "Test Device", "test-type", "1.0.0", "test-api-key",
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return storage, cleanup
}

func TestSQLiteMetrics_Store(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Run migrations
	version, dirty, err := migrations.MigrateUp(db)
	require.NoError(t, err)
	require.False(t, dirty)
	require.GreaterOrEqual(t, version, 1)

	// Add device registration before testing metrics
	result, err := db.Exec(
		"INSERT INTO device (id, name, type, version, api_key) VALUES (?, ?, ?, ?, ?)",
		"test-device", "Test Device", "test-type", "1.0.0", "test-api-key",
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	storage := &SQLiteMetricsStorage{db: db}
	metric := MetricValue{
		DeviceID:  "test-device",
		Timestamp: time.Now(),
		Value:     42.0,
		Labels:    map[string]string{"test": "label"},
	}

	err = storage.Store(context.Background(), "test_metric", metric)
	require.NoError(t, err)
}

func TestSQLiteMetrics_StoreBatch(t *testing.T) {
	storage, cleanup := setupSQLiteMetrics(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	metrics := map[string][]MetricValue{
		"cpu": {
			{DeviceID: "test-device", Value: float64(50), Timestamp: now, Labels: map[string]string{"host": "host1"}},
			{DeviceID: "test-device", Value: float64(60), Timestamp: now.Add(time.Minute), Labels: map[string]string{"host": "host1"}},
		},
		"memory": {
			{DeviceID: "test-device", Value: float64(1024), Timestamp: now, Labels: map[string]string{"host": "host1"}},
			{DeviceID: "test-device", Value: float64(2048), Timestamp: now.Add(time.Minute), Labels: map[string]string{"host": "host1"}},
		},
	}

	err := storage.StoreBatch(ctx, metrics)
	require.NoError(t, err)

	// Query metrics with wider time range to ensure we catch the data
	result, err := storage.Query(ctx, MetricQuery{
		Names: []string{"cpu", "memory"},
		Filter: MetricFilter{
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(2 * time.Hour),
			DeviceID:  "test-device",
		},
	})
	require.NoError(t, err)
	require.Len(t, result, 2, "Expected results for both cpu and memory metrics")

	// Verify CPU metrics
	cpuSeries := result[0]
	assert.Equal(t, "cpu", cpuSeries.Name)
	assert.Len(t, cpuSeries.Values, 2, "Expected 2 CPU metric values")
	assert.Equal(t, float64(50), cpuSeries.Values[0].Value)
	assert.Equal(t, float64(60), cpuSeries.Values[1].Value)

	// Verify memory metrics
	memorySeries := result[1]
	assert.Equal(t, "memory", memorySeries.Name)
	assert.Len(t, memorySeries.Values, 2, "Expected 2 memory metric values")
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
			{DeviceID: "test-device", Value: float64(50), Timestamp: now, Labels: map[string]string{"host": "host1", "env": "prod"}},
			{DeviceID: "test-device", Value: float64(60), Timestamp: now.Add(time.Minute), Labels: map[string]string{"host": "host1", "env": "prod"}},
			{DeviceID: "test-device", Value: float64(40), Timestamp: now, Labels: map[string]string{"host": "host2", "env": "dev"}},
		},
	}

	err := storage.StoreBatch(ctx, metrics)
	require.NoError(t, err)

	// Add this after StoreBatch and before Query in TestSQLiteMetrics_Query
	rows, err := storage.(*SQLiteMetricsStorage).db.QueryContext(ctx,
		"SELECT device_id, name, value, timestamp, labels FROM metric")
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var deviceID, name, value, timestamp, labels string
		err := rows.Scan(&deviceID, &name, &value, &timestamp, &labels)
		require.NoError(t, err)
	}

	// Test querying with label filter
	result, err := storage.Query(ctx, MetricQuery{
		DeviceID: "test-device",
		Names:    []string{"cpu"},
		Filter: MetricFilter{
			DeviceID:  "test-device",
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(time.Hour),
			Labels:    map[string]string{"env": "prod"},
		},
	})

	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, result[0].Values, 2)
}

func TestSQLiteMetrics_Delete(t *testing.T) {
	storage, cleanup := setupSQLiteMetrics(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Store test data
	metrics := map[string][]MetricValue{
		"cpu": {
			{DeviceID: "test-device", Value: float64(50), Timestamp: now, Labels: map[string]string{"host": "host1", "env": "prod"}},
			{DeviceID: "test-device", Value: float64(60), Timestamp: now.Add(time.Hour), Labels: map[string]string{"host": "host1", "env": "prod"}},
			{DeviceID: "test-device", Value: float64(40), Timestamp: now, Labels: map[string]string{"host": "host2", "env": "dev"}},
		},
	}

	err := storage.StoreBatch(ctx, metrics)
	require.NoError(t, err)

	// Delete metrics with label filter
	err = storage.Delete(ctx, "cpu")
	require.NoError(t, err)

	// Verify deletion
	result, err := storage.Query(ctx, MetricQuery{
		DeviceID: "test-device",
		Names:    []string{"cpu"},
		Filter: MetricFilter{
			DeviceID:  "test-device",
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(2 * time.Hour),
		},
	})
	require.NoError(t, err)
	require.Len(t, result, 0)
}

func TestSQLiteMetrics_ListMetrics(t *testing.T) {
	storage, cleanup := setupSQLiteMetrics(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Store test data
	metrics := map[string][]MetricValue{
		"cpu":    {{DeviceID: "test-device", Value: float64(50), Timestamp: now}},
		"memory": {{DeviceID: "test-device", Value: float64(1024), Timestamp: now}},
		"disk":   {{DeviceID: "test-device", Value: float64(500), Timestamp: now}},
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

	// Store metric info using proper JSON array string
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

	// Test JSON array querying
	var count int
	err = storage.(*SQLiteMetricsStorage).db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM metric_info, json_each(labels)
		 WHERE name = 'cpu' AND json_each.value = 'host'`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Get non-existent metric info
	info, err = storage.GetMetricInfo(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, info)
}
