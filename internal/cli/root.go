package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/sbenjam1n/gamsync/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfg     *config.Config
	rootCmd = &cobra.Command{
		Use:   "gam",
		Short: "GAM+Sync: Agentic Memory with Concept Design, Synchronizations, and Structural Enforcement",
		Long: `GAM+Sync is a CLI tool for managing agentic software development with
concepts, synchronizations, region markers, and structural enforcement.

Start every session:
  gam turn start --region <target>

End every session:
  gam turn end --scratchpad "what you did and what's next"

The CLI handles enforcement. You handle thinking and coding.`,
	}
)

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(turnCmd)
	rootCmd.AddCommand(regionCmd)
	rootCmd.AddCommand(conceptCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(treeCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(flowCmd)
	rootCmd.AddCommand(docsCmd)
	rootCmd.AddCommand(qualityCmd)
	rootCmd.AddCommand(gardenerCmd)
	rootCmd.AddCommand(archCmd)
	rootCmd.AddCommand(queueCmd)
	rootCmd.AddCommand(memorizerCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(skillCmd)
}

func initConfig() {
	var err error
	cfg, err = config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
}

func connectDB(ctx context.Context) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w\nSet GAM_DATABASE_URL environment variable", err)
	}
	return pool, nil
}

func connectRedis() (*redis.Client, error) {
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w\nSet GAM_REDIS_URL environment variable", err)
	}
	return redis.NewClient(opts), nil
}

func projectRoot() string {
	return cfg.ProjectRoot
}

func migrationsDir() string {
	return filepath.Join(projectRoot(), "migrations")
}
