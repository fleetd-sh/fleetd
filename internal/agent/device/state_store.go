package device

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// StateStore provides persistent storage for agent state
type StateStore struct {
	db           *sql.DB
	mu           sync.RWMutex
	metricsCache []Metrics
	maxMetrics   int
}

// NewStateStore creates a new state store
func NewStateStore(dbPath string) (*StateStore, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &StateStore{
		db:         db,
		maxMetrics: 1000, // Keep last 1000 metrics in cache
	}

	if err := store.initialize(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return store, nil
}

// initialize creates the necessary database tables
func (s *StateStore) initialize() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS agent_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			state TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS metrics_buffer (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TIMESTAMP NOT NULL,
			metrics TEXT NOT NULL,
			sent BOOLEAN DEFAULT FALSE
		)`,
		`CREATE TABLE IF NOT EXISTS update_history (
			id TEXT PRIMARY KEY,
			version TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			error TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_sent ON metrics_buffer(sent)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics_buffer(timestamp)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	// Set pragmas for better performance and reliability
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=10000",
		"PRAGMA temp_store=MEMORY",
	}

	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			log.Printf("Warning: failed to set pragma %s: %v", pragma, err)
		}
	}

	return nil
}

// SaveState persists the current agent state
func (s *StateStore) SaveState(state *State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	query := `INSERT OR REPLACE INTO agent_state (id, state, updated_at) VALUES (1, ?, ?)`
	_, err = s.db.Exec(query, string(data), time.Now())
	if err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

// LoadState retrieves the saved agent state
func (s *StateStore) LoadState() (*State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stateData string
	query := `SELECT state FROM agent_state WHERE id = 1`
	err := s.db.QueryRow(query).Scan(&stateData)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No saved state
		}
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	var state State
	if err := json.Unmarshal([]byte(stateData), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &state, nil
}

// BufferMetrics stores metrics for later transmission
func (s *StateStore) BufferMetrics(metrics *Metrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	query := `INSERT INTO metrics_buffer (timestamp, metrics, sent) VALUES (?, ?, FALSE)`
	_, err = s.db.Exec(query, metrics.Timestamp, string(data))
	if err != nil {
		return fmt.Errorf("failed to buffer metrics: %w", err)
	}

	// Clean up old metrics
	cleanup := `DELETE FROM metrics_buffer
				WHERE id NOT IN (
					SELECT id FROM metrics_buffer
					ORDER BY timestamp DESC
					LIMIT ?
				)`
	s.db.Exec(cleanup, s.maxMetrics)

	return nil
}

// GetUnsentMetrics retrieves metrics that haven't been sent
func (s *StateStore) GetUnsentMetrics(limit int) ([]Metrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, metrics FROM metrics_buffer
			  WHERE sent = FALSE
			  ORDER BY timestamp ASC
			  LIMIT ?`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	var metrics []Metrics
	var ids []int64

	for rows.Next() {
		var id int64
		var metricsData string
		if err := rows.Scan(&id, &metricsData); err != nil {
			continue
		}

		var m Metrics
		if err := json.Unmarshal([]byte(metricsData), &m); err != nil {
			continue
		}

		metrics = append(metrics, m)
		ids = append(ids, id)
	}

	// Mark as sent
	if len(ids) > 0 {
		s.markMetricsSent(ids)
	}

	return metrics, nil
}

// markMetricsSent marks metrics as sent
func (s *StateStore) markMetricsSent(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE metrics_buffer SET sent = TRUE WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range ids {
		stmt.Exec(id)
	}

	return tx.Commit()
}

// SaveUpdateHistory records an update attempt
func (s *StateStore) SaveUpdateHistory(updateID, version, status string, startTime time.Time, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var completedAt *time.Time
	if status == "completed" || status == "failed" {
		now := time.Now()
		completedAt = &now
	}

	query := `INSERT OR REPLACE INTO update_history
			  (id, version, status, started_at, completed_at, error)
			  VALUES (?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query, updateID, version, status, startTime, completedAt, errorMsg)
	if err != nil {
		return fmt.Errorf("failed to save update history: %w", err)
	}

	return nil
}

// GetUpdateHistory retrieves recent update history
func (s *StateStore) GetUpdateHistory(limit int) ([]map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, version, status, started_at, completed_at, error
			  FROM update_history
			  ORDER BY started_at DESC
			  LIMIT ?`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query update history: %w", err)
	}
	defer rows.Close()

	var history []map[string]interface{}
	for rows.Next() {
		var id, version, status string
		var startedAt time.Time
		var completedAt sql.NullTime
		var errorMsg sql.NullString

		if err := rows.Scan(&id, &version, &status, &startedAt, &completedAt, &errorMsg); err != nil {
			continue
		}

		entry := map[string]interface{}{
			"id":         id,
			"version":    version,
			"status":     status,
			"started_at": startedAt,
		}

		if completedAt.Valid {
			entry["completed_at"] = completedAt.Time
		}
		if errorMsg.Valid {
			entry["error"] = errorMsg.String
		}

		history = append(history, entry)
	}

	return history, nil
}

// Close closes the database connection
func (s *StateStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Vacuum optimizes the database
func (s *StateStore) Vacuum() error {
	_, err := s.db.Exec("VACUUM")
	return err
}