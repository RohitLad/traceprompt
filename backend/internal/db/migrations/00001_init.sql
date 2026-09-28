-- +goose Up
-- Initial schema. Source of truth going forward; GORM AutoMigrate is
-- dev/test-only (SQLite). All statements are IF NOT EXISTS so upgrading
-- early dev databases created by AutoMigrate is safe.

CREATE TABLE IF NOT EXISTS organizations (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  email TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  password_hash TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memberships (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  user_id UUID NOT NULL,
  organization_id UUID NOT NULL,
  role TEXT NOT NULL DEFAULT 'member',
  UNIQUE (user_id, organization_id)
);

CREATE TABLE IF NOT EXISTS projects (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  organization_id UUID NOT NULL,
  name TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_projects_organization_id ON projects (organization_id);

CREATE TABLE IF NOT EXISTS api_keys (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  project_id UUID NOT NULL,
  name TEXT NOT NULL,
  public_key TEXT NOT NULL UNIQUE,
  secret_hash TEXT NOT NULL,
  created_by UUID,
  revoked_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_api_keys_project_id ON api_keys (project_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_revoked_at ON api_keys (revoked_at);

CREATE TABLE IF NOT EXISTS traces (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  project_id UUID NOT NULL,
  trace_id TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL DEFAULT '',
  user_id TEXT,
  session_id TEXT,
  metadata JSONB NOT NULL DEFAULT '{}',
  tags TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_traces_project_time ON traces (project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_traces_name ON traces (name);
CREATE INDEX IF NOT EXISTS idx_traces_user_id ON traces (user_id);
CREATE INDEX IF NOT EXISTS idx_traces_session_id ON traces (session_id);

CREATE TABLE IF NOT EXISTS observations (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  project_id UUID NOT NULL,
  trace_id TEXT NOT NULL,
  observation_id TEXT NOT NULL UNIQUE,
  parent_id TEXT,
  type TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ,
  input TEXT,
  output TEXT,
  metadata JSONB NOT NULL DEFAULT '{}',
  model TEXT,
  model_parameters JSONB NOT NULL DEFAULT '{}',
  usage_input BIGINT,
  usage_output BIGINT,
  usage_total BIGINT,
  level TEXT NOT NULL DEFAULT 'DEFAULT',
  status_message TEXT,
  environment TEXT NOT NULL DEFAULT 'default',
  prompt_name TEXT,
  prompt_version BIGINT
);
CREATE INDEX IF NOT EXISTS idx_obs_project_time ON observations (project_id, start_time);
CREATE INDEX IF NOT EXISTS idx_obs_trace ON observations (trace_id);
CREATE INDEX IF NOT EXISTS idx_obs_parent_id ON observations (parent_id);
CREATE INDEX IF NOT EXISTS idx_obs_type ON observations (type);
CREATE INDEX IF NOT EXISTS idx_obs_name ON observations (name);
CREATE INDEX IF NOT EXISTS idx_obs_model ON observations (model);
CREATE INDEX IF NOT EXISTS idx_obs_environment ON observations (environment);
CREATE INDEX IF NOT EXISTS idx_obs_prompt_name ON observations (prompt_name);

CREATE TABLE IF NOT EXISTS scores (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  project_id UUID NOT NULL,
  external_id TEXT UNIQUE,
  trace_id TEXT NOT NULL,
  observation_id TEXT,
  session_id TEXT,
  name TEXT NOT NULL,
  value_num DOUBLE PRECISION,
  value_str TEXT,
  data_type TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'API',
  comment TEXT
);
CREATE INDEX IF NOT EXISTS idx_scores_project_id ON scores (project_id);
CREATE INDEX IF NOT EXISTS idx_scores_trace_id ON scores (trace_id);
CREATE INDEX IF NOT EXISTS idx_scores_observation_id ON scores (observation_id);
CREATE INDEX IF NOT EXISTS idx_scores_session_id ON scores (session_id);
CREATE INDEX IF NOT EXISTS idx_scores_name ON scores (name);

CREATE TABLE IF NOT EXISTS prompts (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  project_id UUID NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL DEFAULT 'text',
  UNIQUE (project_id, name)
);

CREATE TABLE IF NOT EXISTS prompt_versions (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  prompt_id UUID NOT NULL,
  version BIGINT NOT NULL,
  template TEXT,
  messages TEXT NOT NULL DEFAULT '[]',
  config JSONB NOT NULL DEFAULT '{}',
  labels TEXT NOT NULL DEFAULT '[]',
  commit_message TEXT,
  UNIQUE (prompt_id, version)
);

CREATE TABLE IF NOT EXISTS datasets (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  project_id UUID NOT NULL,
  name TEXT NOT NULL,
  description TEXT,
  UNIQUE (project_id, name)
);

CREATE TABLE IF NOT EXISTS dataset_items (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  dataset_id UUID NOT NULL,
  input TEXT,
  expected_output TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_dataset_items_dataset_id ON dataset_items (dataset_id);

CREATE TABLE IF NOT EXISTS dataset_runs (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  dataset_id UUID NOT NULL,
  name TEXT NOT NULL,
  description TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_dataset_runs_dataset_id ON dataset_runs (dataset_id);
CREATE INDEX IF NOT EXISTS idx_dataset_runs_name ON dataset_runs (name);

CREATE TABLE IF NOT EXISTS dataset_run_items (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  run_id UUID NOT NULL,
  item_id UUID NOT NULL,
  trace_id TEXT,
  UNIQUE (run_id, item_id)
);
CREATE INDEX IF NOT EXISTS idx_dataset_run_items_trace_id ON dataset_run_items (trace_id);

-- +goose Down
DROP TABLE IF EXISTS dataset_run_items;
DROP TABLE IF EXISTS dataset_runs;
DROP TABLE IF EXISTS dataset_items;
DROP TABLE IF EXISTS datasets;
DROP TABLE IF EXISTS prompt_versions;
DROP TABLE IF EXISTS prompts;
DROP TABLE IF EXISTS scores;
DROP TABLE IF EXISTS observations;
DROP TABLE IF EXISTS traces;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
