package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DeviceStorage defines the interface for on-device storage
type DeviceStorage interface {
	StoreMetric(metric Metric) error
	StoreBatch(metrics []Metric) error
	GetUnsynced(limit int) ([]Metric, error)
	MarkSynced(ids []int64) error
	GetStorageInfo() StorageInfo
	Close() error
}

// Metric represents a single metric data point
type Metric struct {
	ID        int64             `json:"id,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
	Synced    bool              `json:"synced,omitempty"`
}

// StorageInfo provides information about storage usage
type StorageInfo struct {
	TotalMetrics    int64
	UnsyncedMetrics int64
	OldestMetric    time.Time
	NewestMetric    time.Time
	StorageBytes    int64
	RetentionHours  int
}

// SQLiteStorage implements DeviceStorage using SQLite
type SQLiteStorage struct {
	db            *sql.DB
	mu            sync.RWMutex
	maxSize       int64
	retention     time.Duration
	maxMetrics    int
	flushInterval time.Duration
	buffer        []Metric
	bufferMu      sync.Mutex
}

// SQLiteOption configures SQLiteStorage
type SQLiteOption func(*SQLiteStorage)

// WithRetention sets the retention period
func WithRetention(d time.Duration) SQLiteOption {
	return func(s *SQLiteStorage) {
		s.retention = d
	}
}

// WithMaxSize sets the maximum database size
func WithMaxSize(bytes int64) SQLiteOption {
	return func(s *SQLiteStorage) {
		s.maxSize = bytes
	}
}

// WithMaxMetrics sets the maximum number of metrics to store
func WithMaxMetrics(n int) SQLiteOption {
	return func(s *SQLiteStorage) {
		s.maxMetrics = n
	}
}

// NewSQLiteStorage creates a new SQLite-based storage
func NewSQLiteStorage(dbPath string, opts ...SQLiteOption) (*SQLiteStorage, error) {
	s := &SQLiteStorage{
		maxSize:       100 * 1024 * 1024,  // 100MB default
		retention:     7 * 24 * time.Hour, // 7 days default
		maxMetrics:    100000,             // 100k metrics default
		flushInterval: 5 * time.Second,
		buffer:        make([]Metric, 0, 1000),
	}

	for _, opt := range opts {
		opt(s)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	s.db = db

	if err := s.initialize(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Start background workers
	go s.retentionWorker()
	go s.flushWorker()

	return s, nil
}

func (s *SQLiteStorage) initialize() error {
	schema := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		name TEXT NOT NULL,
		value REAL NOT NULL,
		labels TEXT,
		synced INTEGER DEFAULT 0,
		created_at INTEGER DEFAULT (strftime('%s', 'now'))
	);

	CREATE INDEX IF NOT EXISTS idx_metrics_synced ON metrics(synced);
	CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp);
	CREATE INDEX IF NOT EXISTS idx_metrics_created ON metrics(created_at);

	-- Ring buffer trigger to limit metrics count
	CREATE TRIGGER IF NOT EXISTS limit_metrics
	AFTER INSERT ON metrics
	WHEN (SELECT COUNT(*) FROM metrics) > %d
	BEGIN
		DELETE FROM metrics
		WHERE id IN (
			SELECT id FROM metrics
			ORDER BY id ASC
			LIMIT (SELECT COUNT(*) - %d FROM metrics)
		);
	END;

	-- Sync queue for failed uploads
	CREATE TABLE IF NOT EXISTS sync_queue (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		batch_data BLOB NOT NULL,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		retry_count INTEGER DEFAULT 0,
		next_retry INTEGER
	);

	-- Storage metadata
	CREATE TABLE IF NOT EXISTS storage_meta (
		key TEXT PRIMARY KEY,
		value TEXT
	);
	`

	_, err := s.db.Exec(fmt.Sprintf(schema, s.maxMetrics, s.maxMetrics))
	return err
}

// StoreMetric stores a single metric
func (s *SQLiteStorage) StoreMetric(metric Metric) error {
	s.bufferMu.Lock()
	s.buffer = append(s.buffer, metric)
	shouldFlush := len(s.buffer) >= 1000
	s.bufferMu.Unlock()

	if shouldFlush {
		return s.flush()
	}
	return nil
}

// StoreBatch stores multiple metrics at once
func (s *SQLiteStorage) StoreBatch(metrics []Metric) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO metrics (timestamp, name, value, labels, synced)
		VALUES (?, ?, ?, ?, 0)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range metrics {
		labels, _ := json.Marshal(m.Labels)
		_, err = stmt.Exec(
			m.Timestamp.Unix(),
			m.Name,
			m.Value,
			string(labels),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetUnsynced retrieves unsynced metrics
func (s *SQLiteStorage) GetUnsynced(limit int) ([]Metric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, timestamp, name, value, labels
		FROM metrics
		WHERE synced = 0
		ORDER BY id ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []Metric
	for rows.Next() {
		var m Metric
		var labelsJSON sql.NullString

		err := rows.Scan(&m.ID, &m.Timestamp, &m.Name, &m.Value, &labelsJSON)
		if err != nil {
			return nil, err
		}

		if labelsJSON.Valid {
			json.Unmarshal([]byte(labelsJSON.String), &m.Labels)
		}

		metrics = append(metrics, m)
	}

	return metrics, rows.Err()
}

// MarkSynced marks metrics as synchronized
func (s *SQLiteStorage) MarkSynced(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE metrics SET synced = 1 WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range ids {
		_, err = stmt.Exec(id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetStorageInfo returns storage statistics
func (s *SQLiteStorage) GetStorageInfo() StorageInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info := StorageInfo{
		RetentionHours: int(s.retention.Hours()),
	}

	// Get counts
	s.db.QueryRow("SELECT COUNT(*) FROM metrics").Scan(&info.TotalMetrics)
	s.db.QueryRow("SELECT COUNT(*) FROM metrics WHERE synced = 0").Scan(&info.UnsyncedMetrics)

	// Get date range
	var oldest, newest sql.NullInt64
	s.db.QueryRow("SELECT MIN(timestamp), MAX(timestamp) FROM metrics").Scan(&oldest, &newest)

	if oldest.Valid {
		info.OldestMetric = time.Unix(oldest.Int64, 0)
	}
	if newest.Valid {
		info.NewestMetric = time.Unix(newest.Int64, 0)
	}

	// Get database size
	var pageCount, pageSize int64
	s.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	s.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	info.StorageBytes = pageCount * pageSize

	return info
}

// flush writes buffered metrics to database
func (s *SQLiteStorage) flush() error {
	s.bufferMu.Lock()
	if len(s.buffer) == 0 {
		s.bufferMu.Unlock()
		return nil
	}

	toFlush := s.buffer
	s.buffer = make([]Metric, 0, 1000)
	s.bufferMu.Unlock()

	return s.StoreBatch(toFlush)
}

// flushWorker periodically flushes the buffer
func (s *SQLiteStorage) flushWorker() {
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.flush()
	}
}

// retentionWorker removes old metrics
func (s *SQLiteStorage) retentionWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-s.retention).Unix()
		s.mu.Lock()
		s.db.Exec("DELETE FROM metrics WHERE timestamp < ? AND synced = 1", cutoff)
		s.mu.Unlock()
	}
}

// Close closes the database connection
func (s *SQLiteStorage) Close() error {
	s.flush()
	return s.db.Close()
}

// MemoryStorage implements in-memory storage for constrained devices
type MemoryStorage struct {
	metrics []Metric
	mu      sync.RWMutex
	maxSize int
}

// NewMemoryStorage creates a memory-only storage
func NewMemoryStorage(maxSize int) *MemoryStorage {
	return &MemoryStorage{
		metrics: make([]Metric, 0, maxSize),
		maxSize: maxSize,
	}
}

// StoreMetric stores a metric in memory
func (m *MemoryStorage) StoreMetric(metric Metric) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = append(m.metrics, metric)

	// Ring buffer behavior
	if len(m.metrics) > m.maxSize {
		copy(m.metrics, m.metrics[1:])
		m.metrics = m.metrics[:len(m.metrics)-1]
	}

	return nil
}

// StoreBatch stores multiple metrics
func (m *MemoryStorage) StoreBatch(metrics []Metric) error {
	for _, metric := range metrics {
		if err := m.StoreMetric(metric); err != nil {
			return err
		}
	}
	return nil
}

// GetUnsynced returns all metrics (memory storage doesn't track sync state)
func (m *MemoryStorage) GetUnsynced(limit int) ([]Metric, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit > len(m.metrics) {
		limit = len(m.metrics)
	}

	result := make([]Metric, limit)
	copy(result, m.metrics[:limit])
	return result, nil
}

// MarkSynced clears synced metrics from memory
func (m *MemoryStorage) MarkSynced(ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// For memory storage, just clear the synced metrics
	if len(ids) > 0 {
		m.metrics = m.metrics[len(ids):]
	}

	return nil
}

// GetStorageInfo returns storage statistics
func (m *MemoryStorage) GetStorageInfo() StorageInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info := StorageInfo{
		TotalMetrics:    int64(len(m.metrics)),
		UnsyncedMetrics: int64(len(m.metrics)),
	}

	if len(m.metrics) > 0 {
		info.OldestMetric = m.metrics[0].Timestamp
		info.NewestMetric = m.metrics[len(m.metrics)-1].Timestamp
	}

	return info
}

// Close is a no-op for memory storage
func (m *MemoryStorage) Close() error {
	return nil
}
