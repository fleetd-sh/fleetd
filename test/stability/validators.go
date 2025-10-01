package stability

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

// MemoryLeakValidator detects memory leaks
type MemoryLeakValidator struct {
	logger      *logrus.Logger
	config      *Config
	baselines   []uint64
	measurements int
	maxMeasurements int
}

// NewMemoryLeakValidator creates a new memory leak validator
func NewMemoryLeakValidator(config *Config, logger *logrus.Logger) *MemoryLeakValidator {
	return &MemoryLeakValidator{
		logger:          logger,
		config:          config,
		baselines:       make([]uint64, 0),
		maxMeasurements: 100,
	}
}

func (v *MemoryLeakValidator) Name() string { return "memory_leak" }

func (v *MemoryLeakValidator) Configure(config map[string]interface{}) error { return nil }

func (v *MemoryLeakValidator) Reset() error {
	v.baselines = make([]uint64, 0)
	v.measurements = 0
	return nil
}

func (v *MemoryLeakValidator) Validate(ctx context.Context) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	v.baselines = append(v.baselines, m.Alloc)
	v.measurements++

	// Keep only recent measurements
	if len(v.baselines) > v.maxMeasurements {
		v.baselines = v.baselines[1:]
	}

	// Need at least 10 measurements to detect trends
	if len(v.baselines) < 10 {
		return nil
	}

	// Calculate trend
	trend := v.calculateMemoryTrend()
	increase := v.calculateMemoryIncrease()

	if increase > v.config.MemoryLeakThreshold {
		return fmt.Errorf("memory leak detected: %.2f%% increase, trend: %.2f bytes/measurement",
			increase, trend)
	}

	return nil
}

func (v *MemoryLeakValidator) calculateMemoryTrend() float64 {
	n := len(v.baselines)
	if n < 2 {
		return 0
	}

	// Simple linear regression
	var sumX, sumY, sumXY, sumXX float64
	for i, value := range v.baselines {
		x := float64(i)
		y := float64(value)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	denominator := float64(n)*sumXX - sumX*sumX
	if denominator == 0 {
		return 0
	}

	return (float64(n)*sumXY - sumX*sumY) / denominator
}

func (v *MemoryLeakValidator) calculateMemoryIncrease() float64 {
	if len(v.baselines) < 2 {
		return 0
	}

	first := v.baselines[0]
	last := v.baselines[len(v.baselines)-1]

	return float64(last-first) / float64(first) * 100
}

// ConnectionStabilityValidator validates network connection stability
type ConnectionStabilityValidator struct {
	logger           *logrus.Logger
	config           *Config
	endpoints        []string
	connectionErrors int
	totalAttempts    int
	mu               sync.RWMutex
}

// NewConnectionStabilityValidator creates a new connection stability validator
func NewConnectionStabilityValidator(config *Config, logger *logrus.Logger, endpoints []string) *ConnectionStabilityValidator {
	return &ConnectionStabilityValidator{
		logger:    logger,
		config:    config,
		endpoints: endpoints,
	}
}

func (v *ConnectionStabilityValidator) Name() string { return "connection_stability" }

func (v *ConnectionStabilityValidator) Configure(config map[string]interface{}) error {
	if endpoints, ok := config["endpoints"].([]string); ok {
		v.endpoints = endpoints
	}
	return nil
}

func (v *ConnectionStabilityValidator) Reset() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.connectionErrors = 0
	v.totalAttempts = 0
	return nil
}

func (v *ConnectionStabilityValidator) Validate(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, endpoint := range v.endpoints {
		v.totalAttempts++

		// Test HTTP connection
		if strings.HasPrefix(endpoint, "http") {
			if err := v.testHTTPConnection(ctx, endpoint); err != nil {
				v.connectionErrors++
				v.logger.WithField("endpoint", endpoint).WithError(err).Debug("HTTP connection failed")
			}
		} else {
			// Test TCP connection
			if err := v.testTCPConnection(ctx, endpoint); err != nil {
				v.connectionErrors++
				v.logger.WithField("endpoint", endpoint).WithError(err).Debug("TCP connection failed")
			}
		}
	}

	// Calculate error rate
	errorRate := float64(v.connectionErrors) / float64(v.totalAttempts) * 100

	// Fail if error rate exceeds 5%
	if errorRate > 5.0 {
		return fmt.Errorf("connection error rate (%.2f%%) exceeds threshold (5.0%%)", errorRate)
	}

	return nil
}

func (v *ConnectionStabilityValidator) testHTTPConnection(ctx context.Context, endpoint string) error {
	client := &http.Client{
		Timeout: v.config.NetworkTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}

	return nil
}

func (v *ConnectionStabilityValidator) testTCPConnection(ctx context.Context, endpoint string) error {
	dialer := &net.Dialer{
		Timeout: v.config.NetworkTimeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()

	return nil
}

// DatabaseIntegrityValidator validates database integrity and connection stability
type DatabaseIntegrityValidator struct {
	logger     *logrus.Logger
	config     *Config
	db         *sql.DB
	testData   map[string]string
	mu         sync.RWMutex
}

// NewDatabaseIntegrityValidator creates a new database integrity validator
func NewDatabaseIntegrityValidator(config *Config, logger *logrus.Logger) (*DatabaseIntegrityValidator, error) {
	db, err := sql.Open("sqlite3", config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(config.MaxDBConnections)
	db.SetMaxIdleConns(config.MaxDBConnections / 2)
	db.SetConnMaxLifetime(5 * time.Minute)

	validator := &DatabaseIntegrityValidator{
		logger:   logger,
		config:   config,
		db:       db,
		testData: make(map[string]string),
	}

	// Initialize test table
	if err := validator.initTestTable(); err != nil {
		return nil, fmt.Errorf("failed to initialize test table: %w", err)
	}

	return validator, nil
}

func (v *DatabaseIntegrityValidator) Name() string { return "database_integrity" }

func (v *DatabaseIntegrityValidator) Configure(config map[string]interface{}) error { return nil }

func (v *DatabaseIntegrityValidator) Reset() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Clear test data
	v.testData = make(map[string]string)

	// Clean test table
	_, err := v.db.Exec("DELETE FROM stability_test")
	return err
}

func (v *DatabaseIntegrityValidator) initTestTable() error {
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS stability_test (
			id TEXT PRIMARY KEY,
			data TEXT NOT NULL,
			checksum TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := v.db.Exec(createTableSQL)
	return err
}

func (v *DatabaseIntegrityValidator) Validate(ctx context.Context) error {
	// Test connection
	if err := v.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Test write operation
	testID := fmt.Sprintf("test_%d", time.Now().UnixNano())
	testData := "stability test data"
	checksum := v.calculateChecksum(testData)

	if err := v.insertTestData(ctx, testID, testData, checksum); err != nil {
		return fmt.Errorf("failed to insert test data: %w", err)
	}

	v.mu.Lock()
	v.testData[testID] = testData
	v.mu.Unlock()

	// Test read operation
	if err := v.verifyTestData(ctx, testID, testData, checksum); err != nil {
		return fmt.Errorf("data integrity check failed: %w", err)
	}

	// Test connection pool
	if err := v.testConnectionPool(ctx); err != nil {
		return fmt.Errorf("connection pool test failed: %w", err)
	}

	return nil
}

func (v *DatabaseIntegrityValidator) insertTestData(ctx context.Context, id, data, checksum string) error {
	query := "INSERT INTO stability_test (id, data, checksum) VALUES (?, ?, ?)"
	_, err := v.db.ExecContext(ctx, query, id, data, checksum)
	return err
}

func (v *DatabaseIntegrityValidator) verifyTestData(ctx context.Context, id, expectedData, expectedChecksum string) error {
	query := "SELECT data, checksum FROM stability_test WHERE id = ?"
	row := v.db.QueryRowContext(ctx, query, id)

	var data, checksum string
	if err := row.Scan(&data, &checksum); err != nil {
		return fmt.Errorf("failed to read test data: %w", err)
	}

	if data != expectedData {
		return fmt.Errorf("data mismatch: expected %s, got %s", expectedData, data)
	}

	if checksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, checksum)
	}

	return nil
}

func (v *DatabaseIntegrityValidator) testConnectionPool(ctx context.Context) error {
	// Test multiple concurrent connections
	concurrency := 10
	errChan := make(chan error, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			testID := fmt.Sprintf("pool_test_%d_%d", i, time.Now().UnixNano())
			err := v.db.PingContext(ctx)
			if err != nil {
				errChan <- fmt.Errorf("connection %d failed: %w", i, err)
				return
			}

			// Quick insert/select test
			query := "INSERT INTO stability_test (id, data, checksum) VALUES (?, ?, ?)"
			_, err = v.db.ExecContext(ctx, query, testID, "pool_test", "checksum")
			if err != nil {
				errChan <- fmt.Errorf("insert %d failed: %w", i, err)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		return err
	}

	return nil
}

func (v *DatabaseIntegrityValidator) calculateChecksum(data string) string {
	// Simple checksum for testing
	var sum int
	for _, c := range data {
		sum += int(c)
	}
	return fmt.Sprintf("%x", sum)
}

// DeadlockDetector detects potential deadlocks
type DeadlockDetector struct {
	logger      *logrus.Logger
	config      *Config
	goroutineCount int
	stableCount    int
}

// NewDeadlockDetector creates a new deadlock detector
func NewDeadlockDetector(config *Config, logger *logrus.Logger) *DeadlockDetector {
	return &DeadlockDetector{
		logger: logger,
		config: config,
	}
}

func (d *DeadlockDetector) Name() string { return "deadlock_detector" }

func (d *DeadlockDetector) Configure(config map[string]interface{}) error { return nil }

func (d *DeadlockDetector) Reset() error {
	d.goroutineCount = 0
	d.stableCount = 0
	return nil
}

func (d *DeadlockDetector) Validate(ctx context.Context) error {
	currentCount := runtime.NumGoroutine()

	// Check if goroutine count is stable (indicating potential deadlock)
	if currentCount == d.goroutineCount {
		d.stableCount++
	} else {
		d.stableCount = 0
	}

	d.goroutineCount = currentCount

	// If goroutine count is stable for too long, might indicate deadlock
	if d.stableCount > 20 && currentCount > 100 {
		return fmt.Errorf("potential deadlock detected: goroutine count stable at %d for %d checks",
			currentCount, d.stableCount)
	}

	return nil
}

// TLSValidator validates TLS certificate status and renewal
type TLSValidator struct {
	logger   *logrus.Logger
	config   *Config
	certPath string
	keyPath  string
}

// NewTLSValidator creates a new TLS validator
func NewTLSValidator(config *Config, logger *logrus.Logger) *TLSValidator {
	return &TLSValidator{
		logger:   logger,
		config:   config,
		certPath: config.TLSCertPath,
		keyPath:  config.TLSKeyPath,
	}
}

func (v *TLSValidator) Name() string { return "tls_validator" }

func (v *TLSValidator) Configure(config map[string]interface{}) error {
	if certPath, ok := config["cert_path"].(string); ok {
		v.certPath = certPath
	}
	if keyPath, ok := config["key_path"].(string); ok {
		v.keyPath = keyPath
	}
	return nil
}

func (v *TLSValidator) Reset() error { return nil }

func (v *TLSValidator) Validate(ctx context.Context) error {
	if v.certPath == "" {
		return nil // TLS not configured
	}

	// Check certificate expiry
	// This is a simplified version - real implementation would parse the certificate
	// and check its validity period

	// For now, just check if certificate files exist and are readable
	if err := v.checkCertificateFiles(); err != nil {
		return fmt.Errorf("certificate check failed: %w", err)
	}

	return nil
}

func (v *TLSValidator) checkCertificateFiles() error {
	// Check if certificate file exists and is readable
	if v.certPath != "" {
		if err := v.checkFileReadable(v.certPath); err != nil {
			return fmt.Errorf("certificate file check failed: %w", err)
		}
	}

	// Check if key file exists and is readable
	if v.keyPath != "" {
		if err := v.checkFileReadable(v.keyPath); err != nil {
			return fmt.Errorf("key file check failed: %w", err)
		}
	}

	return nil
}

func (v *TLSValidator) checkFileReadable(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Try to read at least one byte
	buf := make([]byte, 1)
	_, err = file.Read(buf)
	if err != nil {
		return err
	}

	return nil
}

// PerformanceValidator validates performance characteristics
type PerformanceValidator struct {
	logger        *logrus.Logger
	config        *Config
	baselines     []time.Duration
	measurements  int
}

// NewPerformanceValidator creates a new performance validator
func NewPerformanceValidator(config *Config, logger *logrus.Logger) *PerformanceValidator {
	return &PerformanceValidator{
		logger:    logger,
		config:    config,
		baselines: make([]time.Duration, 0),
	}
}

func (v *PerformanceValidator) Name() string { return "performance" }

func (v *PerformanceValidator) Configure(config map[string]interface{}) error { return nil }

func (v *PerformanceValidator) Reset() error {
	v.baselines = make([]time.Duration, 0)
	v.measurements = 0
	return nil
}

func (v *PerformanceValidator) Validate(ctx context.Context) error {
	// Measure performance of a simple operation
	start := time.Now()

	// Simulate some work (could be replaced with actual operation timing)
	for i := 0; i < 1000; i++ {
		_ = fmt.Sprintf("test_%d", i)
	}

	duration := time.Since(start)
	v.baselines = append(v.baselines, duration)
	v.measurements++

	// Keep only recent measurements
	if len(v.baselines) > 50 {
		v.baselines = v.baselines[1:]
	}

	// Need baseline measurements
	if len(v.baselines) < 10 {
		return nil
	}

	// Calculate average of first 10 measurements as baseline
	var baselineSum time.Duration
	for i := 0; i < 10; i++ {
		baselineSum += v.baselines[i]
	}
	baseline := baselineSum / 10

	// Calculate average of recent measurements
	recentCount := min(len(v.baselines), 10)
	var recentSum time.Duration
	for i := len(v.baselines) - recentCount; i < len(v.baselines); i++ {
		recentSum += v.baselines[i]
	}
	recentAvg := recentSum / time.Duration(recentCount)

	// Check for performance degradation
	if recentAvg > baseline {
		degradation := float64(recentAvg-baseline) / float64(baseline) * 100
		if degradation > v.config.PerformanceThreshold {
			return fmt.Errorf("performance degradation detected: %.2f%% slower than baseline", degradation)
		}
	}

	return nil
}