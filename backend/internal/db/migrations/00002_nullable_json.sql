-- +goose Up
-- GORM writes explicit NULL for nil slices/maps even when a column has a
-- DEFAULT, so serializer-backed columns must accept NULL. (00001 declared
-- them NOT NULL, which broke prompt creation: text prompts store NULL
-- messages.) Reads already handle NULL as empty.
ALTER TABLE traces            ALTER COLUMN metadata DROP NOT NULL;
ALTER TABLE traces            ALTER COLUMN tags DROP NOT NULL;
ALTER TABLE observations      ALTER COLUMN metadata DROP NOT NULL;
ALTER TABLE observations      ALTER COLUMN model_parameters DROP NOT NULL;
ALTER TABLE prompt_versions   ALTER COLUMN messages DROP NOT NULL;
ALTER TABLE prompt_versions   ALTER COLUMN config DROP NOT NULL;
ALTER TABLE prompt_versions   ALTER COLUMN labels DROP NOT NULL;
ALTER TABLE dataset_items     ALTER COLUMN metadata DROP NOT NULL;
ALTER TABLE dataset_runs      ALTER COLUMN metadata DROP NOT NULL;

-- +goose Down
-- Down keeps NULL allowed (data may already contain NULLs); no-op by design.
SELECT 1;
