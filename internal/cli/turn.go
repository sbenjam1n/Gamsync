package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/sbenjam1n/gamsync/internal/memorizer"
	"github.com/sbenjam1n/gamsync/internal/region"
	"github.com/sbenjam1n/gamsync/internal/validator"
	"github.com/spf13/cobra"
)

var turnCmd = &cobra.Command{
	Use:   "turn",
	Short: "Turn lifecycle management",
}

var turnStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new turn: generate ID, search pertinent memory, compile context",
	RunE: func(cmd *cobra.Command, args []string) error {
		regionPath, _ := cmd.Flags().GetString("region")
		if regionPath == "" {
			return fmt.Errorf("--region is required")
		}
		prompt, _ := cmd.Flags().GetString("prompt")

		ctx := context.Background()
		pool, err := connectDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		turnID := memorizer.GenerateTurnID()

		// Insert the turn
		_, err = pool.Exec(ctx, `
			INSERT INTO turns (id, agent_role, scope_path, status, task_type)
			VALUES ($1, 'researcher', $2, 'ACTIVE', 'implement')
		`, turnID, regionPath)
		if err != nil {
			return fmt.Errorf("create turn: %w", err)
		}

		fmt.Printf("Turn started: %s\n", turnID)
		fmt.Printf("Region: %s\n", regionPath)

		// --- Full memory search (3 strategies) ---

		// Strategy 1: Region-scoped scratchpads (ancestors + descendants)
		regionRows, _ := pool.Query(ctx, `
			SELECT t.scratchpad, t.id, t.scope_path, t.completed_at
			FROM turns t
			JOIN turn_regions tr ON tr.turn_id = t.id
			JOIN regions r ON r.id = tr.region_id
			WHERE (r.path <@ $1::ltree OR r.path @> $1::ltree)
			  AND t.scratchpad IS NOT NULL AND t.status = 'COMPLETED'
			ORDER BY t.completed_at DESC NULLS LAST
			LIMIT 10
		`, regionPath)
		seenTurns := make(map[string]bool)
		if regionRows != nil {
			fmt.Println("\n--- Turn Memory (region-scoped) ---")
			for regionRows.Next() {
				var sp, tid, scopePath string
				var completedAt *time.Time
				regionRows.Scan(&sp, &tid, &scopePath, &completedAt)
				seenTurns[tid] = true
				ts := ""
				if completedAt != nil {
					ts = completedAt.Format("2006-01-02 15:04")
				}
				fmt.Printf("[%s] scope=%s %s\n%s\n\n", tid, scopePath, ts, sp)
			}
			regionRows.Close()
		}

		// Strategy 2: Concept-scoped scratchpads
		conceptRows, _ := pool.Query(ctx, `
			SELECT DISTINCT t.scratchpad, t.id, t.scope_path, t.completed_at
			FROM turns t
			JOIN turn_regions tr ON tr.turn_id = t.id
			JOIN regions r ON r.id = tr.region_id
			JOIN concept_region_assignments cra ON cra.region_id = r.id
			JOIN concepts c ON c.id = cra.concept_id
			WHERE c.id IN (
				SELECT c2.id FROM regions r2
				JOIN concept_region_assignments cra2 ON cra2.region_id = r2.id
				JOIN concepts c2 ON c2.id = cra2.concept_id
				WHERE r2.path @> $1::ltree OR r2.path = $1::ltree
			)
			AND t.scratchpad IS NOT NULL AND t.status = 'COMPLETED'
			ORDER BY t.completed_at DESC NULLS LAST
			LIMIT 10
		`, regionPath)
		if conceptRows != nil {
			first := true
			for conceptRows.Next() {
				var sp, tid, scopePath string
				var completedAt *time.Time
				conceptRows.Scan(&sp, &tid, &scopePath, &completedAt)
				if seenTurns[tid] {
					continue
				}
				if first {
					fmt.Println("--- Turn Memory (concept-scoped) ---")
					first = false
				}
				seenTurns[tid] = true
				ts := ""
				if completedAt != nil {
					ts = completedAt.Format("2006-01-02 15:04")
				}
				fmt.Printf("[%s] scope=%s %s\n%s\n\n", tid, scopePath, ts, sp)
			}
			conceptRows.Close()
		}

		// Strategy 3: Prompt-relevance search (if prompt provided)
		if prompt != "" {
			simRows, _ := pool.Query(ctx, `
				SELECT t.id, t.scope_path, t.scratchpad, t.completed_at,
				       similarity(t.scratchpad, $1) AS sim
				FROM turns t
				WHERE t.scratchpad IS NOT NULL AND t.scratchpad % $1
				ORDER BY sim DESC
				LIMIT 5
			`, prompt)
			if simRows != nil {
				first := true
				for simRows.Next() {
					var tid, scope, sp string
					var completedAt *time.Time
					var sim float64
					simRows.Scan(&tid, &scope, &sp, &completedAt, &sim)
					if seenTurns[tid] || sim < 0.1 {
						continue
					}
					if first {
						fmt.Println("--- Turn Memory (prompt-relevant) ---")
						first = false
					}
					seenTurns[tid] = true
					fmt.Printf("[%s] scope=%s (relevance=%.0f%%)\n%s\n\n", tid, scope, sim*100, sp)
				}
				simRows.Close()
			}
		}

		// Show concept assignments
		rows, _ := pool.Query(ctx, `
			SELECT c.name, c.purpose, cra.role
			FROM regions r
			JOIN concept_region_assignments cra ON cra.region_id = r.id
			JOIN concepts c ON c.id = cra.concept_id
			WHERE r.path @> $1::ltree OR r.path = $1::ltree
		`, regionPath)
		if rows != nil {
			fmt.Println("Concepts in scope:")
			for rows.Next() {
				var name, purpose, role string
				rows.Scan(&name, &purpose, &role)
				fmt.Printf("  [%s] %s: %s\n", role, name, purpose)
			}
			rows.Close()
		}

		return nil
	},
}

var turnEndCmd = &cobra.Command{
	Use:   "end",
	Short: "End a turn: validate (blocks on failure), save memory, queue proposals",
	RunE: func(cmd *cobra.Command, args []string) error {
		scratchpad, _ := cmd.Flags().GetString("scratchpad")
		if scratchpad == "" {
			return fmt.Errorf("--scratchpad is required")
		}
		skipValidation, _ := cmd.Flags().GetBool("skip-validation")

		ctx := context.Background()
		pool, err := connectDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		// Find the most recent active turn
		var turnID, scopePath string
		err = pool.QueryRow(ctx, `
			SELECT id, scope_path FROM turns WHERE status = 'ACTIVE' ORDER BY created_at DESC LIMIT 1
		`).Scan(&turnID, &scopePath)
		if err != nil {
			return fmt.Errorf("no active turn found: %w", err)
		}

		// --- Validation gate: blocks turn end on failure ---
		if !skipValidation {
			fmt.Printf("Validating turn %s (scope: %s)...\n", turnID, scopePath)

			v := validator.New(pool, projectRoot())

			// Check 1: arch.md namespace alignment
			archIssues := v.ValidateArchAlignment(ctx, projectRoot())
			if len(archIssues) > 0 {
				fmt.Println("\nVALIDATION FAILED: arch.md alignment issues")
				for _, issue := range archIssues {
					fmt.Printf("  %s\n", issue)
				}
				fmt.Println("\nTurn end blocked. Fix the issues above and retry.")
				fmt.Println("Use --skip-validation to bypass (not recommended).")
				return fmt.Errorf("validation failed: %d arch.md alignment issues", len(archIssues))
			}

			// Check 2: Region markers in source code
			gamignore := region.ParseGamignore(projectRoot())
			markers, warnings, _ := region.ScanDirectory(projectRoot(), gamignore)

			if len(warnings) > 0 {
				fmt.Println("\nVALIDATION FAILED: region marker issues")
				for _, w := range warnings {
					fmt.Printf("  %s\n", w)
				}
				fmt.Println("\nTurn end blocked. Fix region marker issues above.")
				return fmt.Errorf("validation failed: %d region marker warnings", len(warnings))
			}

			// Check 3: Source regions match arch.md
			archPaths, _ := region.ParseArchMd(projectRoot())
			archSet := make(map[string]bool)
			for _, p := range archPaths {
				archSet[p] = true
			}

			var unregistered []string
			for _, m := range markers {
				if !archSet[m.Path] {
					unregistered = append(unregistered, m.Path)
				}
			}

			if len(unregistered) > 0 {
				fmt.Println("\nVALIDATION FAILED: source regions not in arch.md")
				for _, p := range unregistered {
					fmt.Printf("  %s (found in source, missing from arch.md)\n", p)
				}
				fmt.Println("\nAdd these to arch.md or remove the region markers.")
				return fmt.Errorf("validation failed: %d unregistered regions", len(unregistered))
			}

			fmt.Println("  Validation passed.")
		}

		now := time.Now()
		_, err = pool.Exec(ctx, `
			UPDATE turns
			SET scratchpad = $1, status = 'COMPLETED', completed_at = $2
			WHERE id = $3
		`, scratchpad, now, turnID)
		if err != nil {
			return fmt.Errorf("end turn: %w", err)
		}

		fmt.Printf("Turn ended: %s\n", turnID)
		fmt.Printf("Scratchpad saved.\n")
		return nil
	},
}

var turnStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show active turns",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		pool, err := connectDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		rows, err := pool.Query(ctx, `
			SELECT t.id, t.scope_path, t.task_type, t.agent_role, t.created_at,
			       ep.name as plan_name
			FROM turns t
			LEFT JOIN plan_turns pt ON pt.turn_id = t.id
			LEFT JOIN execution_plans ep ON ep.id = pt.plan_id
			WHERE t.status = 'ACTIVE'
			ORDER BY t.created_at DESC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		fmt.Println("Active Turns:")
		found := false
		for rows.Next() {
			found = true
			var id, scope, taskType string
			var agentRole *string
			var createdAt time.Time
			var planName *string
			rows.Scan(&id, &scope, &taskType, &agentRole, &createdAt, &planName)
			role := "unknown"
			if agentRole != nil {
				role = *agentRole
			}
			fmt.Printf("  %s  scope=%s  type=%s  role=%s  started=%s",
				id, scope, taskType, role, createdAt.Format(time.RFC3339))
			if planName != nil {
				fmt.Printf("  plan=%s", *planName)
			}
			fmt.Println()
		}
		if !found {
			fmt.Println("  (none)")
		}
		return nil
	},
}

var turnMemoryCmd = &cobra.Command{
	Use:   "memory [region]",
	Short: "Query scratchpads from turns that touched a region",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		regionPath := args[0]

		ctx := context.Background()
		pool, err := connectDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		rows, err := pool.Query(ctx, `
			SELECT t.id, t.scratchpad, t.completed_at
			FROM turns t
			JOIN turn_regions tr ON tr.turn_id = t.id
			JOIN regions r ON r.id = tr.region_id
			WHERE r.path <@ $1::ltree AND t.scratchpad IS NOT NULL
			ORDER BY t.completed_at DESC NULLS LAST
			LIMIT 10
		`, regionPath)
		if err != nil {
			return err
		}
		defer rows.Close()

		fmt.Printf("Turn memory for %s:\n\n", regionPath)
		for rows.Next() {
			var id, scratchpad string
			var completedAt *time.Time
			rows.Scan(&id, &scratchpad, &completedAt)
			ts := "(active)"
			if completedAt != nil {
				ts = completedAt.Format(time.RFC3339)
			}
			fmt.Printf("[%s] (%s)\n%s\n\n", id, ts, scratchpad)
		}
		return nil
	},
}

var turnSearchCmd = &cobra.Command{
	Use:   "search [text]",
	Short: "Full-text search across all scratchpads",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		searchText := args[0]

		ctx := context.Background()
		pool, err := connectDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		rows, err := pool.Query(ctx, `
			SELECT t.id, t.scope_path, t.scratchpad, t.completed_at,
			       similarity(t.scratchpad, $1) AS sim
			FROM turns t
			WHERE t.scratchpad % $1
			ORDER BY sim DESC
			LIMIT 10
		`, searchText)
		if err != nil {
			return err
		}
		defer rows.Close()

		fmt.Printf("Search results for \"%s\":\n\n", searchText)
		for rows.Next() {
			var id, scope, scratchpad string
			var completedAt *time.Time
			var sim float64
			rows.Scan(&id, &scope, &scratchpad, &completedAt, &sim)
			fmt.Printf("[%s] scope=%s (similarity=%.2f)\n%s\n\n", id, scope, sim, scratchpad)
		}
		return nil
	},
}

var turnDiffCmd = &cobra.Command{
	Use:   "diff [turn_id]",
	Short: "Show structural diff for a turn",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		turnID := args[0]

		ctx := context.Background()
		pool, err := connectDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		rows, err := pool.Query(ctx, `
			SELECT r.path, tr.action
			FROM turn_regions tr
			JOIN regions r ON r.id = tr.region_id
			WHERE tr.turn_id = $1
			ORDER BY tr.action, r.path
		`, turnID)
		if err != nil {
			return err
		}
		defer rows.Close()

		fmt.Printf("Structural diff for %s:\n\n", turnID)
		for rows.Next() {
			var path, action string
			rows.Scan(&path, &action)
			prefix := "~"
			switch action {
			case "created":
				prefix = "+"
			case "deleted":
				prefix = "-"
			}
			fmt.Printf("  %s %s\n", prefix, path)
		}
		return nil
	},
}

func init() {
	turnStartCmd.Flags().String("region", "", "Target region path")
	turnStartCmd.MarkFlagRequired("region")
	turnStartCmd.Flags().String("prompt", "", "Task description for relevance-based memory search")

	turnEndCmd.Flags().String("scratchpad", "", "What you did and what's next")
	turnEndCmd.MarkFlagRequired("scratchpad")
	turnEndCmd.Flags().Bool("skip-validation", false, "Skip validation gate (not recommended)")

	turnCmd.AddCommand(turnStartCmd)
	turnCmd.AddCommand(turnEndCmd)
	turnCmd.AddCommand(turnStatusCmd)
	turnCmd.AddCommand(turnMemoryCmd)
	turnCmd.AddCommand(turnSearchCmd)
	turnCmd.AddCommand(turnDiffCmd)
}
