-- +goose Up
-- Evaluation: score schemas + human-review queues.

CREATE TABLE IF NOT EXISTS score_configs (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  project_id UUID NOT NULL,
  name TEXT NOT NULL,
  data_type TEXT NOT NULL,
  min_value DOUBLE PRECISION,
  max_value DOUBLE PRECISION,
  categories TEXT NOT NULL DEFAULT '[]',
  description TEXT,
  UNIQUE (project_id, name)
);

CREATE TABLE IF NOT EXISTS annotation_queues (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  project_id UUID NOT NULL,
  name TEXT NOT NULL,
  description TEXT,
  UNIQUE (project_id, name)
);

CREATE TABLE IF NOT EXISTS annotation_queue_items (
  id UUID PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  queue_id UUID NOT NULL,
  trace_id TEXT,
  observation_id TEXT,
  status TEXT NOT NULL DEFAULT 'PENDING'
);
CREATE INDEX IF NOT EXISTS idx_anno_items_queue_id ON annotation_queue_items (queue_id);
CREATE INDEX IF NOT EXISTS idx_anno_items_trace_id ON annotation_queue_items (trace_id);
CREATE INDEX IF NOT EXISTS idx_anno_items_status ON annotation_queue_items (status);

-- +goose Down
DROP TABLE IF EXISTS annotation_queue_items;
DROP TABLE IF EXISTS annotation_queues;
DROP TABLE IF EXISTS score_configs;
