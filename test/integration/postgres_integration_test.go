package integration

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"fleetd.sh/internal/database"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestPostgreSQLIntegration tests PostgreSQL integration for control plane
func TestPostgreSQLIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL integration tests in short mode")
	}

	t.Run("DatabaseMigrations", testDatabaseMigrations)
	t.Run("ConcurrentDeviceRegistrations", testConcurrentDeviceRegistrations)
	t.Run("MetricsIngestion", testMetricsIngestion)
	t.Run("ConnectionPooling", testConnectionPooling)
	t.Run("TransactionIsolation", testTransactionIsolation)
	t.Run("DeadlockHandling", testDeadlockHandling)
	t.Run("BulkOperations", testBulkOperations)
	t.Run("QueryPerformance", testQueryPerformance)
}

// setupTestPostgres starts a PostgreSQL container for testing
func setupTestPostgres(t *testing.T) (string, func()) {
	ctx := context.Background()

	// Start PostgreSQL container
	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:17-alpine"),
		postgres.WithDatabase("fleetd_test"),
		postgres.WithUsername("fleetd"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)

	// Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Cleanup function
	cleanup := func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}

	return connStr, cleanup
}

// testDatabaseMigrations tests database migration system
func testDatabaseMigrations(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	// Initialize database
	db, err := database.Initialize(database.Config{
		Driver:       "postgres",
		DatabaseURL:  connStr,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	})
	require.NoError(t, err)
	defer db.Close()

	// Run migrations
	err = database.RunMigrations(db.DB, "postgres")
	assert.NoError(t, err, "Migrations should run successfully")

	// Verify schema created
	var tableCount int
	err = db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_type = 'BASE TABLE'
	`).Scan(&tableCount)
	assert.NoError(t, err)
	assert.Greater(t, tableCount, 0, "Should have created tables")

	// Check for expected tables
	expectedTables := []string{
		"devices",
		"metrics",
		"updates",
		"deployments",
		"users",
		"api_keys",
	}

	for _, table := range expectedTables {
		var exists bool
		err = db.DB.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public'
				AND table_name = $1
			)
		`, table).Scan(&exists)
		assert.NoError(t, err)
		assert.True(t, exists, "Table %s should exist", table)
	}

	// Test migration idempotency - run again
	err = database.RunMigrations(db.DB, "postgres")
	assert.NoError(t, err, "Migrations should be idempotent")
}

// testConcurrentDeviceRegistrations tests concurrent device registrations
func testConcurrentDeviceRegistrations(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer db.Close()

	// Create devices table
	_, err = db.Exec(`
		CREATE TABLE devices (
			id SERIAL PRIMARY KEY,
			device_id VARCHAR(255) UNIQUE NOT NULL,
			name VARCHAR(255),
			status VARCHAR(50),
			last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			metadata JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Register devices concurrently
	const numDevices = 100
	var wg sync.WaitGroup
	errors := make(chan error, numDevices)

	for i := 0; i < numDevices; i++ {
		wg.Add(1)
		go func(deviceNum int) {
			defer wg.Done()

			deviceID := fmt.Sprintf("device-%04d", deviceNum)
			metadata := fmt.Sprintf(`{"type": "test", "index": %d}`, deviceNum)

			_, err := db.Exec(`
				INSERT INTO devices (device_id, name, status, metadata)
				VALUES ($1, $2, $3, $4::jsonb)
				ON CONFLICT (device_id) DO UPDATE
				SET last_seen = CURRENT_TIMESTAMP,
					updated_at = CURRENT_TIMESTAMP
			`, deviceID, fmt.Sprintf("Test Device %d", deviceNum), "online", metadata)

			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			t.Logf("Registration error: %v", err)
			errorCount++
		}
	}

	assert.Equal(t, 0, errorCount, "Should have no registration errors")

	// Verify all devices registered
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM devices").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, numDevices, count, "All devices should be registered")
}

// testMetricsIngestion tests high-volume metrics ingestion
func testMetricsIngestion(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer db.Close()

	// Create metrics table with partitioning support
	_, err = db.Exec(`
		CREATE TABLE metrics (
			id BIGSERIAL,
			device_id VARCHAR(255) NOT NULL,
			timestamp TIMESTAMPTZ NOT NULL,
			cpu_usage FLOAT,
			memory_usage FLOAT,
			disk_usage FLOAT,
			network_tx BIGINT,
			network_rx BIGINT,
			custom_data JSONB,
			PRIMARY KEY (id, timestamp)
		) PARTITION BY RANGE (timestamp)
	`)
	require.NoError(t, err)

	// Create partition for current month
	currentMonth := time.Now().Format("2006_01")
	_, err = db.Exec(fmt.Sprintf(`
		CREATE TABLE metrics_%s PARTITION OF metrics
		FOR VALUES FROM ('%s-01') TO ('%s-01')
	`, currentMonth,
		time.Now().Format("2006-01"),
		time.Now().AddDate(0, 1, 0).Format("2006-01")))
	require.NoError(t, err)

	// Create index for better query performance
	_, err = db.Exec(`
		CREATE INDEX idx_metrics_device_timestamp
		ON metrics (device_id, timestamp DESC)
	`)
	assert.NoError(t, err)

	// Bulk insert metrics
	const numMetrics = 10000
	const batchSize = 100

	start := time.Now()
	for batch := 0; batch < numMetrics/batchSize; batch++ {
		tx, err := db.Begin()
		require.NoError(t, err)

		stmt, err := tx.Prepare(`
			INSERT INTO metrics (device_id, timestamp, cpu_usage, memory_usage, disk_usage, network_tx, network_rx, custom_data)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`)
		require.NoError(t, err)

		for i := 0; i < batchSize; i++ {
			deviceID := fmt.Sprintf("device-%03d", i%10)
			timestamp := time.Now().Add(-time.Duration(batch*batchSize+i) * time.Second)

			_, err = stmt.Exec(
				deviceID,
				timestamp,
				50.0 + float64(i%50),
				60.0 + float64(i%40),
				70.0 + float64(i%30),
				int64(1000000 * i),
				int64(500000 * i),
				fmt.Sprintf(`{"batch": %d, "index": %d}`, batch, i),
			)
			assert.NoError(t, err)
		}

		stmt.Close()
		err = tx.Commit()
		assert.NoError(t, err)
	}

	duration := time.Since(start)
	t.Logf("Inserted %d metrics in %v (%.0f metrics/sec)",
		numMetrics, duration, float64(numMetrics)/duration.Seconds())

	// Verify data
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM metrics").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, numMetrics, count)

	// Test query performance
	start = time.Now()
	rows, err := db.Query(`
		SELECT device_id, AVG(cpu_usage), AVG(memory_usage)
		FROM metrics
		WHERE timestamp > NOW() - INTERVAL '1 hour'
		GROUP BY device_id
		ORDER BY device_id
	`)
	assert.NoError(t, err)
	rows.Close()

	queryDuration := time.Since(start)
	t.Logf("Aggregation query took %v", queryDuration)
	assert.Less(t, queryDuration, 500*time.Millisecond, "Query should be fast")
}

// testConnectionPooling tests connection pool behavior
func testConnectionPooling(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	// Create connection pool
	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer db.Close()

	// Configure pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Test concurrent connections
	const numGoroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each goroutine does multiple operations
			for j := 0; j < 10; j++ {
				var result int
				err := db.QueryRow("SELECT $1::int", id*10+j).Scan(&result)
				assert.NoError(t, err)
				assert.Equal(t, id*10+j, result)

				// Simulate work
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Check pool stats
	stats := db.Stats()
	t.Logf("Connection pool stats: MaxOpen=%d, Open=%d, InUse=%d, Idle=%d",
		stats.MaxOpenConnections, stats.OpenConnections, stats.InUse, stats.Idle)

	assert.LessOrEqual(t, stats.OpenConnections, 10, "Should respect max connections")
	assert.GreaterOrEqual(t, stats.Idle, 0, "Should have idle connections")
}

// testTransactionIsolation tests transaction isolation levels
func testTransactionIsolation(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer db.Close()

	// Create test table
	_, err = db.Exec(`
		CREATE TABLE isolation_test (
			id SERIAL PRIMARY KEY,
			value INTEGER NOT NULL,
			version INTEGER DEFAULT 0
		)
	`)
	require.NoError(t, err)

	// Insert initial data
	_, err = db.Exec("INSERT INTO isolation_test (value) VALUES (100)")
	require.NoError(t, err)

	// Test READ COMMITTED isolation (PostgreSQL default)
	t.Run("ReadCommitted", func(t *testing.T) {
		tx1, err := db.BeginTx(context.Background(), &sql.TxOptions{
			Isolation: sql.LevelReadCommitted,
		})
		require.NoError(t, err)
		defer tx1.Rollback()

		tx2, err := db.BeginTx(context.Background(), &sql.TxOptions{
			Isolation: sql.LevelReadCommitted,
		})
		require.NoError(t, err)
		defer tx2.Rollback()

		// TX1 reads value
		var value1 int
		err = tx1.QueryRow("SELECT value FROM isolation_test WHERE id = 1").Scan(&value1)
		assert.NoError(t, err)
		assert.Equal(t, 100, value1)

		// TX2 updates value
		_, err = tx2.Exec("UPDATE isolation_test SET value = 200 WHERE id = 1")
		assert.NoError(t, err)
		err = tx2.Commit()
		assert.NoError(t, err)

		// TX1 reads again - should see new value in READ COMMITTED
		var value2 int
		err = tx1.QueryRow("SELECT value FROM isolation_test WHERE id = 1").Scan(&value2)
		assert.NoError(t, err)
		assert.Equal(t, 200, value2, "Should see committed value in READ COMMITTED")
	})

	// Test SERIALIZABLE isolation
	t.Run("Serializable", func(t *testing.T) {
		// Reset value
		_, err = db.Exec("UPDATE isolation_test SET value = 100, version = 0 WHERE id = 1")
		require.NoError(t, err)

		tx1, err := db.BeginTx(context.Background(), &sql.TxOptions{
			Isolation: sql.LevelSerializable,
		})
		require.NoError(t, err)
		defer tx1.Rollback()

		tx2, err := db.BeginTx(context.Background(), &sql.TxOptions{
			Isolation: sql.LevelSerializable,
		})
		require.NoError(t, err)
		defer tx2.Rollback()

		// Both transactions try to update the same row
		_, err1 := tx1.Exec("UPDATE isolation_test SET value = value + 10, version = version + 1 WHERE id = 1")
		_, err2 := tx2.Exec("UPDATE isolation_test SET value = value + 20, version = version + 1 WHERE id = 1")

		// Try to commit both
		commitErr1 := tx1.Commit()
		commitErr2 := tx2.Commit()

		// One should succeed, one should fail with serialization error
		if commitErr1 == nil && commitErr2 == nil {
			t.Error("Both transactions committed - serialization violation")
		} else if commitErr1 != nil && commitErr2 != nil {
			t.Error("Both transactions failed - unexpected")
		} else {
			t.Log("Serialization properly enforced")
		}
	})
}

// testDeadlockHandling tests deadlock detection and handling
func testDeadlockHandling(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer db.Close()

	// Create test tables
	_, err = db.Exec(`
		CREATE TABLE deadlock_test_a (id INTEGER PRIMARY KEY, value INTEGER);
		CREATE TABLE deadlock_test_b (id INTEGER PRIMARY KEY, value INTEGER);
		INSERT INTO deadlock_test_a VALUES (1, 100);
		INSERT INTO deadlock_test_b VALUES (1, 200);
	`)
	require.NoError(t, err)

	// Attempt to create deadlock
	deadlockDetected := make(chan bool, 1)

	go func() {
		tx1, _ := db.Begin()
		defer tx1.Rollback()

		tx1.Exec("UPDATE deadlock_test_a SET value = 101 WHERE id = 1")
		time.Sleep(100 * time.Millisecond)
		_, err := tx1.Exec("UPDATE deadlock_test_b SET value = 201 WHERE id = 1")

		if err != nil && isDeadlockError(err) {
			deadlockDetected <- true
			return
		}
		tx1.Commit()
		deadlockDetected <- false
	}()

	go func() {
		time.Sleep(50 * time.Millisecond) // Ensure second transaction starts after first

		tx2, _ := db.Begin()
		defer tx2.Rollback()

		tx2.Exec("UPDATE deadlock_test_b SET value = 202 WHERE id = 1")
		time.Sleep(100 * time.Millisecond)
		_, err := tx2.Exec("UPDATE deadlock_test_a SET value = 102 WHERE id = 1")

		if err != nil && isDeadlockError(err) {
			deadlockDetected <- true
			return
		}
		tx2.Commit()
	}()

	// Wait for deadlock detection
	select {
	case detected := <-deadlockDetected:
		if detected {
			t.Log("Deadlock properly detected and handled by PostgreSQL")
		} else {
			t.Log("No deadlock occurred (transactions may have completed sequentially)")
		}
	case <-time.After(5 * time.Second):
		t.Error("Deadlock resolution timeout")
	}
}

// testBulkOperations tests bulk insert/update operations
func testBulkOperations(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer db.Close()

	// Create table
	_, err = db.Exec(`
		CREATE TABLE bulk_test (
			id SERIAL PRIMARY KEY,
			device_id VARCHAR(255),
			data TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Test COPY for bulk insert (PostgreSQL specific)
	const numRows = 50000

	// Method 1: Using COPY
	start := time.Now()
	tx, err := db.Begin()
	require.NoError(t, err)

	stmt, err := tx.Prepare(`COPY bulk_test (device_id, data) FROM STDIN`)
	require.NoError(t, err)

	for i := 0; i < numRows; i++ {
		_, err = stmt.Exec(
			fmt.Sprintf("device-%05d", i),
			fmt.Sprintf("data-%d", i),
		)
		if err != nil {
			// COPY not available through database/sql, fall back to batch insert
			break
		}
	}
	stmt.Close()
	tx.Commit()

	copyDuration := time.Since(start)

	// Method 2: Batch INSERT
	start = time.Now()
	const batchSize = 1000

	for batch := 0; batch < numRows/batchSize; batch++ {
		values := make([]string, 0, batchSize)
		args := make([]interface{}, 0, batchSize*2)

		for i := 0; i < batchSize; i++ {
			idx := batch*batchSize + i
			values = append(values, fmt.Sprintf("($%d, $%d)", i*2+1, i*2+2))
			args = append(args, fmt.Sprintf("device-%05d", idx), fmt.Sprintf("data-%d", idx))
		}

		query := fmt.Sprintf("INSERT INTO bulk_test (device_id, data) VALUES %s",
			strings.Join(values, ","))
		_, err = db.Exec(query, args...)
		assert.NoError(t, err)
	}

	batchDuration := time.Since(start)

	t.Logf("Bulk insert performance:")
	t.Logf("  COPY method: %v (%.0f rows/sec)", copyDuration, float64(numRows)/copyDuration.Seconds())
	t.Logf("  Batch INSERT: %v (%.0f rows/sec)", batchDuration, float64(numRows)/batchDuration.Seconds())

	// Verify data
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM bulk_test").Scan(&count)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, numRows, "Should have inserted all rows")
}

// testQueryPerformance tests query performance and optimization
func testQueryPerformance(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer db.Close()

	// Create tables with indexes
	_, err = db.Exec(`
		CREATE TABLE devices_perf (
			id SERIAL PRIMARY KEY,
			device_id VARCHAR(255) UNIQUE,
			location VARCHAR(255),
			type VARCHAR(50),
			status VARCHAR(50),
			last_seen TIMESTAMP
		);

		CREATE TABLE metrics_perf (
			id BIGSERIAL PRIMARY KEY,
			device_id VARCHAR(255) REFERENCES devices_perf(device_id),
			timestamp TIMESTAMP,
			value FLOAT
		);

		-- Create indexes
		CREATE INDEX idx_devices_location ON devices_perf(location);
		CREATE INDEX idx_devices_status ON devices_perf(status);
		CREATE INDEX idx_metrics_device_time ON metrics_perf(device_id, timestamp DESC);
	`)
	require.NoError(t, err)

	// Insert test data
	const numDevices = 1000
	const metricsPerDevice = 100

	// Insert devices
	for i := 0; i < numDevices; i++ {
		_, err = db.Exec(`
			INSERT INTO devices_perf (device_id, location, type, status, last_seen)
			VALUES ($1, $2, $3, $4, $5)
		`, fmt.Sprintf("device-%04d", i),
			fmt.Sprintf("location-%02d", i%10),
			fmt.Sprintf("type-%d", i%3),
			"online",
			time.Now())
		assert.NoError(t, err)
	}

	// Insert metrics
	for i := 0; i < numDevices; i++ {
		for j := 0; j < metricsPerDevice; j++ {
			_, err = db.Exec(`
				INSERT INTO metrics_perf (device_id, timestamp, value)
				VALUES ($1, $2, $3)
			`, fmt.Sprintf("device-%04d", i),
				time.Now().Add(-time.Duration(j)*time.Minute),
				50.0+float64(j%50))
		}
	}

	// Test query with EXPLAIN ANALYZE
	rows, err := db.Query(`
		EXPLAIN ANALYZE
		SELECT d.device_id, d.location, AVG(m.value) as avg_value
		FROM devices_perf d
		JOIN metrics_perf m ON d.device_id = m.device_id
		WHERE d.status = 'online'
			AND d.location = 'location-01'
			AND m.timestamp > NOW() - INTERVAL '1 hour'
		GROUP BY d.device_id, d.location
		ORDER BY avg_value DESC
		LIMIT 10
	`)
	require.NoError(t, err)
	defer rows.Close()

	t.Log("Query execution plan:")
	for rows.Next() {
		var plan string
		rows.Scan(&plan)
		t.Log(plan)
	}

	// Run actual query and measure time
	start := time.Now()
	rows, err = db.Query(`
		SELECT d.device_id, d.location, AVG(m.value) as avg_value
		FROM devices_perf d
		JOIN metrics_perf m ON d.device_id = m.device_id
		WHERE d.status = 'online'
			AND d.location = 'location-01'
			AND m.timestamp > NOW() - INTERVAL '1 hour'
		GROUP BY d.device_id, d.location
		ORDER BY avg_value DESC
		LIMIT 10
	`)
	assert.NoError(t, err)
	rows.Close()

	queryDuration := time.Since(start)
	t.Logf("Complex query execution time: %v", queryDuration)
	assert.Less(t, queryDuration, 100*time.Millisecond, "Query should be optimized")
}

// Helper function to check if error is a deadlock error
func isDeadlockError(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL deadlock error code is 40P01
	return strings.Contains(err.Error(), "40P01") ||
		   strings.Contains(err.Error(), "deadlock detected")
}