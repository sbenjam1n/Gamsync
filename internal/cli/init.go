package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sbenjam1n/gamsync/internal/db"
	"github.com/sbenjam1n/gamsync/internal/queue"
	"github.com/spf13/cobra"
)

var minimal bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a GAM+Sync project",
	Long:  "Initialize project: arch.md, .gamignore, docs/, PostgreSQL schema, Redis streams",
	RunE: func(cmd *cobra.Command, args []string) error {
		root := projectRoot()
		ctx := context.Background()

		// Create arch.md
		archPath := filepath.Join(root, "arch.md")
		if _, err := os.Stat(archPath); os.IsNotExist(err) {
			archContent := `# Architecture

# @region:app
# @endregion:app
`
			if err := os.WriteFile(archPath, []byte(archContent), 0644); err != nil {
				return fmt.Errorf("create arch.md: %w", err)
			}
			fmt.Println("Created arch.md")
		} else {
			fmt.Println("arch.md already exists")
		}

		// Create .gamignore
		gamignorePath := filepath.Join(root, ".gamignore")
		if _, err := os.Stat(gamignorePath); os.IsNotExist(err) {
			gamignoreContent := `# .gamignore
# Glob patterns for paths that Tier 0 skips when checking "code exists outside region boundaries"

# Vendored dependencies
vendor/

# Generated code
gen/
*.pb.go
*_sqlc.go

# Configuration
*.yaml
*.toml
*.env*

# Build artifacts
bin/
dist/

# Shared utilities that cross concept boundaries
pkg/util/
pkg/middleware/

# Test fixtures
testdata/
`
			if err := os.WriteFile(gamignorePath, []byte(gamignoreContent), 0644); err != nil {
				return fmt.Errorf("create .gamignore: %w", err)
			}
			fmt.Println("Created .gamignore")
		} else {
			fmt.Println(".gamignore already exists")
		}

		// Create docs/ directory structure
		docsDir := filepath.Join(root, "docs")
		for _, sub := range []string{
			"concepts",
			"syncs",
			"exec-plans/active",
			"exec-plans/completed",
			"quality",
			"design",
		} {
			dir := filepath.Join(docsDir, sub)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("create docs/%s: %w", sub, err)
			}
		}
		fmt.Println("Created docs/ directory structure")

		if minimal {
			fmt.Println("\nMinimal init complete. Run 'gam init' (without --minimal) to set up PostgreSQL and Redis.")
			return nil
		}

		// Set up PostgreSQL
		fmt.Println("Connecting to PostgreSQL...")
		pool, err := db.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer pool.Close()

		fmt.Println("Running migrations...")
		if err := db.Migrate(ctx, pool, migrationsDir()); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
		fmt.Println("PostgreSQL schema created")

		// Set up Redis streams
		fmt.Println("Connecting to Redis...")
		rdb, err := connectRedis()
		if err != nil {
			return fmt.Errorf("redis connection failed: %w", err)
		}
		defer rdb.Close()

		q := queue.New(rdb)
		if err := q.EnsureStreams(ctx); err != nil {
			return fmt.Errorf("redis stream setup failed: %w", err)
		}
		fmt.Println("Redis streams created")

		fmt.Println("\nGAM+Sync project initialized successfully.")
		fmt.Println("Next steps:")
		fmt.Println("  1. Edit arch.md to define your namespace tree")
		fmt.Println("  2. Run: gam arch sync")
		fmt.Println("  3. Run: gam concept add <name> --spec <file>")
		fmt.Println("  4. Run: gam turn start --region <path>")
		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&minimal, "minimal", false, "Minimal init: arch.md + .gamignore + docs/ only")
}
