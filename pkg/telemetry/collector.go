package telemetry

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Metric represents a single telemetry measurement
type Metric struct {
	Name      string    `json:"name"`
	Value     any       `json:"value"`
	Timestamp time.Time `json:"timestamp"`
	Labels    Labels    `json:"labels,omitempty"`
}

// Labels are key-value pairs attached to metrics
type Labels map[string]string

// Collector manages telemetry collection
type Collector struct {
	ctx      context.Context
	cancel   context.CancelFunc
	interval time.Duration
	handlers []Handler
	sources  []Source
	mu       sync.RWMutex
	wg       sync.WaitGroup
}

// Handler processes collected metrics
type Handler interface {
	Handle(context.Context, []Metric) error
}

// Source provides metrics
type Source interface {
	Collect(context.Context) ([]Metric, error)
}

// New creates a new Collector
func New(interval time.Duration) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	return &Collector{
		ctx:      ctx,
		cancel:   cancel,
		interval: interval,
		handlers: make([]Handler, 0),
		sources:  make([]Source, 0),
	}
}

// AddHandler registers a new metric handler
func (c *Collector) AddHandler(h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, h)
}

// AddSource registers a new metric source
func (c *Collector) AddSource(s Source) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sources = append(c.sources, s)
}

func (c *Collector) Start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				if err := c.collect(); err != nil {
					// Log error but continue collection
					slog.Error("Collection error", "error", err)
				}
			}
		}
	}()
}

func (c *Collector) Stop() {
	c.cancel()
	c.wg.Wait()
}

func (c *Collector) collect() error {
	c.mu.RLock()
	sources := c.sources
	handlers := c.handlers
	c.mu.RUnlock()

	var allMetrics []Metric

	// Collect from all sources
	for _, source := range sources {
		metrics, err := source.Collect(c.ctx)
		if err != nil {
			slog.Error("Source collection error", "error", err)
			continue
		}
		allMetrics = append(allMetrics, metrics...)
	}

	for _, handler := range handlers {
		if err := handler.Handle(c.ctx, allMetrics); err != nil {
			slog.Error("Handler error", "error", err)
		}
	}

	return nil
}
