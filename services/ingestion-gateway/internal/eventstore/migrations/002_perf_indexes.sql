-- Perf indexes for raw-speed track. Run offline with CONCURRENTLY to avoid locking
-- live tables. App startup uses plain CREATE INDEX IF NOT EXISTS (fast on empty
-- tables); use this file for existing large tables.
--
-- psql $DATABASE_URL -f 002_perf_indexes.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_webhook_events_source_received_at
  ON webhook_events(source, received_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_webhook_events_provider_event_id
  ON webhook_events(provider_event_id) WHERE provider_event_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_webhook_events_body_hash;
-- Optional BRIN for body_hash analytics (async, tiny index):
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_webhook_events_body_hash_brin
--   ON webhook_events USING BRIN (body_hash);
