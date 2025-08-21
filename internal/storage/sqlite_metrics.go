package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteMetricsStorage implements MetricsStorage using SQLite
type SQLiteMetricsStorage struct {
	db *sql.DB
}

// SQLiteMetricsFactory creates SQLite metrics storage backends
type SQLiteMetricsFactory struct{}

func init() {
	RegisterMetricsStorageFactory("sqlite", &SQLiteMetricsFactory{})
}

// Create implements MetricsStorageFactory
func (f *SQLiteMetricsFactory) Create(config MetricsStorageConfig) (MetricsStorage, error) {
	dbPath, ok := config.Options["path"].(string)
	if !ok {
		return nil, fmt.Errorf("sqlite storage requires 'path' option")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &SQLiteMetricsStorage{db: db}, nil
}

// Store implements MetricsStorage
func (s *SQLiteMetricsStorage) Store(ctx context.Context, name string, value MetricValue) error {
	labels, err := json.Marshal(value.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	valueJSON, err := json.Marshal(value.Value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO metric (device_id, name, value, timestamp, labels)
		 VALUES (?, ?, ?, datetime(?), ?)`,
		value.DeviceID, name, valueJSON, value.Timestamp.UTC().Format(time.RFC3339), labels)
	if err != nil {
		return fmt.Errorf("failed to store metric: %w", err)
	}

	return nil
}

// StoreBatch implements MetricsStorage
func (s *SQLiteMetricsStorage) StoreBatch(ctx context.Context, metrics map[string][]MetricValue) error {
	if len(metrics) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO metric (device_id, name, value, timestamp, labels)
		 VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for name, values := range metrics {
		for _, value := range values {
			labels, err := json.Marshal(value.Labels)
			if err != nil {
				return fmt.Errorf("failed to marshal labels: %w", err)
			}

			valueJSON, err := json.Marshal(value.Value)
			if err != nil {
				return fmt.Errorf("failed to marshal value: %w", err)
			}

			_, err = stmt.ExecContext(ctx,
				value.DeviceID,
				name,
				valueJSON,
				value.Timestamp.UTC().Format(time.RFC3339),
				labels)
			if err != nil {
				return fmt.Errorf("failed to store metric batch: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Query implements MetricsStorage
func (s *SQLiteMetricsStorage) Query(ctx context.Context, query MetricQuery) ([]MetricSeries, error) {
	placeholders := make([]string, len(query.Names))
	args := make([]any, 0, len(query.Names)+3)

	// Add metric names to args first
	for i := range query.Names {
		placeholders[i] = "?"
		args = append(args, query.Names[i])
	}

	deviceID := query.DeviceID
	if deviceID == "" {
		deviceID = query.Filter.DeviceID
	}

	// Add device ID and timestamps to args
	args = append(args, deviceID)
	args = append(args, query.Filter.StartTime.UTC().Format(time.RFC3339))
	args = append(args, query.Filter.EndTime.UTC().Format(time.RFC3339))

	// Base query
	query_str := fmt.Sprintf(`
		SELECT name,
			   strftime('%%Y-%%m-%%dT%%H:%%M:%%SZ', timestamp) as timestamp,
			   value,
			   labels,
			   device_id
		FROM metric
		WHERE name IN (%s)
		AND device_id = ?
		AND timestamp BETWEEN ? AND ?`,
		strings.Join(placeholders, ","))

	// Add label filters if present
	if len(query.Filter.Labels) > 0 {
		for key, value := range query.Filter.Labels {
			query_str += fmt.Sprintf(` AND json_extract(labels, '$.%s') = ?`, key)
			args = append(args, value)
		}
	}

	query_str += " ORDER BY name ASC, timestamp ASC"

	rows, err := s.db.QueryContext(ctx, query_str, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	// Initialize series for all requested metric names
	seriesByName := make(map[string]*MetricSeries, len(query.Names))
	for _, name := range query.Names {
		seriesByName[name] = &MetricSeries{
			Name:   name,
			Values: make([]MetricValue, 0),
		}
	}

	// Scan rows into series
	hasResults := false
	for rows.Next() {
		hasResults = true
		var (
			name       string
			timestamp  string
			valueJSON  string
			labelsJSON string
			deviceID   string
		)

		if err := rows.Scan(&name, &timestamp, &valueJSON, &labelsJSON, &deviceID); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		ts, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}

		var value float64
		if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
			return nil, fmt.Errorf("failed to unmarshal value: %w", err)
		}

		var labels map[string]string
		if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
		}

		seriesByName[name].Values = append(seriesByName[name].Values, MetricValue{
			Value:     value,
			Timestamp: ts,
			Labels:    labels,
			DeviceID:  deviceID,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	if !hasResults {
		return nil, nil
	}

	// Convert map to slice, preserving order of requested names
	result := make([]MetricSeries, 0, len(query.Names))
	for _, name := range query.Names {
		if series, ok := seriesByName[name]; ok && len(series.Values) > 0 {
			result = append(result, *series)
		}
	}

	return result, nil
}

func matchLabels(labels, filter map[string]string) bool {
	for k, v := range filter {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// ListMetrics implements MetricsStorage
func (s *SQLiteMetricsStorage) ListMetrics(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT name FROM metric GROUP BY device_id, name")
	if err != nil {
		return nil, fmt.Errorf("failed to list metrics: %w", err)
	}
	defer rows.Close()

	var metrics []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan metric name: %w", err)
		}
		metrics = append(metrics, name)
	}
	return metrics, nil
}

// GetMetricInfo implements MetricsStorage
func (s *SQLiteMetricsStorage) GetMetricInfo(ctx context.Context, name string) (*MetricInfo, error) {
	var info MetricInfo
	var labelsJSON string
	err := s.db.QueryRowContext(ctx,
		"SELECT name, description, type, unit, labels FROM metric_info WHERE name = ?",
		name).Scan(&info.Name, &info.Description, &info.Type, &info.Unit, &labelsJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get metric info: %w", err)
	}

	if err := json.Unmarshal([]byte(labelsJSON), &info.Labels); err != nil {
		return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
	}
	return &info, nil
}

// Delete implements MetricsStorage
func (s *SQLiteMetricsStorage) Delete(ctx context.Context, name string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM metric WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("failed to delete metrics: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("no metrics deleted")
	}
	return nil
}
