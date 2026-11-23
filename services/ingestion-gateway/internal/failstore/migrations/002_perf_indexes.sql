-- psql $DATABASE_URL -f 002_perf_indexes.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_failed_events_source_failed_at
  ON failed_events(source, failed_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_failed_events_reason_failed_at
  ON failed_events(reason, failed_at DESC);
