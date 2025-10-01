package webhook

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EventType represents the type of webhook event
type EventType string

const (
	// Device events
	EventDeviceRegistered   EventType = "device.registered"
	EventDeviceConnected    EventType = "device.connected"
	EventDeviceDisconnected EventType = "device.disconnected"
	EventDeviceUpdated      EventType = "device.updated"
	EventDeviceDeleted      EventType = "device.deleted"

	// Update events
	EventUpdateCreated    EventType = "update.created"
	EventUpdateStarted    EventType = "update.started"
	EventUpdateCompleted  EventType = "update.completed"
	EventUpdateFailed     EventType = "update.failed"
	EventUpdateRolledBack EventType = "update.rolled_back"

	// Binary events
	EventBinaryUploaded EventType = "binary.uploaded"
	EventBinaryDeleted  EventType = "binary.deleted"

	// Health events
	EventHealthWarning   EventType = "health.warning"
	EventHealthCritical  EventType = "health.critical"
	EventHealthRecovered EventType = "health.recovered"
)

// Event represents a webhook event
type Event struct {
	ID        string         `json:"id"`
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Data      any            `json:"data"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// WebhookConfig represents the configuration for a webhook
type WebhookConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Secret      string            `json:"secret"`
	Events      []EventType       `json:"events"`
	Headers     map[string]string `json:"headers,omitempty"`
	RetryConfig RetryConfig       `json:"retry_config,omitempty"`
	MaxParallel int               `json:"max_parallel,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty"`
	Enabled     bool              `json:"enabled"`
}

// RetryConfig represents retry configuration for webhooks
type RetryConfig struct {
	MaxRetries  int           `json:"max_retries"`
	InitialWait time.Duration `json:"initial_wait"`
	MaxWait     time.Duration `json:"max_wait"`
}

// WebhookDelivery represents a webhook delivery attempt
type WebhookDelivery struct {
	ID          string    `json:"id"`
	WebhookID   string    `json:"webhook_id"`
	EventID     string    `json:"event_id"`
	URL         string    `json:"url"`
	Status      int       `json:"status"`
	Request     string    `json:"request"`
	Response    string    `json:"response"`
	Error       string    `json:"error,omitempty"`
	Duration    float64   `json:"duration"`
	Timestamp   time.Time `json:"timestamp"`
	RetryCount  int       `json:"retry_count"`
	NextRetryAt time.Time `json:"next_retry_at,omitempty"`
}

// WebhookManager manages webhook subscriptions and deliveries
type WebhookManager interface {
	// Subscribe creates a new webhook subscription
	Subscribe(ctx context.Context, config WebhookConfig) error

	// Unsubscribe removes a webhook subscription
	Unsubscribe(ctx context.Context, webhookID string) error

	// UpdateSubscription updates an existing webhook subscription
	UpdateSubscription(ctx context.Context, config WebhookConfig) error

	// GetSubscription gets a webhook subscription by ID
	GetSubscription(ctx context.Context, webhookID string) (*WebhookConfig, error)

	// ListSubscriptions lists all webhook subscriptions
	ListSubscriptions(ctx context.Context) ([]WebhookConfig, error)

	// Publish publishes an event to all subscribed webhooks
	Publish(ctx context.Context, event Event) error

	// GetDelivery gets a webhook delivery by ID
	GetDelivery(ctx context.Context, deliveryID string) (*WebhookDelivery, error)

	// ListDeliveries lists webhook deliveries with optional filtering
	ListDeliveries(ctx context.Context, filter DeliveryFilter) ([]WebhookDelivery, error)

	// RetryDelivery retries a failed webhook delivery
	RetryDelivery(ctx context.Context, deliveryID string) error
}

// DeliveryFilter represents filtering options for listing webhook deliveries
type DeliveryFilter struct {
	WebhookID  string
	EventType  EventType
	Status     int
	StartTime  time.Time
	EndTime    time.Time
	MaxResults int
	NextToken  string
}

// WebhookSender sends webhook requests
type WebhookSender interface {
	// Send sends a webhook request
	Send(ctx context.Context, config WebhookConfig, event Event) (*WebhookDelivery, error)
}

// DefaultWebhookSender is the default implementation of WebhookSender
type DefaultWebhookSender struct {
	client *http.Client
}

// NewDefaultWebhookSender creates a new DefaultWebhookSender
func NewDefaultWebhookSender(client *http.Client) *DefaultWebhookSender {
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	return &DefaultWebhookSender{client: client}
}

// Send implements WebhookSender
func (s *DefaultWebhookSender) Send(ctx context.Context, config WebhookConfig, event Event) (*WebhookDelivery, error) {
	delivery := &WebhookDelivery{
		ID:        generateID(),
		WebhookID: config.ID,
		EventID:   event.ID,
		URL:       config.URL,
		Timestamp: time.Now(),
	}

	// Prepare request body
	body, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %v", err)
	}
	delivery.Request = string(body)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", config.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "fleetd-Webhook/1.0")
	req.Header.Set("X-fleetd-Event", string(event.Type))
	req.Header.Set("X-fleetd-Delivery", delivery.ID)
	if config.Secret != "" {
		signature := generateSignature(body, config.Secret, time.Now())
		req.Header.Set("X-fleetd-Signature", signature)
	}
	for k, v := range config.Headers {
		req.Header.Set(k, v)
	}

	// Send request
	start := time.Now()
	resp, err := s.client.Do(req)
	duration := time.Since(start).Seconds()

	if err != nil {
		delivery.Error = err.Error()
		delivery.Duration = duration
		return delivery, fmt.Errorf("failed to send webhook: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		delivery.Error = err.Error()
		delivery.Duration = duration
		return delivery, fmt.Errorf("failed to read response: %v", err)
	}

	delivery.Status = resp.StatusCode
	delivery.Response = string(respBody)
	delivery.Duration = duration

	return delivery, nil
}

func generateID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return fmt.Sprintf("whd_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("whd_%x", b)
}
