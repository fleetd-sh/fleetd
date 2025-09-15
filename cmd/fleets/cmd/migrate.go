package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	"fleetd.sh/internal/database"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database migration commands",
	Long:  `Manage database schema migrations for FleetD`,
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Run all pending migrations",
	Long:  `Apply all pending database migrations to bring the schema up to date`,
	RunE:  runMigrateUp,
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback one migration",
	Long:  `Rollback the most recent database migration`,
	RunE:  runMigrateDown,
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Long:  `Display the current migration version and any pending migrations`,
	RunE:  runMigrateStatus,
}

var migrateResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset database (DESTRUCTIVE)",
	Long:  `Drop all tables and rerun all migrations. WARNING: This will delete all data!`,
	RunE:  runMigrateReset,
}

var migrateVersionCmd = &cobra.Command{
	Use:   "version [version]",
	Short: "Migrate to specific version",
	Long:  `Migrate the database to a specific version (up or down)`,
	Args:  cobra.ExactArgs(1),
	RunE:  runMigrateVersion,
}

var migrateForceCmd = &cobra.Command{
	Use:   "force [version]",
	Short: "Force migration version",
	Long:  `Force the migration version without running migrations (use with caution)`,
	Args:  cobra.ExactArgs(1),
	RunE:  runMigrateForce,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateResetCmd)
	migrateCmd.AddCommand(migrateVersionCmd)
	migrateCmd.AddCommand(migrateForceCmd)

	// Add flags
	migrateCmd.PersistentFlags().String("database-url", "", "Database connection URL")
	migrateCmd.PersistentFlags().String("driver", "postgres", "Database driver (postgres, sqlite3)")
	migrateResetCmd.Flags().Bool("confirm", false, "Confirm database reset")
}

func runMigrateUp(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	db, driver, err := connectDatabase(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	migrator, err := database.NewMigrator(nil)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Initialize(db, driver); err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}

	slog.Info("Running migrations...")
	if err := migrator.Up(ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	version, _, _ := migrator.Version()
	slog.Info("Migrations completed", "version", version)
	return nil
}

func runMigrateDown(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	db, driver, err := connectDatabase(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	migrator, err := database.NewMigrator(nil)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Initialize(db, driver); err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}

	slog.Info("Rolling back migration...")
	if err := migrator.Down(ctx); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	version, _, _ := migrator.Version()
	slog.Info("Migration rolled back", "version", version)
	return nil
}

func runMigrateStatus(cmd *cobra.Command, args []string) error {
	db, driver, err := connectDatabase(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	migrator, err := database.NewMigrator(nil)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Initialize(db, driver); err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}

	version, dirty, err := migrator.Version()
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}

	fmt.Printf("Current Version: %d\n", version)
	fmt.Printf("Dirty: %v\n", dirty)

	if dirty {
		fmt.Println("\n⚠️  Database is in dirty state. Manual intervention may be required.")
		fmt.Println("You can use 'fleets migrate force <version>' to reset the version.")
	}

	return nil
}

func runMigrateReset(cmd *cobra.Command, args []string) error {
	confirm, _ := cmd.Flags().GetBool("confirm")
	if !confirm {
		fmt.Println("⚠️  WARNING: This will DELETE ALL DATA in the database!")
		fmt.Println("Run with --confirm flag to proceed.")
		return nil
	}

	ctx := context.Background()

	db, driver, err := connectDatabase(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	migrator, err := database.NewMigrator(nil)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Initialize(db, driver); err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}

	slog.Warn("Resetting database - all data will be lost!")
	if err := migrator.Reset(ctx); err != nil {
		return fmt.Errorf("reset failed: %w", err)
	}

	slog.Info("Database reset completed")
	return nil
}

func runMigrateVersion(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	var targetVersion uint
	if _, err := fmt.Sscanf(args[0], "%d", &targetVersion); err != nil {
		return fmt.Errorf("invalid version number: %s", args[0])
	}

	db, driver, err := connectDatabase(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	migrator, err := database.NewMigrator(nil)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Initialize(db, driver); err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}

	slog.Info("Migrating to version", "target", targetVersion)
	if err := migrator.Migrate(ctx, targetVersion); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	version, _, _ := migrator.Version()
	slog.Info("Migration completed", "version", version)
	return nil
}

func runMigrateForce(cmd *cobra.Command, args []string) error {
	var forceVersion int
	if _, err := fmt.Sscanf(args[0], "%d", &forceVersion); err != nil {
		return fmt.Errorf("invalid version number: %s", args[0])
	}

	db, driver, err := connectDatabase(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	migrator, err := database.NewMigrator(nil)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Initialize(db, driver); err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}

	slog.Warn("Forcing migration version", "version", forceVersion)
	if err := migrator.Force(forceVersion); err != nil {
		return fmt.Errorf("force failed: %w", err)
	}

	slog.Info("Migration version forced", "version", forceVersion)
	return nil
}

func connectDatabase(cmd *cobra.Command) (*sql.DB, string, error) {
	databaseURL, _ := cmd.Flags().GetString("database-url")
	driver, _ := cmd.Flags().GetString("driver")

	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			return nil, "", fmt.Errorf("database URL not provided (use --database-url or DATABASE_URL env var)")
		}
	}

	db, err := sql.Open(driver, databaseURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, driver, nil
}
