# GAM+Sync

## Agentic Memory with Concept Design, Synchronizations, and Structural Enforcement

---

# Part I: Conceptual Foundation

## 1. The Problem

Three failures compound in LLM-driven software development.

**Context rot.** As a project grows, the agent's context window fills with irrelevant, redundant, or conflicting data. Performance degrades. The agent hallucinates structure that doesn't exist, forgets decisions made two sessions ago, and generates code that contradicts what's already there. This is not a model limitation — it's an information architecture problem. The agent has no mechanism to distinguish what matters right now from the accumulated noise of every prior session.

**Illegibility.** Software lacks a direct correspondence between code and observed behavior. A change to "how users register" might require modifications across a routing module, a controller, a model, a middleware, and a test file — none of which mention "registration" in their organizational structure. When an LLM is asked to extend registration behavior, it must reconstruct this scattered mapping from code, which it does unreliably. The code is illegible not because it's poorly written but because its structure doesn't reflect its function.

**Integrity failure.** LLM coding assistants break previously working functionality at rates that make incremental development unreliable. A benchmark study found that when controlled for solution leakage and false positives, LLM success rates on realistic coding tasks drop dramatically. Programmers report that patches recommended by coding assistants often break prior features, and whole-app builders hit undefined complexity ceilings. The root cause is the same: the agent cannot reason about which parts of the system are affected by a change because the boundaries between concerns are not enforced.

These three failures share a structural root. The software lacks modularity that corresponds to functionality, lacks a compositional layer that makes inter-module behavior explicit, and lacks enforcement infrastructure that prevents autonomous agents from violating module boundaries.

## 2. Four Approaches

GAM+Sync synthesizes four approaches that each solve a different face of this problem.

GAM+Sync uses a skeletal architecture file (arch.md) that agents use for orientation, region markers in source code that tag files with namespace paths, a two-command turn ritual (start/end) with a scratchpad turn memory field for cross-session continuity, and the principle that enforcement lives in tool commands rather than agent-remembered rules.

Concept Design and Synchronizations (Meng & Jackson, Onward! 2025) contributed the design methodology where concepts are independent services with purpose, state, typed actions, and operational principles. The synchronization language (when/where/then) declaratively connects concepts without creating dependencies. GAM+Sync adopts concept specifications, the synchronization engine with flow tokens for causal tracking, spec-first generation, and provenance-based debugging via sync attribution. Dropped elements—RDF/SPARQL storage, the Web bootstrap concept, and Turtle-format action records—were replaced by PostgreSQL JSONB, a CLI entry point, and a flow_log table.

GAM+Sync provides enforcement infrastructure: a dual-agent architecture (Memorizer auditor, Researcher coder), proposals as structured change requests with typed transitions, tiered validation (four tiers) with advisory locking on LTREE paths, correction briefings with typed error codes, Redis streams for inter-role queuing, and LTREE storage with ancestor-based invariant inheritance.

OpenAi's harness engineering (Lopopolo, OpenAI 2026) contributed operational learnings from shipping a million-line product with zero manually-written code. GAM+Sync adopts the docs/ directory as repo-local knowledge, execution plans as multi-turn artifacts with decision logs, iterative Tier 3 review loops, runtime validation (Tier 4), gardening agents for entropy management with quality grades, agent-actionable rejection messages, and a preference for boring, well-documented tools well-represented in training data.

## 3. Core Abstractions

### 3.1 Concepts

A concept is a self-contained unit of user-facing functionality with a single compelling purpose. It owns its state, defines its actions, and has no dependencies on other concepts. Concepts are understandable independently — when a concept refers to values from other concepts, those values are treated as fully polymorphic type parameters.

A concept specification includes:

- **Purpose**: A single statement describing why the concept exists and what value it delivers.
- **Type parameters**: Fully polymorphic variables (e.g., `[U]` for users, `[A, U]` for articles and users) ensuring no external coupling.
- **State**: Relational state components in the style of Alloy — typed mappings like `password: U -> string`, `status: Invoice -> {UNPAID, PAID, CANCELLED}`.
- **Actions**: Named operations with typed input/output signatures, split into cases by output pattern (success vs. error). Named arguments enable partial matching in synchronizations.
- **Invariants**: Rules that must always hold — representation invariants for internal data, abstract invariants for public behavior.
- **Operational principle**: An archetypal scenario demonstrating how the concept fulfills its purpose. Doubles as a test scenario.

Example:

```
concept SearchSource [S]
purpose
  to register and query torrent index providers
state
  sources: set S
  name: S -> string
  endpoint: S -> url
  enabled: S -> boolean
  rate_limit: S -> int
actions
  register [source: S; name: string; endpoint: url]
    => [source: S]
    add source to sources, set enabled true
    return source reference
  register [source: S; name: string; endpoint: url]
    => [error: string]
    if name not unique or endpoint unreachable
    return error description
  query [source: S; terms: string]
    => [results: []Result]
    check rate limit, execute search, return results
  query [source: S; terms: string]
    => [error: string]
    if source disabled or rate limited or unreachable
    return error description
  disable [source: S]
    => [source: S]
    set enabled false, return source reference
invariants
  - rate_limit_positive: rate_limit(s) > 0 for all s in sources
  - name_unique: no two sources share a name
operational principle
  after register [source: x; name: "nyaa"; endpoint: "https://nyaa.si/api"]
    => [source: x]
  then query [source: x; terms: "ubuntu"]
    => [results: rs]
  where len(rs) >= 0
  and after disable [source: x] => [source: x]
  then query [source: x; terms: "ubuntu"]
    => [error: e]
  where e contains "disabled"
```

Concepts are similar to microservices in spirit but stricter: microservices routinely call each other and query each other's state, creating dependency tangles. Concepts have no such dependencies. A concept may depend on lower-level services (databases, networking, datatype libraries) but never on other concepts.

### 3.2 Synchronizations

Synchronizations are small declarative rules that express all inter-concept behavior. They have the form: when these actions happen, where the current state satisfies these conditions, then invoke these other actions.

```
sync FanOutSearch
when {
  Web/request: [
    method: "search" ;
    terms: ?terms ]
  => [request: ?request] }
where {
  SearchSource: { ?s enabled: true } }
then {
  SearchSource/query: [
    source: ?s ;
    terms: ?terms ] }
```

This rule says: when a web request for "search" arrives with some terms, for every enabled search source, invoke a query on that source with those terms. The where clause maps a single binding (the search terms) to multiple bindings (one per enabled source), producing one invocation per source without explicit iteration.

Synchronizations are independent of each other. Adding "send analytics event on search" is one new sync. Removing it deletes one sync. No code changes to any concept.

```
sync SearchAnalytics
when {
  SearchSource/query: [terms: ?terms]
  => [results: ?results] }
then {
  Analytics/track: [
    event: "search_completed" ;
    data: [terms: ?terms; result_count: ?count] ] }
```

The three clauses:

- **when**: Pattern-matches action completions. Multiple actions in the when clause must share the same flow token (see §3.3). Partial matching — empty argument lists match any successful completion.
- **where**: Queries concept states and binds variables. Semantics: a function from one binding to a set of bindings. Can cross concept boundaries (e.g., joining User and Profile state). Supports OPTIONAL for conditional joins, BIND for computed values, FILTER for constraints.
- **then**: Invokes actions on concepts with bound variables. One invocation per binding from the where clause.

Error handling requires no special mechanism. An error output is just another named argument:

```
sync SearchError
when {
  Web/request: [method: "search"]
  => [request: ?request]
  SearchSource/query: []
  => [error: ?error] }
then {
  Web/respond: [
    request: ?request ;
    error: ?error ;
    code: 502 ] }
```

### 3.3 Flow Tokens

A flow token is a unique identifier assigned to the root action of a causal chain (typically a web request or CLI command) and propagated to every action triggered by synchronizations within that chain. All actions in a sync's when clause must share the same flow token. All actions invoked in the then clause inherit that token.

Flow tokens prevent cross-request interference. When two users search simultaneously, their SearchSource/query completions carry different flow tokens. A sync that matches on both a search request and a query completion only fires when both carry the same token — preventing User A's results from leaking into User B's response.

Flow tokens also enable provenance tracking. Given a flow token, you can reconstruct the full causal graph: which request initiated the chain, which syncs fired, which actions resulted, and what state was read. This is the foundation for debugging — trace back from a failure to the sync that caused it and the conditions that made it fire.

### 3.4 Proposals

A proposal is a structured change request emitted by a Researcher agent. It carries:

- **Scope**: The LTREE region path and concept assignment(s) that the change targets.
- **Transition**: The action taken, the current concept state, and the proposed next state.
- **Synchronizations**: Any new, modified, or deleted sync rules (optional — proposals can be pure code changes or pure sync changes or both).
- **Evidence**: Structured analysis blocks required by the concept's invariants — API surface analysis, migration analysis, dependency analysis. The agent declares what changed; the validator verifies the claims.
- **Deferred actions**: Tasks that belong to other concepts, queued for separate Researcher sessions.
- **Scratchpad**: What the agent did and what should happen next.

Proposals are the handshake between the Researcher (who writes code) and the Memorizer (who validates and approves). Every change to the codebase flows through a proposal. No direct commits.

### 3.5 Regions

A region is a named, hierarchical scope marker that appears in two places:

**In arch.md**, as the namespace skeleton:
```
# @region:app.search.sources Search Source Implementations
# @endregion:app.search.sources
```

**In source code**, as structural comments:
```go
// @region:app.search.sources.btv2
package search

type BTv2Source struct { ... }

// @region:app.search.sources.btv2.adapter
func (s *BTv2Source) Query(terms string) ([]Result, error) { ... }
// @endregion:app.search.sources.btv2.adapter

// @region:app.search.sources.btv2.rate_limit
func (s *BTv2Source) checkRateLimit() bool { ... }
// @endregion:app.search.sources.btv2.rate_limit

// @endregion:app.search.sources.btv2
```

Region markers use the comment syntax native to each language. The namespace path is language-agnostic. The CLI detects comment style from file extension.

Regions serve four purposes:

1. **Context steering**: arch.md is ~80 lines of region markers. An agent reads it and knows the full system structure without reading any code.
2. **Structural enforcement**: Tier 0 validation checks that code regions match arch.md, code doesn't exist outside regions (unless .gamignore'd), and agents don't modify regions outside their declared scope.
3. **Tree view generation**: The CLI scans region markers across all source files and produces a tree view showing the actual code structure, with file locations, unregioned code warnings, and mismatches against arch.md.
4. **Generation quality**: Wrapping code blocks in region markers forces the LLM to name its structural decisions. Every block declares where it lives in the namespace. This commitment reduces structural drift during generation and makes subsequent edits precisely scopable.

### 3.6 Turns

A turn is one bounded agent session. It starts with `gam turn start` and ends with `gam turn end`. Everything the agent does in between — code written, regions touched, proposals emitted — is associated with the turn.

The turn carries a scratchpad: a text field that says what the agent did and what should happen next. The next `gam turn start` displays the previous turn's scratchpad. This is the cheapest effective cross-session continuity mechanism — no infrastructure beyond a database column and a CLI command that reads it.

Turns are recorded in PostgreSQL with their scratchpads, touched regions, proposals, and status. `gam turn memory <region>` queries all scratchpads from turns that touched a region. `gam turn search "pagination"` does full-text search across scratchpads.

---

# Part II: Engineering Design

## 4. The Split and Progressive Disclosure

arch.md is a skeleton. It is `@region`/`@endregion` markers forming a namespace tree with one-line descriptions. Nothing else. No prose, no specs, no scratchpads, no JSON. It exists to be read by agents as a map — pure token-efficient structure that steers attention toward the right namespaces before the agent touches anything.

PostgreSQL mirrors the same LTREE hierarchy and carries everything: concept specifications, synchronization definitions, invariant rules, action signatures, turn history, scratchpads, execution plans, proposals, flow logs, evidence, and the concept-region junction table. The database is the source of truth. arch.md is a projection of its namespace tree.

A `docs/` directory provides a second projection — repo-local, versioned markdown files that any agent can read without CLI access. This matters because an agent working through a different tool (Codex, Claude Code, a CI runner) may not have the `gam` CLI but always has the filesystem. The docs directory contains:

```
docs/
├── concepts/
│   ├── index.md              # Concept catalog with purposes
│   ├── search-source.md      # Full concept spec as markdown
│   ├── billing-lifecycle.md
│   └── ...
├── syncs/
│   ├── index.md              # Sync catalog with descriptions
│   ├── fan-out-search.md     # Sync rule with referenced concepts
│   └── ...
├── exec-plans/
│   ├── active/
│   │   └── add-pagination.md # Multi-turn plan with progress
│   ├── completed/
│   │   └── btv2-source.md    # Completed plan with decisions
│   └── tech-debt.md          # Known debt with quality grades
├── quality/
│   ├── grades.md             # Per-region quality assessment
│   └── golden-principles.md  # Mechanical rules for codebase coherence
└── design/
    ├── core-beliefs.md       # Agent-first operating principles
    └── ...
```

`gam docs export` generates this directory from PostgreSQL. `gam docs import` reads it back. The database is the source of truth for querying and validation. The docs directory is the source of truth for agent context when the CLI is unavailable. The CLI keeps them in sync.

This structure enables **progressive disclosure**: agents start with arch.md (the map, ~80 lines), then navigate to docs/ for depth on specific concepts or plans, and use CLI commands for interactive queries. At no point is the agent overwhelmed with the full system state. Each layer provides exactly the resolution needed for the current task.

Redis provides durable inter-role queuing. Two streams with consumer groups — one for tasks (Memorizer pushes, Researcher pops), one for proposals (Researcher pushes, Memorizer pops). Messages persist until acknowledged. Process restarts don't lose in-flight work. The same queues support sequential single-model execution (human switches between Memorizer and Researcher skills) and true parallel multi-model execution without changes.

The CLI (`gam`) bridges all three stores. `gam turn start` reads the DB and generates context. `gam turn end` validates work and writes results back. `gam concept show`, `gam sync list`, `gam turn memory` query the database and produce focused extracts. The agent never writes SQL. It reads arch.md for orientation, reads docs/ for depth, and runs CLI commands for interactive queries.

This split exists because context windows are finite and expensive. An agent working on `app.search.sources` doesn't need the full concept spec for every concept in the system — it needs the namespace tree (arch.md, always in context) plus the specific concept specs and syncs relevant to its task (docs/ or CLI extract, injected on demand).

## 5. The Junction Table

The concept-region relationship is many-to-many with a role discriminator.

A region can implement multiple concepts. `app.billing.stripe` implements BillingLifecycle (role=implementation) and PaymentGateway (role=integration). Both concepts' invariants apply when validating a proposal against that region.

A concept can span multiple regions. BillingLifecycle governs `app.billing`, `app.billing.stripe`, `app.billing.invoice`. When an agent proposes a change to `app.billing.stripe`, the validator walks LTREE ancestors (`app.billing` → `app`) and collects all concept assignments via the junction table. Invariants accumulate from root to leaf, most specific last.

Roles distinguish the relationship type:
- **implementation**: This region defines the concept's behavior.
- **integration**: This region bridges two concepts (both apply).
- **test**: This region tests the concept (operational principle enforcement).
- **consumer**: This region uses the concept but doesn't define it.

The junction table enables impact analysis. "If I change BillingLifecycle's state machine, which regions are affected?" is a single indexed join.

## 6. How Synchronizations Work

Synchronizations replace imperative cross-concept code. Instead of an agent writing a handler that calls User.register then Profile.create then JWT.generate, three independent syncs exist:

```
Registration:       Web/request[register] → User/register
DefaultProfile:     User/register[success] → Profile/register
NewUserToken:       User/register[success] → JWT/generate
```

Each rule is independent. Adding "send welcome email on registration" is one new sync row. Removing it deletes one row. No code changes to any concept.

Syncs are stored in PostgreSQL as JSONB (when_clause, where_clause, then_clause) and referenced by name in the sync_refs table. The sync engine evaluates them reactively: when an action completes, it checks all syncs whose when_clause matches, evaluates where clauses against concept state, and invokes then_clause actions with bound variables.

**Flow-scoped matching is mandatory.** All actions in a sync's when clause must carry the same flow token. This is not just a provenance feature — it is an execution semantic. Without it, a sync can fire because an action from a different request happened to match its when clause. The sync engine enforces this by joining on the flow column when evaluating when clauses.

Flow tokens connect the chain. The initial Web/request gets a flow token. Every action triggered by a sync inherits that token. When debugging, `gam flow trace <token>` shows the full causal graph with sync attribution at every edge.

The validator checks sync integrity at Tier 1: if a proposal removes an action that a sync references, that's a rejection before any code runs. The sync_refs table makes this check a simple indexed query.

## 7. The Execution Model

There is flexibility in how the Memorizer and Researcher roles are executed. Three configurations are supported by the same infrastructure.

**Sequential single-model (human-in-the-loop).** One model endpoint. The human runs the model with Memorizer skill, then with Researcher skill, then with Memorizer skill again. Redis queues persist between invocations. The human can inspect queues, intervene, or redirect between any step.

```bash
gam memorizer run    # Process pending proposals, create turns from human prompt
gam researcher run   # Pop task queue, do work, push proposals
gam memorizer run    # Validate proposals, approve/reject, report to human
```

Or the combined command with human checkpoints:

```bash
gam run              # Memorizer → wait → Researcher → wait → Memorizer
```

**Sequential automated.** Same as above but the loop runs without human checkpoints. `gam run --auto` cycles Memorizer and Researcher passes until the task queue is empty or a rejection requires human input.

**Parallel multi-model.** Multiple model instances running simultaneously. One or more Memorizers consuming the proposal stream. One or more Researchers consuming the task stream. Redis consumer groups ensure exactly-once processing. Advisory locks on LTREE paths prevent concurrent modification of the same region.

All three configurations use the same Redis streams, the same consumer groups, the same PostgreSQL schema, and the same CLI commands. The difference is only in how many model instances are running and whether a human gates the transitions.

**The two queues:**

```
agent_tasks       — Memorizer pushes, Researcher pops
                    Payload: {turn_id, region_path, compiled_context_ref, task_type}

agent_proposals   — Researcher pushes, Memorizer pops
                    Payload: {turn_id, proposal_id, region_path}
```

**Consumer groups:**

```
researcher_pool   — on agent_tasks stream
memorizer_pool    — on agent_proposals stream
```

## 8. Execution Plans

A turn is one agent session. An execution plan spans multiple turns toward a single goal. When the Memorizer decomposes a task like "add cursor-based pagination to the article feed" into work touching the article concept, the list endpoint, and the test suite, it creates an execution plan that tracks the full effort.

An execution plan contains:

- **Goal**: What the multi-turn effort achieves.
- **Turns**: Ordered list of turns with their target regions, dependencies (which turns must complete first), and status (pending, active, completed, blocked).
- **Decisions**: A log of design choices made during execution — why pagination uses cursor-based instead of offset, why the cursor encodes created_at instead of id. These are checked into the repo so future agents understand the rationale.
- **Progress**: Which turns are done, which are in flight, what's blocked and why.
- **Quality grade**: An assessment of the current state — are tests passing, is documentation current, are there known gaps.

Plans are stored in PostgreSQL and projected to `docs/exec-plans/active/` (in-progress) and `docs/exec-plans/completed/` (finished, retained as decision history). When a Researcher starts a turn that's part of a plan, the compiled context includes the plan with progress markers. The Researcher knows what's already done, what's pending, and what constraints the Memorizer established.

Completed plans are not deleted. They form a versioned decision log that explains *why* the system looks the way it does. When a future agent asks "why does pagination use cursors instead of offsets?", the answer is in `docs/exec-plans/completed/add-pagination.md`, not in someone's memory or a Slack thread.

Plans also track known technical debt. `docs/exec-plans/tech-debt.md` catalogs gaps with quality grades per region and concept. The gardening agent (see §14) uses this as its work queue.

## 9. Lifecycle Hooks

The turn ritual commands (`gam turn start`, `gam region touch`, `gam turn end`) are not just utilities — they are enforcement points where the system injects behavior the agent would otherwise forget. Each command fires a registered set of hooks.

### 9.1 on_turn_start

Fires when `gam turn start --region <path>` runs.

| Hook | Action |
|------|--------|
| load_scratchpad | Display previous turn's scratchpad for the target region |
| compile_context | Extract concept specs, syncs, recent turn memory from DB; write context file |
| create_branch | Create git branch for this turn |
| check_queues | Process any pending proposals or correction briefings in Memorizer queue |
| check_conflicts | Warn if region has active turns from other agents |

### 9.2 on_region_enter

Fires when `gam region touch <path>` runs (or when the agent first modifies a file containing a region).

| Hook | Action |
|------|--------|
| validate_exists | Reject if region doesn't exist in arch.md (architecture-first enforcement) |
| check_scope | Reject if region is outside agent's declared scope for this turn |
| load_concept | Append region's concept spec(s) from junction table to agent context |
| load_syncs | Append syncs referencing this region's concept actions to context |
| check_gamignore | If file matches .gamignore, skip structural enforcement for it |
| scaffold_regions | If file or region markers don't exist, create skeleton with correct comment syntax |

### 9.3 on_turn_end

Fires when `gam turn end --scratchpad "..."` runs.

| Hook | Action |
|------|--------|
| tier_0_validate | Structural checks: region markers match arch.md, no unregioned code |
| tier_1_validate | State machine transitions, invariant rules, sync reference integrity |
| tier_2_integrate | Dagger container: compile, test, evidence truthfulness |
| tier_4_runtime | (If configured) Boot app, run operational principle live, check traces |
| generate_tree | Produce tree view of all modified files showing region structure |
| record_regions | Log which regions were touched in turn_regions table |
| save_scratchpad | Write scratchpad to turns table |
| update_plan | If turn is part of an execution plan, update plan progress |
| update_states | Update region lifecycle states if proposal transitions them |
| export_docs | Regenerate docs/ projections for any modified concepts or syncs |
| push_proposals | Queue proposals for Memorizer if running in async mode (Tier 3 loop runs there) |
| commit_branch | Commit git branch with turn metadata |

### 9.4 Custom Hooks

The hook registry supports user-defined hooks:

- "On turn end for any region under `app.billing`, run the Stripe webhook test suite."
- "On region enter for anything tagged `phase=stable`, require a justification field in the proposal."
- "On turn start, check if the target region's tests are passing before allowing work to begin."

Hooks are stored in PostgreSQL and configurable per-region, per-concept, or globally. They follow the same when/then pattern as synchronizations — the development workflow is itself a set of concepts with rules governing transitions.

## 10. The Validation Pipeline

Each tier gates the next. Most proposals stop at Tier 0 or 1.

### Tier 0: Structural (CLI, instant, no DB)

- Modified files have region markers.
- Region namespaces match arch.md's hierarchy.
- No code exists outside region boundaries (unless .gamignore'd).
- Modified regions are within the agent's declared scope.
- Region nesting in code matches arch.md nesting.

This catches "agent wrote code without tagging it" and "agent modified a region it wasn't assigned to." Cheap and immediate.

### Tier 1: State Machine and Sync Integrity (Go validator, microseconds, reads DB)

- Proposal's transition is legal in the concept's state machine.
- Required evidence blocks are present (RequireAnalysis flags from concept invariants).
- Invariant rules pass against evidence (API surface, migration, dependency checks).
- No broken sync references: action removal doesn't orphan a sync; state field changes don't break sync where clauses.
- New syncs reference only actions and state fields that exist in defined concepts.

Pure function, deterministic, typed results. Runs against concepts collected via LTREE ancestor walk through the junction table.

### Tier 2: Integration (Dagger container, seconds)

- Code compiles in isolation (region-scoped container build via context pinning).
- Tests pass.
- Evidence is truthful: actual API exports match declared APIAnalysis, actual SQL operations match declared MigrationAnalysis.
- Operational principles from concept specs execute successfully as test scenarios.

Dagger containers receive only the files relevant to the LTREE path (context pinning), preventing pollution. Cache volumes keyed by LTREE path avoid redundant rebuilds.

### Tier 3: LLM Review Loop (optional, seconds per iteration)

- Only for high-risk changes: state transitions to STABLE, sync modifications, concept spec changes.
- Focused review with concept spec + affected syncs + proposal as context.
- Checks architectural alignment, missing behavioral rules, sync ordering issues (the class of bug demonstrated in the registration ordering problem from the Jackson paper).

Unlike Tiers 0-2, Tier 3 is not a single-pass gate — it is a **review loop**. When the LLM auditor identifies a concern, instead of rejecting outright, it pushes the concern back to the Researcher as a typed review comment via the Redis task queue. The Researcher addresses the comment, resubmits, and Tier 3 re-evaluates. This loop runs until the auditor is satisfied or a configurable maximum iteration count is reached (default: 3), at which point it escalates to human review.

The infrastructure for this is Redis queue round-trips between Memorizer and Researcher — the same mechanism used for normal turn processing. The change is in the Memorizer's logic: rejection at Tier 3 becomes "request changes" with structured feedback, not a terminal state.

### Tier 4: Runtime Validation (optional, Dagger container, minutes)

- Boots the application in an ephemeral environment (Dagger container with the full app stack).
- Runs the operational principle as a live test — not a unit test but an actual HTTP request/response or UI interaction sequence.
- Checks observability output (logs, traces) against expectations.
- Compares flow log entries ("what the sync rules say should happen") against actual runtime action traces ("what happened when we ran it").

Runtime validation catches the class of bugs where code compiles, tests pass, but the system doesn't behave correctly in integration. This tier is expensive and optional — configured per-concept or per-region for critical paths.

The ephemeral app environment feeds its action traces into the flow log. The Memorizer compares expected sync-driven behavior against observed behavior. Discrepancies produce typed review comments (Tier 3 loop) or hard rejections depending on severity.

For agents with access to browser automation (e.g., Chrome DevTools Protocol), Tier 4 can include UI-level validation: navigate to a page, trigger an action, verify the result. This is configured per-region via lifecycle hooks.

## 11. .gamignore

Same semantics as .gitignore. Glob patterns for paths that Tier 0 skips when checking "code exists outside region boundaries."

```
# .gamignore

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
```

Ignored files are invisible to Tier 0 only. Dagger still builds them, tests still run against them. .gamignore says "this file is not governed by concept boundaries." If code in an ignored path starts accumulating behavior that belongs in a concept, that's a signal to extract it into a proper region with a concept assignment.

## 12. Region Validations and Tree View

### 12.1 Tree Generation

The CLI scans region markers across all source files and produces a structural view:

```
$ gam tree src/

app
├── search
│   ├── sources
│   │   ├── btv2                    [src/search/btv2.go:1-45]
│   │   │   ├── adapter             [src/search/btv2.go:5-22]
│   │   │   └── rate_limit          [src/search/btv2.go:24-38]
│   │   ├── nyaa                    [src/search/nyaa.go:1-52]
│   │   │   ├── adapter             [src/search/nyaa.go:5-28]
│   │   │   └── parser              [src/search/nyaa.go:30-48]
│   │   └── rarbg                   [src/search/rarbg.go:1-39]
│   ├── aggregator                  [src/search/aggregator.go:1-67]
│   │   ├── fanout                  [src/search/aggregator.go:8-34]
│   │   ├── collect                 [src/search/aggregator.go:36-52]
│   │   └── finalize                [src/search/aggregator.go:54-65]
│   └── ranker                      [src/search/ranker.go:1-43]
├── billing
│   ├── lifecycle                   [src/billing/lifecycle.go:1-89]
│   └── stripe                      [src/billing/stripe.go:1-56]
└── notification                    [src/notification/sender.go:1-34]

⚠ UNREGIONED CODE:
  src/search/util.go:1-18 (no region markers — add to .gamignore or wrap in region)
  src/billing/stripe.go:57-63 (code after @endregion — probable generation artifact)

⚠ ARCH.MD MISMATCH:
  app.search.cache exists in arch.md but has no code regions
```

### 12.2 Structural Diff

On turn end, the tree view is generated for all modified files and stored with the turn record. This produces a structural diff:

```
$ gam turn diff T_20260212_143052_abc123

REGIONS ADDED:
  + app.search.sources.btv2          [src/search/btv2.go]
  + app.search.sources.btv2.adapter  [src/search/btv2.go]
  + app.search.sources.btv2.rate_limit [src/search/btv2.go]

REGIONS MODIFIED:
  ~ app.search.aggregator.fanout     [src/search/aggregator.go:8-34]

REGIONS UNCHANGED:
  = app.search.sources.nyaa          [src/search/nyaa.go]
  = app.search.sources.rarbg         [src/search/rarbg.go]

SCOPE CHECK: All changes within declared scope (app.search) ✓
```

### 12.3 Comment Convention

Region markers use the comment syntax native to each language:

| Language | Start | End |
|----------|-------|-----|
| Go, C, Java, JS, TS, Rust, Swift | `// @region:path` | `// @endregion:path` |
| Python, Ruby, Shell, YAML | `# @region:path` | `# @endregion:path` |
| HTML, XML, Vue, Svelte | `<!-- @region:path -->` | `<!-- @endregion:path -->` |
| SQL | `-- @region:path` | `-- @endregion:path` |
| CSS | `/* @region:path */` | `/* @endregion:path */` |
| Lua, Haskell | `-- @region:path` | `-- @endregion:path` |

The CLI detects comment style from file extension. The namespace path is identical regardless of language.

### 12.4 Scaffold Generation

`gam region touch <path> --file <filepath>` creates region marker skeletons:

If the file doesn't exist:
```go
// @region:app.search.sources.btv2
package search

// @endregion:app.search.sources.btv2
```

If the file exists but the region doesn't, the hook appends at the appropriate nesting level. The agent never writes region markers manually — the tool creates them, the agent fills in implementation.

## 13. The Agent Prompt

The 1361-line HRM V2 prompt with 18 absolute rules and 18 stop conditions collapses to this:

```
You are working in a GAM+Sync codebase.

Start every session:
  gam turn start --region <target>
  This gives you: previous scratchpad, concept specs, relevant syncs, a git branch.

When writing code, ensure it's inside region markers:
  // @region:namespace.path  (tool creates these — use gam region touch)
  // @endregion:namespace.path

When behavior crosses concepts, write synchronizations, not imperative handlers.

When your task involves a concept you're not assigned to, emit a deferred action.

End every session:
  gam turn end --scratchpad "what you did and what's next"
  This validates your work, saves memory, generates tree view, queues proposals.

The CLI handles enforcement. You handle thinking and coding.
```

Twelve of the eighteen original rules became hooks that fire automatically. The remaining rules are either eliminated by the architecture (aspects as DB fields) or are genuinely things the agent needs to decide.

## 14. Entropy Management and Gardening

Agent-generated codebases accumulate entropy. Agents replicate patterns that already exist — including suboptimal ones. Over time, this produces drift: inconsistent naming conventions, duplicated utility functions, dead code paths, documentation that no longer reflects behavior. Without intervention, each new turn compounds the problem.

The Harness engineering approach (OpenAI, 2026) demonstrated that manual cleanup ("20% of the week cleaning AI slop") doesn't scale. The solution is automated gardening: recurring background turns that scan for deviations and open targeted fix-up proposals.

### 14.1 The Gardening Agent

`gam gardener run` triggers a set of background turns, not prompted by a human task but by a scheduled sweep. The gardener checks for:

- **Stale scratchpads**: Turns that reference TODOs never picked up. If a scratchpad says "TODO: add idempotency key check" and no subsequent turn in that region addresses it, the gardener flags it.
- **Orphaned regions**: arch.md entries with no corresponding code, or code regions with no arch.md entry.
- **Sync drift**: Syncs that haven't fired in N days despite matching action completions existing in the flow log. This catches the premium-ads class of bug — a sync that should fire but doesn't because the underlying state representation changed.
- **Concept spec divergence**: The actual code behavior doesn't match the operational principle. The gardener runs operational principles as tests and flags failures.
- **Documentation staleness**: Concept specs in `docs/concepts/` that reference actions or state fields no longer present in the code.
- **Pattern duplication**: Multiple regions implementing the same utility logic instead of sharing a common package.
- **Quality grade degradation**: Per-region quality assessments (test coverage, documentation completeness, invariant coverage) that have dropped below thresholds.

Each finding becomes a proposal — a fix (if mechanical) or a flagged concern in `docs/exec-plans/tech-debt.md` (if it requires judgment). Mechanical fixes are auto-merged if they pass all validation tiers. Judgment calls are queued for human review.

The gardener runs on the same Memorizer/Researcher infrastructure as feature development. It's just a different task type in the Redis queue. Register it as a cron-triggered lifecycle hook or run it manually.

### 14.2 Golden Principles

Golden principles are opinionated, mechanical rules that keep the codebase coherent for future agent runs. They are stored in `docs/quality/golden-principles.md` and enforced by the gardener and by custom linters (which are themselves agent-generated).

Examples:

- Prefer shared utility packages over hand-rolled helpers to keep invariants centralized.
- Validate data shapes at boundaries — don't probe data speculatively.
- Use structured logging with consistent field names.
- Enforce file size limits per region (e.g., no single region exceeds 500 lines).
- Enforce naming conventions for concept actions (verb_noun), state fields (noun), and syncs (EventOutcome).

When a golden principle is violated, the lint error message is written as a remediation instruction for the agent: not "naming violation on line 42" but "Action name `getData` in concept SearchSource should follow verb_noun convention — rename to `get_data` or `fetch_results` and update all sync references via sync_refs table."

### 14.3 Agent-Actionable Rejection Messages

Every validation rejection — at any tier — is written as if it's a prompt for the next Researcher turn. This is a design property, not just a formatting preference.

Bad: `"Invariant violation: tier field type mismatch"`

Good: `"Invariant violation: sync SuppressAdsForPremium references state field 'tier' with expected type string, but concept Subscription now stores tier as integer. Update the sync's where clause to use integer comparison (tier: 2 for PREMIUM), or add a string accessor to the Subscription concept spec. Affected syncs: SuppressAdsForPremium, TierBasedRateLimiting. Run 'gam sync list --concept Subscription' to see all affected syncs."`

The correction briefing is the compiled context for the next turn. If the rejection message is vague, the Researcher wastes its turn figuring out what went wrong. If it's specific and actionable, the Researcher fixes the issue in one pass.

The `ValidationDetail` struct's `Fix` field is mandatory for all non-passing checks. The Memorizer's `rejectProposal` method composes these into a correction briefing that reads like a task description, not an error log.

## 15. Technology Preferences

Agents work best with technologies that are well-represented in training data, have stable APIs, and compose predictably. The GAM+Sync stack reflects this:

- **PostgreSQL**: The most documented relational database. LTREE, JSONB, advisory locks, and pg_trgm are all stable, well-understood extensions.
- **Redis**: Streams with consumer groups provide exactly-once processing with minimal configuration. The API is small and stable.
- **Go**: Statically typed, simple concurrency model, excellent tooling. Agents generate correct Go more reliably than languages with complex type systems or implicit behavior.
- **Dagger**: Container-based execution with a Go SDK. Composable, cacheable, deterministic.
- **JSONB over custom DSL parsers**: The sync engine stores clauses as JSONB. Less elegant than a custom parser, but agents already know how to work with JSON. Agent-legibility beats aesthetic preference.

When a library is opaque or poorly documented in training data, it's sometimes cheaper to reimplement the needed functionality in-repo with full test coverage than to debug the agent's misunderstanding of the library. This applies especially to niche libraries with unstable APIs.

Prefer "boring" technology. Boring means well-documented, stable, and composable. Boring compounds.

---

# Part III: Implementation

## 16. PostgreSQL Schema

```sql
-- Extensions
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;  -- for scratchpad full-text search

-- ────────────────────────────────────────────
-- Concepts
-- ────────────────────────────────────────────

CREATE TABLE concepts (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name          VARCHAR(255) UNIQUE NOT NULL,
  purpose       TEXT NOT NULL,
  spec          JSONB NOT NULL,        -- Full concept spec: state, actions, operational_principle
  state_machine JSONB NOT NULL,        -- States, transitions (subset of spec, denormalized for fast gate)
  invariants    JSONB NOT NULL DEFAULT '[]',
  created_at    TIMESTAMPTZ DEFAULT NOW(),
  updated_at    TIMESTAMPTZ DEFAULT NOW()
);

-- spec JSONB structure:
-- {
--   "type_params": ["S"],
--   "state": {
--     "sources": {"type": "set", "of": "S"},
--     "name": {"type": "map", "from": "S", "to": "string"},
--     "endpoint": {"type": "map", "from": "S", "to": "url"},
--     "enabled": {"type": "map", "from": "S", "to": "boolean"}
--   },
--   "actions": {
--     "register": {
--       "cases": [
--         {"input": {"source": "S", "name": "string", "endpoint": "url"},
--          "output": {"source": "S"},
--          "description": "add source to sources, set enabled true"},
--         {"input": {"source": "S", "name": "string", "endpoint": "url"},
--          "output": {"error": "string"},
--          "description": "if name not unique or endpoint unreachable"}
--       ]
--     },
--     "query": { ... },
--     "disable": { ... }
--   },
--   "operational_principle": "after register[source:x] => [source:x] ..."
-- }

-- state_machine JSONB structure:
-- {
--   "states": ["ACTIVE", "DISABLED"],
--   "transitions": [
--     {"from": "ACTIVE", "to": "DISABLED", "action": "disable"},
--     {"from": "DISABLED", "to": "ACTIVE", "action": "enable"}
--   ]
-- }

-- invariants JSONB structure:
-- [
--   {"name": "rate_limit_positive", "rule": "rate_limit > 0", "type": "representation"},
--   {"name": "name_unique", "rule": "unique(name)", "type": "representation"},
--   {"name": "api_rule", "config": {"no_removals": true}, "type": "api"},
--   {"name": "migration_rule", "config": {"forbidden": ["DROP_COLUMN"]}, "type": "migration"}
-- ]


-- ────────────────────────────────────────────
-- Regions
-- ────────────────────────────────────────────

CREATE TABLE regions (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  path            ltree UNIQUE NOT NULL,
  description     TEXT,
  lifecycle_state VARCHAR(50) DEFAULT 'draft',  -- draft, implementation, testing, stable, deprecated
  updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_regions_path_gist ON regions USING GIST (path);
CREATE INDEX idx_regions_path_btree ON regions USING BTREE (path);


-- ────────────────────────────────────────────
-- Concept-Region Junction
-- ────────────────────────────────────────────

CREATE TABLE concept_region_assignments (
  concept_id UUID REFERENCES concepts(id) ON DELETE CASCADE,
  region_id  UUID REFERENCES regions(id) ON DELETE CASCADE,
  role       VARCHAR(50) NOT NULL DEFAULT 'implementation',
  -- implementation | integration | test | consumer
  PRIMARY KEY (concept_id, region_id)
);

CREATE INDEX idx_cra_concept ON concept_region_assignments(concept_id);
CREATE INDEX idx_cra_region ON concept_region_assignments(region_id);


-- ────────────────────────────────────────────
-- Synchronizations
-- ────────────────────────────────────────────

CREATE TABLE synchronizations (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name         VARCHAR(255) UNIQUE NOT NULL,
  when_clause  JSONB NOT NULL,
  where_clause JSONB,                  -- null if no state queries needed
  then_clause  JSONB NOT NULL,
  description  TEXT,
  enabled      BOOLEAN DEFAULT true,
  created_at   TIMESTAMPTZ DEFAULT NOW(),
  updated_at   TIMESTAMPTZ DEFAULT NOW()
);

-- when_clause example:
-- [
--   {"concept": "Web", "action": "request",
--    "input_match": {"method": "search", "terms": "?terms"},
--    "output_match": {"request": "?request"}},
--   {"concept": "SearchSource", "action": "query",
--    "input_match": {},
--    "output_match": {"results": "?results"}}
-- ]

-- where_clause example:
-- [
--   {"concept": "SearchSource", "pattern": {"?s": {"enabled": true}}}
-- ]

-- then_clause example:
-- [
--   {"concept": "SearchSource", "action": "query",
--    "args": {"source": "?s", "terms": "?terms"}}
-- ]


-- ────────────────────────────────────────────
-- Sync References (for impact analysis)
-- ────────────────────────────────────────────

CREATE TABLE sync_refs (
  sync_id       UUID REFERENCES synchronizations(id) ON DELETE CASCADE,
  concept_name  VARCHAR(255) NOT NULL,
  action_name   VARCHAR(255),          -- null if referencing state only
  state_field   VARCHAR(255),          -- null if referencing action only
  clause_type   VARCHAR(10) NOT NULL,  -- when | where | then
  PRIMARY KEY (sync_id, concept_name, COALESCE(action_name,''), COALESCE(state_field,''), clause_type)
);

CREATE INDEX idx_sync_refs_concept ON sync_refs(concept_name);
CREATE INDEX idx_sync_refs_action ON sync_refs(concept_name, action_name);
CREATE INDEX idx_sync_refs_field ON sync_refs(concept_name, state_field);


-- ────────────────────────────────────────────
-- Turns
-- ────────────────────────────────────────────

CREATE TYPE turn_status AS ENUM ('ACTIVE', 'COMPLETED', 'ABANDONED');

CREATE TABLE turns (
  id          VARCHAR(64) PRIMARY KEY,  -- T_{date}_{time}_{hex}
  agent_id    VARCHAR(255),
  agent_role  VARCHAR(50),              -- memorizer | researcher
  scope_path  ltree,                    -- declared scope for this turn
  plan_id     UUID REFERENCES execution_plans(id), -- execution plan this turn belongs to (nullable)
  task_type   VARCHAR(50) DEFAULT 'implement', -- implement | review_response | gardener | fix
  scratchpad  TEXT,
  status      turn_status DEFAULT 'ACTIVE',
  tree_before JSONB,                    -- tree view snapshot at turn start
  tree_after  JSONB,                    -- tree view snapshot at turn end
  created_at  TIMESTAMPTZ DEFAULT NOW(),
  completed_at TIMESTAMPTZ
);

CREATE INDEX idx_turns_status ON turns(status);
CREATE INDEX idx_turns_scope ON turns USING GIST(scope_path);


-- ────────────────────────────────────────────
-- Turn-Region Log
-- ────────────────────────────────────────────

CREATE TABLE turn_regions (
  turn_id    VARCHAR(64) REFERENCES turns(id),
  region_id  UUID REFERENCES regions(id),
  action     VARCHAR(50) NOT NULL,     -- created | modified | deleted
  PRIMARY KEY (turn_id, region_id)
);


-- ────────────────────────────────────────────
-- Proposals
-- ────────────────────────────────────────────

CREATE TYPE proposal_status AS ENUM
  ('PENDING', 'VALIDATING', 'APPROVED', 'REJECTED');

CREATE TABLE proposals (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  turn_id              VARCHAR(64) REFERENCES turns(id),
  region_id            UUID REFERENCES regions(id) NOT NULL,
  action_taken         VARCHAR(100) NOT NULL,
  current_state        VARCHAR(100),
  proposed_state       VARCHAR(100),
  sync_changes         JSONB,           -- added/modified/deleted sync rules
  evidence             JSONB NOT NULL,
  deferred_actions     JSONB DEFAULT '[]',
  status               proposal_status DEFAULT 'PENDING',
  review_iterations    INT DEFAULT 0,   -- how many Tier 3 review loop rounds
  review_history       JSONB DEFAULT '[]', -- [{iteration, concern, remediation, severity}]
  validation_error_code INTEGER,
  violation_details    JSONB,
  rejection_reason     TEXT,
  branch_name          VARCHAR(255),
  commit_sha           CHAR(40),
  created_at           TIMESTAMPTZ DEFAULT NOW()
);

ALTER TABLE proposals ADD CONSTRAINT check_rejection_data
  CHECK (status != 'REJECTED' OR validation_error_code IS NOT NULL);

CREATE INDEX idx_proposals_turn ON proposals(turn_id);
CREATE INDEX idx_proposals_region ON proposals(region_id);
CREATE INDEX idx_proposals_status ON proposals(status);


-- ────────────────────────────────────────────
-- Flow Log (runtime provenance)
-- ────────────────────────────────────────────

CREATE TABLE flow_log (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  flow_token     UUID NOT NULL,
  concept_name   VARCHAR(255) NOT NULL,
  action_name    VARCHAR(255) NOT NULL,
  input_args     JSONB,
  output_args    JSONB,
  sync_name      VARCHAR(255),          -- which sync triggered this (null for root actions)
  parent_id      UUID REFERENCES flow_log(id),  -- causal predecessor
  created_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_flow_token ON flow_log(flow_token);
CREATE INDEX idx_flow_sync ON flow_log(sync_name);
CREATE INDEX idx_flow_concept_action ON flow_log(concept_name, action_name);


-- ────────────────────────────────────────────
-- Lifecycle Hooks
-- ────────────────────────────────────────────

CREATE TABLE lifecycle_hooks (
  id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event     VARCHAR(50) NOT NULL,       -- on_turn_start, on_region_enter, on_turn_end
  hook_name VARCHAR(255) NOT NULL,
  priority  INT DEFAULT 100,            -- lower runs first
  handler   VARCHAR(255) NOT NULL,      -- Go function name or plugin path
  config    JSONB,
  enabled   BOOLEAN DEFAULT true,
  scope     ltree,                      -- optional: only fire for regions under this path
  UNIQUE(event, hook_name)
);

CREATE INDEX idx_hooks_event ON lifecycle_hooks(event) WHERE enabled = true;


-- ────────────────────────────────────────────
-- Execution Plans
-- ────────────────────────────────────────────

CREATE TYPE plan_status AS ENUM ('ACTIVE', 'COMPLETED', 'ABANDONED');

CREATE TABLE execution_plans (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        VARCHAR(255) NOT NULL,
  goal        TEXT NOT NULL,
  status      plan_status DEFAULT 'ACTIVE',
  decisions   JSONB DEFAULT '[]',       -- [{decision, rationale, alternatives, turn_id}]
  quality_grade VARCHAR(10),            -- A, B, C, D, F
  created_at  TIMESTAMPTZ DEFAULT NOW(),
  completed_at TIMESTAMPTZ
);

CREATE TABLE plan_turns (
  plan_id     UUID REFERENCES execution_plans(id) ON DELETE CASCADE,
  turn_id     VARCHAR(64) REFERENCES turns(id),
  region_path ltree NOT NULL,
  ordering    INT NOT NULL,             -- execution order within plan
  depends_on  VARCHAR(64)[],            -- turn_ids that must complete first
  status      VARCHAR(50) DEFAULT 'pending', -- pending, active, completed, blocked
  PRIMARY KEY (plan_id, turn_id)
);

CREATE INDEX idx_plan_turns_plan ON plan_turns(plan_id);
CREATE INDEX idx_plan_turns_status ON plan_turns(status);


-- ────────────────────────────────────────────
-- Quality Tracking
-- ────────────────────────────────────────────

CREATE TABLE quality_grades (
  region_id     UUID REFERENCES regions(id) ON DELETE CASCADE,
  category      VARCHAR(50) NOT NULL,   -- test_coverage, doc_completeness, invariant_coverage
  grade         VARCHAR(10) NOT NULL,   -- A, B, C, D, F
  details       JSONB,
  assessed_at   TIMESTAMPTZ DEFAULT NOW(),
  assessed_by   VARCHAR(64),            -- turn_id of the gardener run
  PRIMARY KEY (region_id, category)
);

CREATE TABLE golden_principles (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        VARCHAR(255) UNIQUE NOT NULL,
  rule        TEXT NOT NULL,            -- human-readable rule
  lint_check  VARCHAR(255),             -- Go function or linter ID that enforces this
  remediation TEXT NOT NULL,            -- agent-actionable fix instruction
  enabled     BOOLEAN DEFAULT true,
  created_at  TIMESTAMPTZ DEFAULT NOW()
);
```

## 17. Go Core Types

```go
package gam

import "time"

// ────────────────────────────────────────────
// Concept
// ────────────────────────────────────────────

type Concept struct {
    ID           string       `json:"id" db:"id"`
    Name         string       `json:"name" db:"name"`
    Purpose      string       `json:"purpose" db:"purpose"`
    Spec         ConceptSpec  `json:"spec"`
    StateMachine StateMachine `json:"state_machine"`
    Invariants   []Invariant  `json:"invariants"`
    CreatedAt    time.Time    `json:"created_at" db:"created_at"`
    UpdatedAt    time.Time    `json:"updated_at" db:"updated_at"`
}

type ConceptSpec struct {
    TypeParams          []string                    `json:"type_params"`
    State               map[string]StateComponent   `json:"state"`
    Actions             map[string]ActionSpec        `json:"actions"`
    OperationalPrinciple string                     `json:"operational_principle"`
}

type StateComponent struct {
    Type string `json:"type"` // "set", "map"
    From string `json:"from,omitempty"`
    To   string `json:"to,omitempty"`
    Of   string `json:"of,omitempty"`
}

type ActionSpec struct {
    Cases []ActionCase `json:"cases"`
}

type ActionCase struct {
    Input       map[string]string `json:"input"`       // arg_name -> type
    Output      map[string]string `json:"output"`      // arg_name -> type
    Description string            `json:"description"`
}

type StateMachine struct {
    States      []string     `json:"states"`
    Transitions []Transition `json:"transitions"`
}

type Transition struct {
    From   string `json:"from"`
    To     string `json:"to"`
    Action string `json:"action"`
}

type Invariant struct {
    Name   string         `json:"name"`
    Rule   string         `json:"rule,omitempty"`
    Config map[string]any `json:"config,omitempty"`
    Type   string         `json:"type"` // representation, abstract, api, migration, dependency
}

// ────────────────────────────────────────────
// Synchronization
// ────────────────────────────────────────────

type Synchronization struct {
    ID          string        `json:"id" db:"id"`
    Name        string        `json:"name" db:"name"`
    WhenClause  []WhenPattern `json:"when_clause"`
    WhereClause []WherePattern `json:"where_clause,omitempty"`
    ThenClause  []ThenAction  `json:"then_clause"`
    Description string        `json:"description,omitempty"`
    Enabled     bool          `json:"enabled" db:"enabled"`
}

type WhenPattern struct {
    Concept     string            `json:"concept"`
    Action      string            `json:"action"`
    InputMatch  map[string]string `json:"input_match"`  // arg -> literal or ?variable
    OutputMatch map[string]string `json:"output_match"`
}

type WherePattern struct {
    Concept string                 `json:"concept"`
    Pattern map[string]any         `json:"pattern"` // ?var -> {field: value_or_?var}
    Optional bool                  `json:"optional,omitempty"`
    Bind    map[string]string      `json:"bind,omitempty"` // ?var -> expression
    Filter  string                 `json:"filter,omitempty"`
}

type ThenAction struct {
    Concept string            `json:"concept"`
    Action  string            `json:"action"`
    Args    map[string]string `json:"args"` // arg -> literal or ?variable
}

// ────────────────────────────────────────────
// Proposal
// ────────────────────────────────────────────

type Proposal struct {
    ID               string           `json:"id" db:"id"`
    TurnID           string           `json:"turn_id" db:"turn_id"`
    RegionID         string           `json:"region_id" db:"region_id"`
    RegionPath       string           `json:"region_path"`
    ActionTaken      string           `json:"action_taken" db:"action_taken"`
    CurrentState     string           `json:"current_state"`
    ProposedState    string           `json:"proposed_state" db:"proposed_state"`
    SyncChanges      *SyncChanges     `json:"sync_changes,omitempty"`
    Evidence         ProposalEvidence `json:"evidence"`
    DeferredActions  []DeferredAction `json:"deferred_actions"`
    Status           string           `json:"status" db:"status"`
    ErrorCode        *int             `json:"validation_error_code"`
    ViolationDetails any              `json:"violation_details"`
    RejectionReason  string           `json:"rejection_reason,omitempty"`
    BranchName       string           `json:"branch_name" db:"branch_name"`
    CommitSHA        string           `json:"commit_sha" db:"commit_sha"`
    CreatedAt        time.Time        `json:"created_at" db:"created_at"`
}

type SyncChanges struct {
    Added    []Synchronization `json:"added,omitempty"`
    Modified []Synchronization `json:"modified,omitempty"`
    Deleted  []string          `json:"deleted,omitempty"` // sync names
}

type ProposalEvidence struct {
    APIAnalysis       *APIAnalysis       `json:"api_analysis,omitempty"`
    MigrationAnalysis *MigrationAnalysis `json:"migration_analysis,omitempty"`
    DependencyAnalysis *DependencyAnalysis `json:"dependency_analysis,omitempty"`
    ModifiedRegions   []ModifiedRegion   `json:"modified_regions"`
    Summary           string             `json:"summary"`
}

type APIAnalysis struct {
    ExportsBefore []string `json:"exports_before"`
    ExportsAfter  []string `json:"exports_after"`
    Removals      []string `json:"removals"`
    Additions     []string `json:"additions"`
}

type MigrationAnalysis struct {
    Operations  []string `json:"operations"` // ADD_COLUMN, ALTER_TYPE, etc.
    Reversible  bool     `json:"reversible"`
    DataLoss    bool     `json:"data_loss"`
}

type DependencyAnalysis struct {
    Added   []string `json:"added"`
    Removed []string `json:"removed"`
    Changed []string `json:"changed"`
}

type ModifiedRegion struct {
    Path        string `json:"path"`
    File        string `json:"file"`
    Description string `json:"description"`
    Hash        string `json:"hash"`
}

type DeferredAction struct {
    TaskType     string `json:"task_type"`
    Reason       string `json:"reason"`
    TargetRegion string `json:"target_region"`
}

// ────────────────────────────────────────────
// Turn
// ────────────────────────────────────────────

type Turn struct {
    ID          string    `json:"id" db:"id"`
    AgentID     string    `json:"agent_id" db:"agent_id"`
    AgentRole   string    `json:"agent_role" db:"agent_role"`
    ScopePath   string    `json:"scope_path" db:"scope_path"`
    PlanID      string    `json:"plan_id,omitempty" db:"plan_id"`  // execution plan this turn belongs to
    TaskType    string    `json:"task_type" db:"task_type"`        // implement, review, gardener, fix
    Scratchpad  string    `json:"scratchpad" db:"scratchpad"`
    Status      string    `json:"status" db:"status"`
    TreeBefore  any       `json:"tree_before" db:"tree_before"`
    TreeAfter   any       `json:"tree_after" db:"tree_after"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    CompletedAt *time.Time `json:"completed_at" db:"completed_at"`
}

// ────────────────────────────────────────────
// Flow Log Entry
// ────────────────────────────────────────────

type FlowEntry struct {
    ID          string    `json:"id" db:"id"`
    FlowToken   string    `json:"flow_token" db:"flow_token"`
    ConceptName string    `json:"concept_name" db:"concept_name"`
    ActionName  string    `json:"action_name" db:"action_name"`
    InputArgs   any       `json:"input_args" db:"input_args"`
    OutputArgs  any       `json:"output_args" db:"output_args"`
    SyncName    string    `json:"sync_name,omitempty" db:"sync_name"`
    ParentID    string    `json:"parent_id,omitempty" db:"parent_id"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// ────────────────────────────────────────────
// Lifecycle Hook
// ────────────────────────────────────────────

type LifecycleHook struct {
    ID       string `json:"id" db:"id"`
    Event    string `json:"event" db:"event"`
    HookName string `json:"hook_name" db:"hook_name"`
    Priority int    `json:"priority" db:"priority"`
    Handler  string `json:"handler" db:"handler"`
    Config   any    `json:"config" db:"config"`
    Enabled  bool   `json:"enabled" db:"enabled"`
    Scope    string `json:"scope,omitempty" db:"scope"`
}

// ────────────────────────────────────────────
// Validation Result
// ────────────────────────────────────────────

type ValidationResult struct {
    Tier     int               `json:"tier"`
    Passed   bool              `json:"passed"`
    Code     int               `json:"code"`
    Message  string            `json:"message"`
    Details  []ValidationDetail `json:"details,omitempty"`
}

type ValidationDetail struct {
    Check    string `json:"check"`
    Passed   bool   `json:"passed"`
    Expected string `json:"expected,omitempty"`
    Got      string `json:"got,omitempty"`
    Fix      string `json:"fix,omitempty"` // MANDATORY for non-passing checks. Agent-actionable.
}

// ────────────────────────────────────────────
// Execution Plan
// ────────────────────────────────────────────

type ExecutionPlan struct {
    ID           string        `json:"id" db:"id"`
    Name         string        `json:"name" db:"name"`
    Goal         string        `json:"goal" db:"goal"`
    Status       string        `json:"status" db:"status"` // ACTIVE, COMPLETED, ABANDONED
    Decisions    []Decision    `json:"decisions"`
    QualityGrade string        `json:"quality_grade" db:"quality_grade"`
    CreatedAt    time.Time     `json:"created_at" db:"created_at"`
    CompletedAt  *time.Time    `json:"completed_at" db:"completed_at"`
}

type Decision struct {
    Description  string   `json:"description"`
    Rationale    string   `json:"rationale"`
    Alternatives []string `json:"alternatives"`
    TurnID       string   `json:"turn_id"`
    DecidedAt    string   `json:"decided_at"`
}

type PlanTurn struct {
    PlanID     string   `json:"plan_id" db:"plan_id"`
    TurnID     string   `json:"turn_id" db:"turn_id"`
    RegionPath string   `json:"region_path" db:"region_path"`
    Ordering   int      `json:"ordering" db:"ordering"`
    DependsOn  []string `json:"depends_on" db:"depends_on"`
    Status     string   `json:"status" db:"status"`
}

// ────────────────────────────────────────────
// Quality Tracking
// ────────────────────────────────────────────

type QualityGrade struct {
    RegionID   string    `json:"region_id" db:"region_id"`
    Category   string    `json:"category" db:"category"`
    Grade      string    `json:"grade" db:"grade"`
    Details    any       `json:"details" db:"details"`
    AssessedAt time.Time `json:"assessed_at" db:"assessed_at"`
    AssessedBy string    `json:"assessed_by" db:"assessed_by"` // gardener turn_id
}

type GoldenPrinciple struct {
    ID          string `json:"id" db:"id"`
    Name        string `json:"name" db:"name"`
    Rule        string `json:"rule" db:"rule"`
    LintCheck   string `json:"lint_check" db:"lint_check"`
    Remediation string `json:"remediation" db:"remediation"` // agent-actionable
    Enabled     bool   `json:"enabled" db:"enabled"`
}

// ────────────────────────────────────────────
// Review Loop (Tier 3)
// ────────────────────────────────────────────

type ReviewComment struct {
    ProposalID  string `json:"proposal_id"`
    Tier        int    `json:"tier"`
    Iteration   int    `json:"iteration"`     // which round of the review loop
    Concern     string `json:"concern"`        // what the auditor flagged
    Remediation string `json:"remediation"`    // agent-actionable fix instruction
    Severity    string `json:"severity"`       // request_changes | reject | escalate_human
}
```

## 18. Validator Implementation

```go
package validator

import (
    "context"
    "fmt"
    "github.com/jackc/pgx/v5/pgxpool"
    gam "your/module/path"
)

type Validator struct {
    db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Validator {
    return &Validator{db: db}
}

// Validate runs tiers 0 and 1. Tier 2 (Dagger) and Tier 3 (LLM) are separate.
func (v *Validator) Validate(ctx context.Context, p *gam.Proposal) (*gam.ValidationResult, error) {
    // Tier 0: Structural
    if result := v.tier0Structural(ctx, p); !result.Passed {
        return result, nil
    }

    // Tier 1: State machine + sync integrity
    return v.tier1StateMachine(ctx, p)
}

// ── Tier 0 ──────────────────────────────────

func (v *Validator) tier0Structural(ctx context.Context, p *gam.Proposal) *gam.ValidationResult {
    result := &gam.ValidationResult{Tier: 0, Passed: true, Code: 0}

    // Check region exists in DB (mirrors arch.md)
    var exists bool
    v.db.QueryRow(ctx,
        "SELECT EXISTS(SELECT 1 FROM regions WHERE path = $1)",
        p.RegionPath,
    ).Scan(&exists)

    if !exists {
        result.Passed = false
        result.Code = 1
        result.Message = fmt.Sprintf("Region %s not found in arch.md", p.RegionPath)
        return result
    }

    // Check scope: is proposal region under the turn's declared scope?
    var inScope bool
    v.db.QueryRow(ctx, `
        SELECT $1::ltree <@ (SELECT scope_path FROM turns WHERE id = $2)
    `, p.RegionPath, p.TurnID).Scan(&inScope)

    if !inScope {
        result.Passed = false
        result.Code = 2
        result.Message = fmt.Sprintf("Region %s is outside turn scope", p.RegionPath)
        return result
    }

    // Check modified regions have region markers (delegated to tree scanner)
    for _, mr := range p.Evidence.ModifiedRegions {
        if !v.fileHasRegionMarkers(mr.File, mr.Path) {
            result.Passed = false
            result.Code = 3
            result.Message = fmt.Sprintf("File %s missing region markers for %s", mr.File, mr.Path)
            result.Details = append(result.Details, gam.ValidationDetail{
                Check: "region_markers",
                Passed: false,
                Got: mr.File,
                Fix: fmt.Sprintf("Run: gam region touch %s --file %s", mr.Path, mr.File),
            })
            return result
        }
    }

    result.Message = "Tier 0 passed"
    return result
}

// ── Tier 1 ──────────────────────────────────

func (v *Validator) tier1StateMachine(ctx context.Context, p *gam.Proposal) (*gam.ValidationResult, error) {
    result := &gam.ValidationResult{Tier: 1, Passed: true, Code: 0}

    // Collect concepts via LTREE ancestor walk through junction table
    concepts, err := v.getConceptsForRegion(ctx, p.RegionPath)
    if err != nil {
        return nil, fmt.Errorf("concept lookup: %w", err)
    }

    // Check state transition legality
    for _, concept := range concepts {
        if p.CurrentState != "" && p.ProposedState != "" {
            if !v.isLegalTransition(concept.StateMachine, p.CurrentState, p.ProposedState, p.ActionTaken) {
                result.Passed = false
                result.Code = -2
                result.Message = fmt.Sprintf(
                    "Illegal transition: %s -> %s via %s in concept %s",
                    p.CurrentState, p.ProposedState, p.ActionTaken, concept.Name,
                )
                return result, nil
            }
        }

        // Check invariant rules against evidence
        for _, inv := range concept.Invariants {
            detail := v.checkInvariant(inv, p.Evidence)
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
        // Deleted syncs: verify no other syncs depend on them (rare, but check)
        for _, deleted := range p.SyncChanges.Deleted {
            _ = deleted // Log deletion for audit trail
        }

        // New/modified syncs: verify all referenced actions and state fields exist
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
                    "Removing action %s would break %d sync(s): %v",
                    removed, len(refs), refs,
                )
                return result, nil
            }
        }
    }

    result.Message = "Tier 1 passed"
    return result, nil
}

// ── Helpers ─────────────────────────────────

func (v *Validator) getConceptsForRegion(ctx context.Context, path string) ([]gam.Concept, error) {
    rows, err := v.db.Query(ctx, `
        SELECT DISTINCT c.id, c.name, c.purpose, c.spec, c.state_machine, c.invariants
        FROM regions r
        JOIN concept_region_assignments cra ON cra.region_id = r.id
        JOIN concepts c ON c.id = cra.concept_id
        WHERE r.path @> $1 OR r.path = $1
        ORDER BY nlevel(r.path) ASC
    `, path)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var concepts []gam.Concept
    for rows.Next() {
        var c gam.Concept
        if err := rows.Scan(&c.ID, &c.Name, &c.Purpose, &c.Spec, &c.StateMachine, &c.Invariants); err != nil {
            return nil, err
        }
        concepts = append(concepts, c)
    }
    return concepts, nil
}

func (v *Validator) isLegalTransition(sm gam.StateMachine, from, to, action string) bool {
    for _, t := range sm.Transitions {
        if t.From == from && t.To == to && t.Action == action {
            return true
        }
    }
    return false
}

func (v *Validator) checkInvariant(inv gam.Invariant, evidence gam.ProposalEvidence) gam.ValidationDetail {
    detail := gam.ValidationDetail{Check: inv.Name, Passed: true}

    switch inv.Type {
    case "api":
        if evidence.APIAnalysis == nil {
            detail.Passed = false
            detail.Expected = "APIAnalysis block required"
            detail.Got = "missing"
            detail.Fix = "Add api_analysis to proposal evidence"
            return detail
        }
        cfg := inv.Config
        if noRemovals, ok := cfg["no_removals"].(bool); ok && noRemovals {
            if len(evidence.APIAnalysis.Removals) > 0 {
                detail.Passed = false
                detail.Expected = "no API removals"
                detail.Got = fmt.Sprintf("removed: %v", evidence.APIAnalysis.Removals)
                detail.Fix = "Restore removed exports or update invariant"
            }
        }

    case "migration":
        if evidence.MigrationAnalysis == nil {
            detail.Passed = false
            detail.Expected = "MigrationAnalysis block required"
            detail.Got = "missing"
            detail.Fix = "Add migration_analysis to proposal evidence"
            return detail
        }
        if forbidden, ok := inv.Config["forbidden"].([]any); ok {
            for _, op := range evidence.MigrationAnalysis.Operations {
                for _, f := range forbidden {
                    if op == f.(string) {
                        detail.Passed = false
                        detail.Expected = fmt.Sprintf("operation %s forbidden", f)
                        detail.Got = op
                        detail.Fix = "Use a non-destructive migration strategy"
                    }
                }
            }
        }

    case "dependency":
        if evidence.DependencyAnalysis == nil {
            // Not required unless invariant demands it
            return detail
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
            detail.Fix = fmt.Sprintf("Define action %s in concept %s or fix sync reference", w.Action, w.Concept)
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
            detail.Fix = fmt.Sprintf("Define action %s in concept %s or fix sync reference", t.Action, t.Concept)
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
                        return detail
                    }
                }
            }
        }
    }

    return detail
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

func (v *Validator) fileHasRegionMarkers(file, regionPath string) bool {
    // Delegates to tree scanner — reads file, checks for @region:path markers
    // Implementation depends on file I/O and comment style detection
    return true // placeholder
}
```

## 19. Memorizer Service

```go
package memorizer

import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/redis/go-redis/v9"
    gam "your/module/path"
    "your/module/path/validator"
)

type Memorizer struct {
    db        *pgxpool.Pool
    redis     *redis.Client
    validator *validator.Validator
    hooks     *HookRunner
}

func New(db *pgxpool.Pool, rdb *redis.Client) *Memorizer {
    return &Memorizer{
        db:        db,
        redis:     rdb,
        validator: validator.New(db),
        hooks:     NewHookRunner(db),
    }
}

// ConsumeProposals blocks on Redis, processing proposals as they arrive.
func (m *Memorizer) ConsumeProposals(ctx context.Context) error {
    // Ensure consumer group exists
    m.redis.XGroupCreateMkStream(ctx, "agent_proposals", "memorizer_pool", "0").Err()

    for {
        streams, err := m.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    "memorizer_pool",
            Consumer: "memorizer_1",
            Streams:  []string{"agent_proposals", ">"},
            Count:    1,
            Block:    0,
        }).Result()
        if err != nil {
            return fmt.Errorf("stream read: %w", err)
        }

        for _, stream := range streams {
            for _, msg := range stream.Messages {
                proposalID := msg.Values["proposal_id"].(string)
                regionPath := msg.Values["region_path"].(string)

                if err := m.processProposal(ctx, proposalID, regionPath); err != nil {
                    log.Printf("proposal %s failed: %v", proposalID, err)
                }

                m.redis.XAck(ctx, "agent_proposals", "memorizer_pool", msg.ID)
            }
        }
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

    // Tier 2: Dagger integration tests (if configured for this region)
    if m.requiresTier2(ctx, path) {
        if err := m.runDaggerTests(ctx, path, proposal); err != nil {
            return m.rejectProposal(ctx, id, &gam.ValidationResult{
                Tier:    2,
                Passed:  false,
                Code:    -99,
                Message: "Integration test failure: " + err.Error(),
            })
        }
    }

    // Tier 3: LLM review loop (if high-risk change)
    if m.requiresTier3(ctx, proposal) {
        comment := m.reviewLoop(ctx, id, proposal)
        if comment != nil && comment.Severity == "reject" {
            return m.rejectProposal(ctx, id, &gam.ValidationResult{
                Tier:    3,
                Passed:  false,
                Code:    -100,
                Message: comment.Concern,
            })
        }
        if comment != nil && comment.Severity == "escalate_human" {
            return m.escalateToHuman(ctx, id, comment)
        }
        // If review loop resolved (comment is nil), fall through to approve
    }

    // Tier 4: Runtime validation (if configured for this region)
    if m.requiresTier4(ctx, path) {
        if err := m.runtimeValidate(ctx, path, proposal); err != nil {
            return m.requestChanges(ctx, id, &gam.ReviewComment{
                ProposalID:  id,
                Tier:        4,
                Concern:     "Runtime validation failure: " + err.Error(),
                Remediation: "Boot the app and verify the operational principle passes end-to-end.",
                Severity:    "request_changes",
            })
        }
    }

    return m.approveProposal(ctx, id, proposal)
}

// reviewLoop runs LLM audit and allows iterative feedback.
// Returns nil if the auditor is satisfied, or a final ReviewComment if not.
func (m *Memorizer) reviewLoop(ctx context.Context, proposalID string, p *gam.Proposal) *gam.ReviewComment {
    maxIterations := 3

    for i := 0; i < maxIterations; i++ {
        concern := m.llmAudit(ctx, p)
        if concern == "" {
            return nil // auditor satisfied
        }

        comment := &gam.ReviewComment{
            ProposalID:  proposalID,
            Tier:        3,
            Iteration:   i + 1,
            Concern:     concern,
            Remediation: m.generateRemediation(ctx, concern, p),
            Severity:    "request_changes",
        }

        if i == maxIterations-1 {
            comment.Severity = "escalate_human"
            return comment
        }

        // Push review comment back to Researcher via Redis
        m.requestChanges(ctx, proposalID, comment)

        // Wait for revised proposal (blocking read from proposal stream)
        revised, err := m.waitForRevision(ctx, proposalID)
        if err != nil {
            comment.Severity = "escalate_human"
            return comment
        }
        p = revised
    }
    return nil
}

// requestChanges pushes a review comment to the Researcher queue for revision.
func (m *Memorizer) requestChanges(ctx context.Context, proposalID string, comment *gam.ReviewComment) error {
    commentJSON, _ := json.Marshal(comment)
    m.redis.XAdd(ctx, &redis.XAddArgs{
        Stream: "agent_tasks",
        Values: map[string]any{
            "turn_id":     comment.ProposalID,
            "task_type":   "review_response",
            "review":      string(commentJSON),
        },
    })
    return nil
}

// escalateToHuman marks the proposal as needing human attention.
func (m *Memorizer) escalateToHuman(ctx context.Context, proposalID string, comment *gam.ReviewComment) error {
    briefing := fmt.Sprintf("ESCALATED TO HUMAN (Tier %d, %d iterations)\n%s\nRemediation: %s",
        comment.Tier, comment.Iteration, comment.Concern, comment.Remediation)
    _, err := m.db.Exec(ctx, `
        UPDATE proposals
        SET status = 'PENDING', rejection_reason = $1
        WHERE id = $2
    `, briefing, proposalID)
    return err
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
            m.insertSync(ctx, tx, sync)
        }
        for _, sync := range p.SyncChanges.Modified {
            m.updateSync(ctx, tx, sync)
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

    // Update execution plan progress if this turn belongs to one
    var planID string
    m.db.QueryRow(ctx, `
        SELECT pt.plan_id FROM plan_turns pt WHERE pt.turn_id = $1
    `, p.TurnID).Scan(&planID)
    if planID != "" {
        m.UpdatePlanProgress(ctx, planID, p.TurnID)
    }

    // Re-export docs/ for any modified concepts or syncs
    m.exportDocs(ctx)

    return nil
}

// CreateTurn is called by the Memorizer when processing a human prompt.
func (m *Memorizer) CreateTurn(ctx context.Context, regionPath, prompt string) (string, error) {
    turnID := generateTurnID()

    // Insert turn
    _, err := m.db.Exec(ctx, `
        INSERT INTO turns (id, agent_role, scope_path, status)
        VALUES ($1, 'researcher', $2, 'ACTIVE')
    `, turnID, regionPath)
    if err != nil {
        return "", err
    }

    // Compile context for the researcher
    contextRef, err := m.compileContext(ctx, regionPath)
    if err != nil {
        return "", err
    }

    // Push task to researcher queue
    m.redis.XAdd(ctx, &redis.XAddArgs{
        Stream: "agent_tasks",
        Values: map[string]any{
            "turn_id":      turnID,
            "region_path":  regionPath,
            "context_ref":  contextRef,
            "task_type":    "implement",
            "prompt":       prompt,
        },
    })

    return turnID, nil
}

// compileContext extracts concept specs, syncs, plan context, quality grades,
// and turn memory for a region. This is the progressive disclosure mechanism:
// the agent gets exactly what it needs for the current task, nothing more.
func (m *Memorizer) compileContext(ctx context.Context, regionPath string) (string, error) {
    // Get concept specs via junction table + LTREE ancestors
    concepts, _ := m.validator.GetConceptsForRegion(ctx, regionPath)

    // Get syncs that reference these concepts
    var syncNames []string
    for _, c := range concepts {
        rows, _ := m.db.Query(ctx, `
            SELECT DISTINCT s.name
            FROM sync_refs sr
            JOIN synchronizations s ON s.id = sr.sync_id
            WHERE sr.concept_name = $1 AND s.enabled = true
        `, c.Name)
        for rows.Next() {
            var name string
            rows.Scan(&name)
            syncNames = append(syncNames, name)
        }
        rows.Close()
    }

    // Get execution plan context if this turn belongs to one
    var planContext string
    rows2, _ := m.db.Query(ctx, `
        SELECT ep.name, ep.goal, ep.decisions, ep.quality_grade,
               pt.turn_id, pt.region_path, pt.ordering, pt.status
        FROM plan_turns pt
        JOIN execution_plans ep ON ep.id = pt.plan_id
        WHERE pt.plan_id IN (
            SELECT plan_id FROM plan_turns WHERE region_path <@ $1 AND status IN ('pending', 'active')
        )
        ORDER BY pt.ordering
    `, regionPath)
    if rows2 != nil {
        defer rows2.Close()
        // Format plan context with progress markers
        // [x] completed, [ ] pending, [>] this turn
    }

    // Get quality grades for this region
    var qualityGrades []gam.QualityGrade
    rows3, _ := m.db.Query(ctx, `
        SELECT qg.category, qg.grade, qg.details
        FROM quality_grades qg
        JOIN regions r ON r.id = qg.region_id
        WHERE r.path = $1
    `, regionPath)
    if rows3 != nil {
        defer rows3.Close()
        for rows3.Next() {
            var qg gam.QualityGrade
            rows3.Scan(&qg.Category, &qg.Grade, &qg.Details)
            qualityGrades = append(qualityGrades, qg)
        }
    }

    // Get recent scratchpads for this region
    rows4, _ := m.db.Query(ctx, `
        SELECT t.scratchpad, t.id, t.completed_at
        FROM turns t
        JOIN turn_regions tr ON tr.turn_id = t.id
        JOIN regions r ON r.id = tr.region_id
        WHERE r.path <@ $1 AND t.scratchpad IS NOT NULL
        ORDER BY t.completed_at DESC NULLS LAST
        LIMIT 5
    `, regionPath)
    defer rows4.Close()

    var scratchpads []string
    for rows4.Next() {
        var sp, tid string
        var completedAt interface{}
        rows4.Scan(&sp, &tid, &completedAt)
        scratchpads = append(scratchpads, fmt.Sprintf("[%s] %s", tid, sp))
    }

    // Get applicable golden principles (for agent awareness)
    var principles []gam.GoldenPrinciple
    rows5, _ := m.db.Query(ctx, `
        SELECT name, rule, remediation FROM golden_principles WHERE enabled = true
    `)
    if rows5 != nil {
        defer rows5.Close()
        for rows5.Next() {
            var gp gam.GoldenPrinciple
            rows5.Scan(&gp.Name, &gp.Rule, &gp.Remediation)
            principles = append(principles, gp)
        }
    }

    // Write compiled context to a temp file and return reference
    contextRef := fmt.Sprintf("/tmp/gam_context_%s.md", regionPath)
    // Format: plan context → scratchpads → quality grades → concept specs →
    //         syncs → tree view → golden principles
    // Deliberately excludes implementation code from other concepts.

    return contextRef, nil
}

func (m *Memorizer) queueTask(ctx context.Context, regionPath, taskType, reason string) {
    turnID := generateTurnID()
    m.db.Exec(ctx, `
        INSERT INTO turns (id, agent_role, scope_path, status)
        VALUES ($1, 'researcher', $2, 'ACTIVE')
    `, turnID, regionPath)

    m.redis.XAdd(ctx, &redis.XAddArgs{
        Stream: "agent_tasks",
        Values: map[string]any{
            "turn_id":     turnID,
            "region_path": regionPath,
            "task_type":   taskType,
            "prompt":      reason,
        },
    })
}

// ── Execution Plan Management ───────────────

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
        turnID := generateTurnID()
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

    // Queue the first turn(s) with no dependencies
    m.queueReadyPlanTurns(ctx, plan.ID)

    // Export plan to docs/
    m.exportPlanDoc(ctx, plan)

    return plan, nil
}

// RecordDecision appends a design decision to an active plan.
func (m *Memorizer) RecordDecision(ctx context.Context, planID string, decision gam.Decision) error {
    _, err := m.db.Exec(ctx, `
        UPDATE execution_plans
        SET decisions = decisions || $1::jsonb
        WHERE id = $2 AND status = 'ACTIVE'
    `, decision, planID)
    return err
}

// UpdatePlanProgress marks a turn as completed and queues newly unblocked turns.
func (m *Memorizer) UpdatePlanProgress(ctx context.Context, planID, turnID string) error {
    m.db.Exec(ctx, `
        UPDATE plan_turns SET status = 'completed' WHERE plan_id = $1 AND turn_id = $2
    `, planID, turnID)

    // Check if all turns are done
    var remaining int
    m.db.QueryRow(ctx, `
        SELECT COUNT(*) FROM plan_turns WHERE plan_id = $1 AND status != 'completed'
    `, planID).Scan(&remaining)

    if remaining == 0 {
        m.db.Exec(ctx, `
            UPDATE execution_plans SET status = 'COMPLETED', completed_at = NOW() WHERE id = $1
        `, planID)
        m.exportPlanDoc(ctx, nil) // re-export moves to completed/
    }

    // Queue turns whose dependencies are now satisfied
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
    defer rows.Close()

    for rows.Next() {
        var turnID, regionPath string
        rows.Scan(&turnID, &regionPath)
        m.db.Exec(ctx, `UPDATE plan_turns SET status = 'active' WHERE plan_id = $1 AND turn_id = $2`, planID, turnID)
        contextRef, _ := m.compileContext(ctx, regionPath)
        m.redis.XAdd(ctx, &redis.XAddArgs{
            Stream: "agent_tasks",
            Values: map[string]any{
                "turn_id":     turnID,
                "region_path": regionPath,
                "context_ref": contextRef,
                "task_type":   "implement",
            },
        })
    }
}

// ── Gardening Agent ─────────────────────────

// RunGardener performs a full entropy sweep and queues fix-up turns.
func (m *Memorizer) RunGardener(ctx context.Context) error {
    findings := make([]gardenFinding, 0)

    // Check for stale scratchpads (TODOs never picked up)
    findings = append(findings, m.findStaleTodos(ctx)...)

    // Check for orphaned regions (arch.md vs code mismatch)
    findings = append(findings, m.findOrphanedRegions(ctx)...)

    // Check for sync drift (syncs that should fire but haven't)
    findings = append(findings, m.findSyncDrift(ctx)...)

    // Check for concept spec divergence (operational principles failing)
    findings = append(findings, m.findSpecDivergence(ctx)...)

    // Check for documentation staleness
    findings = append(findings, m.findStaleDocumentation(ctx)...)

    // Assess quality grades per region
    m.updateQualityGrades(ctx)

    // Create fix-up turns for mechanical findings
    for _, f := range findings {
        if f.Mechanical {
            m.queueTask(ctx, f.RegionPath, "gardener", f.Description)
        } else {
            // Append to tech-debt tracker
            m.appendTechDebt(ctx, f)
        }
    }

    // Re-export docs/ with updated quality grades
    m.exportDocs(ctx)

    return nil
}

type gardenFinding struct {
    RegionPath  string
    Category    string // stale_todo, orphaned_region, sync_drift, spec_divergence, stale_docs, duplication
    Description string
    Mechanical  bool   // can be fixed by an agent without human judgment
}

func (m *Memorizer) findSyncDrift(ctx context.Context) []gardenFinding {
    var findings []gardenFinding
    // Find syncs that have matching action completions in flow_log
    // but no outgoing sync edges in the last N days
    rows, _ := m.db.Query(ctx, `
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
    if rows != nil {
        defer rows.Close()
        for rows.Next() {
            var syncName, conceptName, actionName string
            rows.Scan(&syncName, &conceptName, &actionName)
            findings = append(findings, gardenFinding{
                Category:    "sync_drift",
                Description: fmt.Sprintf("Sync %s: action %s/%s is completing but sync never fires. Likely state representation mismatch in where clause.", syncName, conceptName, actionName),
                Mechanical:  false,
            })
        }
    }
    return findings
}

// ── Docs Projection ─────────────────────────

// exportDocs regenerates the docs/ directory from PostgreSQL state.
func (m *Memorizer) exportDocs(ctx context.Context) error {
    // Export concept catalog
    m.exportConceptDocs(ctx)
    // Export sync catalog
    m.exportSyncDocs(ctx)
    // Export active and completed plans
    m.exportPlanDocs(ctx)
    // Export quality grades and tech debt
    m.exportQualityDocs(ctx)
    return nil
}

func (m *Memorizer) exportPlanDoc(ctx context.Context, plan *gam.ExecutionPlan) {
    // Write plan as markdown to docs/exec-plans/active/ or completed/
    // Implementation: query plan_turns, format as markdown with progress markers
}

func (m *Memorizer) exportConceptDocs(ctx context.Context) {
    // Query all concepts, write each to docs/concepts/<name>.md
    // Write docs/concepts/index.md with catalog
}

func (m *Memorizer) exportSyncDocs(ctx context.Context) {
    // Query all syncs, write each to docs/syncs/<name>.md
    // Write docs/syncs/index.md with catalog
}

func (m *Memorizer) exportPlanDocs(ctx context.Context) {
    // Query active and completed plans, write to docs/exec-plans/
}

func (m *Memorizer) exportQualityDocs(ctx context.Context) {
    // Query quality_grades, format as docs/quality/grades.md
    // Query golden_principles, format as docs/quality/golden-principles.md
}

func hashTo64Bit(s string) int64 {
    // FNV-1a hash to int64 for advisory lock
    var h uint64 = 14695981039346656037
    for _, c := range []byte(s) {
        h ^= uint64(c)
        h *= 1099511628211
    }
    return int64(h)
}

func generateTurnID() string {
    // T_{date}_{time}_{hex}
    now := time.Now().UTC()
    return fmt.Sprintf("T_%s_%s_%06x",
        now.Format("20060102"),
        now.Format("150405"),
        rand.Intn(0xFFFFFF),
    )
}
```

## 20. CLI Commands

```
## ── Project Setup ──────────────────────────────

gam init                                Initialize project: arch.md, .gamignore, docs/, PostgreSQL schema, Redis streams
gam init --minimal                      Minimal: arch.md + .gamignore + docs/ only, PostgreSQL later

## ── Turn Lifecycle ─────────────────────────────

gam turn start --region <path>          Start turn: generate ID, create branch, load scratchpad, compile context
gam turn end --scratchpad "..."         End turn: validate, save memory, generate tree, export docs, queue proposals
gam turn status                         Show active turns (including which plan they belong to)
gam turn memory <region>                Query scratchpads from turns that touched this region
gam turn search "text"                  Full-text search across all scratchpads
gam turn diff <turn_id>                 Show structural diff (regions added/modified/deleted)

## ── Region Management ──────────────────────────

gam region touch <path> --file <f>      Create/scaffold region markers in a file
gam region list                         List all regions from arch.md
gam region show <path>                  Show region details: concept assignments, syncs, recent turns, quality grade

## ── Concept Management ─────────────────────────

gam concept add <n> --spec <file>    Register a concept from a spec file
gam concept show <n>                 Display concept spec
gam concept list                        List all concepts with purposes
gam concept assign <concept> <region> --role <role>  Create junction table entry

## ── Sync Management ────────────────────────────

gam sync add <n> --spec <file>       Register a synchronization from a spec file
gam sync list                           List all syncs
gam sync list --concept <n>          List syncs referencing a concept
gam sync show <n>                    Display sync details with referenced concepts
gam sync check                          Verify all sync references are valid (Tier 1 subset)

## ── Flow Provenance ────────────────────────────

gam flow trace <token>                  Show causal graph for a flow token
gam flow list --recent <N>              Show recent flow tokens with root actions

## ── Structure and Validation ───────────────────

gam tree <dir>                          Generate tree view from region markers in source files
gam tree --diff <turn_id>               Tree view showing changes from a specific turn
gam validate <path>                     Run Tier 0 + 1 against a region or file
gam validate --all                      Validate entire project

## ── Execution Plans ────────────────────────────

gam plan create <n> --goal "..."     Create a new execution plan
gam plan show <n>                    Show plan with progress, decisions, quality grade
gam plan list                           List active and completed plans
gam plan list --active                  List only active plans
gam plan decide <n> --decision "..." --rationale "..."  Record a design decision
gam plan close <n>                   Mark plan as completed

## ── Docs Projection ────────────────────────────

gam docs export                         Export all DB state to docs/ (concepts, syncs, plans, quality)
gam docs import                         Import docs/ back to DB (bootstrapping or external edits)
gam docs status                         Show which docs/ files are stale vs DB

## ── Quality and Gardening ──────────────────────

gam quality grades                      Show quality grades for all regions
gam quality grades --region <path>      Show quality grades for a specific region
gam quality principles                  List golden principles with enforcement status
gam quality principles add --name "..." --rule "..." --remediation "..."
gam gardener run                        Run full entropy sweep and queue fix-up turns
gam gardener run --dry                  Preview findings without creating turns

## ── Architecture Sync ──────────────────────────

gam arch sync                           Bidirectional sync between arch.md and PostgreSQL
gam arch export                         Export DB namespace tree to arch.md
gam arch import                         Import arch.md namespace tree to DB

## ── Queue Management ───────────────────────────

gam queue status                        Show pending tasks and proposals in Redis
gam queue inspect <id>                  Show details of a queued item
gam queue escalated                     Show proposals awaiting human review (Tier 3 escalations)

## ── Agent Execution ────────────────────────────

gam memorizer run                       Run Memorizer: process proposals, create turns, manage plans
gam researcher run                      Run Researcher: process task queue, emit proposals
gam run                                 Sequential: Memorizer → Researcher → Memorizer with checkpoints
gam run --auto                          Automated loop until queues empty or human escalation
gam run --auto --gardener               Automated loop including periodic gardener sweeps
```

## 21. Context Compilation Example

When `gam turn start --region app.search.sources` runs, the compiled context file contains:

```markdown
# Turn Context: app.search.sources
# Turn ID: T_20260212_143052_abc123
# Branch: turn/T_20260212_143052_abc123

## Execution Plan: add-btv2-source
Goal: Add BitTorrent v2 index as a new search source with rate limiting and health checks.
Status: ACTIVE (turn 2 of 3)
Progress:
  [x] T_20260211_091500_def456 — app.search.sources: Implement btv2 adapter (COMPLETED)
  [ ] T_20260212_143052_abc123 — app.search.sources: Add rate limiting and health check (THIS TURN)
  [ ] T_20260213_xxxxxx_xxxxxx — app.search.tests: Integration tests for btv2
Decisions:
  - Rate limit: 2 req/s (rationale: btv2 API terms of service specify max 2 req/s)
  - Health check: HTTP HEAD to /api/status (rationale: cheapest endpoint, no auth required)

## Previous Scratchpad
[T_20260211_091500_def456] Added nyaa and rarbg sources. Both passing tests.
Rate limiting works. TODO: Add btv2 index source. Consider adding health check
endpoint for source monitoring.

## Quality Grade: app.search.sources
  test_coverage: B (unit tests present, no integration tests for btv2 yet)
  doc_completeness: C (concept spec current, but no usage docs for btv2)
  invariant_coverage: A (all invariants have validators)

## Concept: SearchSource [S]
Purpose: to register and query torrent index providers
State:
  sources: set S
  name: S -> string
  endpoint: S -> url
  enabled: S -> boolean
  rate_limit: S -> int
Actions:
  register [source: S; name: string; endpoint: url] => [source: S]
  register [source: S; name: string; endpoint: url] => [error: string]
  query [source: S; terms: string] => [results: []Result]
  query [source: S; terms: string] => [error: string]
  disable [source: S] => [source: S]
Invariants:
  - rate_limit_positive: rate_limit > 0
  - name_unique: unique(name)
  - api_rule: {no_removals: true}
Operational Principle:
  after register[source:x; name:"nyaa"; endpoint:"https://nyaa.si/api"] => [source:x]
  then query[source:x; terms:"ubuntu"] => [results: rs] where len(rs) >= 0

## Synchronizations Referencing SearchSource

sync FanOutSearch
  when: Web/request[method:"search"; terms:?terms] => [request:?request]
  where: SearchSource: {?s enabled: true}
  then: SearchSource/query[source:?s; terms:?terms]

sync CollectResults
  when: SearchAggregator/search[] => [query:?q]
        SearchSource/query[] => [results:?results]
  then: SearchAggregator/collect[query:?q; source_results:?results]

sync SearchError
  when: Web/request[method:"search"] => [request:?request]
        SearchSource/query[] => [error:?error]
  then: Web/respond[request:?request; error:?error; code:502]

## Current Tree: app.search.sources
├── nyaa                    [src/search/nyaa.go:1-52]
│   ├── adapter             [src/search/nyaa.go:5-28]
│   └── parser              [src/search/nyaa.go:30-48]
└── rarbg                   [src/search/rarbg.go:1-39]
```

The agent reads this and knows: what plan it's part of and what's already done, what concept it's working within, what actions exist, what invariants apply, what syncs will compose with its work, what the previous agent decided, what the quality gaps are, and what the current code structure looks like. It never sees the SearchAggregator's implementation code or the notification system's internals. Only concept specs and syncs.

## 22. Spec-First Generation Rule

An explicit design property, not an inherited benefit: the CLI's context compilation for sync authoring deliberately excludes implementation code and includes only concept specs.

When a Researcher is writing synchronizations, its context contains:
- Concept specs for all involved concepts (purpose, state, actions)
- Existing syncs for reference
- The API contract (action signatures with named arguments)

It does not contain:
- Implementation code of any concept
- Database schemas
- Internal state representations

This is not just a context window optimization. It is an independence invariant. If an agent sees another concept's code while writing a sync, it will write syncs that depend on implementation details rather than action interfaces. Those syncs break when the implementation changes. The spec is the interface. The sync binds to the interface. The validator enforces the binding.

## 23. Design Properties

**arch.md is always legible.** An agent or human reads ~80 lines of region markers and knows the full system structure. No scrolling through specs. This is the table of contents, not the encyclopedia.

**docs/ is always navigable.** Any agent — with or without the `gam` CLI — can read concept specs, sync definitions, execution plans, and quality grades from versioned markdown files in the repository. Knowledge that isn't in the repo doesn't exist to agents.

**PostgreSQL is always queryable.** "What concepts govern this region?" "What syncs reference this action?" "What did the last agent decide about pagination?" — single indexed queries.

**The CLI bridges without coupling.** arch.md doesn't contain database IDs. PostgreSQL doesn't contain markdown. docs/ is a projection, not a source of truth. The CLI translates between all three.

**Progressive disclosure governs context.** Agents start with arch.md (the map), navigate to docs/ (the depth), and use CLI commands (the queries). At no point is the full system state loaded into context. Each layer provides exactly the resolution needed for the current task.

**Two commands enforce everything.** `gam turn start` and `gam turn end` are the only mandatory agent steps. All validation, memory saving, DB updates, tree generation, docs export, and arch.md synchronization happen inside those commands via lifecycle hooks.

**Synchronizations make composition legible.** Every inter-concept behavior is a named, auditable rule. No implicit dependencies. No hidden call chains.

**Flow tokens make provenance traceable.** Every runtime action traces back to the sync that caused it and the request that initiated the chain. Flow-scoped matching prevents cross-request interference.

**Proposals make changes auditable.** Every modification to code, syncs, or concept specs flows through a structured proposal that the validator checks before anything reaches the codebase.

**Review is iterative, not terminal.** Tier 3 validation uses a feedback loop: the auditor flags concerns, the Researcher addresses them, the auditor re-evaluates. Escalation to human review happens only when the loop exhausts its iteration budget.

**Region markers make structure visible.** The tree view shows what code exists, where it lives in the namespace, and what's outside boundaries — for any commit, any turn, any file.

**Execution plans make strategy visible.** Multi-turn efforts are tracked as first-class artifacts with progress markers, design decisions, and quality grades. The next agent knows what's done, what's pending, and why decisions were made.

**Entropy is managed, not ignored.** The gardening agent sweeps for stale TODOs, orphaned regions, sync drift, spec divergence, and quality degradation on a recurring schedule. Technical debt is catalogued and graded, not discovered by accident.

**Rejection messages are prompts.** Every validation failure is written as an agent-actionable instruction. The correction briefing is the compiled context for the next turn. Vague error messages waste agent turns; specific remediation instructions fix issues in one pass.

**Boring technology compounds.** PostgreSQL, Redis, Go, JSONB — well-documented, stable, composable, heavily represented in training data. Agent-legibility beats aesthetic preference. When a library is opaque, reimplement the needed functionality in-repo with full test coverage.

**The agent prompt is short.** The tool handles enforcement. The agent handles thinking.
