package validator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sbenjam1n/gamsync/internal/gam"
	"github.com/sbenjam1n/gamsync/internal/region"
)

// Validator runs Tier 0 (structural) and Tier 1 (state machine + sync integrity) validation.
type Validator struct {
	db          *pgxpool.Pool
	projectRoot string
}

// New creates a new Validator.
func New(db *pgxpool.Pool, projectRoot string) *Validator {
	return &Validator{db: db, projectRoot: projectRoot}
}

// Validate runs Tier 0 and Tier 1 validation on a proposal.
func (v *Validator) Validate(ctx context.Context, p *gam.Proposal) (*gam.ValidationResult, error) {
	if result := v.Tier0Structural(ctx, p); !result.Passed {
		return result, nil
	}
	return v.Tier1StateMachine(ctx, p)
}

// Tier0Structural performs structural checks: region exists, scope check, region markers present.
func (v *Validator) Tier0Structural(ctx context.Context, p *gam.Proposal) *gam.ValidationResult {
	result := &gam.ValidationResult{Tier: 0, Passed: true, Code: 0}

	// Check region exists in DB (mirrors arch.md)
	var exists bool
	err := v.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM regions WHERE path = $1)",
		p.RegionPath,
	).Scan(&exists)
	if err != nil {
		result.Passed = false
		result.Code = 1
		result.Message = fmt.Sprintf("Database error checking region %s: %v", p.RegionPath, err)
		return result
	}

	if !exists {
		result.Passed = false
		result.Code = 1
		result.Message = fmt.Sprintf("Region %s not found in arch.md", p.RegionPath)
		result.Details = append(result.Details, gam.ValidationDetail{
			Check:    "region_exists",
			Passed:   false,
			Expected: fmt.Sprintf("region %s exists", p.RegionPath),
			Got:      "not found",
			Fix:      fmt.Sprintf("Add '%s' to arch.md and add @region:%s / @endregion:%s markers to source code. Then run: gam validate --arch", p.RegionPath, p.RegionPath, p.RegionPath),
		})
		return result
	}

	// Check scope: is proposal region under the turn's declared scope?
	if p.TurnID != "" {
		var inScope bool
		err := v.db.QueryRow(ctx, `
			SELECT $1::ltree <@ (SELECT scope_path FROM turns WHERE id = $2)
		`, p.RegionPath, p.TurnID).Scan(&inScope)
		if err == nil && !inScope {
			result.Passed = false
			result.Code = 2
			result.Message = fmt.Sprintf("Region %s is outside turn scope", p.RegionPath)
			result.Details = append(result.Details, gam.ValidationDetail{
				Check:    "scope_check",
				Passed:   false,
				Expected: "region within turn scope",
				Got:      fmt.Sprintf("region %s outside scope", p.RegionPath),
				Fix:      fmt.Sprintf("Start a new turn with scope including %s, or widen the current turn's scope.", p.RegionPath),
			})
			return result
		}
	}

	// Check modified regions have region markers
	for _, mr := range p.Evidence.ModifiedRegions {
		if !region.FileHasRegionMarkers(mr.File, mr.Path) {
			result.Passed = false
			result.Code = 3
			result.Message = fmt.Sprintf("File %s missing region markers for %s", mr.File, mr.Path)
			result.Details = append(result.Details, gam.ValidationDetail{
				Check:    "region_markers",
				Passed:   false,
				Expected: fmt.Sprintf("@region:%s in %s", mr.Path, mr.File),
				Got:      "missing",
				Fix:      fmt.Sprintf("Add @region:%s / @endregion:%s markers to %s", mr.Path, mr.Path, mr.File),
			})
			return result
		}
	}

	result.Message = "Tier 0 passed"
	return result
}

// Tier1StateMachine validates state transitions, invariants, and sync references.
func (v *Validator) Tier1StateMachine(ctx context.Context, p *gam.Proposal) (*gam.ValidationResult, error) {
	result := &gam.ValidationResult{Tier: 1, Passed: true, Code: 0}

	// Collect concepts via LTREE ancestor walk through junction table
	concepts, err := v.GetConceptsForRegion(ctx, p.RegionPath)
	if err != nil {
		return nil, fmt.Errorf("concept lookup: %w", err)
	}

	// Check state transition legality
	for _, concept := range concepts {
		if p.CurrentState != "" && p.ProposedState != "" {
			if !isLegalTransition(concept.StateMachine, p.CurrentState, p.ProposedState, p.ActionTaken) {
				result.Passed = false
				result.Code = -2
				result.Message = fmt.Sprintf(
					"Illegal transition: %s -> %s via %s in concept %s",
					p.CurrentState, p.ProposedState, p.ActionTaken, concept.Name,
				)
				result.Details = append(result.Details, gam.ValidationDetail{
					Check:    "state_transition",
					Passed:   false,
					Expected: fmt.Sprintf("legal transition from %s via %s", p.CurrentState, p.ActionTaken),
					Got:      fmt.Sprintf("proposed %s -> %s", p.CurrentState, p.ProposedState),
					Fix: fmt.Sprintf(
						"Check the state machine for concept %s. Legal transitions from %s: %s",
						concept.Name, p.CurrentState,
						legalTransitionsFrom(concept.StateMachine, p.CurrentState),
					),
				})
				return result, nil
			}
		}

		// Check invariant rules against evidence
		for _, inv := range concept.Invariants {
			detail := checkInvariant(inv, p.Evidence)
			result.Details = append(result.Details, detail)
			if !detail.Passed {
				result.Passed = false
				result.Code = -1
				result.Message = fmt.Sprintf("Invariant violation: %s in concept %s", inv.Name, concept.Name)
				return result, nil
			}
		}
	}

	// Check sync reference integrity
	if p.SyncChanges != nil {
		allSyncs := append(p.SyncChanges.Added, p.SyncChanges.Modified...)
		for _, sync := range allSyncs {
			if detail := v.validateSyncRefs(ctx, sync); !detail.Passed {
				result.Passed = false
				result.Code = -3
				result.Message = fmt.Sprintf("Sync %s references invalid action or state field", sync.Name)
				result.Details = append(result.Details, detail)
				return result, nil
			}
		}
	}

	// Check if proposal removes an action referenced by existing syncs
	if p.Evidence.APIAnalysis != nil {
		for _, removed := range p.Evidence.APIAnalysis.Removals {
			refs, _ := v.findSyncRefsForAction(ctx, removed)
			if len(refs) > 0 {
				result.Passed = false
				result.Code = -4
				result.Message = fmt.Sprintf(
					"Removing action %s would break %d sync(s): %v. "+
						"Update or delete the affected syncs first. "+
						"Run 'gam sync list --concept <name>' to see all affected syncs.",
					removed, len(refs), refs,
				)
				result.Details = append(result.Details, gam.ValidationDetail{
					Check:    "action_removal",
					Passed:   false,
					Expected: "no syncs reference removed action",
					Got:      fmt.Sprintf("%d syncs reference %s", len(refs), removed),
					Fix:      fmt.Sprintf("Update syncs %v before removing action %s", refs, removed),
				})
				return result, nil
			}
		}
	}

	result.Message = "Tier 1 passed"
	return result, nil
}

// GetConceptsForRegion collects concepts via LTREE ancestor walk through the junction table.
func (v *Validator) GetConceptsForRegion(ctx context.Context, path string) ([]gam.Concept, error) {
	rows, err := v.db.Query(ctx, `
		SELECT DISTINCT c.id, c.name, c.purpose, c.spec, c.state_machine, c.invariants
		FROM regions r
		JOIN concept_region_assignments cra ON cra.region_id = r.id
		JOIN concepts c ON c.id = cra.concept_id
		WHERE r.path @> $1::ltree OR r.path = $1::ltree
		ORDER BY c.name
	`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var concepts []gam.Concept
	for rows.Next() {
		var c gam.Concept
		var specJSON, smJSON, invJSON []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.Purpose, &specJSON, &smJSON, &invJSON); err != nil {
			return nil, err
		}
		json.Unmarshal(specJSON, &c.Spec)
		json.Unmarshal(smJSON, &c.StateMachine)
		json.Unmarshal(invJSON, &c.Invariants)
		concepts = append(concepts, c)
	}
	return concepts, nil
}

func isLegalTransition(sm gam.StateMachine, from, to, action string) bool {
	for _, t := range sm.Transitions {
		if t.From == from && t.To == to && t.Action == action {
			return true
		}
	}
	return false
}

func legalTransitionsFrom(sm gam.StateMachine, from string) string {
	var transitions []string
	for _, t := range sm.Transitions {
		if t.From == from {
			transitions = append(transitions, fmt.Sprintf("%s->%s via %s", t.From, t.To, t.Action))
		}
	}
	if len(transitions) == 0 {
		return "(none)"
	}
	return fmt.Sprintf("[%s]", joinStrings(transitions, ", "))
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func checkInvariant(inv gam.Invariant, evidence gam.ProposalEvidence) gam.ValidationDetail {
	detail := gam.ValidationDetail{Check: inv.Name, Passed: true}

	switch inv.Type {
	case "api":
		if evidence.APIAnalysis == nil {
			detail.Passed = false
			detail.Expected = "APIAnalysis block required by api invariant"
			detail.Got = "missing"
			detail.Fix = "Add api_analysis to proposal evidence with exports_before, exports_after, removals, and additions fields."
			return detail
		}
		if cfg := inv.Config; cfg != nil {
			if noRemovals, ok := cfg["no_removals"].(bool); ok && noRemovals {
				if len(evidence.APIAnalysis.Removals) > 0 {
					detail.Passed = false
					detail.Expected = "no API removals (no_removals invariant)"
					detail.Got = fmt.Sprintf("removed: %v", evidence.APIAnalysis.Removals)
					detail.Fix = "Restore removed exports or update the concept's api invariant to allow removals."
				}
			}
		}

	case "migration":
		if evidence.MigrationAnalysis == nil {
			detail.Passed = false
			detail.Expected = "MigrationAnalysis block required by migration invariant"
			detail.Got = "missing"
			detail.Fix = "Add migration_analysis to proposal evidence with operations, reversible, and data_loss fields."
			return detail
		}
		if cfg := inv.Config; cfg != nil {
			if forbidden, ok := cfg["forbidden"].([]any); ok {
				for _, op := range evidence.MigrationAnalysis.Operations {
					for _, f := range forbidden {
						if op == f.(string) {
							detail.Passed = false
							detail.Expected = fmt.Sprintf("operation %s forbidden by migration invariant", f)
							detail.Got = op
							detail.Fix = fmt.Sprintf("Use a non-destructive migration strategy instead of %s. Consider ADD_COLUMN + backfill instead.", op)
						}
					}
				}
			}
		}

	case "dependency":
		if evidence.DependencyAnalysis == nil {
			return detail // not required unless invariant demands it
		}
	}

	return detail
}

func (v *Validator) validateSyncRefs(ctx context.Context, sync gam.Synchronization) gam.ValidationDetail {
	detail := gam.ValidationDetail{Check: "sync_refs_" + sync.Name, Passed: true}

	// Check when clause: all referenced concept/action pairs must exist
	for _, w := range sync.WhenClause {
		var exists bool
		v.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM concepts c
				WHERE c.name = $1
				AND c.spec->'actions' ? $2
			)
		`, w.Concept, w.Action).Scan(&exists)

		if !exists {
			detail.Passed = false
			detail.Expected = fmt.Sprintf("action %s/%s exists", w.Concept, w.Action)
			detail.Got = "not found"
			detail.Fix = fmt.Sprintf("Define action '%s' in concept '%s' spec, or fix the sync's when clause reference.", w.Action, w.Concept)
			return detail
		}
	}

	// Check then clause similarly
	for _, t := range sync.ThenClause {
		var exists bool
		v.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM concepts c
				WHERE c.name = $1
				AND c.spec->'actions' ? $2
			)
		`, t.Concept, t.Action).Scan(&exists)

		if !exists {
			detail.Passed = false
			detail.Expected = fmt.Sprintf("action %s/%s exists", t.Concept, t.Action)
			detail.Got = "not found"
			detail.Fix = fmt.Sprintf("Define action '%s' in concept '%s' spec, or fix the sync's then clause reference.", t.Action, t.Concept)
			return detail
		}
	}

	// Check where clause: referenced state fields must exist in concept specs
	for _, w := range sync.WhereClause {
		for _, patternVal := range w.Pattern {
			if fields, ok := patternVal.(map[string]any); ok {
				for fieldName := range fields {
					var exists bool
					v.db.QueryRow(ctx, `
						SELECT EXISTS(
							SELECT 1 FROM concepts c
							WHERE c.name = $1
							AND c.spec->'state' ? $2
						)
					`, w.Concept, fieldName).Scan(&exists)

					if !exists {
						detail.Passed = false
						detail.Expected = fmt.Sprintf("state field %s.%s exists", w.Concept, fieldName)
						detail.Got = "not found"
						detail.Fix = fmt.Sprintf("Add state field '%s' to concept '%s' spec, or fix the sync's where clause.", fieldName, w.Concept)
						return detail
					}
				}
			}
		}
	}

	return detail
}

// ValidateArchAlignment checks that source code region markers align with arch.md
// and that arch.md namespaces are hierarchically consistent.
func (v *Validator) ValidateArchAlignment(ctx context.Context, projectRoot string) []string {
	var issues []string

	// Check 1: arch.md namespace hierarchy consistency (dotwalk validation)
	nsIssues := region.ValidateArchNamespaces(projectRoot)
	issues = append(issues, nsIssues...)

	// Check 2: source regions match arch.md
	archPaths, err := region.ParseArchMd(projectRoot)
	if err != nil {
		issues = append(issues, fmt.Sprintf("cannot parse arch.md: %v", err))
		return issues
	}
	archSet := make(map[string]bool)
	for _, p := range archPaths {
		archSet[p] = true
	}

	gamignore := region.ParseGamignore(projectRoot)
	markers, warnings, _ := region.ScanDirectory(projectRoot, gamignore)

	// Marker warnings are issues
	issues = append(issues, warnings...)

	// Source regions not in arch.md
	sourceSet := make(map[string]bool)
	for _, m := range markers {
		sourceSet[m.Path] = true
		if !archSet[m.Path] {
			issues = append(issues, fmt.Sprintf(
				"region %s found in source (%s:%d) but not in arch.md — add it to arch.md",
				m.Path, m.File, m.StartLine,
			))
		}
	}

	// arch.md entries with no source regions (informational warning)
	for _, p := range archPaths {
		if !sourceSet[p] {
			issues = append(issues, fmt.Sprintf(
				"arch.md declares %s but no source region markers found — either add @region:%s markers to source or remove from arch.md",
				p, p,
			))
		}
	}

	return issues
}

func (v *Validator) findSyncRefsForAction(ctx context.Context, actionRef string) ([]string, error) {
	rows, err := v.db.Query(ctx, `
		SELECT DISTINCT s.name
		FROM sync_refs sr
		JOIN synchronizations s ON s.id = sr.sync_id
		WHERE sr.action_name = $1
		AND s.enabled = true
	`, actionRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		names = append(names, name)
	}
	return names, nil
}
