package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// LokiClient handles log ingestion to Grafana Loki
type LokiClient struct {
	url           string
	httpClient    *http.Client
	batchSize     int
	flushInterval time.Duration

	// Buffering
	buffer     []LogEntry
	bufferChan chan LogEntry
	flushChan  chan struct{}
	doneChan   chan struct{}
}

// NewLokiClient creates a new Loki client
func NewLokiClient(url string) *LokiClient {
	client := &LokiClient{
		url: url,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		batchSize:     1000,
		flushInterval: 5 * time.Second,
		buffer:        make([]LogEntry, 0, 1000),
		bufferChan:    make(chan LogEntry, 10000),
		flushChan:     make(chan struct{}, 1),
		doneChan:      make(chan struct{}),
	}

	go client.worker()

	return client
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Level     string
	DeviceID  string
	OrgID     string
	Source    string
	Message   string
	Labels    map[string]string
}

// lokiStream represents a stream in Loki's push format
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// lokiPushRequest represents the push request format for Loki
type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

func (c *LokiClient) Log(entry LogEntry) {
	select {
	case c.bufferChan <- entry:
		// Successfully queued
	default:
		// Buffer full, drop log (could implement backpressure here)
		slog.Warn("log buffer full, dropping log",
			"device_id", entry.DeviceID,
			"org_id", entry.OrgID,
			"source", entry.Source)
	}
}

// worker processes logs in the background
func (c *LokiClient) worker() {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case entry := <-c.bufferChan:
			c.buffer = append(c.buffer, entry)
			if len(c.buffer) >= c.batchSize {
				c.flush()
			}

		case <-ticker.C:
			if len(c.buffer) > 0 {
				c.flush()
			}

		case <-c.flushChan:
			c.flush()

		case <-c.doneChan:
			c.flush()
			return
		}
	}
}

// flush sends buffered logs to Loki
func (c *LokiClient) flush() {
	if len(c.buffer) == 0 {
		return
	}

	// Group logs by labels (create streams)
	streams := make(map[string]*lokiStream)

	for _, entry := range c.buffer {
		// Create label set
		labels := map[string]string{
			"device_id": entry.DeviceID,
			"org_id":    entry.OrgID,
			"level":     entry.Level,
			"source":    entry.Source,
		}

		// Add custom labels
		for k, v := range entry.Labels {
			labels[k] = v
		}

		// Create stream key
		streamKey := fmt.Sprintf("%s_%s_%s", entry.DeviceID, entry.Level, entry.Source)

		// Get or create stream
		stream, exists := streams[streamKey]
		if !exists {
			stream = &lokiStream{
				Stream: labels,
				Values: make([][]string, 0),
			}
			streams[streamKey] = stream
		}

		stream.Values = append(stream.Values, []string{
			strconv.FormatInt(entry.Timestamp.UnixNano(), 10),
			entry.Message,
		})
	}

	// Convert to slice
	streamSlice := make([]lokiStream, 0, len(streams))
	for _, stream := range streams {
		streamSlice = append(streamSlice, *stream)
	}

	pushReq := lokiPushRequest{
		Streams: streamSlice,
	}

	// Send to Loki
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.push(ctx, pushReq); err != nil {
		slog.Error("failed to push logs to Loki",
			"error", err,
			"batch_size", len(c.buffer),
			"streams", len(streamSlice))
		// Could implement retry logic here
	}

	// Clear buffer
	c.buffer = c.buffer[:0]
}

// push sends a push request to Loki
func (c *LokiClient) push(ctx context.Context, req lokiPushRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/loki/api/v1/push", c.url),
		bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// Query executes a LogQL query against Loki
func (c *LokiClient) Query(ctx context.Context, query string, start, end time.Time, limit int) (*QueryResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/loki/api/v1/query_range", c.url), nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("query", query)
	q.Add("start", fmt.Sprintf("%d", start.UnixNano()))
	q.Add("end", fmt.Sprintf("%d", end.UnixNano()))
	q.Add("limit", strconv.Itoa(limit))
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result QueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// QueryResponse represents a Loki query response
type QueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func (c *LokiClient) Flush() {
	select {
	case c.flushChan <- struct{}{}:
	default:
		// Flush already in progress
	}
}

func (c *LokiClient) Close() error {
	close(c.doneChan)
	c.httpClient.CloseIdleConnections()
	return nil
}
