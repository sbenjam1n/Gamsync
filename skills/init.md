# Init Skill

You are bootstrapping a new GAM+Sync project. This skill guides you through project initialization, namespace design, and first concept creation.

## When to Use This Skill

Use this skill when:
- Starting a new project from scratch
- Converting an existing codebase to GAM+Sync
- Redesigning the namespace hierarchy of a project

## Project Bootstrap Workflow

### 1. Initialize the Project

```
gam init --minimal    # Files only: arch.md, .gamignore, docs/
gam init              # Full: + PostgreSQL schema + Redis streams
```

Minimal init creates:
- `arch.md` — Namespace skeleton (`@region`/`@endregion` markers)
- `.gamignore` — Patterns for files that skip region enforcement
- `docs/` — Structured documentation directory

Full init additionally:
- Runs PostgreSQL migrations (14 tables, LTREE + JSONB)
- Creates Redis streams and consumer groups

### 2. Design the Namespace Hierarchy

Edit arch.md to define your project's namespace tree. Use `@region`/`@endregion` markers with dotwalked namespace paths and one-line descriptions:

```
# @region:app
# @region:app.auth Authentication
# @region:app.auth.session Session management
# @endregion:app.auth.session
# @region:app.auth.tokens JWT / refresh tokens
# @endregion:app.auth.tokens
# @endregion:app.auth
# @region:app.billing Billing
# @region:app.billing.plans Subscription plans
# @endregion:app.billing.plans
# @region:app.billing.invoices Invoice generation
# @endregion:app.billing.invoices
# @endregion:app.billing
# @region:app.search Search
# @region:app.search.sources Source adapters
# @region:app.search.sources.elastic Elasticsearch adapter
# @endregion:app.search.sources.elastic
# @region:app.search.sources.postgres PostgreSQL full-text search
# @endregion:app.search.sources.postgres
# @endregion:app.search.sources
# @region:app.search.ranking Result ranking
# @endregion:app.search.ranking
# @endregion:app.search
# @endregion:app
```

Design principles:
- **Top-down**: Start with broad domains, then subdivide
- **One concept per leaf**: Each leaf namespace should map to roughly one concept
- **Depth 2-4**: Most namespaces are 2-4 segments deep. Deeper is usually over-engineering
- **Every child has a parent**: `app.search.sources` requires `app.search` to exist
- **Flat where possible**: Prefer `app.auth` and `app.billing` over `app.core.auth` and `app.core.billing` unless grouping adds clarity

Validate your design:
```
gam validate --arch
```

### 3. Add Region Markers to Source Code

For each namespace in arch.md, add `@region/@endregion` markers to source files:

```go
// @region:app.auth.session
package session

type Session struct {
    ID        string
    UserID    string
    ExpiresAt time.Time
}

func Create(userID string) (*Session, error) {
    // ...
}
// @endregion:app.auth.session
```

A single namespace can appear in multiple files — for example, `app.auth.session` might have markers in both `session.go` and `session_store.go`.

Validate alignment:
```
gam validate --arch
```

### 4. Define Concepts

For each leaf namespace, create a concept spec:

```
gam concept add Session --spec session.json
```

Where `session.json` contains:
```json
{
  "purpose": "Manage authenticated user sessions with time-bounded access",
  "state": {
    "sessions": {"type": "set", "fields": {"id": "string", "user_id": "string", "expires_at": "timestamp"}},
    "active_count": {"type": "counter"}
  },
  "actions": {
    "create": {"input": {"user_id": "string"}, "output": {"session": "Session"}},
    "validate": {"input": {"session_id": "string"}, "output": {"valid": "boolean"}},
    "revoke": {"input": {"session_id": "string"}, "output": {}}
  },
  "operational_principle": "After create(user_id) succeeds, validate(session.id) returns valid=true until expires_at is reached or revoke(session.id) is called.",
  "invariants": [
    {"name": "no_expired_active", "type": "state", "rule": "No session in the active set has expires_at in the past"}
  ]
}
```

Then assign it to a region:
```
gam concept assign Session --region app.auth.session
```

### 5. Define Synchronizations

Once concepts exist, define how they compose:

```
gam sync add SessionOnLogin --spec session_on_login.json
```

```json
{
  "name": "SessionOnLogin",
  "when_clause": [{"concept": "Auth", "action": "login", "output_match": {"user_id": "?uid"}}],
  "where_clause": [],
  "then_clause": [{"concept": "Session", "action": "create", "args": {"user_id": "?uid"}}],
  "description": "Create a session when a user successfully logs in"
}
```

### 6. Sync with Database

```
gam arch sync
```

This registers all arch.md namespaces in the database.

### 7. First Turn

```
gam turn start --region app.auth.session --prompt "implement session creation"
# ... write code ...
gam turn end --scratchpad "implemented Session.create with UUID generation and TTL. Next: implement validate and revoke."
```

## Converting an Existing Codebase

When adding GAM+Sync to an existing project:

1. `gam init --minimal` — creates arch.md and .gamignore without touching existing code
2. Map your existing directory/package structure to namespaces in arch.md
3. Add `@region/@endregion` markers to existing source files around logical boundaries
4. Define concepts for each namespace
5. Run `gam validate --arch` to verify alignment
6. Update `.gamignore` to exclude generated code, vendored deps, test fixtures, etc.

### .gamignore Patterns

```
# Vendored dependencies
vendor/

# Generated code (don't enforce regions on generated files)
gen/
*.pb.go
*_sqlc.go

# Build artifacts
bin/
dist/

# Config files
*.yaml
*.toml
*.env*

# Shared utilities (cross concept boundaries by design)
pkg/util/
pkg/middleware/

# Test fixtures
testdata/
```

## What This Skill Does NOT Cover

- Writing application code (use the Researcher skill)
- Validating proposals (use the Memorizer skill)
- Entropy sweeps (use the Gardener skill)

This skill is for one-time project setup. Once the namespace hierarchy exists and concepts are defined, switch to the Memorizer and Researcher skills for ongoing development.
