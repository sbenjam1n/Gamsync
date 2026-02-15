package cli

import (
	"context"
	"encoding/json"
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

// captureTreeSnapshot scans source for region markers and returns a JSON-encoded
// map of region path -> list of "file:startLine-endLine" locations.
func captureTreeSnapshot(root string) ([]byte, map[string][]string) {
	gamignore := region.ParseGamignore(root)
	markers, _, _ := region.ScanDirectory(root, gamignore)
	snapshot := make(map[string][]string)
	for _, mk := range markers {
		snapshot[mk.Path] = append(snapshot[mk.Path],
			fmt.Sprintf("%s:%d-%d", mk.File, mk.StartLine, mk.EndLine))
	}
	data, _ := json.Marshal(snapshot)
	return data, snapshot
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

		// Capture tree_before snapshot
		root := projectRoot()
		treeBefore, _ := captureTreeSnapshot(root)

		// Insert the turn with tree_before
		_, err = pool.Exec(ctx, `
			INSERT INTO turns (id, agent_role, scope_path, status, task_type, tree_before)
			VALUES ($1, 'researcher', $2, 'ACTIVE', 'implement', $3)
		`, turnID, regionPath, treeBefore)
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
	Short: "End a turn: validate (blocks on failure), save memory, record structural diff",
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

		// Scan source regions once (used for validation, tree snapshot, and turn_regions)
		root := projectRoot()
		treeAfterJSON, afterSnapshot := captureTreeSnapshot(root)

		gamignore := region.ParseGamignore(root)
		_, warnings, _ := region.ScanDirectory(root, gamignore)

		// --- Validation gate: blocks turn end on failure ---
		if !skipValidation {
			fmt.Printf("Validating turn %s (scope: %s)...\n", turnID, scopePath)

			v := validator.New(pool, root)

			// Check 1: arch.md namespace alignment
			archIssues := v.ValidateArchAlignment(ctx, root)
			if len(archIssues) > 0 {
				fmt.Println("\nVALIDATION FAILED: arch.md alignment issues")
				for _, issue := range archIssues {
					fmt.Printf("  %s\n", issue)
				}
				fmt.Println("\nTurn end blocked. Fix the issues above and retry.")
				fmt.Println("Use --skip-validation to bypass (not recommended).")
				return fmt.Errorf("validation failed: %d arch.md alignment issues", len(archIssues))
			}

			// Check 2: Region marker integrity
			if len(warnings) > 0 {
				fmt.Println("\nVALIDATION FAILED: region marker issues")
				for _, w := range warnings {
					fmt.Printf("  %s\n", w)
				}
				fmt.Println("\nTurn end blocked. Fix region marker issues above.")
				return fmt.Errorf("validation failed: %d region marker warnings", len(warnings))
			}

			// Check 3: Source regions match arch.md
			archPaths, _ := region.ParseArchMd(root)
			archSet := make(map[string]bool)
			for _, p := range archPaths {
				archSet[p] = true
			}

			var unregistered []string
			for path := range afterSnapshot {
				if !archSet[path] {
					unregistered = append(unregistered, path)
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

		// Record turn_regions by diffing tree_before vs tree_after
		var treeBeforeJSON []byte
		pool.QueryRow(ctx, "SELECT tree_before FROM turns WHERE id = $1", turnID).Scan(&treeBeforeJSON)
		var beforeSnapshot map[string][]string
		if treeBeforeJSON != nil {
			json.Unmarshal(treeBeforeJSON, &beforeSnapshot)
		}

		for path := range afterSnapshot {
			action := "modified"
			if beforeSnapshot == nil || beforeSnapshot[path] == nil {
				action = "created"
			}
			var regionID string
			pool.QueryRow(ctx, "SELECT id FROM regions WHERE path = $1", path).Scan(&regionID)
			if regionID != "" {
				pool.Exec(ctx, `
					INSERT INTO turn_regions (turn_id, region_id, action)
					VALUES ($1, $2, $3)
					ON CONFLICT (turn_id, region_id) DO UPDATE SET action = $3
				`, turnID, regionID, action)
			}
		}
		if beforeSnapshot != nil {
			for path := range beforeSnapshot {
				if afterSnapshot[path] == nil {
					var regionID string
					pool.QueryRow(ctx, "SELECT id FROM regions WHERE path = $1", path).Scan(&regionID)
					if regionID != "" {
						pool.Exec(ctx, `
							INSERT INTO turn_regions (turn_id, region_id, action)
							VALUES ($1, $2, 'deleted')
							ON CONFLICT (turn_id, region_id) DO UPDATE SET action = 'deleted'
						`, turnID, regionID)
					}
				}
			}
		}

		// Complete the turn with scratchpad and tree_after
		now := time.Now()
		_, err = pool.Exec(ctx, `
			UPDATE turns
			SET scratchpad = $1, status = 'COMPLETED', completed_at = $2, tree_after = $3
			WHERE id = $4
		`, scratchpad, now, treeAfterJSON, turnID)
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
