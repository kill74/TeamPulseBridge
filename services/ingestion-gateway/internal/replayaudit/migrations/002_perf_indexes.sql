-- psql $DATABASE_URL -f 002_perf_indexes.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_replay_audit_actor_replayed_at
  ON replay_audit(actor, replayed_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_replay_audit_result_replayed_at
  ON replay_audit(result, replayed_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_replay_audit_event_id
  ON replay_audit(event_id);
