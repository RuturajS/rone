package cmd

import (
	"database/sql"
	"fmt"

	"github.com/RuturajS/rone/database"
	_ "modernc.org/sqlite"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Create the database schema (for testing/validation)",
	RunE:  runMigrate,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer conn.Close()

	if err := database.Migrate(conn); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Verify tables exist
	tables := []string{"channels", "messages", "tasks", "execution_logs"}
	for _, t := range tables {
		var name string
		err := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", t).Scan(&name)
		if err != nil {
			return fmt.Errorf("table '%s' not found after migration: %w", t, err)
		}
		fmt.Printf("  ✅ %s\n", t)
	}

	fmt.Println("\n✅ schema migration successful")
	return nil
}
