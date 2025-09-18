package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newDbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management commands",
		Long:  `Manage Fleet database including migrations, seeding, and maintenance.`,
	}

	cmd.AddCommand(
		newDbMigrateCmd(),
		newDbResetCmd(),
		newDbSeedCmd(),
		newDbCreateMigrationCmd(),
		newDbStatusCmd(),
	)

	return cmd
}

func newDbMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		Long:  `Apply all pending database migrations to bring the database schema up to date.`,
		RunE:  runDbMigrate,
	}
}

func newDbResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset database",
		Long:  `Drop and recreate the database, then run all migrations. WARNING: This will delete all data!`,
		RunE:  runDbReset,
	}
}

func newDbSeedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Seed database with sample data",
		Long:  `Populate the database with sample data for development and testing.`,
		RunE:  runDbSeed,
	}
}

func newDbCreateMigrationCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create-migration [name]",
		Short: "Create a new migration file",
		Long:  `Create a new timestamped migration file in the migrations directory.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runDbCreateMigration,
	}
}

func newDbStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		Long:  `Display the current migration status and list of pending migrations.`,
		RunE:  runDbStatus,
	}
}

func runDbMigrate(cmd *cobra.Command, args []string) error {
	printHeader("Running database migrations")
	fmt.Println()

	// Get database connection
	db, err := getDBConnection()
	if err != nil {
		printError("Failed to connect to database: %v", err)
		return err
	}
	defer db.Close()

	// Ensure migrations table exists
	if err := createMigrationsTable(db); err != nil {
		printError("Failed to create migrations table: %v", err)
		return err
	}

	// Get migration files
	migrations, err := getMigrationFiles()
	if err != nil {
		printError("Failed to read migrations: %v", err)
		return err
	}

	if len(migrations) == 0 {
		printInfo("No migration files found")
		return nil
	}

	// Get applied migrations
	applied, err := getAppliedMigrations(db)
	if err != nil {
		printError("Failed to get applied migrations: %v", err)
		return err
	}

	// Run pending migrations
	pending := 0
	for _, migration := range migrations {
		if _, ok := applied[migration]; ok {
			continue
		}

		pending++
		printInfo("Applying migration: %s", migration)

		if err := applyMigration(db, migration); err != nil {
			printError("Failed to apply migration %s: %v", migration, err)
			return err
		}

		printSuccess("Applied: %s", migration)
	}

	if pending == 0 {
		printInfo("Database is up to date")
	} else {
		printSuccess("Applied %d migration(s)", pending)
	}

	return nil
}

func runDbReset(cmd *cobra.Command, args []string) error {
	printHeader("Resetting database")
	printWarning("This will DELETE ALL DATA in the database!")
	fmt.Print("Are you sure? Type 'yes' to confirm: ")

	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		printInfo("Reset cancelled")
		return nil
	}

	// Connect to postgres database to drop/create
	dbName := viper.GetString("db.name")
	if dbName == "" {
		dbName = "fleetd"
	}

	// Connect without database name to drop/create
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s sslmode=disable",
		viper.GetString("db.host"),
		viper.GetInt("db.port"),
		viper.GetString("db.user"),
		viper.GetString("db.password"),
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		printError("Failed to connect to PostgreSQL: %v", err)
		return err
	}
	defer db.Close()

	// Drop database if exists
	printInfo("Dropping database: %s", dbName)
	// Validate database name to prevent SQL injection
	if !isValidDatabaseName(dbName) {
		return fmt.Errorf("invalid database name: %s", dbName)
	}
	_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %q", dbName))
	if err != nil {
		printError("Failed to drop database: %v", err)
		return err
	}

	// Create database
	printInfo("Creating database: %s", dbName)
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %q", dbName))
	if err != nil {
		printError("Failed to create database: %v", err)
		return err
	}

	printSuccess("Database reset complete")

	// Run migrations
	printInfo("Running migrations...")
	return runDbMigrate(cmd, args)
}

func runDbSeed(cmd *cobra.Command, args []string) error {
	printHeader("Seeding database")

	db, err := getDBConnection()
	if err != nil {
		printError("Failed to connect to database: %v", err)
		return err
	}
	defer db.Close()

	// Check for seed files
	seedFile := filepath.Join(getProjectRoot(), "migrations", "seed.sql")
	if _, err := os.Stat(seedFile); err != nil {
		printWarning("No seed file found at %s", seedFile)
		printInfo("Creating sample seed data...")

		// Create sample data
		if err := createSampleSeedData(db); err != nil {
			printError("Failed to create seed data: %v", err)
			return err
		}
	} else {
		// Execute seed file
		content, err := os.ReadFile(seedFile)
		if err != nil {
			printError("Failed to read seed file: %v", err)
			return err
		}

		if _, err := db.Exec(string(content)); err != nil {
			printError("Failed to execute seed file: %v", err)
			return err
		}
	}

	printSuccess("Database seeded successfully")
	return nil
}

func runDbCreateMigration(cmd *cobra.Command, args []string) error {
	name := args[0]
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s.sql", timestamp, strings.ToLower(name))

	migrationsDir := filepath.Join(getProjectRoot(), "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		printError("Failed to create migrations directory: %v", err)
		return err
	}

	migrationPath := filepath.Join(migrationsDir, filename)

	template := fmt.Sprintf(`-- Migration: %s
-- Created: %s

-- Up Migration


-- Down Migration (optional)
-- Note: Down migrations are not automatically applied
`, name, time.Now().Format("2006-01-02 15:04:05"))

	if err := os.WriteFile(migrationPath, []byte(template), 0o644); err != nil {
		printError("Failed to create migration file: %v", err)
		return err
	}

	printSuccess("Created migration: %s", filename)
	printInfo("Edit the migration file at: %s", migrationPath)

	return nil
}

func runDbStatus(cmd *cobra.Command, args []string) error {
	printHeader("Database Migration Status")
	fmt.Println()

	db, err := getDBConnection()
	if err != nil {
		printError("Failed to connect to database: %v", err)
		return err
	}
	defer db.Close()

	// Check if migrations table exists
	var tableExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'schema_migrations'
		)
	`).Scan(&tableExists)

	if err != nil || !tableExists {
		printWarning("Migrations table does not exist")
		printInfo("Run 'fleet db migrate' to initialize")
		return nil
	}

	// Get applied migrations
	applied, err := getAppliedMigrations(db)
	if err != nil {
		printError("Failed to get applied migrations: %v", err)
		return err
	}

	// Get all migration files
	migrations, err := getMigrationFiles()
	if err != nil {
		printError("Failed to read migrations: %v", err)
		return err
	}

	// Display status
	printInfo("Total migrations: %d", len(migrations))
	printInfo("Applied migrations: %d", len(applied))
	fmt.Println()

	pending := 0
	for _, migration := range migrations {
		if timestamp, ok := applied[migration]; ok {
			fmt.Printf("%s %s (applied: %s)\n", green("[OK]"), migration, timestamp.Format("2006-01-02 15:04:05"))
		} else {
			fmt.Printf("%s %s (pending)\n", yellow("○"), migration)
			pending++
		}
	}

	if pending > 0 {
		fmt.Println()
		printWarning("%d pending migration(s)", pending)
		printInfo("Run 'fleet db migrate' to apply pending migrations")
	} else {
		fmt.Println()
		printSuccess("Database is up to date")
	}

	return nil
}

func getDBConnection() (*sql.DB, error) {
	host := viper.GetString("db.host")
	if host == "" {
		host = os.Getenv("DB_HOST")
		if host == "" {
			host = "localhost"
		}
	}

	port := viper.GetInt("db.port")
	if port == 0 {
		portStr := os.Getenv("DB_PORT")
		if portStr != "" {
			fmt.Sscanf(portStr, "%d", &port)
		}
		if port == 0 {
			port = 5432
		}
	}

	dbName := viper.GetString("db.name")
	if dbName == "" {
		dbName = os.Getenv("DB_NAME")
		if dbName == "" {
			dbName = "fleetd"
		}
	}

	user := viper.GetString("db.user")
	if user == "" {
		user = os.Getenv("DB_USER")
		if user == "" {
			user = "fleetd"
		}
	}

	password := viper.GetString("db.password")
	if password == "" {
		password = os.Getenv("DB_PASSWORD")
		if password == "" {
			// Don't use a default password in production
			if os.Getenv("FLEET_ENV") == "production" {
				return nil, fmt.Errorf("database password not configured")
			}
			// Only use default in development
			password = "fleetd_dev"
		}
	}

	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbName,
	)

	// Add connection timeout
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

func createMigrationsTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	_, err := db.Exec(query)
	return err
}

func getMigrationFiles() ([]string, error) {
	migrationsDir := filepath.Join(getProjectRoot(), "migrations")

	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, err
	}

	var migrations []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sql") {
			migrations = append(migrations, file.Name())
		}
	}

	return migrations, nil
}

func getAppliedMigrations(db *sql.DB) (map[string]time.Time, error) {
	rows, err := db.Query("SELECT version, applied_at FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]time.Time)
	for rows.Next() {
		var version string
		var appliedAt time.Time
		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, err
		}
		applied[version] = appliedAt
	}

	return applied, nil
}

func applyMigration(db *sql.DB, filename string) error {
	migrationPath := filepath.Join(getProjectRoot(), "migrations", filename)
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		return err
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration
	if _, err := tx.Exec(string(content)); err != nil {
		return err
	}

	// Record migration
	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version) VALUES ($1)",
		filename,
	); err != nil {
		return err
	}

	// Commit transaction
	return tx.Commit()
}

// isValidDatabaseName validates database name to prevent SQL injection
func isValidDatabaseName(name string) bool {
	// Allow only alphanumeric characters, underscores, and hyphens
	// Must start with a letter
	validName := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)
	return validName.MatchString(name) && len(name) <= 63 // PostgreSQL limit
}

func createSampleSeedData(db *sql.DB) error {
	// Create sample seed data
	// This is a placeholder - actual implementation would create relevant test data

	seedSQL := `
	-- Sample seed data for Fleet

	-- Insert sample devices (if table exists)
	INSERT INTO devices (id, name, status)
	SELECT 'device-001', 'Test Device 1', 'online'
	WHERE EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'devices')
	AND NOT EXISTS (SELECT 1 FROM devices WHERE id = 'device-001');
	`

	_, err := db.Exec(seedSQL)
	return err
}
