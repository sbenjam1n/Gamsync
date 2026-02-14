package memorizer

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/sbenjam1n/gamsync/internal/gam"
	"github.com/sbenjam1n/gamsync/internal/queue"
	"github.com/sbenjam1n/gamsync/internal/validator"
)

// Memorizer is the auditor agent that validates proposals and manages turns.
type Memorizer struct {
	db        *pgxpool.Pool
	rdb       *redis.Client
	queue     *queue.Queue
	validator *validator.Validator
}

// New creates a new Memorizer.
func New(db *pgxpool.Pool, rdb *redis.Client, projectRoot string) *Memorizer {
	return &Memorizer{
		db:        db,
		rdb:       rdb,
		queue:     queue.New(rdb),
		validator: validator.New(db, projectRoot),
	}
}

// ConsumeProposals blocks on Redis, processing proposals as they arrive.
func (m *Memorizer) ConsumeProposals(ctx context.Context) error {
	if err := m.queue.EnsureStreams(ctx); err != nil {
		return err
	}

	for {
		msg, msgID, err := m.queue.ReadProposal(ctx, "memorizer_1")
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("proposal read error: %v", err)
			continue
		}

		if err := m.processProposal(ctx, msg.ProposalID, msg.RegionPath); err != nil {
			log.Printf("proposal %s failed: %v", msg.ProposalID, err)
		}

		m.queue.AckProposal(ctx, msgID)
	}
}

func (m *Memorizer) processProposal(ctx context.Context, id, path string) error {
	// Advisory lock on LTREE path
	pathHash := hashTo64Bit(path)
	_, err := m.db.Exec(ctx, "SELECT pg_advisory_lock($1)", pathHash)
	if err != nil {
		return fmt.Errorf("lock %s: %w", path, err)
	}
	defer m.db.Exec(ctx, "SELECT pg_advisory_unlock($1)", pathHash)

	// Fetch proposal
	proposal, err := m.getProposal(ctx, id)
	if err != nil {
		return err
	}

	// Tier 0 + 1: Fast validation
	result, err := m.validator.Validate(ctx, proposal)
	if err != nil {
		return err
	}
	if !result.Passed {
		return m.rejectProposal(ctx, id, result)
	}

	return m.approveProposal(ctx, id, proposal)
}

func (m *Memorizer) getProposal(ctx context.Context, id string) (*gam.Proposal, error) {
	var p gam.Proposal
	var evidenceJSON, syncChangesJSON, deferredJSON []byte
	err := m.db.QueryRow(ctx, `
		SELECT p.id, p.turn_id, p.region_id, r.path, p.action_taken,
		       p.current_state, p.proposed_state, p.sync_changes,
		       p.evidence, p.deferred_actions, p.status
		FROM proposals p
		JOIN regions r ON r.id = p.region_id
		WHERE p.id = $1
	`, id).Scan(
		&p.ID, &p.TurnID, &p.RegionID, &p.RegionPath, &p.ActionTaken,
		&p.CurrentState, &p.ProposedState, &syncChangesJSON,
		&evidenceJSON, &deferredJSON, &p.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch proposal %s: %w", id, err)
	}

	json.Unmarshal(evidenceJSON, &p.Evidence)
	if syncChangesJSON != nil {
		json.Unmarshal(syncChangesJSON, &p.SyncChanges)
	}
	if deferredJSON != nil {
		json.Unmarshal(deferredJSON, &p.DeferredActions)
	}

	return &p, nil
}

func (m *Memorizer) rejectProposal(ctx context.Context, id string, result *gam.ValidationResult) error {
	briefing := fmt.Sprintf("REJECTION (Tier %d, Code %d)\n%s", result.Tier, result.Code, result.Message)
	for _, d := range result.Details {
		if !d.Passed {
			briefing += fmt.Sprintf("\n  Check: %s | Expected: %s | Got: %s", d.Check, d.Expected, d.Got)
			if d.Fix != "" {
				briefing += fmt.Sprintf("\n  Fix: %s", d.Fix)
			}
		}
	}

	detailsJSON, _ := json.Marshal(result.Details)

	_, err := m.db.Exec(ctx, `
		UPDATE proposals
		SET status = 'REJECTED',
			validation_error_code = $1,
			violation_details = $2,
			rejection_reason = $3
		WHERE id = $4
	`, result.Code, detailsJSON, briefing, id)
	return err
}

func (m *Memorizer) approveProposal(ctx context.Context, id string, p *gam.Proposal) error {
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Update proposal status
	tx.Exec(ctx, "UPDATE proposals SET status = 'APPROVED' WHERE id = $1", id)

	// Update region lifecycle state if transition specified
	if p.ProposedState != "" {
		tx.Exec(ctx, `
			UPDATE regions SET lifecycle_state = $1, updated_at = NOW()
			WHERE path = $2
		`, p.ProposedState, p.RegionPath)
	}

	// Insert sync changes if any
	if p.SyncChanges != nil {
		for _, sync := range p.SyncChanges.Added {
			m.insertSync(ctx, sync)
		}
		for _, sync := range p.SyncChanges.Modified {
			m.updateSync(ctx, sync)
		}
		for _, name := range p.SyncChanges.Deleted {
			tx.Exec(ctx, "DELETE FROM synchronizations WHERE name = $1", name)
		}
	}

	// Queue deferred actions as new tasks
	for _, deferred := range p.DeferredActions {
		m.queueTask(ctx, deferred.TargetRegion, deferred.TaskType, deferred.Reason)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Update execution plan progress if turn belongs to one
	var planID string
	m.db.QueryRow(ctx, `
		SELECT pt.plan_id FROM plan_turns pt WHERE pt.turn_id = $1
	`, p.TurnID).Scan(&planID)
	if planID != "" {
		m.UpdatePlanProgress(ctx, planID, p.TurnID)
	}

	return nil
}

func (m *Memorizer) insertSync(ctx context.Context, sync gam.Synchronization) {
	whenJSON, _ := json.Marshal(sync.WhenClause)
	whereJSON, _ := json.Marshal(sync.WhereClause)
	thenJSON, _ := json.Marshal(sync.ThenClause)

	m.db.Exec(ctx, `
		INSERT INTO synchronizations (name, when_clause, where_clause, then_clause, description, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, sync.Name, whenJSON, whereJSON, thenJSON, sync.Description, true)

	// Build sync_refs index
	m.buildSyncRefs(ctx, sync)
}

func (m *Memorizer) updateSync(ctx context.Context, sync gam.Synchronization) {
	whenJSON, _ := json.Marshal(sync.WhenClause)
	whereJSON, _ := json.Marshal(sync.WhereClause)
	thenJSON, _ := json.Marshal(sync.ThenClause)

	m.db.Exec(ctx, `
		UPDATE synchronizations
		SET when_clause = $1, where_clause = $2, then_clause = $3,
		    description = $4, updated_at = NOW()
		WHERE name = $5
	`, whenJSON, whereJSON, thenJSON, sync.Description, sync.Name)

	// Rebuild sync_refs index
	m.db.Exec(ctx, `DELETE FROM sync_refs WHERE sync_id = (SELECT id FROM synchronizations WHERE name = $1)`, sync.Name)
	m.buildSyncRefs(ctx, sync)
}

func (m *Memorizer) buildSyncRefs(ctx context.Context, sync gam.Synchronization) {
	var syncID string
	m.db.QueryRow(ctx, "SELECT id FROM synchronizations WHERE name = $1", sync.Name).Scan(&syncID)
	if syncID == "" {
		return
	}

	for _, w := range sync.WhenClause {
		m.db.Exec(ctx, `
			INSERT INTO sync_refs (sync_id, concept_name, action_name, clause_type)
			VALUES ($1, $2, $3, 'when')
			ON CONFLICT DO NOTHING
		`, syncID, w.Concept, w.Action)
	}

	for _, t := range sync.ThenClause {
		m.db.Exec(ctx, `
			INSERT INTO sync_refs (sync_id, concept_name, action_name, clause_type)
			VALUES ($1, $2, $3, 'then')
			ON CONFLICT DO NOTHING
		`, syncID, t.Concept, t.Action)
	}

	for _, w := range sync.WhereClause {
		for _, patternVal := range w.Pattern {
			if fields, ok := patternVal.(map[string]any); ok {
				for fieldName := range fields {
					m.db.Exec(ctx, `
						INSERT INTO sync_refs (sync_id, concept_name, state_field, clause_type)
						VALUES ($1, $2, $3, 'where')
						ON CONFLICT DO NOTHING
					`, syncID, w.Concept, fieldName)
				}
			}
		}
	}
}

// CreateTurn creates a new turn for a researcher to work on.
func (m *Memorizer) CreateTurn(ctx context.Context, regionPath, prompt string) (string, error) {
	turnID := GenerateTurnID()

	_, err := m.db.Exec(ctx, `
		INSERT INTO turns (id, agent_role, scope_path, status)
		VALUES ($1, 'researcher', $2, 'ACTIVE')
	`, turnID, regionPath)
	if err != nil {
		return "", err
	}

	contextRef, err := m.CompileContext(ctx, regionPath, prompt)
	if err != nil {
		return "", err
	}

	m.queue.PushTask(ctx, queue.TaskMessage{
		TurnID:     turnID,
		RegionPath: regionPath,
		ContextRef: contextRef,
		TaskType:   "implement",
		Prompt:     prompt,
	})

	return turnID, nil
}

// CompileContext extracts concept specs, syncs, plan context, quality grades,
// and turn memory for a region, implementing progressive disclosure.
// The prompt parameter enables relevance-based memory search across all turns.
func (m *Memorizer) CompileContext(ctx context.Context, regionPath string, prompt ...string) (string, error) {
	var parts []string

	parts = append(parts, fmt.Sprintf("# Turn Context: %s\n", regionPath))

	// Get concept specs via junction table + LTREE ancestors
	concepts, _ := m.validator.GetConceptsForRegion(ctx, regionPath)
	if len(concepts) > 0 {
		parts = append(parts, "## Concepts\n")
		for _, c := range concepts {
			parts = append(parts, fmt.Sprintf("### %s\nPurpose: %s\n", c.Name, c.Purpose))
			specJSON, _ := json.MarshalIndent(c.Spec, "", "  ")
			parts = append(parts, fmt.Sprintf("Spec:\n```json\n%s\n```\n", string(specJSON)))
		}
	}

	// Get syncs that reference these concepts
	var syncNames []string
	for _, c := range concepts {
		rows, _ := m.db.Query(ctx, `
			SELECT DISTINCT s.name
			FROM sync_refs sr
			JOIN synchronizations s ON s.id = sr.sync_id
			WHERE sr.concept_name = $1 AND s.enabled = true
		`, c.Name)
		if rows != nil {
			for rows.Next() {
				var name string
				rows.Scan(&name)
				syncNames = append(syncNames, name)
			}
			rows.Close()
		}
	}

	if len(syncNames) > 0 {
		parts = append(parts, "## Synchronizations\n")
		for _, name := range syncNames {
			parts = append(parts, fmt.Sprintf("- %s\n", name))
		}
	}

	// --- Turn Memory: multi-strategy search ---
	// Strategy 1: Region-scoped scratchpads (turns that touched this region or ancestors)
	regionRows, _ := m.db.Query(ctx, `
		SELECT t.scratchpad, t.id, t.scope_path, t.completed_at
		FROM turns t
		JOIN turn_regions tr ON tr.turn_id = t.id
		JOIN regions r ON r.id = tr.region_id
		WHERE (r.path <@ $1::ltree OR r.path @> $1::ltree) AND t.scratchpad IS NOT NULL
		ORDER BY t.completed_at DESC NULLS LAST
		LIMIT 10
	`, regionPath)
	seenTurns := make(map[string]bool)
	if regionRows != nil {
		parts = append(parts, "\n## Turn Memory (region-scoped)\n")
		for regionRows.Next() {
			var sp, tid string
			var scopePath string
			var completedAt interface{}
			regionRows.Scan(&sp, &tid, &scopePath, &completedAt)
			seenTurns[tid] = true
			parts = append(parts, fmt.Sprintf("[%s] scope=%s\n%s\n\n", tid, scopePath, sp))
		}
		regionRows.Close()
	}

	// Strategy 2: Concept-scoped scratchpads (turns touching regions assigned to the same concepts)
	if len(concepts) > 0 {
		conceptNames := make([]string, len(concepts))
		for i, c := range concepts {
			conceptNames[i] = c.Name
		}
		conceptRows, _ := m.db.Query(ctx, `
			SELECT DISTINCT t.scratchpad, t.id, t.scope_path, t.completed_at
			FROM turns t
			JOIN turn_regions tr ON tr.turn_id = t.id
			JOIN regions r ON r.id = tr.region_id
			JOIN concept_region_assignments cra ON cra.region_id = r.id
			JOIN concepts c ON c.id = cra.concept_id
			WHERE c.name = ANY($1) AND t.scratchpad IS NOT NULL
			ORDER BY t.completed_at DESC NULLS LAST
			LIMIT 10
		`, conceptNames)
		if conceptRows != nil {
			var conceptMemory []string
			for conceptRows.Next() {
				var sp, tid string
				var scopePath string
				var completedAt interface{}
				conceptRows.Scan(&sp, &tid, &scopePath, &completedAt)
				if !seenTurns[tid] {
					seenTurns[tid] = true
					conceptMemory = append(conceptMemory, fmt.Sprintf("[%s] scope=%s\n%s\n", tid, scopePath, sp))
				}
			}
			conceptRows.Close()
			if len(conceptMemory) > 0 {
				parts = append(parts, "\n## Turn Memory (concept-scoped)\n")
				for _, m := range conceptMemory {
					parts = append(parts, m+"\n")
				}
			}
		}
	}

	// Strategy 3: Prompt-relevance search (similarity search across all scratchpads)
	if len(prompt) > 0 && prompt[0] != "" {
		simRows, _ := m.db.Query(ctx, `
			SELECT t.id, t.scope_path, t.scratchpad, t.completed_at,
			       similarity(t.scratchpad, $1) AS sim
			FROM turns t
			WHERE t.scratchpad IS NOT NULL AND t.scratchpad % $1
			ORDER BY sim DESC
			LIMIT 5
		`, prompt[0])
		if simRows != nil {
			var relevantMemory []string
			for simRows.Next() {
				var tid, scope, sp string
				var completedAt interface{}
				var sim float64
				simRows.Scan(&tid, &scope, &sp, &completedAt, &sim)
				if !seenTurns[tid] && sim > 0.1 {
					seenTurns[tid] = true
					relevantMemory = append(relevantMemory, fmt.Sprintf("[%s] scope=%s (relevance=%.0f%%)\n%s\n", tid, scope, sim*100, sp))
				}
			}
			simRows.Close()
			if len(relevantMemory) > 0 {
				parts = append(parts, "\n## Turn Memory (prompt-relevant)\n")
				for _, m := range relevantMemory {
					parts = append(parts, m+"\n")
				}
			}
		}
	}

	// Get quality grades
	gradeRows, _ := m.db.Query(ctx, `
		SELECT qg.category, qg.grade
		FROM quality_grades qg
		JOIN regions r ON r.id = qg.region_id
		WHERE r.path = $1::ltree
	`, regionPath)
	if gradeRows != nil {
		parts = append(parts, "\n## Quality Grades\n")
		for gradeRows.Next() {
			var cat, grade string
			gradeRows.Scan(&cat, &grade)
			parts = append(parts, fmt.Sprintf("  %s: %s\n", cat, grade))
		}
		gradeRows.Close()
	}

	contextRef := fmt.Sprintf("/tmp/gam_context_%s.md", regionPath)
	content := ""
	for _, p := range parts {
		content += p
	}
	_ = content // will be written to contextRef in full implementation

	return contextRef, nil
}

func (m *Memorizer) queueTask(ctx context.Context, regionPath, taskType, reason string) {
	turnID := GenerateTurnID()
	m.db.Exec(ctx, `
		INSERT INTO turns (id, agent_role, scope_path, status, task_type)
		VALUES ($1, 'researcher', $2, 'ACTIVE', $3)
	`, turnID, regionPath, taskType)

	m.queue.PushTask(ctx, queue.TaskMessage{
		TurnID:     turnID,
		RegionPath: regionPath,
		TaskType:   taskType,
		Prompt:     reason,
	})
}

// CreatePlan decomposes a goal into ordered turns and stores the plan.
func (m *Memorizer) CreatePlan(ctx context.Context, name, goal string, turns []gam.PlanTurn) (*gam.ExecutionPlan, error) {
	plan := &gam.ExecutionPlan{
		ID:     generatePlanID(),
		Name:   name,
		Goal:   goal,
		Status: "ACTIVE",
	}

	tx, err := m.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	tx.Exec(ctx, `
		INSERT INTO execution_plans (id, name, goal, status)
		VALUES ($1, $2, $3, 'ACTIVE')
	`, plan.ID, plan.Name, plan.Goal)

	for _, pt := range turns {
		turnID := GenerateTurnID()
		tx.Exec(ctx, `
			INSERT INTO turns (id, agent_role, scope_path, status, task_type)
			VALUES ($1, 'researcher', $2, 'ACTIVE', 'implement')
		`, turnID, pt.RegionPath)
		tx.Exec(ctx, `
			INSERT INTO plan_turns (plan_id, turn_id, region_path, ordering, depends_on, status)
			VALUES ($1, $2, $3, $4, $5, 'pending')
		`, plan.ID, turnID, pt.RegionPath, pt.Ordering, pt.DependsOn)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	m.queueReadyPlanTurns(ctx, plan.ID)
	return plan, nil
}

// RecordDecision appends a design decision to an active plan.
func (m *Memorizer) RecordDecision(ctx context.Context, planID string, decision gam.Decision) error {
	decJSON, _ := json.Marshal(decision)
	_, err := m.db.Exec(ctx, `
		UPDATE execution_plans
		SET decisions = decisions || $1::jsonb
		WHERE id = $2 AND status = 'ACTIVE'
	`, decJSON, planID)
	return err
}

// UpdatePlanProgress marks a turn as completed and queues newly unblocked turns.
func (m *Memorizer) UpdatePlanProgress(ctx context.Context, planID, turnID string) error {
	m.db.Exec(ctx, `
		UPDATE plan_turns SET status = 'completed' WHERE plan_id = $1 AND turn_id = $2
	`, planID, turnID)

	var remaining int
	m.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM plan_turns WHERE plan_id = $1 AND status != 'completed'
	`, planID).Scan(&remaining)

	if remaining == 0 {
		m.db.Exec(ctx, `
			UPDATE execution_plans SET status = 'COMPLETED', completed_at = NOW() WHERE id = $1
		`, planID)
	}

	m.queueReadyPlanTurns(ctx, planID)
	return nil
}

func (m *Memorizer) queueReadyPlanTurns(ctx context.Context, planID string) {
	rows, _ := m.db.Query(ctx, `
		SELECT pt.turn_id, pt.region_path
		FROM plan_turns pt
		WHERE pt.plan_id = $1
		  AND pt.status = 'pending'
		  AND NOT EXISTS (
			  SELECT 1 FROM unnest(pt.depends_on) dep
			  JOIN plan_turns dep_pt ON dep_pt.turn_id = dep AND dep_pt.plan_id = $1
			  WHERE dep_pt.status != 'completed'
		  )
	`, planID)
	if rows == nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var turnID, regionPath string
		rows.Scan(&turnID, &regionPath)
		m.db.Exec(ctx, `UPDATE plan_turns SET status = 'active' WHERE plan_id = $1 AND turn_id = $2`, planID, turnID)
		m.queue.PushTask(ctx, queue.TaskMessage{
			TurnID:     turnID,
			RegionPath: regionPath,
			TaskType:   "implement",
		})
	}
}

// GenerateTurnID creates a turn ID in the format T_{date}_{time}_{hex}.
func GenerateTurnID() string {
	now := time.Now().UTC()
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("T_%s_%s_%x",
		now.Format("20060102"),
		now.Format("150405"),
		b,
	)
}

func generatePlanID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func hashTo64Bit(s string) int64 {
	var h uint64 = 14695981039346656037
	for _, c := range []byte(s) {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return int64(h)
}
