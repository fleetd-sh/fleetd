package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
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

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// Create tables if they don't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS metrics (
			name TEXT NOT NULL,
			value_numeric REAL,
			value_text TEXT,
			timestamp TIMESTAMP NOT NULL,
			labels TEXT NOT NULL DEFAULT '{}'
		);

		CREATE TABLE IF NOT EXISTS metric_info (
			name TEXT PRIMARY KEY,
			description TEXT NOT NULL,
			type TEXT NOT NULL,
			unit TEXT NOT NULL,
			labels TEXT NOT NULL DEFAULT '[]'
		);

		CREATE INDEX IF NOT EXISTS idx_metrics_name ON metrics(name);
		CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp);
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %v", err)
	}

	return &SQLiteMetricsStorage{db: db}, nil
}

// Store implements MetricsStorage
func (s *SQLiteMetricsStorage) Store(ctx context.Context, name string, value MetricValue) error {
	labels, err := json.Marshal(value.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %v", err)
	}

	var valueNumeric sql.NullFloat64
	var valueText sql.NullString

	switch v := value.Value.(type) {
	case float64:
		valueNumeric = sql.NullFloat64{Float64: v, Valid: true}
	case int:
		valueNumeric = sql.NullFloat64{Float64: float64(v), Valid: true}
	case int64:
		valueNumeric = sql.NullFloat64{Float64: float64(v), Valid: true}
	case string:
		valueText = sql.NullString{String: v, Valid: true}
	default:
		return fmt.Errorf("unsupported value type: %T", value.Value)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO metrics (name, value_numeric, value_text, timestamp, labels)
		 VALUES (?, ?, ?, ?, ?)`,
		name, valueNumeric, valueText, value.Timestamp, string(labels))
	if err != nil {
		return fmt.Errorf("failed to store metric: %v", err)
	}

	return nil
}

// StoreBatch implements MetricsStorage
func (s *SQLiteMetricsStorage) StoreBatch(ctx context.Context, metrics map[string][]MetricValue) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO metrics (name, value_numeric, value_text, timestamp, labels)
		 VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	for name, values := range metrics {
		for _, value := range values {
			labels, err := json.Marshal(value.Labels)
			if err != nil {
				return fmt.Errorf("failed to marshal labels: %v", err)
			}

			var valueNumeric sql.NullFloat64
			var valueText sql.NullString

			switch v := value.Value.(type) {
			case float64:
				valueNumeric = sql.NullFloat64{Float64: v, Valid: true}
			case int:
				valueNumeric = sql.NullFloat64{Float64: float64(v), Valid: true}
			case int64:
				valueNumeric = sql.NullFloat64{Float64: float64(v), Valid: true}
			case string:
				valueText = sql.NullString{String: v, Valid: true}
			default:
				return fmt.Errorf("unsupported value type: %T", value.Value)
			}

			_, err = stmt.ExecContext(ctx, name, valueNumeric, valueText, value.Timestamp, string(labels))
			if err != nil {
				return fmt.Errorf("failed to store metric: %v", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

// Query implements MetricsStorage
func (s *SQLiteMetricsStorage) Query(ctx context.Context, query MetricQuery) ([]MetricSeries, error) {
	baseQuery := `SELECT name, value_numeric, value_text, timestamp, labels
				  FROM metrics
				  WHERE timestamp BETWEEN ? AND ?`
	args := []interface{}{query.Filter.StartTime, query.Filter.EndTime}

	if len(query.Names) > 0 {
		placeholders := make([]string, len(query.Names))
		for i := range placeholders {
			placeholders[i] = "?"
			args = append(args, query.Names[i])
		}
		baseQuery += fmt.Sprintf(" AND name IN (%s)", strings.Join(placeholders, ","))
	}

	if len(query.Filter.Labels) > 0 {
		// SQLite doesn't have good JSON support, so we'll filter labels in memory
		baseQuery += " AND labels != '{}'"
	}

	baseQuery += " ORDER BY timestamp ASC"

	rows, err := s.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %v", err)
	}
	defer rows.Close()

	// Group metrics by name
	seriesByName := make(map[string]*MetricSeries)
	for rows.Next() {
		var (
			name        string
			valueNum    sql.NullFloat64
			valueText   sql.NullString
			timestamp   time.Time
			labelsJSON  string
			metricValue MetricValue
		)

		if err := rows.Scan(&name, &valueNum, &valueText, &timestamp, &labelsJSON); err != nil {
			return nil, fmt.Errorf("failed to scan metric: %v", err)
		}

		if err := json.Unmarshal([]byte(labelsJSON), &metricValue.Labels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal labels: %v", err)
		}

		// Filter by labels if specified
		if len(query.Filter.Labels) > 0 {
			match := true
			for k, v := range query.Filter.Labels {
				if metricValue.Labels[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		metricValue.Timestamp = timestamp
		if valueNum.Valid {
			metricValue.Value = valueNum.Float64
		} else if valueText.Valid {
			metricValue.Value = valueText.String
		}

		series, ok := seriesByName[name]
		if !ok {
			series = &MetricSeries{
				Name:   name,
				Values: make([]MetricValue, 0),
			}
			seriesByName[name] = series
		}
		series.Values = append(series.Values, metricValue)
	}

	// Convert map to slice
	result := make([]MetricSeries, 0, len(seriesByName))
	for _, series := range seriesByName {
		// Apply aggregation if specified
		if query.Aggregation != AggregationNone && query.Interval > 0 {
			series.Values = aggregateValues(series.Values, query.Aggregation, query.Interval)
		}
		result = append(result, *series)
	}

	return result, nil
}

// Delete implements MetricsStorage
func (s *SQLiteMetricsStorage) Delete(ctx context.Context, filter MetricFilter) error {
	query := "DELETE FROM metrics WHERE timestamp BETWEEN ? AND ?"
	args := []interface{}{filter.StartTime, filter.EndTime}

	if len(filter.Labels) > 0 {
		// SQLite doesn't have good JSON support, so we'll need to scan labels
		rows, err := s.db.QueryContext(ctx,
			"SELECT ROWID, labels FROM metrics WHERE timestamp BETWEEN ? AND ?",
			filter.StartTime, filter.EndTime)
		if err != nil {
			return fmt.Errorf("failed to query metrics: %v", err)
		}
		defer rows.Close()

		var rowsToDelete []int64
		for rows.Next() {
			var (
				rowid      int64
				labelsJSON string
				labels     map[string]string
			)

			if err := rows.Scan(&rowid, &labelsJSON); err != nil {
				return fmt.Errorf("failed to scan metric: %v", err)
			}

			if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
				return fmt.Errorf("failed to unmarshal labels: %v", err)
			}

			match := true
			for k, v := range filter.Labels {
				if labels[k] != v {
					match = false
					break
				}
			}
			if match {
				rowsToDelete = append(rowsToDelete, rowid)
			}
		}

		if len(rowsToDelete) > 0 {
			placeholders := make([]string, len(rowsToDelete))
			for i := range placeholders {
				placeholders[i] = "?"
				args = append(args, rowsToDelete[i])
			}
			query += fmt.Sprintf(" AND ROWID IN (%s)", strings.Join(placeholders, ","))
		}
	}

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete metrics: %v", err)
	}

	return nil
}

// ListMetrics implements MetricsStorage
func (s *SQLiteMetricsStorage) ListMetrics(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT name FROM metrics")
	if err != nil {
		return nil, fmt.Errorf("failed to list metrics: %v", err)
	}
	defer rows.Close()

	var metrics []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan metric name: %v", err)
		}
		metrics = append(metrics, name)
	}

	return metrics, nil
}

// GetMetricInfo implements MetricsStorage
func (s *SQLiteMetricsStorage) GetMetricInfo(ctx context.Context, name string) (*MetricInfo, error) {
	var (
		info       MetricInfo
		labelsJSON string
	)

	err := s.db.QueryRowContext(ctx,
		"SELECT name, description, type, unit, labels FROM metric_info WHERE name = ?",
		name).Scan(&info.Name, &info.Description, &info.Type, &info.Unit, &labelsJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get metric info: %v", err)
	}

	if err := json.Unmarshal([]byte(labelsJSON), &info.Labels); err != nil {
		return nil, fmt.Errorf("failed to unmarshal labels: %v", err)
	}

	return &info, nil
}

func aggregateValues(values []MetricValue, aggregation MetricAggregation, interval time.Duration) []MetricValue {
	if len(values) == 0 {
		return values
	}

	// Group values by time bucket
	buckets := make(map[time.Time][]float64)
	for _, v := range values {
		bucket := v.Timestamp.Truncate(interval)
		if n, ok := v.Value.(float64); ok {
			buckets[bucket] = append(buckets[bucket], n)
		}
	}

	// Aggregate values in each bucket
	result := make([]MetricValue, 0, len(buckets))
	for t, nums := range buckets {
		if len(nums) == 0 {
			continue
		}

		var value float64
		switch aggregation {
		case AggregationSum:
			for _, n := range nums {
				value += n
			}
		case AggregationAvg:
			for _, n := range nums {
				value += n
			}
			value /= float64(len(nums))
		case AggregationMin:
			value = nums[0]
			for _, n := range nums[1:] {
				if n < value {
					value = n
				}
			}
		case AggregationMax:
			value = nums[0]
			for _, n := range nums[1:] {
				if n > value {
					value = n
				}
			}
		case AggregationCount:
			value = float64(len(nums))
		}

		result = append(result, MetricValue{
			Value:     value,
			Timestamp: t,
			Labels:    values[0].Labels, // Use labels from first value
		})
	}

	return result
}
