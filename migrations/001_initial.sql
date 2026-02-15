-- GAM+Sync Initial Schema (idempotent)
-- Extensions
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Concepts
CREATE TABLE IF NOT EXISTS concepts (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name          VARCHAR(255) UNIQUE NOT NULL,
  purpose       TEXT NOT NULL,
  spec          JSONB NOT NULL,
  state_machine JSONB NOT NULL,
  invariants    JSONB NOT NULL DEFAULT '[]',
  created_at    TIMESTAMPTZ DEFAULT NOW(),
  updated_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Regions
CREATE TABLE IF NOT EXISTS regions (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  path            ltree UNIQUE NOT NULL,
  description     TEXT,
  lifecycle_state VARCHAR(50) DEFAULT 'draft',
  updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_regions_path_gist ON regions USING GIST (path);
CREATE INDEX IF NOT EXISTS idx_regions_path_btree ON regions USING BTREE (path);

-- Concept-Region Junction
CREATE TABLE IF NOT EXISTS concept_region_assignments (
  concept_id UUID REFERENCES concepts(id) ON DELETE CASCADE,
  region_id  UUID REFERENCES regions(id) ON DELETE CASCADE,
  role       VARCHAR(50) NOT NULL DEFAULT 'implementation',
  PRIMARY KEY (concept_id, region_id)
);

CREATE INDEX IF NOT EXISTS idx_cra_concept ON concept_region_assignments(concept_id);
CREATE INDEX IF NOT EXISTS idx_cra_region ON concept_region_assignments(region_id);

-- Synchronizations
CREATE TABLE IF NOT EXISTS synchronizations (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name         VARCHAR(255) UNIQUE NOT NULL,
  when_clause  JSONB NOT NULL,
  where_clause JSONB,
  then_clause  JSONB NOT NULL,
  description  TEXT,
  enabled      BOOLEAN DEFAULT true,
  created_at   TIMESTAMPTZ DEFAULT NOW(),
  updated_at   TIMESTAMPTZ DEFAULT NOW()
);

-- Sync References (for impact analysis)
CREATE TABLE IF NOT EXISTS sync_refs (
  sync_id       UUID REFERENCES synchronizations(id) ON DELETE CASCADE,
  concept_name  VARCHAR(255) NOT NULL,
  action_name   VARCHAR(255),
  state_field   VARCHAR(255),
  clause_type   VARCHAR(10) NOT NULL,
  PRIMARY KEY (sync_id, concept_name, COALESCE(action_name,''), COALESCE(state_field,''), clause_type)
);

CREATE INDEX IF NOT EXISTS idx_sync_refs_concept ON sync_refs(concept_name);
CREATE INDEX IF NOT EXISTS idx_sync_refs_action ON sync_refs(concept_name, action_name);
CREATE INDEX IF NOT EXISTS idx_sync_refs_field ON sync_refs(concept_name, state_field);

-- Execution Plans (must be before turns due to FK)
DO $$ BEGIN
    CREATE TYPE plan_status AS ENUM ('ACTIVE', 'COMPLETED', 'ABANDONED');
EXCEPTION WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS execution_plans (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        VARCHAR(255) NOT NULL,
  goal        TEXT NOT NULL,
  status      plan_status DEFAULT 'ACTIVE',
  decisions   JSONB DEFAULT '[]',
  quality_grade VARCHAR(10),
  created_at  TIMESTAMPTZ DEFAULT NOW(),
  completed_at TIMESTAMPTZ
);

-- Turns
DO $$ BEGIN
    CREATE TYPE turn_status AS ENUM ('ACTIVE', 'COMPLETED', 'ABANDONED');
EXCEPTION WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS turns (
  id          VARCHAR(64) PRIMARY KEY,
  agent_id    VARCHAR(255),
  agent_role  VARCHAR(50),
  scope_path  ltree,
  plan_id     UUID REFERENCES execution_plans(id),
  task_type   VARCHAR(50) DEFAULT 'implement',
  scratchpad  TEXT,
  status      turn_status DEFAULT 'ACTIVE',
  tree_before JSONB,
  tree_after  JSONB,
  created_at  TIMESTAMPTZ DEFAULT NOW(),
  completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_turns_status ON turns(status);
CREATE INDEX IF NOT EXISTS idx_turns_scope ON turns USING GIST(scope_path);

-- Turn-Region Log
CREATE TABLE IF NOT EXISTS turn_regions (
  turn_id    VARCHAR(64) REFERENCES turns(id),
  region_id  UUID REFERENCES regions(id),
  action     VARCHAR(50) NOT NULL,
  PRIMARY KEY (turn_id, region_id)
);

-- Proposals
DO $$ BEGIN
    CREATE TYPE proposal_status AS ENUM
      ('PENDING', 'VALIDATING', 'APPROVED', 'REJECTED');
EXCEPTION WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS proposals (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  turn_id              VARCHAR(64) REFERENCES turns(id),
  region_id            UUID REFERENCES regions(id) NOT NULL,
  action_taken         VARCHAR(100) NOT NULL,
  current_state        VARCHAR(100),
  proposed_state       VARCHAR(100),
  sync_changes         JSONB,
  evidence             JSONB NOT NULL,
  deferred_actions     JSONB DEFAULT '[]',
  status               proposal_status DEFAULT 'PENDING',
  review_iterations    INT DEFAULT 0,
  review_history       JSONB DEFAULT '[]',
  validation_error_code INTEGER,
  violation_details    JSONB,
  rejection_reason     TEXT,
  branch_name          VARCHAR(255),
  commit_sha           CHAR(40),
  created_at           TIMESTAMPTZ DEFAULT NOW()
);

DO $$ BEGIN
    ALTER TABLE proposals ADD CONSTRAINT check_rejection_data
      CHECK (status != 'REJECTED' OR validation_error_code IS NOT NULL);
EXCEPTION WHEN duplicate_object THEN null;
END $$;

CREATE INDEX IF NOT EXISTS idx_proposals_turn ON proposals(turn_id);
CREATE INDEX IF NOT EXISTS idx_proposals_region ON proposals(region_id);
CREATE INDEX IF NOT EXISTS idx_proposals_status ON proposals(status);

-- Flow Log (runtime provenance)
CREATE TABLE IF NOT EXISTS flow_log (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  flow_token     UUID NOT NULL,
  concept_name   VARCHAR(255) NOT NULL,
  action_name    VARCHAR(255) NOT NULL,
  input_args     JSONB,
  output_args    JSONB,
  sync_name      VARCHAR(255),
  parent_id      UUID REFERENCES flow_log(id),
  created_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_flow_token ON flow_log(flow_token);
CREATE INDEX IF NOT EXISTS idx_flow_sync ON flow_log(sync_name);
CREATE INDEX IF NOT EXISTS idx_flow_concept_action ON flow_log(concept_name, action_name);

-- Lifecycle Hooks
CREATE TABLE IF NOT EXISTS lifecycle_hooks (
  id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event     VARCHAR(50) NOT NULL,
  hook_name VARCHAR(255) NOT NULL,
  priority  INT DEFAULT 100,
  handler   VARCHAR(255) NOT NULL,
  config    JSONB,
  enabled   BOOLEAN DEFAULT true,
  scope     ltree,
  UNIQUE(event, hook_name)
);

CREATE INDEX IF NOT EXISTS idx_hooks_event ON lifecycle_hooks(event) WHERE enabled = true;

-- Plan Turns
CREATE TABLE IF NOT EXISTS plan_turns (
  plan_id     UUID REFERENCES execution_plans(id) ON DELETE CASCADE,
  turn_id     VARCHAR(64) REFERENCES turns(id),
  region_path ltree NOT NULL,
  ordering    INT NOT NULL,
  depends_on  VARCHAR(64)[],
  status      VARCHAR(50) DEFAULT 'pending',
  PRIMARY KEY (plan_id, turn_id)
);

CREATE INDEX IF NOT EXISTS idx_plan_turns_plan ON plan_turns(plan_id);
CREATE INDEX IF NOT EXISTS idx_plan_turns_status ON plan_turns(status);

-- Quality Tracking
CREATE TABLE IF NOT EXISTS quality_grades (
  region_id     UUID REFERENCES regions(id) ON DELETE CASCADE,
  category      VARCHAR(50) NOT NULL,
  grade         VARCHAR(10) NOT NULL,
  details       JSONB,
  assessed_at   TIMESTAMPTZ DEFAULT NOW(),
  assessed_by   VARCHAR(64),
  PRIMARY KEY (region_id, category)
);

CREATE TABLE IF NOT EXISTS golden_principles (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        VARCHAR(255) UNIQUE NOT NULL,
  rule        TEXT NOT NULL,
  lint_check  VARCHAR(255),
  remediation TEXT NOT NULL,
  enabled     BOOLEAN DEFAULT true,
  created_at  TIMESTAMPTZ DEFAULT NOW()
);
