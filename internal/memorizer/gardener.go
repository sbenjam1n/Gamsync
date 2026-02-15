package memorizer

import (
	"context"
	"fmt"

	"github.com/sbenjam1n/gamsync/internal/region"
)

// GardenFinding represents an entropy issue discovered by the gardener.
type GardenFinding struct {
	RegionPath  string `json:"region_path"`
	Category    string `json:"category"` // stale_todo, orphaned_region, sync_drift, spec_divergence, stale_docs, duplication
	Description string `json:"description"`
	Mechanical  bool   `json:"mechanical"` // can be fixed without human judgment
}

// RunGardener performs a full entropy sweep and queues fix-up turns.
func (m *Memorizer) RunGardener(ctx context.Context, dryRun bool) ([]GardenFinding, error) {
	var findings []GardenFinding

	staleTodos, err := m.findStaleTodos(ctx)
	if err != nil {
		return nil, fmt.Errorf("stale todos: %w", err)
	}
	findings = append(findings, staleTodos...)

	orphaned, err := m.findOrphanedRegions(ctx)
	if err != nil {
		return nil, fmt.Errorf("orphaned regions: %w", err)
	}
	findings = append(findings, orphaned...)

	syncDrift, err := m.findSyncDrift(ctx)
	if err != nil {
		return nil, fmt.Errorf("sync drift: %w", err)
	}
	findings = append(findings, syncDrift...)

	if !dryRun {
		for _, f := range findings {
			if f.Mechanical {
				m.queueTask(ctx, f.RegionPath, "gardener", f.Description)
			}
		}
	}

	return findings, nil
}

func (m *Memorizer) findStaleTodos(ctx context.Context) ([]GardenFinding, error) {
	var findings []GardenFinding

	rows, err := m.db.Query(ctx, `
		SELECT t.id, t.scratchpad, t.scope_path
		FROM turns t
		WHERE t.scratchpad LIKE '%TODO%'
		  AND t.status = 'COMPLETED'
		  AND t.completed_at < NOW() - INTERVAL '7 days'
		  AND NOT EXISTS (
			  SELECT 1 FROM turns t2
			  JOIN turn_regions tr2 ON tr2.turn_id = t2.id
			  JOIN regions r2 ON r2.id = tr2.region_id
			  WHERE r2.path <@ t.scope_path
			    AND t2.created_at > t.completed_at
		  )
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var turnID, scratchpad, scopePath string
		rows.Scan(&turnID, &scratchpad, &scopePath)
		findings = append(findings, GardenFinding{
			RegionPath:  scopePath,
			Category:    "stale_todo",
			Description: fmt.Sprintf("Turn %s has unaddressed TODO in scratchpad: %s", turnID, truncate(scratchpad, 100)),
			Mechanical:  false,
		})
	}
	return findings, nil
}

func (m *Memorizer) findOrphanedRegions(ctx context.Context) ([]GardenFinding, error) {
	var findings []GardenFinding

	// Scan source code for actual region markers
	gamignore := region.ParseGamignore(m.projectRoot)
	sourceMarkers, _, _ := region.ScanDirectory(m.projectRoot, gamignore)
	sourceRegions := make(map[string]bool)
	for _, mk := range sourceMarkers {
		sourceRegions[mk.Path] = true
	}

	// Find DB regions with no source code markers
	rows, err := m.db.Query(ctx, `
		SELECT r.path FROM regions r
		WHERE r.lifecycle_state != 'deprecated'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		rows.Scan(&path)
		if !sourceRegions[path] {
			findings = append(findings, GardenFinding{
				RegionPath:  path,
				Category:    "orphaned_region",
				Description: fmt.Sprintf("Region %s exists in database but has no @region markers in source code. Either add source markers or remove from arch.md and database.", path),
				Mechanical:  false,
			})
		}
	}
	return findings, nil
}

func (m *Memorizer) findSyncDrift(ctx context.Context) ([]GardenFinding, error) {
	var findings []GardenFinding

	rows, err := m.db.Query(ctx, `
		SELECT s.name, sr.concept_name, sr.action_name
		FROM synchronizations s
		JOIN sync_refs sr ON sr.sync_id = s.id AND sr.clause_type = 'when'
		WHERE s.enabled = true
		  AND EXISTS (
			  SELECT 1 FROM flow_log fl
			  WHERE fl.concept_name = sr.concept_name
				AND fl.action_name = sr.action_name
				AND fl.created_at > NOW() - INTERVAL '7 days'
		  )
		  AND NOT EXISTS (
			  SELECT 1 FROM flow_log fl
			  WHERE fl.sync_name = s.name
				AND fl.created_at > NOW() - INTERVAL '7 days'
		  )
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var syncName, conceptName, actionName string
		rows.Scan(&syncName, &conceptName, &actionName)
		findings = append(findings, GardenFinding{
			RegionPath:  "",
			Category:    "sync_drift",
			Description: fmt.Sprintf("Sync %s: action %s/%s is completing but sync never fires. Likely state representation mismatch in where clause.", syncName, conceptName, actionName),
			Mechanical:  false,
		})
	}
	return findings, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
