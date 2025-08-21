package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
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

	retry, err := json.Marshal(config.RetryConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal retry config: %v", err)
	}

	_, err = m.db.ExecContext(ctx,
		`INSERT INTO webhook (id, name, url, secret, events, headers, retry_config, max_parallel, timeout, enabled)
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
	result, err := m.db.ExecContext(ctx, "DELETE FROM webhook WHERE id = ?", webhookID)
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

	retry, err := json.Marshal(config.RetryConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal retry config: %v", err)
	}

	result, err := m.db.ExecContext(ctx,
		`UPDATE webhook
		 SET name = ?, url = ?, secret = ?, events = ?, headers = ?,
			 retry_config = ?, max_parallel = ?, timeout = ?, enabled = ?,
			 updated_at = CURRENT_TIMESTAMP
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
		 FROM webhook WHERE id = ?`,
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
	if err := json.Unmarshal([]byte(retry), &config.RetryConfig); err != nil {
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
		 FROM webhook`)
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
		if err := json.Unmarshal([]byte(retry), &config.RetryConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal retry config: %v", err)
		}

		config.Timeout = time.Duration(timeoutMs) * time.Millisecond
		configs = append(configs, config)
	}

	return configs, nil
}

// Publish implements WebhookManager
func (m *SQLiteWebhookManager) Publish(ctx context.Context, event Event) error {
	// Get all active subscriptions for this event type
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, url, secret, headers, retry_config, max_parallel, timeout, enabled
		 FROM webhook
		 WHERE enabled = 1
		 AND events LIKE ?`,
		"%"+string(event.Type)+"%")
	if err != nil {
		return fmt.Errorf("failed to query webhooks: %w", err)
	}
	defer rows.Close()

	var wg sync.WaitGroup
	for rows.Next() {
		var webhook WebhookConfig
		var headersJSON, retryConfigJSON string
		err := rows.Scan(
			&webhook.ID,
			&webhook.URL,
			&webhook.Secret,
			&headersJSON,
			&retryConfigJSON,
			&webhook.MaxParallel,
			&webhook.Timeout,
			&webhook.Enabled,
		)
		if err != nil {
			return fmt.Errorf("failed to scan webhook: %w", err)
		}

		// Parse headers
		if err := json.Unmarshal([]byte(headersJSON), &webhook.Headers); err != nil {
			return fmt.Errorf("failed to unmarshal headers: %w", err)
		}

		// Parse retry config
		if err := json.Unmarshal([]byte(retryConfigJSON), &webhook.RetryConfig); err != nil {
			return fmt.Errorf("failed to unmarshal retry config: %w", err)
		}

		wg.Add(1)
		go func(webhook WebhookConfig) {
			defer wg.Done()

			delivery, err := m.sender.Send(ctx, webhook, event)
			if err != nil {
				// Log error but continue with other webhooks
				log.Printf("Failed to send webhook %s: %v", webhook.ID, err)
			}

			if delivery != nil {
				err = m.storeDelivery(ctx, delivery)
				if err != nil {
					log.Printf("Failed to store delivery for webhook %s: %v", webhook.ID, err)
				}
			}
		}(webhook)
	}

	wg.Wait()
	return rows.Err()
}

// GetDelivery implements WebhookManager
func (m *SQLiteWebhookManager) GetDelivery(ctx context.Context, deliveryID string) (*WebhookDelivery, error) {
	var (
		delivery     WebhookDelivery
		timestampStr sql.NullString
		nextRetryStr sql.NullString
	)

	err := m.db.QueryRowContext(ctx,
		`SELECT id, webhook_id, event_id, url, status, request, response,
			error, duration, strftime('%Y-%m-%dT%H:%M:%SZ', timestamp), retry_count,
			strftime('%Y-%m-%dT%H:%M:%SZ', next_retry_at)
		 FROM webhook_delivery WHERE id = ?`,
		deliveryID).Scan(
		&delivery.ID, &delivery.WebhookID, &delivery.EventID,
		&delivery.URL, &delivery.Status, &delivery.Request,
		&delivery.Response, &delivery.Error, &delivery.Duration,
		&timestampStr, &delivery.RetryCount, &nextRetryStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get delivery: %v", err)
	}

	if timestampStr.Valid {
		timestamp, err := time.Parse(time.RFC3339, timestampStr.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %v", err)
		}
		delivery.Timestamp = timestamp
	}

	if nextRetryStr.Valid {
		nextRetry, err := time.Parse(time.RFC3339, nextRetryStr.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse next_retry_at: %v", err)
		}
		delivery.NextRetryAt = nextRetry
	}

	return &delivery, nil
}

// ListDeliveries implements WebhookManager
func (m *SQLiteWebhookManager) ListDeliveries(ctx context.Context, filter DeliveryFilter) ([]WebhookDelivery, error) {
	query := `SELECT id, webhook_id, event_id, url, status, request, response,
				error, duration, timestamp, retry_count,
				next_retry_at
			  FROM webhook_delivery WHERE 1=1`
	args := []any{}

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
		args = append(args, filter.StartTime.UTC().Format(time.RFC3339))
	}
	if !filter.EndTime.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, filter.EndTime.UTC().Format(time.RFC3339))
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
		var (
			delivery     WebhookDelivery
			timestampStr sql.NullString
			nextRetryStr sql.NullString
		)

		err := rows.Scan(
			&delivery.ID, &delivery.WebhookID, &delivery.EventID,
			&delivery.URL, &delivery.Status, &delivery.Request,
			&delivery.Response, &delivery.Error, &delivery.Duration,
			&timestampStr, &delivery.RetryCount, &nextRetryStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan delivery: %v", err)
		}

		if timestampStr.Valid {
			timestamp, err := time.Parse(time.RFC3339, timestampStr.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse timestamp: %v", err)
			}
			delivery.Timestamp = timestamp
		}

		if nextRetryStr.Valid {
			nextRetry, err := time.Parse(time.RFC3339, nextRetryStr.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse next_retry_at: %v", err)
			}
			delivery.NextRetryAt = nextRetry
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
		`INSERT INTO webhook_delivery (
			id, webhook_id, event_id, url, status, request, response, error,
			duration, timestamp, retry_count, next_retry_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		delivery.ID,
		delivery.WebhookID,
		delivery.EventID,
		delivery.URL,
		delivery.Status,
		delivery.Request,
		delivery.Response,
		delivery.Error,
		delivery.Duration,
		delivery.Timestamp.UTC().Format(time.RFC3339),
		delivery.RetryCount,
		delivery.NextRetryAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to store delivery: %w", err)
	}
	return nil
}
