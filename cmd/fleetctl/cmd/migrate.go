package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fleetd.sh/internal/database"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newMigrateCmd creates the migrate command
func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Database migration tools",
		Long:  `Manage database schema migrations for the fleet server`,
	}

	cmd.AddCommand(
		newMigrateUpCmd(),
		newMigrateDownCmd(),
		newMigrateStatusCmd(),
		newMigrateCreateCmd(),
		newMigrateResetCmd(),
	)

	return cmd
}

// newMigrateUpCmd runs migrations up
func newMigrateUpCmd() *cobra.Command {
	var (
		steps   int
		version string
	)

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Run pending migrations",
		Long:  `Apply all pending database migrations`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if required services are running
			if err := ensureServicesRunning(true); err != nil {
				return err
			}

			printInfo("Running database migrations...")

			// Get database connection
			db, driver, err := getMigrationDB()
			if err != nil {
				// Provide helpful error message based on the error type
				if strings.Contains(err.Error(), "connection refused") {
					printError("Cannot connect to PostgreSQL database")
					printInfo("\nPlease ensure the Fleet platform is running:")
					printInfo("  fleetctl start")
					printInfo("\nTo check service status:")
					printInfo("  fleetctl status")
					return fmt.Errorf("database not available")
				}
				printError("Failed to connect to database: %v", err)
				return err
			}
			defer db.Close()

			// Create migrator
			migrator, err := database.NewMigrator(nil)
			if err != nil {
				return err
			}
			defer migrator.Close()

			if err := migrator.Initialize(db, driver); err != nil {
				printError("Failed to initialize migrator: %v", err)
				return err
			}

			ctx := context.Background()

			// Handle specific version or steps
			if version != "" {
				var targetVersion uint
				fmt.Sscanf(version, "%d", &targetVersion)
				if err := migrator.Migrate(ctx, targetVersion); err != nil {
					printError("Failed to migrate to version %d: %v", targetVersion, err)
					return err
				}
			} else {
				// Run all migrations
				if err := migrator.Up(ctx); err != nil {
					printError("Failed to run migrations: %v", err)
					return err
				}
			}

			// Get current version
			currentVersion, dirty, _ := migrator.Version()
			if dirty {
				printWarning("Database is in dirty state at version %d", currentVersion)
			} else {
				printSuccess("Successfully migrated to version %d", currentVersion)
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&steps, "steps", 0, "Number of migrations to run (0 = all)")
	cmd.Flags().StringVar(&version, "version", "", "Migrate to specific version")

	return cmd
}

// newMigrateDownCmd rolls back migrations
func newMigrateDownCmd() *cobra.Command {
	var (
		steps   int
		version string
		force   bool
	)

	cmd := &cobra.Command{
		Use:   "down",
		Short: "Rollback migrations",
		Long:  `Rollback database migrations`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				printWarning("This will rollback database migrations and may result in data loss")
				fmt.Print("Continue? [y/N]: ")
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Cancelled")
					return nil
				}
			}

			printInfo("Rolling back migrations...")

			// Get database connection
			db, driver, err := getMigrationDB()
			if err != nil {
				printError("Failed to connect to database: %v", err)
				return err
			}
			defer db.Close()

			// Create migrator
			migrator, err := database.NewMigrator(nil)
			if err != nil {
				return err
			}
			defer migrator.Close()

			if err := migrator.Initialize(db, driver); err != nil {
				printError("Failed to initialize migrator: %v", err)
				return err
			}

			ctx := context.Background()

			if version != "" {
				// Rollback to specific version
				var targetVersion uint
				fmt.Sscanf(version, "%d", &targetVersion)
				if err := migrator.Migrate(ctx, targetVersion); err != nil {
					printError("Failed to rollback to version %d: %v", targetVersion, err)
					return err
				}
			} else {
				// Rollback N steps (default 1)
				if steps == 0 {
					steps = 1
				}
				for i := 0; i < steps; i++ {
					if err := migrator.Down(ctx); err != nil {
						printError("Failed to rollback migration: %v", err)
						return err
					}
				}
			}

			// Get current version
			currentVersion, dirty, _ := migrator.Version()
			if dirty {
				printWarning("Database is in dirty state at version %d", currentVersion)
			} else {
				printSuccess("Successfully rolled back to version %d", currentVersion)
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&steps, "steps", 1, "Number of migrations to rollback")
	cmd.Flags().StringVar(&version, "version", "", "Rollback to specific version")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

// newMigrateStatusCmd shows migration status
func newMigrateStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		Long:  `Display current database migration status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			printInfo("Database Migration Status")
			fmt.Println()

			// Get database connection
			db, driver, err := getMigrationDB()
			if err != nil {
				printError("Failed to connect to database: %v", err)
				return err
			}
			defer db.Close()

			// Create migrator
			migrator, err := database.NewMigrator(nil)
			if err != nil {
				return err
			}
			defer migrator.Close()

			if err := migrator.Initialize(db, driver); err != nil {
				printError("Failed to initialize migrator: %v", err)
				return err
			}

			// Get current version
			currentVersion, dirty, err := migrator.Version()
			if err != nil {
				printError("Failed to get migration version: %v", err)
				return err
			}

			fmt.Printf("%s\n", bold("Current Status:"))
			fmt.Printf("  Version: %d\n", currentVersion)
			if dirty {
				fmt.Printf("  State:   %s\n", red("DIRTY"))
				printWarning("Database is in dirty state. Manual intervention may be required.")
			} else {
				fmt.Printf("  State:   %s\n", green("CLEAN"))
			}

			// List migration files
			migrationsDir := filepath.Join("internal", "database", "migrations")
			files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
			if err == nil && len(files) > 0 {
				fmt.Printf("\n%s\n", bold("Available Migrations:"))
				applied := 0
				pending := 0

				for _, file := range files {
					base := filepath.Base(file)
					parts := strings.Split(base, "_")
					if len(parts) > 0 {
						var version uint
						fmt.Sscanf(parts[0], "%03d", &version)
						name := strings.TrimSuffix(strings.Join(parts[1:], "_"), ".up.sql")

						if version <= currentVersion {
							fmt.Printf("  %s %03d_%s\n", green("[✓]"), version, name)
							applied++
						} else {
							fmt.Printf("  %s %03d_%s\n", yellow("[ ]"), version, name)
							pending++
						}
					}
				}

				fmt.Printf("\n%s\n", bold("Summary:"))
				fmt.Printf("  Applied: %d\n", applied)
				fmt.Printf("  Pending: %d\n", pending)
			}

			return nil
		},
	}

	return cmd
}

// newMigrateCreateCmd creates a new migration
func newMigrateCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new migration",
		Long:  `Generate a new database migration file`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			printInfo("Creating new migration: %s", name)

			// Generate migration files
			migrationsDir := filepath.Join("internal", "database", "migrations")
			if err := os.MkdirAll(migrationsDir, 0755); err != nil {
				printError("Failed to create migrations directory: %v", err)
				return err
			}

			// Find next version number
			files, _ := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
			nextVersion := len(files) + 1

			// Create migration files
			upFile := filepath.Join(migrationsDir, fmt.Sprintf("%03d_%s.up.sql", nextVersion, name))
			downFile := filepath.Join(migrationsDir, fmt.Sprintf("%03d_%s.down.sql", nextVersion, name))

			// Write up migration
			upContent := fmt.Sprintf(`-- Migration: %s
-- Version: %03d
-- Date: %s

-- Add your UP migration SQL here
`, name, nextVersion, time.Now().Format("2006-01-02 15:04:05"))

			if err := os.WriteFile(upFile, []byte(upContent), 0644); err != nil {
				printError("Failed to create up migration: %v", err)
				return err
			}

			// Write down migration
			downContent := fmt.Sprintf(`-- Migration: %s (rollback)
-- Version: %03d
-- Date: %s

-- Add your DOWN migration SQL here
`, name, nextVersion, time.Now().Format("2006-01-02 15:04:05"))

			if err := os.WriteFile(downFile, []byte(downContent), 0644); err != nil {
				printError("Failed to create down migration: %v", err)
				return err
			}

			printSuccess("Created migration files:")
			fmt.Printf("  - %s\n", upFile)
			fmt.Printf("  - %s\n", downFile)

			printInfo("Edit the migration files and run 'fleetctl migrate up' to apply")

			return nil
		},
	}

	return cmd
}

// newMigrateResetCmd resets the database
func newMigrateResetCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset database to initial state",
		Long:  `Drop all tables and re-run all migrations from scratch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				printError("WARNING: This will DELETE ALL DATA in the database!")
				fmt.Print("Type 'DELETE ALL DATA' to confirm: ")
				var response string
				fmt.Scanln(&response)
				if response != "DELETE ALL DATA" {
					fmt.Println("Cancelled")
					return nil
				}
			}

			printInfo("Resetting database...")

			// Get database connection
			db, driver, err := getMigrationDB()
			if err != nil {
				printError("Failed to connect to database: %v", err)
				return err
			}
			defer db.Close()

			// Create migrator
			migrator, err := database.NewMigrator(nil)
			if err != nil {
				return err
			}
			defer migrator.Close()

			if err := migrator.Initialize(db, driver); err != nil {
				printError("Failed to initialize migrator: %v", err)
				return err
			}

			ctx := context.Background()

			fmt.Printf("  %s Dropping all tables...\n", red("[X]"))
			fmt.Printf("  %s Creating fresh schema...\n", cyan("→"))
			fmt.Printf("  %s Running initial migrations...\n", cyan("→"))

			if err := migrator.Reset(ctx); err != nil {
				printError("Failed to reset database: %v", err)
				return err
			}

			printSuccess("Database reset completed")
			printInfo("Database is now at initial state")

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

// getMigrationDB returns a database connection suitable for migrations
func getMigrationDB() (*sql.DB, string, error) {
	// Check if we should use SQLite for development
	dbType := viper.GetString("database.type")
	if dbType == "" {
		dbType = os.Getenv("DATABASE_TYPE")
	}
	if dbType == "" {
		dbType = "postgres" // Default to PostgreSQL
	}

	switch dbType {
	case "sqlite", "sqlite3":
		dbPath := viper.GetString("database.path")
		if dbPath == "" {
			dbPath = os.Getenv("DATABASE_PATH")
		}
		if dbPath == "" {
			dbPath = "fleetd.db"
		}

		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			return nil, "", err
		}

		// Enable foreign keys for SQLite
		if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			db.Close()
			return nil, "", err
		}

		return db, "sqlite3", nil

	case "postgres", "postgresql":
		// Build PostgreSQL connection string
		host := viper.GetString("database.host")
		if host == "" {
			host = os.Getenv("DB_HOST")
		}
		if host == "" {
			host = "localhost"
		}

		port := viper.GetInt("database.port")
		if port == 0 {
			portStr := os.Getenv("DB_PORT")
			if portStr != "" {
				fmt.Sscanf(portStr, "%d", &port)
			}
		}
		if port == 0 {
			port = 5432
		}

		dbName := viper.GetString("database.name")
		if dbName == "" {
			dbName = os.Getenv("DB_NAME")
		}
		if dbName == "" {
			dbName = "fleetd"
		}

		user := viper.GetString("database.user")
		if user == "" {
			user = os.Getenv("DB_USER")
		}
		if user == "" {
			user = "postgres"
		}

		password := viper.GetString("database.password")
		if password == "" {
			password = os.Getenv("DB_PASSWORD")
		}

		sslMode := viper.GetString("database.sslmode")
		if sslMode == "" {
			sslMode = os.Getenv("DB_SSLMODE")
		}
		if sslMode == "" {
			sslMode = "disable"
		}

		connStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s sslmode=%s",
			host, port, dbName, user, sslMode)

		if password != "" {
			connStr += fmt.Sprintf(" password=%s", password)
		}

		db, err := sql.Open("postgres", connStr)
		if err != nil {
			return nil, "", err
		}

		// Test connection
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, "", fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}

		return db, "postgres", nil

	default:
		return nil, "", fmt.Errorf("unsupported database type: %s", dbType)
	}
}
