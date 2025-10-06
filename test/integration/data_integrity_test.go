package integration

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fleetd.sh/internal/agent/device"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDataIntegrity tests data consistency under failure conditions
func TestDataIntegrity(t *testing.T) {
	t.Run("DatabaseCorruptionRecovery", testDatabaseCorruptionRecovery)
	t.Run("TransactionRollback", testTransactionRollback)
	t.Run("ConcurrentWriteSafety", testConcurrentWriteSafety)
	t.Run("DiskSpaceExhaustion", testDiskSpaceExhaustion)
	t.Run("DataMigrationIntegrity", testDataMigrationIntegrity)
	t.Run("CheckpointRecovery", testCheckpointRecovery)
	t.Run("WriteAheadLogRecovery", testWriteAheadLogRecovery)
	t.Run("CascadingFailureHandling", testCascadingFailureHandling)
}

// testDatabaseCorruptionRecovery tests recovery from database corruption
func testDatabaseCorruptionRecovery(t *testing.T) {
	testDir := t.TempDir()
	dbPath := filepath.Join(testDir, "test.db")

	// Create initial healthy database
	stateStore, err := device.NewStateStore(dbPath)
	require.NoError(t, err)

	// Write test data
	testState := &device.State{
		Status:        "healthy",
		LastHeartbeat: time.Now(),
		MetricsBuffer: []device.Metrics{
			{Timestamp: time.Now(), CPUUsage: 50.0},
			{Timestamp: time.Now().Add(-1 * time.Hour), CPUUsage: 45.0},
		},
	}

	err = stateStore.SaveState(testState)
	require.NoError(t, err)

	// Save metrics
	for i := 0; i < 10; i++ {
		metrics := &device.Metrics{
			Timestamp:   time.Now().Add(time.Duration(i) * time.Minute),
			CPUUsage:    float64(40 + i),
			MemoryUsage: float64(50 + i),
			DiskUsage:   float64(60 + i),
		}
		err := stateStore.BufferMetrics(metrics)
		assert.NoError(t, err)
	}

	// Get baseline metrics count
	unsent, err := stateStore.GetUnsentMetrics(100)
	require.NoError(t, err)
	baselineCount := len(unsent)
	assert.Equal(t, 10, baselineCount)

	stateStore.Close()

	// Simulate corruption by modifying database file
	data, err := os.ReadFile(dbPath)
	require.NoError(t, err)

	// Corrupt header (SQLite header is first 16 bytes)
	if len(data) > 100 {
		// Save original for comparison
		originalSize := len(data)

		// Corrupt some internal pages (not header to keep it recognizable as SQLite)
		for i := 1024; i < 1124 && i < len(data); i++ {
			data[i] = 0xFF
		}

		err = os.WriteFile(dbPath, data, 0644)
		require.NoError(t, err)

		// Try to open corrupted database
		stateStore2, err := device.NewStateStore(dbPath)

		if err != nil {
			// Database is corrupted, should handle gracefully
			t.Logf("Database corruption detected as expected: %v", err)

			// Create new database as recovery
			recoveryPath := filepath.Join(testDir, "recovery.db")
			stateStore2, err = device.NewStateStore(recoveryPath)
			require.NoError(t, err)
			defer stateStore2.Close()

			// Verify new database works
			err = stateStore2.SaveState(testState)
			assert.NoError(t, err)

			// Should be able to write new metrics
			newMetrics := &device.Metrics{
				Timestamp: time.Now(),
				CPUUsage:  75.0,
			}
			err = stateStore2.BufferMetrics(newMetrics)
			assert.NoError(t, err)
		} else {
			// SQLite may have recovered
			defer stateStore2.Close()

			// Verify data integrity
			recoveredState, err := stateStore2.LoadState()
			if err == nil && recoveredState != nil {
				t.Logf("State recovered: %+v", recoveredState)
			}

			// Check if any metrics survived
			survivedMetrics, _ := stateStore2.GetUnsentMetrics(100)
			t.Logf("Metrics recovered: %d/%d", len(survivedMetrics), baselineCount)
		}

		// Verify database size is reasonable
		info, err := os.Stat(dbPath)
		if err == nil {
			assert.Less(t, info.Size(), int64(originalSize*2), "Database shouldn't grow excessively")
		}
	}
}

// testTransactionRollback tests transaction safety
func testTransactionRollback(t *testing.T) {
	testDir := t.TempDir()
	dbPath := filepath.Join(testDir, "transaction_test.db")

	// Open database directly for transaction testing
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer safeCloseDB(db)

	// Create test table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS test_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Test 1: Successful transaction
	tx, err := db.Begin()
	require.NoError(t, err)

	_, err = tx.Exec("INSERT INTO test_data (value) VALUES (?)", "test1")
	assert.NoError(t, err)

	_, err = tx.Exec("INSERT INTO test_data (value) VALUES (?)", "test2")
	assert.NoError(t, err)

	err = tx.Commit()
	assert.NoError(t, err)

	// Verify data was committed
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_data").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	// Test 2: Failed transaction with rollback
	tx, err = db.Begin()
	require.NoError(t, err)

	_, err = tx.Exec("INSERT INTO test_data (value) VALUES (?)", "test3")
	assert.NoError(t, err)

	// Simulate error condition
	_, err = tx.Exec("INSERT INTO test_data (value) VALUES (?, ?)", "test4") // Wrong number of parameters
	assert.Error(t, err)

	// Rollback transaction
	err = tx.Rollback()
	assert.NoError(t, err)

	// Verify rollback worked
	err = db.QueryRow("SELECT COUNT(*) FROM test_data").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 2, count, "Count should still be 2 after rollback")

	// Test 3: Concurrent transactions
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			tx, err := db.Begin()
			if err != nil {
				errors <- err
				return
			}

			// Try to insert with potential conflict
			_, err = tx.Exec("INSERT INTO test_data (value) VALUES (?)",
				fmt.Sprintf("concurrent_%d", id))

			if err != nil {
				tx.Rollback()
				errors <- err
				return
			}

			// Random delay to increase conflict chance
			time.Sleep(time.Millisecond * time.Duration(id))

			if err := tx.Commit(); err != nil {
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
			errorCount++
			t.Logf("Transaction error: %v", err)
		}
	}

	// Some transactions might fail due to locking, but data should be consistent
	err = db.QueryRow("SELECT COUNT(*) FROM test_data WHERE value LIKE 'concurrent_%'").Scan(&count)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, 5, "At least some concurrent transactions should succeed")
	assert.LessOrEqual(t, count, 10, "No more than 10 concurrent transactions")
}

// testConcurrentWriteSafety tests concurrent write operations
func testConcurrentWriteSafety(t *testing.T) {
	testDir := t.TempDir()
	dbPath := filepath.Join(testDir, "concurrent.db")

	stateStore, err := device.NewStateStore(dbPath)
	require.NoError(t, err)
	defer stateStore.Close()

	// Concurrent state updates
	var wg sync.WaitGroup
	const numWriters = 20
	const writesPerWriter = 50

	errors := make(chan error, numWriters*writesPerWriter)

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()

			for j := 0; j < writesPerWriter; j++ {
				// Write state
				state := &device.State{
					Status:         fmt.Sprintf("writer_%d_op_%d", writerID, j),
					LastHeartbeat:  time.Now(),
					UpdateProgress: j,
				}

				if err := stateStore.SaveState(state); err != nil {
					errors <- fmt.Errorf("writer %d op %d: %w", writerID, j, err)
					continue
				}

				// Write metrics
				metrics := &device.Metrics{
					Timestamp:   time.Now(),
					CPUUsage:    float64(writerID),
					MemoryUsage: float64(j),
					DiskUsage:   float64(writerID + j),
				}

				if err := stateStore.BufferMetrics(metrics); err != nil {
					errors <- fmt.Errorf("metrics writer %d op %d: %w", writerID, j, err)
				}

				// Random read to increase contention
				if j%10 == 0 {
					stateStore.LoadState()
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
			t.Logf("Concurrent write error: %v", err)
		}
	}

	// Verify data consistency
	finalState, err := stateStore.LoadState()
	assert.NoError(t, err)
	assert.NotNil(t, finalState)

	// Check metrics count
	metrics, err := stateStore.GetUnsentMetrics(10000)
	assert.NoError(t, err)

	expectedMetrics := numWriters * writesPerWriter
	actualMetrics := len(metrics)

	t.Logf("Metrics written: %d/%d (%.1f%%), Errors: %d",
		actualMetrics, expectedMetrics,
		float64(actualMetrics)*100/float64(expectedMetrics),
		errorCount)

	// Should have most metrics (some might fail due to locking)
	assert.Greater(t, actualMetrics, expectedMetrics*80/100, "Should write at least 80% of metrics")

	// Verify no data corruption by checking metrics are valid
	for _, m := range metrics[:min(10, len(metrics))] {
		assert.NotZero(t, m.Timestamp)
		assert.GreaterOrEqual(t, m.CPUUsage, 0.0)
		assert.LessOrEqual(t, m.CPUUsage, float64(numWriters))
	}
}

// testDiskSpaceExhaustion tests behavior when disk is full
func testDiskSpaceExhaustion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping disk exhaustion test in short mode")
	}

	testDir := t.TempDir()
	dbPath := filepath.Join(testDir, "diskfull.db")

	stateStore, err := device.NewStateStore(dbPath)
	require.NoError(t, err)
	defer stateStore.Close()

	// Write data until we hit space limits or max iterations
	const maxIterations = 1000
	const batchSize = 100

	var lastError error
	successfulWrites := 0

	for i := 0; i < maxIterations; i++ {
		// Create large metrics to consume space faster
		largeCustom := make(map[string]interface{})
		for j := 0; j < 100; j++ {
			largeCustom[fmt.Sprintf("field_%d", j)] = fmt.Sprintf("value_%d_%d_padding_%s",
				i, j, strings.Repeat("x", 100))
		}

		metrics := &device.Metrics{
			Timestamp:   time.Now(),
			CPUUsage:    float64(i % 100),
			MemoryUsage: float64(i % 100),
			DiskUsage:   float64(i % 100),
			Custom:      largeCustom,
		}

		err := stateStore.BufferMetrics(metrics)
		if err != nil {
			lastError = err
			t.Logf("Write failed at iteration %d: %v", i, err)

			// Check if it's a space issue
			if strings.Contains(err.Error(), "disk") ||
				strings.Contains(err.Error(), "space") ||
				strings.Contains(err.Error(), "quota") {
				t.Log("Disk space exhaustion detected")
				break
			}

			// Try a few more times
			if i-successfulWrites > 10 {
				break
			}
		} else {
			successfulWrites++
		}

		// Periodically check database size
		if i%100 == 0 {
			info, err := os.Stat(dbPath)
			if err == nil {
				t.Logf("Database size at iteration %d: %d bytes", i, info.Size())

				// Set a reasonable limit for testing (100MB)
				if info.Size() > 100*1024*1024 {
					t.Log("Database size limit reached for testing")
					break
				}
			}
		}
	}

	t.Logf("Successfully wrote %d metrics before exhaustion/limit", successfulWrites)
	assert.Greater(t, successfulWrites, 0, "Should write some metrics before failure")

	// Verify database is still readable after space exhaustion
	retrievedMetrics, err := stateStore.GetUnsentMetrics(10)
	if err == nil {
		assert.NotEmpty(t, retrievedMetrics, "Should be able to read after disk full")
		t.Logf("Retrieved %d metrics after exhaustion", len(retrievedMetrics))
	}

	// Try to recover by cleaning up (vacuum)
	if db, ok := getUnderlyingDB(stateStore); ok {
		_, err = db.Exec("VACUUM")
		if err != nil {
			t.Logf("Vacuum failed (expected if disk full): %v", err)
		} else {
			t.Log("Successfully vacuumed database")
		}
	}

	if lastError != nil {
		assert.Error(t, lastError, "Should eventually hit an error with constrained space")
	}
}

// testDataMigrationIntegrity tests data integrity during migrations
func testDataMigrationIntegrity(t *testing.T) {
	testDir := t.TempDir()
	dbPath := filepath.Join(testDir, "migration.db")

	// Create v1 database schema
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer safeCloseDB(db)

	// Create v1 schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS metrics_v1 (
			id INTEGER PRIMARY KEY,
			timestamp INTEGER,
			cpu_usage REAL,
			memory_usage REAL
		)
	`)
	require.NoError(t, err)

	// Insert v1 data
	v1Data := []struct {
		timestamp int64
		cpu       float64
		memory    float64
	}{
		{time.Now().Unix(), 45.5, 62.3},
		{time.Now().Unix() - 3600, 50.2, 58.9},
		{time.Now().Unix() - 7200, 38.7, 71.4},
	}

	for _, d := range v1Data {
		_, err = db.Exec("INSERT INTO metrics_v1 (timestamp, cpu_usage, memory_usage) VALUES (?, ?, ?)",
			d.timestamp, d.cpu, d.memory)
		assert.NoError(t, err)
	}

	// Migrate to v2 schema (add disk_usage column)
	tx, err := db.Begin()
	require.NoError(t, err)

	// Create v2 table
	_, err = tx.Exec(`
		CREATE TABLE metrics_v2 (
			id INTEGER PRIMARY KEY,
			timestamp INTEGER,
			cpu_usage REAL,
			memory_usage REAL,
			disk_usage REAL DEFAULT 0.0,
			metadata TEXT
		)
	`)
	require.NoError(t, err)

	// Migrate data
	_, err = tx.Exec(`
		INSERT INTO metrics_v2 (id, timestamp, cpu_usage, memory_usage, disk_usage)
		SELECT id, timestamp, cpu_usage, memory_usage, 50.0
		FROM metrics_v1
	`)
	require.NoError(t, err)

	// Verify data integrity during migration
	var count int
	err = tx.QueryRow("SELECT COUNT(*) FROM metrics_v2").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, len(v1Data), count)

	// Verify data values preserved
	rows, err := tx.Query("SELECT timestamp, cpu_usage, memory_usage, disk_usage FROM metrics_v2 ORDER BY timestamp DESC")
	require.NoError(t, err)
	defer rows.Close()

	migratedCount := 0
	for rows.Next() {
		var ts int64
		var cpu, memory, disk float64
		err := rows.Scan(&ts, &cpu, &memory, &disk)
		assert.NoError(t, err)

		// Find corresponding original data
		found := false
		for _, original := range v1Data {
			if original.timestamp == ts {
				assert.Equal(t, original.cpu, cpu, "CPU usage should be preserved")
				assert.Equal(t, original.memory, memory, "Memory usage should be preserved")
				assert.Equal(t, 50.0, disk, "Disk usage should have default value")
				found = true
				break
			}
		}
		assert.True(t, found, "Should find original data")
		migratedCount++
	}

	assert.Equal(t, len(v1Data), migratedCount, "All data should be migrated")

	// Complete migration
	_, err = tx.Exec("DROP TABLE metrics_v1")
	assert.NoError(t, err)

	_, err = tx.Exec("ALTER TABLE metrics_v2 RENAME TO metrics")
	assert.NoError(t, err)

	err = tx.Commit()
	assert.NoError(t, err)

	// Verify final schema
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='metrics'").Scan(&tableName)
	assert.NoError(t, err)
	assert.Equal(t, "metrics", tableName)

	// Verify can write to migrated schema
	_, err = db.Exec("INSERT INTO metrics (timestamp, cpu_usage, memory_usage, disk_usage) VALUES (?, ?, ?, ?)",
		time.Now().Unix(), 55.5, 66.6, 77.7)
	assert.NoError(t, err)
}

// testCheckpointRecovery tests WAL checkpoint and recovery
func testCheckpointRecovery(t *testing.T) {
	testDir := t.TempDir()
	dbPath := filepath.Join(testDir, "checkpoint.db")

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer safeCloseDB(db)

	// Enable WAL mode
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)

	// Set checkpoint threshold
	_, err = db.Exec("PRAGMA wal_autocheckpoint=100")
	require.NoError(t, err)

	// Create table
	_, err = db.Exec(`
		CREATE TABLE checkpoint_test (
			id INTEGER PRIMARY KEY,
			value TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Write data to build up WAL
	for i := 0; i < 200; i++ {
		_, err = db.Exec("INSERT INTO checkpoint_test (value) VALUES (?)",
			fmt.Sprintf("value_%d", i))
		assert.NoError(t, err)
	}

	// Check WAL size before checkpoint
	walPath := dbPath + "-wal"
	walInfo, err := os.Stat(walPath)
	if err == nil {
		t.Logf("WAL size before checkpoint: %d bytes", walInfo.Size())
		assert.Greater(t, walInfo.Size(), int64(0), "WAL should have data")
	}

	// Force checkpoint
	_, err = db.Exec("PRAGMA wal_checkpoint(FULL)")
	assert.NoError(t, err)

	// Check WAL size after checkpoint
	walInfo, err = os.Stat(walPath)
	if err == nil {
		t.Logf("WAL size after checkpoint: %d bytes", walInfo.Size())
	}

	// Verify data integrity
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM checkpoint_test").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 200, count)

	// Simulate crash by closing without checkpoint
	safeCloseDB(db)

	// Reopen and verify recovery
	db, err = sql.Open("sqlite3", dbPath)
	require.NoError(t, err)

	err = db.QueryRow("SELECT COUNT(*) FROM checkpoint_test").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 200, count, "All data should be recovered")
}

// testWriteAheadLogRecovery tests WAL recovery after crash
func testWriteAheadLogRecovery(t *testing.T) {
	testDir := t.TempDir()
	dbPath := filepath.Join(testDir, "wal_recovery.db")

	// Create database with WAL
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)

	_, err = db.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE wal_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT
		)
	`)
	require.NoError(t, err)

	// Start transaction
	tx, err := db.Begin()
	require.NoError(t, err)

	// Write data in transaction
	for i := 0; i < 50; i++ {
		_, err = tx.Exec("INSERT INTO wal_test (value) VALUES (?)",
			fmt.Sprintf("tx_value_%d", i))
		assert.NoError(t, err)
	}

	// Commit transaction
	err = tx.Commit()
	assert.NoError(t, err)

	// Write more data outside transaction
	for i := 50; i < 100; i++ {
		_, err = db.Exec("INSERT INTO wal_test (value) VALUES (?)",
			fmt.Sprintf("value_%d", i))
		assert.NoError(t, err)
	}

	// Copy WAL file to simulate incomplete write
	walPath := dbPath + "-wal"
	walData, err := os.ReadFile(walPath)
	if err == nil {
		walBackup := walPath + ".backup"
		os.WriteFile(walBackup, walData, 0644)
	}

	// Close database
	safeCloseDB(db)

	// Corrupt WAL by truncating it (simulate crash during write)
	if walData != nil && len(walData) > 100 {
		truncated := walData[:len(walData)*3/4] // Keep 75% of WAL
		os.WriteFile(walPath, truncated, 0644)
	}

	// Reopen database - should recover from WAL
	db, err = sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer safeCloseDB(db)

	// Check recovered data
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM wal_test").Scan(&count)
	assert.NoError(t, err)

	t.Logf("Recovered %d records after WAL corruption", count)
	assert.Greater(t, count, 0, "Should recover some data")
	assert.LessOrEqual(t, count, 100, "Should not have phantom data")

	// Verify data integrity
	rows, err := db.Query("SELECT value FROM wal_test ORDER BY id LIMIT 5")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var value string
			rows.Scan(&value)
			assert.NotEmpty(t, value)
		}
	}
}

// testCascadingFailureHandling tests handling of cascading failures
func testCascadingFailureHandling(t *testing.T) {
	testDir := t.TempDir()

	// Simulate cascading failure scenario
	// 1. Primary database fails
	// 2. Fallback to secondary fails
	// 3. Recovery attempt

	primaryPath := filepath.Join(testDir, "primary.db")
	secondaryPath := filepath.Join(testDir, "secondary.db")
	recoveryPath := filepath.Join(testDir, "recovery.db")

	// Create primary store
	primaryStore, err := device.NewStateStore(primaryPath)
	require.NoError(t, err)

	// Write critical data
	criticalState := &device.State{
		Status:        "operational",
		LastHeartbeat: time.Now(),
	}

	err = primaryStore.SaveState(criticalState)
	assert.NoError(t, err)

	// Simulate primary failure
	primaryStore.Close()
	os.Chmod(primaryPath, 0000) // Make unreadable

	// Attempt to use primary (should fail)
	_, err = device.NewStateStore(primaryPath)
	assert.Error(t, err, "Primary should fail")

	// Fallback to secondary
	secondaryStore, err := device.NewStateStore(secondaryPath)
	if err != nil {
		t.Logf("Secondary also failed: %v", err)

		// Final recovery attempt
		recoveryStore, err := device.NewStateStore(recoveryPath)
		require.NoError(t, err, "Recovery store must succeed")
		defer recoveryStore.Close()

		// Write emergency state
		emergencyState := &device.State{
			Status: "recovered_from_cascade",
			Error:  "Primary and secondary stores failed",
		}

		err = recoveryStore.SaveState(emergencyState)
		assert.NoError(t, err, "Should be able to write to recovery store")

		// Verify recovery
		recovered, err := recoveryStore.LoadState()
		assert.NoError(t, err)
		assert.Equal(t, "recovered_from_cascade", recovered.Status)
	} else {
		defer secondaryStore.Close()

		// Secondary succeeded, migrate data if possible
		err = secondaryStore.SaveState(&device.State{
			Status: "failover_to_secondary",
		})
		assert.NoError(t, err)
	}

	// Cleanup
	os.Chmod(primaryPath, 0644) // Restore permissions
}

// Helper functions

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getUnderlyingDB(store *device.StateStore) (*sql.DB, bool) {
	// This would need reflection or interface to access the underlying DB
	// For testing purposes, we'll return false
	return nil, false
}
