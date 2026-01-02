package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"homelabgo/internal/config"
	"homelabgo/internal/db"
	"homelabgo/internal/db/migrations"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	database, err := db.Connect(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}

	migrator := migrations.NewMigrator(database)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "migrate":
		if err := migrator.Migrate(); err != nil {
			fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
			os.Exit(1)
		}
	case "rollback":
		if err := migrator.Rollback(); err != nil {
			fmt.Fprintf(os.Stderr, "Rollback failed: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := migrator.Status(); err != nil {
			fmt.Fprintf(os.Stderr, "Status failed: %v\n", err)
			os.Exit(1)
		}
	case "fresh":
		fmt.Println("Dropping all tables...")
		if err := database.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;").Error; err != nil {
			fmt.Fprintf(os.Stderr, "Failed to drop tables: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Re-running migrations...")
		if err := migrator.Migrate(); err != nil {
			fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: go run ./cmd/migrate <command>")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  migrate   Run pending migrations")
	fmt.Println("  rollback  Rollback the last batch of migrations")
	fmt.Println("  status    Show migration status")
	fmt.Println("  fresh     Drop all tables and re-run all migrations")
}
