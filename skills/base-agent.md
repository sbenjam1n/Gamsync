You are working in a GAM+Sync codebase.

GAM+Sync is a framework for agentic software development with concepts (independent units of functionality), synchronizations (declarative inter-concept composition rules), region markers (structural namespace enforcement), and a tiered validation pipeline.

# Orientation

Read arch.md first. It is the namespace skeleton — ~80 lines of `@region`/`@endregion` markers with dotwalked namespace paths and one-line descriptions. This is your map.

```
# @region:app.search.sources Search Source Implementations
# @endregion:app.search.sources
```

For depth on a specific concept, sync, or plan, read the corresponding file in docs/:
  docs/concepts/<name>.md     — Full concept specs
  docs/syncs/<name>.md        — Synchronization definitions
  docs/exec-plans/active/     — In-progress execution plans
  docs/exec-plans/completed/  — Completed plans with decision history
  docs/quality/grades.md      — Per-region quality assessment
  docs/quality/golden-principles.md — Mechanical coherence rules

For interactive queries, use the gam CLI:
  gam concept show <name>           — Display concept spec
  gam sync list --concept <name>    — List syncs referencing a concept
  gam turn memory <region>          — Scratchpads from turns that touched a region
  gam turn search "keyword"         — Full-text search across all scratchpads
  gam region show <path>            — Region details with assignments and quality
  gam plan show <name>              — Execution plan with progress

# Turn Ritual

Start every session:
  gam turn start --region <target> --prompt "what you're working on"
  This gives you: ALL pertinent turn memory (region-scoped, concept-scoped, and
  prompt-relevant via similarity search), concept specs, relevant syncs, quality grades.

  The --prompt flag enables relevance-based memory search — the memorizer searches
  all scratchpads for content similar to your task, not just the last one.

When writing code, wrap it in region markers:
  // @region:namespace.path
  // ... code ...
  // @endregion:namespace.path

Write region markers directly in the correct comment syntax for the language.
The CLI validates their correctness — you don't need a scaffolding tool.

When behavior crosses concepts, write synchronizations, not imperative handlers.

When your task involves a concept you're not assigned to, emit a deferred action.

End every session:
  gam turn end --scratchpad "what you did and what's next"
  This VALIDATES your work (blocks on failure), saves memory, queues proposals.
  Validation checks: arch.md alignment, region marker integrity, namespace hierarchy.
  If validation fails, fix the issues and retry. Use --skip-validation only as last resort.

# Region Markers

All code must live inside region markers. Write them directly in the correct comment style:
  Go, JS, TS, Rust:  // @region:path    // @endregion:path
  Python, Ruby, YAML: # @region:path    # @endregion:path
  SQL, Lua:           -- @region:path   -- @endregion:path
  HTML, Vue:          <!-- @region:path --> <!-- @endregion:path -->
  CSS:                /* @region:path */ /* @endregion:path */

When you create a new region in source code, add it to arch.md too.
When you remove a region from source code, remove it from arch.md too.
Validation (`gam validate --arch`) catches mismatches.
Turn end blocks on validation failure — this is how mirroring is enforced.

Regions can span multiple files. A region can appear in as many files as needed.
Changes can touch multiple regions — validation checks them all.

# arch.md

arch.md is `@region`/`@endregion` markers forming a namespace tree with one-line descriptions.
Nothing else. No prose, no specs, no scratchpads, no JSON. The namespace path is a dotwalked
identifier (e.g., `app.search.sources.btv2`).

```
# @region:app.search Search Subsystem
# @region:app.search.sources Source Management
# @endregion:app.search.sources
# @endregion:app.search
```

Every child namespace must have its parent defined (e.g., `app.search.sources` requires `app.search`).
Validation checks this hierarchy and reports violations.

# Concepts

A concept is a self-contained unit of functionality. It has:
  - Purpose: why it exists
  - State: relational components (sets, maps)
  - Actions: named operations with typed input/output
  - Invariants: rules that must always hold
  - Operational principle: archetypal scenario (doubles as test)

Concepts have NO dependencies on other concepts. They may depend on infrastructure (databases, networking) but never on each other. Composition happens via synchronizations.

# Synchronizations

Syncs are declarative rules: when these actions happen, where these conditions hold, then invoke these other actions.

  sync FanOutSearch
  when { Web/request[method:"search"; terms:?terms] => [request:?request] }
  where { SearchSource: {?s enabled: true} }
  then { SearchSource/query[source:?s; terms:?terms] }

Adding behavior = adding a sync. Removing behavior = deleting a sync. No code changes to any concept.

# Proposals

Every change flows through a proposal carrying:
  - Scope: target region path(s)
  - Transition: current state → proposed state via action
  - Sync changes: added/modified/deleted sync rules
  - Evidence: API analysis, migration analysis, dependency analysis
  - Deferred actions: tasks for other concepts

# Validation

Changes are validated through tiers (each gates the next):
  Tier 0 (Structural): Region markers match arch.md, code inside regions, namespace hierarchy
  Tier 1 (State Machine): Legal transitions, invariant rules, sync reference integrity
  Tier 2 (Integration): Build, tests, evidence truthfulness
  Tier 3 (LLM Review): Architectural alignment, iterative feedback loop
  Tier 4 (Runtime): Boot app, run operational principles live

Turn end blocks on validation failure. Rejection messages are agent-actionable:
they tell you exactly what to fix and how.

# The CLI handles enforcement. You handle thinking and coding.
