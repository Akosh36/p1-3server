-- Postgres can't drop a single enum value in place. Rolling back requires
-- reassigning any row already using it, then recreating the type without
-- it. Not run automatically (internal/db/migrate.go only applies *.up.sql
-- migrations) — kept here as the correct manual recipe if ever needed.
UPDATE traffic_captures SET status = 'error' WHERE status = 'stopped';
ALTER TYPE capture_status RENAME TO capture_status_old;
CREATE TYPE capture_status AS ENUM ('recording', 'rotated', 'downloaded', 'error');
ALTER TABLE traffic_captures ALTER COLUMN status TYPE capture_status USING status::text::capture_status;
DROP TYPE capture_status_old;
