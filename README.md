# GAM+Sync

Agentic Memory with Concept Design, Synchronizations, and Structural Enforcement.

GAM+Sync is a CLI tool (`gam`) for managing agentic software development. It provides concepts (independent units of functionality), synchronizations (declarative inter-concept rules), region markers (structural namespace enforcement), and a validation pipeline — so autonomous coding agents can work reliably on large codebases.

## Architecture

```
┌──────────────┐    ┌──────────────┐
│  Memorizer   │◄──►│  Researcher  │
│  (auditor)   │    │  (coder)     │
└──────┬───────┘    └──────┬───────┘
       │                   │
       ▼                   ▼
┌──────────────────────────────────┐
│         Redis Streams            │
│  agent_tasks  │  agent_proposals │
└──────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────┐
│          PostgreSQL              │
│  concepts, regions, syncs,       │
│  turns, proposals, flow_log,     │
│  execution_plans, quality_grades │
└──────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────┐
│        Filesystem                │
│  arch.md  │  docs/  │  source    │
└──────────────────────────────────┘
```

## Quick Start

```bash
# Build
go build -o gam ./cmd/gam/

# Initialize (filesystem only, no DB required)
gam init --minimal

# Initialize (full: PostgreSQL + Redis)
export GAM_DATABASE_URL="postgres://localhost:5432/gamsync?sslmode=disable"
export GAM_REDIS_URL="redis://localhost:6379/0"
gam init

# Add a region and concept
gam region touch app.search --file src/search/search.go
gam concept add SearchSource --purpose "register and query torrent index providers" --spec search_source.json
gam concept assign SearchSource app.search --role implementation

# Start working
gam turn start --region app.search
# ... write code inside region markers ...
gam turn end --scratchpad "Added search adapter. TODO: add rate limiting."
```

## Core Abstractions

### Concepts

A concept is a self-contained unit of functionality with a purpose, state, actions, invariants, and an operational principle. Concepts have no dependencies on other concepts.

```
concept SearchSource [S]
purpose
  to register and query torrent index providers
state
  sources: set S
  name: S -> string
  endpoint: S -> url
  enabled: S -> boolean
actions
  register [source: S; name: string; endpoint: url] => [source: S]
  query [source: S; terms: string] => [results: []Result]
  disable [source: S] => [source: S]
invariants
  - rate_limit_positive: rate_limit > 0
  - name_unique: unique(name)
```

### Synchronizations

Synchronizations are declarative rules that compose concepts without creating dependencies:

```
sync FanOutSearch
when { Web/request[method:"search"; terms:?terms] => [request:?request] }
where { SearchSource: {?s enabled: true} }
then { SearchSource/query[source:?s; terms:?terms] }
```

### Region Markers

Region markers tag code with namespace paths, enabling structural enforcement:

```go
// @region:app.search.sources.btv2
package search

type BTv2Source struct { ... }

// @region:app.search.sources.btv2.adapter
func (s *BTv2Source) Query(terms string) ([]Result, error) { ... }
// @endregion:app.search.sources.btv2.adapter

// @endregion:app.search.sources.btv2
```

Comment style adapts to the language: `//` for Go/JS/Rust, `#` for Python/Ruby, `--` for SQL, `<!-- -->` for HTML.

## CLI Commands

### Project Setup
```
gam init                              Initialize project (arch.md, .gamignore, docs/, DB, Redis)
gam init --minimal                    Minimal init (arch.md + .gamignore + docs/ only)
```

### Turn Lifecycle
```
gam turn start --region <path>        Start a turn: load scratchpad, compile context
gam turn end --scratchpad "..."       End a turn: validate, save memory, queue proposals
gam turn status                       Show active turns
gam turn memory <region>              Query scratchpads for a region
gam turn search "text"                Full-text search across scratchpads
gam turn diff <turn_id>               Show structural diff for a turn
```

### Region Management
```
gam region touch <path> --file <f>    Scaffold region markers in a file
gam region list                       List all regions
gam region show <path>                Show region details, concept assignments, quality
```

### Concept Management
```
gam concept add <name> --spec <file>  Register a concept from JSON spec
gam concept show <name>               Display concept spec
gam concept list                      List all concepts
gam concept assign <concept> <region> --role <role>
```

### Sync Management
```
gam sync add <name> --spec <file>     Register a synchronization
gam sync list [--concept <name>]      List syncs (optionally filtered by concept)
gam sync show <name>                  Display sync with references
gam sync check                        Verify all sync references are valid
```

### Structure and Validation
```
gam tree [dir]                        Tree view from region markers
gam validate <path>                   Run Tier 0 + Tier 1 validation
gam validate --all                    Validate entire project
```

### Execution Plans
```
gam plan create <name> --goal "..."   Create multi-turn execution plan
gam plan show <name>                  Show plan with progress and decisions
gam plan list [--active]              List plans
gam plan decide <name> --decision "..." --rationale "..."
gam plan close <name>                 Mark plan completed
```

### Flow Provenance
```
gam flow trace <token>                Show causal graph for a flow token
gam flow list --recent <N>            Show recent flow tokens
```

### Docs Projection
```
gam docs export                       Export DB state to docs/ directory
gam docs import                       Import docs/ back to DB
gam docs status                       Check for stale docs
```

### Quality and Gardening
```
gam quality grades [--region <path>]  Show quality grades
gam quality principles                List golden principles
gam quality principles add --name "..." --rule "..." --remediation "..."
gam gardener run [--dry]              Run entropy sweep
```

### Architecture Sync
```
gam arch sync                         Bidirectional sync between arch.md and DB
gam arch export                       Export DB regions to arch.md
gam arch import                       Import arch.md to DB
```

### Agent Execution
```
gam memorizer run                     Run Memorizer (process proposals)
gam run [--auto] [--gardener]         Run Memorizer-Researcher loop
gam queue status                      Show pending tasks/proposals
gam queue escalated                   Show proposals needing human review
```

## Validation Pipeline

Each tier gates the next:

| Tier | Name | Speed | What it checks |
|------|------|-------|----------------|
| 0 | Structural | Instant | Region exists, scope check, markers present |
| 1 | State Machine | Microseconds | Legal transitions, invariants, sync reference integrity |
| 2 | Integration | Seconds | Build, tests, evidence truthfulness (Dagger) |
| 3 | LLM Review | Seconds/iter | Architectural alignment, iterative feedback loop |
| 4 | Runtime | Minutes | Boot app, run operational principles live |

Tiers 0 and 1 are implemented. Tiers 2-4 are specified and stubbed for future implementation.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `GAM_DATABASE_URL` | `postgres://localhost:5432/gamsync?sslmode=disable` | PostgreSQL connection |
| `GAM_REDIS_URL` | `redis://localhost:6379/0` | Redis connection |
| `GAM_PROJECT_ROOT` | Current directory | Project root path |

## Technology Stack

- **Go** — CLI and all services
- **PostgreSQL** — LTREE hierarchical regions, JSONB specs, advisory locks
- **Redis** — Streams with consumer groups for durable inter-agent queuing
- **Cobra** — CLI framework

## Project Structure

```
cmd/gam/                    CLI entry point
internal/
├── cli/                    Command implementations
├── config/                 Environment configuration
├── db/                     PostgreSQL connection and migrations
├── gam/                    Core types (Concept, Sync, Proposal, Turn, etc.)
├── memorizer/              Proposal processing, docs export, gardener
├── queue/                  Redis stream management
├── region/                 Region marker scanning, tree view, scaffolding
└── validator/              Tier 0 + Tier 1 validation
migrations/                 SQL schema
```

## Design

See [gam_sync.md](gam_sync.md) for the full specification covering conceptual foundations, engineering design, and implementation details.
