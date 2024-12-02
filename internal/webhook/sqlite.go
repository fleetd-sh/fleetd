package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SQLiteWebhookManager implements WebhookManager using SQLite
type SQLiteWebhookManager struct {
	db     *sql.DB
	sender WebhookSender
}

// NewSQLiteWebhookManager creates a new SQLiteWebhookManager
func NewSQLiteWebhookManager(db *sql.DB, sender WebhookSender) (*SQLiteWebhookManager, error) {
	if sender == nil {
		sender = NewDefaultWebhookSender(nil)
	}

	// Create tables if they don't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			secret TEXT NOT NULL,
			events TEXT NOT NULL,
			headers TEXT NOT NULL DEFAULT '{}',
			retry_config TEXT NOT NULL DEFAULT '{}',
			max_parallel INTEGER NOT NULL DEFAULT 1,
			timeout INTEGER NOT NULL DEFAULT 30000,
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS webhook_deliveries (
			id TEXT PRIMARY KEY,
			webhook_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			url TEXT NOT NULL,
			status INTEGER,
			request TEXT NOT NULL,
			response TEXT,
			error TEXT,
			duration REAL NOT NULL,
			timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			retry_count INTEGER NOT NULL DEFAULT 0,
			next_retry_at TIMESTAMP,
			FOREIGN KEY (webhook_id) REFERENCES webhooks(id)
		);

		CREATE INDEX IF NOT EXISTS idx_webhooks_enabled ON webhooks(enabled);
		CREATE INDEX IF NOT EXISTS idx_deliveries_webhook ON webhook_deliveries(webhook_id);
		CREATE INDEX IF NOT EXISTS idx_deliveries_timestamp ON webhook_deliveries(timestamp);
		CREATE INDEX IF NOT EXISTS idx_deliveries_retry ON webhook_deliveries(next_retry_at);
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create tables: %v", err)
	}

	return &SQLiteWebhookManager{
		db:     db,
		sender: sender,
	}, nil
}

// Subscribe implements WebhookManager
func (m *SQLiteWebhookManager) Subscribe(ctx context.Context, config WebhookConfig) error {
	events, err := json.Marshal(config.Events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %v", err)
	}

	headers, err := json.Marshal(config.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %v", err)
	}

	retry, err := json.Marshal(config.Retry)
	if err != nil {
		return fmt.Errorf("failed to marshal retry config: %v", err)
	}

	_, err = m.db.ExecContext(ctx,
		`INSERT INTO webhooks (id, name, url, secret, events, headers, retry_config, max_parallel, timeout, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		config.ID, config.Name, config.URL, config.Secret,
		string(events), string(headers), string(retry),
		config.MaxParallel, config.Timeout.Milliseconds(), config.Enabled)
	if err != nil {
		return fmt.Errorf("failed to insert webhook: %v", err)
	}

	return nil
}

// Unsubscribe implements WebhookManager
func (m *SQLiteWebhookManager) Unsubscribe(ctx context.Context, webhookID string) error {
	result, err := m.db.ExecContext(ctx, "DELETE FROM webhooks WHERE id = ?", webhookID)
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %v", err)
	}
	if rows == 0 {
		return fmt.Errorf("webhook not found")
	}

	return nil
}

// UpdateSubscription implements WebhookManager
func (m *SQLiteWebhookManager) UpdateSubscription(ctx context.Context, config WebhookConfig) error {
	events, err := json.Marshal(config.Events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %v", err)
	}

	headers, err := json.Marshal(config.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %v", err)
	}

	retry, err := json.Marshal(config.Retry)
	if err != nil {
		return fmt.Errorf("failed to marshal retry config: %v", err)
	}

	result, err := m.db.ExecContext(ctx,
		`UPDATE webhooks
		 SET name = ?, url = ?, secret = ?, events = ?, headers = ?,
			 retry_config = ?, max_parallel = ?, timeout = ?, enabled = ?
		 WHERE id = ?`,
		config.Name, config.URL, config.Secret,
		string(events), string(headers), string(retry),
		config.MaxParallel, config.Timeout.Milliseconds(), config.Enabled,
		config.ID)
	if err != nil {
		return fmt.Errorf("failed to update webhook: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %v", err)
	}
	if rows == 0 {
		return fmt.Errorf("webhook not found")
	}

	return nil
}

// GetSubscription implements WebhookManager
func (m *SQLiteWebhookManager) GetSubscription(ctx context.Context, webhookID string) (*WebhookConfig, error) {
	var (
		config    WebhookConfig
		events    string
		headers   string
		retry     string
		timeoutMs int64
	)

	err := m.db.QueryRowContext(ctx,
		`SELECT id, name, url, secret, events, headers, retry_config,
			max_parallel, timeout, enabled
		 FROM webhooks WHERE id = ?`,
		webhookID).Scan(
		&config.ID, &config.Name, &config.URL, &config.Secret,
		&events, &headers, &retry,
		&config.MaxParallel, &timeoutMs, &config.Enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %v", err)
	}

	if err := json.Unmarshal([]byte(events), &config.Events); err != nil {
		return nil, fmt.Errorf("failed to unmarshal events: %v", err)
	}
	if err := json.Unmarshal([]byte(headers), &config.Headers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal headers: %v", err)
	}
	if err := json.Unmarshal([]byte(retry), &config.Retry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal retry config: %v", err)
	}

	config.Timeout = time.Duration(timeoutMs) * time.Millisecond

	return &config, nil
}

// ListSubscriptions implements WebhookManager
func (m *SQLiteWebhookManager) ListSubscriptions(ctx context.Context) ([]WebhookConfig, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, name, url, secret, events, headers, retry_config,
			max_parallel, timeout, enabled
		 FROM webhooks`)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %v", err)
	}
	defer rows.Close()

	var configs []WebhookConfig
	for rows.Next() {
		var (
			config    WebhookConfig
			events    string
			headers   string
			retry     string
			timeoutMs int64
		)

		err := rows.Scan(
			&config.ID, &config.Name, &config.URL, &config.Secret,
			&events, &headers, &retry,
			&config.MaxParallel, &timeoutMs, &config.Enabled)
		if err != nil {
			return nil, fmt.Errorf("failed to scan webhook: %v", err)
		}

		if err := json.Unmarshal([]byte(events), &config.Events); err != nil {
			return nil, fmt.Errorf("failed to unmarshal events: %v", err)
		}
		if err := json.Unmarshal([]byte(headers), &config.Headers); err != nil {
			return nil, fmt.Errorf("failed to unmarshal headers: %v", err)
		}
		if err := json.Unmarshal([]byte(retry), &config.Retry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal retry config: %v", err)
		}

		config.Timeout = time.Duration(timeoutMs) * time.Millisecond
		configs = append(configs, config)
	}

	return configs, nil
}

// Publish implements WebhookManager
func (m *SQLiteWebhookManager) Publish(ctx context.Context, event Event) error {
	// Get all enabled webhooks that are subscribed to this event
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, name, url, secret, events, headers, retry_config,
			max_parallel, timeout, enabled
		 FROM webhooks
		 WHERE enabled = true AND events LIKE ?`,
		fmt.Sprintf("%%\"%s\"%%", event.Type))
	if err != nil {
		return fmt.Errorf("failed to get subscribed webhooks: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			config    WebhookConfig
			events    string
			headers   string
			retry     string
			timeoutMs int64
		)

		err := rows.Scan(
			&config.ID, &config.Name, &config.URL, &config.Secret,
			&events, &headers, &retry,
			&config.MaxParallel, &timeoutMs, &config.Enabled)
		if err != nil {
			return fmt.Errorf("failed to scan webhook: %v", err)
		}

		if err := json.Unmarshal([]byte(events), &config.Events); err != nil {
			return fmt.Errorf("failed to unmarshal events: %v", err)
		}
		if err := json.Unmarshal([]byte(headers), &config.Headers); err != nil {
			return fmt.Errorf("failed to unmarshal headers: %v", err)
		}
		if err := json.Unmarshal([]byte(retry), &config.Retry); err != nil {
			return fmt.Errorf("failed to unmarshal retry config: %v", err)
		}

		config.Timeout = time.Duration(timeoutMs) * time.Millisecond

		// Send webhook in a goroutine
		go func(config WebhookConfig) {
			delivery, err := m.sender.Send(context.Background(), config, event)
			if err != nil {
				// Log error and store failed delivery
				fmt.Printf("Failed to send webhook: %v\n", err)
			}

			// Store delivery
			if delivery != nil {
				m.storeDelivery(context.Background(), delivery)
			}
		}(config)
	}

	return nil
}

// GetDelivery implements WebhookManager
func (m *SQLiteWebhookManager) GetDelivery(ctx context.Context, deliveryID string) (*WebhookDelivery, error) {
	var delivery WebhookDelivery

	err := m.db.QueryRowContext(ctx,
		`SELECT id, webhook_id, event_id, url, status, request, response,
			error, duration, timestamp, retry_count, next_retry_at
		 FROM webhook_deliveries WHERE id = ?`,
		deliveryID).Scan(
		&delivery.ID, &delivery.WebhookID, &delivery.EventID,
		&delivery.URL, &delivery.Status, &delivery.Request,
		&delivery.Response, &delivery.Error, &delivery.Duration,
		&delivery.Timestamp, &delivery.RetryCount, &delivery.NextRetryAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get delivery: %v", err)
	}

	return &delivery, nil
}

// ListDeliveries implements WebhookManager
func (m *SQLiteWebhookManager) ListDeliveries(ctx context.Context, filter DeliveryFilter) ([]WebhookDelivery, error) {
	query := `SELECT id, webhook_id, event_id, url, status, request, response,
				error, duration, timestamp, retry_count, next_retry_at
			  FROM webhook_deliveries WHERE 1=1`
	args := []interface{}{}

	if filter.WebhookID != "" {
		query += " AND webhook_id = ?"
		args = append(args, filter.WebhookID)
	}
	if filter.Status != 0 {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if !filter.StartTime.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, filter.EndTime)
	}

	query += " ORDER BY timestamp DESC"

	if filter.MaxResults > 0 {
		query += " LIMIT ?"
		args = append(args, filter.MaxResults)
	}
	if filter.NextToken != "" {
		query += " AND id > ?"
		args = append(args, filter.NextToken)
	}

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list deliveries: %v", err)
	}
	defer rows.Close()

	var deliveries []WebhookDelivery
	for rows.Next() {
		var delivery WebhookDelivery

		err := rows.Scan(
			&delivery.ID, &delivery.WebhookID, &delivery.EventID,
			&delivery.URL, &delivery.Status, &delivery.Request,
			&delivery.Response, &delivery.Error, &delivery.Duration,
			&delivery.Timestamp, &delivery.RetryCount, &delivery.NextRetryAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan delivery: %v", err)
		}

		deliveries = append(deliveries, delivery)
	}

	return deliveries, nil
}

// RetryDelivery implements WebhookManager
func (m *SQLiteWebhookManager) RetryDelivery(ctx context.Context, deliveryID string) error {
	// Get delivery and webhook config
	delivery, err := m.GetDelivery(ctx, deliveryID)
	if err != nil {
		return fmt.Errorf("failed to get delivery: %v", err)
	}
	if delivery == nil {
		return fmt.Errorf("delivery not found")
	}

	config, err := m.GetSubscription(ctx, delivery.WebhookID)
	if err != nil {
		return fmt.Errorf("failed to get webhook config: %v", err)
	}
	if config == nil {
		return fmt.Errorf("webhook not found")
	}

	// Parse original event from request
	var event Event
	if err := json.Unmarshal([]byte(delivery.Request), &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %v", err)
	}

	// Send webhook
	newDelivery, err := m.sender.Send(ctx, *config, event)
	if err != nil {
		return fmt.Errorf("failed to retry webhook: %v", err)
	}

	// Store new delivery
	if newDelivery != nil {
		m.storeDelivery(ctx, newDelivery)
	}

	return nil
}

func (m *SQLiteWebhookManager) storeDelivery(ctx context.Context, delivery *WebhookDelivery) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO webhook_deliveries (
			id, webhook_id, event_id, url, status, request, response,
			error, duration, timestamp, retry_count, next_retry_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		delivery.ID, delivery.WebhookID, delivery.EventID,
		delivery.URL, delivery.Status, delivery.Request,
		delivery.Response, delivery.Error, delivery.Duration,
		delivery.Timestamp, delivery.RetryCount, delivery.NextRetryAt)
	if err != nil {
		return fmt.Errorf("failed to store delivery: %v", err)
	}

	return nil
}
