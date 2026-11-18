ALTER TABLE scope_presets ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
UPDATE scope_presets SET sort_order = id;
