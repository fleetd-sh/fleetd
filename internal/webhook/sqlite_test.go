package webhook

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"fleetd.sh/internal/migrations"
)

func setupWebhookManager(t *testing.T) (*SQLiteWebhookManager, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	// Run migrations
	version, dirty, err := migrations.MigrateUp(db)
	require.NoError(t, err)
	require.False(t, dirty)
	require.GreaterOrEqual(t, version, 1)

	manager, err := NewSQLiteWebhookManager(db, nil)
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return manager, cleanup
}

func TestWebhookManager_Subscribe(t *testing.T) {
	manager, cleanup := setupWebhookManager(t)
	defer cleanup()

	ctx := context.Background()

	config := WebhookConfig{
		ID:      "test-webhook",
		Name:    "Test Webhook",
		URL:     "http://example.com/webhook",
		Secret:  "test-secret",
		Events:  []EventType{EventDeviceRegistered, EventDeviceUpdated},
		Headers: map[string]string{"X-Test": "test"},
		RetryConfig: RetryConfig{
			MaxRetries:  3,
			InitialWait: time.Second,
			MaxWait:     time.Minute,
		},
		MaxParallel: 2,
		Timeout:     30 * time.Second,
		Enabled:     true,
	}

	err := manager.Subscribe(ctx, config)
	require.NoError(t, err)

	// Verify subscription
	stored, err := manager.GetSubscription(ctx, config.ID)
	require.NoError(t, err)
	assert.NotNil(t, stored)
	assert.Equal(t, config.ID, stored.ID)
	assert.Equal(t, config.Name, stored.Name)
	assert.Equal(t, config.URL, stored.URL)
	assert.Equal(t, config.Secret, stored.Secret)
	assert.Equal(t, config.Events, stored.Events)
	assert.Equal(t, config.Headers, stored.Headers)
	assert.Equal(t, config.RetryConfig, stored.RetryConfig)
	assert.Equal(t, config.MaxParallel, stored.MaxParallel)
	assert.Equal(t, config.Timeout, stored.Timeout)
	assert.Equal(t, config.Enabled, stored.Enabled)
}

func TestWebhookManager_UpdateSubscription(t *testing.T) {
	manager, cleanup := setupWebhookManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create initial subscription
	config := WebhookConfig{
		ID:      "test-webhook",
		Name:    "Test Webhook",
		URL:     "http://example.com/webhook",
		Secret:  "test-secret",
		Events:  []EventType{EventDeviceRegistered},
		Enabled: true,
	}

	err := manager.Subscribe(ctx, config)
	require.NoError(t, err)

	// Update subscription
	config.Name = "Updated Webhook"
	config.Events = append(config.Events, EventDeviceUpdated)
	config.Enabled = false

	err = manager.UpdateSubscription(ctx, config)
	require.NoError(t, err)

	// Verify update
	stored, err := manager.GetSubscription(ctx, config.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Webhook", stored.Name)
	assert.Equal(t, []EventType{EventDeviceRegistered, EventDeviceUpdated}, stored.Events)
	assert.False(t, stored.Enabled)
}

func TestWebhookManager_ListSubscriptions(t *testing.T) {
	manager, cleanup := setupWebhookManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create test webhooks
	webhooks := []WebhookConfig{
		{
			ID:      "webhook1",
			Name:    "Webhook 1",
			URL:     "http://example.com/webhook1",
			Events:  []EventType{EventDeviceRegistered},
			Enabled: true,
		},
		{
			ID:      "webhook2",
			Name:    "Webhook 2",
			URL:     "http://example.com/webhook2",
			Events:  []EventType{EventDeviceUpdated},
			Enabled: false,
		},
	}

	for _, config := range webhooks {
		err := manager.Subscribe(ctx, config)
		require.NoError(t, err)
	}

	// List webhooks
	list, err := manager.ListSubscriptions(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestWebhookManager_Publish(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Run migrations
	version, dirty, err := migrations.MigrateUp(db)
	require.NoError(t, err)
	require.False(t, dirty)
	require.GreaterOrEqual(t, version, 1)

	// Add debug logging
	var deliveryReceived bool
	customSender := NewDefaultWebhookSender(nil)
	testSender := &testWebhookSender{
		delegate: customSender,
		onSend: func(delivery *WebhookDelivery) {
			deliveryReceived = true
		},
	}

	manager, err := NewSQLiteWebhookManager(db, testSender)
	require.NoError(t, err)

	// Start test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create webhook subscription
	sub := WebhookConfig{
		ID:      "test-webhook",
		Name:    "Test Webhook", // Add required field
		URL:     server.URL,
		Secret:  "test-secret", // Add required field
		Events:  []EventType{EventDeviceRegistered},
		Enabled: true, // Explicitly enable the webhook
	}
	require.NoError(t, manager.Subscribe(context.Background(), sub))

	// Verify subscription was created
	stored, err := manager.GetSubscription(context.Background(), sub.ID)
	require.NoError(t, err)
	require.NotNil(t, stored, "Subscription should be stored")

	// Publish event
	event := Event{
		ID:        "test-event",
		Type:      EventDeviceRegistered,
		Timestamp: time.Now(),
	}
	err = manager.Publish(context.Background(), event)
	require.NoError(t, err)

	// Wait with timeout
	timeout := time.After(3 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for !deliveryReceived {
		select {
		case <-timeout:
			t.Fatal("Test timed out waiting for delivery")
		case <-ticker.C:
			// Check deliveries
			deliveries, err := manager.ListDeliveries(context.Background(), DeliveryFilter{WebhookID: sub.ID})
			require.NoError(t, err)
			if len(deliveries) > 0 {
				return
			}
		}
	}
}

// Add this helper type for testing
type testWebhookSender struct {
	delegate WebhookSender
	onSend   func(*WebhookDelivery)
}

func (s *testWebhookSender) Send(ctx context.Context, webhook WebhookConfig, event Event) (*WebhookDelivery, error) {
	delivery, err := s.delegate.Send(ctx, webhook, event)
	if delivery != nil && s.onSend != nil {
		s.onSend(delivery)
	}
	return delivery, err
}

func TestWebhookManager_RetryDelivery(t *testing.T) {
	// Create test server that fails first request
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Setup manager with test server URL
	manager, cleanup := setupWebhookManager(t)
	defer cleanup()

	ctx := context.Background()

	// Subscribe to events
	config := WebhookConfig{
		ID:      "test-webhook",
		Name:    "Test Webhook",
		URL:     server.URL,
		Events:  []EventType{EventDeviceRegistered},
		Enabled: true,
		RetryConfig: RetryConfig{
			MaxRetries:  3,
			InitialWait: time.Millisecond,
			MaxWait:     time.Millisecond * 10,
		},
	}

	err := manager.Subscribe(ctx, config)
	require.NoError(t, err)

	// Publish event
	event := Event{
		ID:        "test-event",
		Type:      EventDeviceRegistered,
		Timestamp: time.Now(),
	}

	err = manager.Publish(ctx, event)
	require.NoError(t, err)

	// Wait for delivery
	time.Sleep(50 * time.Millisecond)

	// Get failed delivery
	deliveries, err := manager.ListDeliveries(ctx, DeliveryFilter{
		WebhookID: config.ID,
		Status:    http.StatusInternalServerError,
	})
	require.NoError(t, err)
	require.Len(t, deliveries, 1)

	// Retry delivery
	err = manager.RetryDelivery(ctx, deliveries[0].ID)
	require.NoError(t, err)

	// Wait for retry
	time.Sleep(50 * time.Millisecond)

	// Verify successful retry
	deliveries, err = manager.ListDeliveries(ctx, DeliveryFilter{
		WebhookID: config.ID,
		Status:    http.StatusOK,
	})
	require.NoError(t, err)
	assert.Len(t, deliveries, 1)
}

func TestWebhookManager_ListDeliveries(t *testing.T) {
	manager, cleanup := setupWebhookManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create test webhook
	config := WebhookConfig{
		ID:      "test-webhook",
		Name:    "Test Webhook",
		URL:     "http://example.com/webhook",
		Events:  []EventType{EventDeviceRegistered},
		Enabled: true,
	}

	err := manager.Subscribe(ctx, config)
	require.NoError(t, err)

	// Store test deliveries
	now := time.Now()
	deliveries := []WebhookDelivery{
		{
			ID:        "delivery1",
			WebhookID: config.ID,
			EventID:   "event1",
			URL:       config.URL,
			Status:    http.StatusOK,
			Timestamp: now.Add(-time.Hour),
		},
		{
			ID:        "delivery2",
			WebhookID: config.ID,
			EventID:   "event2",
			URL:       config.URL,
			Status:    http.StatusInternalServerError,
			Timestamp: now,
		},
	}

	for _, d := range deliveries {
		err := manager.storeDelivery(ctx, &d)
		require.NoError(t, err)
	}

	// Test filtering by status
	list, err := manager.ListDeliveries(ctx, DeliveryFilter{
		Status: http.StatusOK,
	})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "delivery1", list[0].ID)

	// Test filtering by time range
	list, err = manager.ListDeliveries(ctx, DeliveryFilter{
		StartTime: now.Add(-30 * time.Minute).UTC(),
		EndTime:   now.Add(time.Minute).UTC(),
	})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "delivery2", list[0].ID)

	// Test pagination
	list, err = manager.ListDeliveries(ctx, DeliveryFilter{
		MaxResults: 1,
	})
	require.NoError(t, err)
	assert.Len(t, list, 1)
}
