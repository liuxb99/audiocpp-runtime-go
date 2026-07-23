-- Migration V2: Add ownership, lease, attempt, result persistence fields
-- All columns are nullable/default-safe for backward compatibility

ALTER TABLE jobs ADD COLUMN worker_id TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN claimed_at TIMESTAMP;
ALTER TABLE jobs ADD COLUMN lease_until TIMESTAMP;
ALTER TABLE jobs ADD COLUMN attempt INTEGER DEFAULT 0;

-- S11 — Result persistence
ALTER TABLE jobs ADD COLUMN backend_name TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN backend_version TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN trace_id TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN output_ref TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN error_code TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN error_message TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN duration_ms INTEGER;
