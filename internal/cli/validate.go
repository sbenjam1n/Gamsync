package cli

import (
	"context"
	"fmt"

	"github.com/sbenjam1n/gamsync/internal/gam"
	"github.com/sbenjam1n/gamsync/internal/region"
	"github.com/sbenjam1n/gamsync/internal/validator"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Run validation: arch.md alignment, region markers, Tier 0 + Tier 1",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		archOnly, _ := cmd.Flags().GetBool("arch")
		ctx := context.Background()

		root := projectRoot()

		// Arch-only mode: validate arch.md without DB
		if archOnly {
			fmt.Println("Validating arch.md namespace alignment...")

			issues := region.ValidateArchNamespaces(root)
			if len(issues) > 0 {
				fmt.Println("\nNamespace hierarchy issues:")
				for _, issue := range issues {
					fmt.Printf("  %s\n", issue)
				}
			}

			// Check source vs arch.md alignment
			archPaths, _ := region.ParseArchMd(root)
			archSet := make(map[string]bool)
			for _, p := range archPaths {
				archSet[p] = true
			}

			gamignore := region.ParseGamignore(root)
			markers, warnings, _ := region.ScanDirectory(root, gamignore)

			for _, w := range warnings {
				fmt.Printf("  Warning: %s\n", w)
			}

			sourceSet := make(map[string]bool)
			for _, m := range markers {
				sourceSet[m.Path] = true
				if !archSet[m.Path] {
					fmt.Printf("  FAIL: region %s in source (%s:%d) not in arch.md\n", m.Path, m.File, m.StartLine)
				}
			}
			for _, p := range archPaths {
				if !sourceSet[p] {
					fmt.Printf("  WARN: arch.md declares %s but no source markers found\n", p)
				}
			}

			if len(issues) == 0 {
				fmt.Println("  arch.md validation passed.")
			}
			return nil
		}

		pool, err := connectDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		v := validator.New(pool, root)

		if all {
			// Full project validation
			fmt.Println("=== arch.md alignment ===")
			archIssues := v.ValidateArchAlignment(ctx, root)
			archFailed := 0
			for _, issue := range archIssues {
				fmt.Printf("  %s\n", issue)
				archFailed++
			}
			if archFailed == 0 {
				fmt.Println("  PASSED")
			}

			fmt.Println("\n=== Tier 0 structural ===")
			rows, err := pool.Query(ctx, `SELECT path FROM regions ORDER BY path`)
			if err != nil {
				return err
			}
			defer rows.Close()

			passed := 0
			failed := 0
			for rows.Next() {
				var path string
				rows.Scan(&path)

				proposal := &gam.Proposal{
					RegionPath: path,
				}
				result := v.Tier0Structural(ctx, proposal)
				if result.Passed {
					passed++
				} else {
					failed++
					fmt.Printf("  FAIL %s: %s\n", path, result.Message)
					for _, d := range result.Details {
						if !d.Passed && d.Fix != "" {
							fmt.Printf("    Fix: %s\n", d.Fix)
						}
					}
				}
			}

			fmt.Printf("\n  %d passed, %d failed\n", passed, failed)

			total := archFailed + failed
			if total > 0 {
				return fmt.Errorf("validation failed: %d total issues", total)
			}
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("specify a region path, use --all, or use --arch")
		}

		regionPath := args[0]

		// Structural validation
		fmt.Printf("Validating %s...\n", regionPath)

		// Check region markers in source files
		gamignore := region.ParseGamignore(root)
		markers, warnings, _ := region.ScanDirectory(root, gamignore)

		found := false
		for _, m := range markers {
			if m.Path == regionPath {
				found = true
				fmt.Printf("  Region markers: found in %s:%d-%d\n", m.File, m.StartLine, m.EndLine)
			}
		}
		if !found {
			fmt.Printf("  Region markers: NOT FOUND in source files\n")
		}

		for _, w := range warnings {
			fmt.Printf("  Warning: %s\n", w)
		}

		// Database validation
		proposal := &gam.Proposal{
			RegionPath: regionPath,
		}

		result := v.Tier0Structural(ctx, proposal)
		fmt.Printf("  Tier 0 (Structural): %s\n", formatValidationResult(result))

		if result.Passed {
			result1, err := v.Tier1StateMachine(ctx, proposal)
			if err != nil {
				fmt.Printf("  Tier 1: ERROR: %v\n", err)
			} else {
				fmt.Printf("  Tier 1 (State Machine): %s\n", formatValidationResult(result1))
			}
		}

		return nil
	},
}

func formatValidationResult(r *gam.ValidationResult) string {
	if r.Passed {
		return "PASSED"
	}
	result := fmt.Sprintf("FAILED (code %d): %s", r.Code, r.Message)
	for _, d := range r.Details {
		if !d.Passed && d.Fix != "" {
			result += fmt.Sprintf("\n    Fix: %s", d.Fix)
		}
	}
	return result
}

func init() {
	validateCmd.Flags().Bool("all", false, "Validate entire project")
	validateCmd.Flags().Bool("arch", false, "Validate arch.md alignment only (no database required)")
}
