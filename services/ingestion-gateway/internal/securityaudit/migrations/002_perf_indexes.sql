-- psql $DATABASE_URL -f 002_perf_indexes.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_security_audit_category_occurred_at
  ON security_audit(category, occurred_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_security_audit_source_occurred_at
  ON security_audit(source, occurred_at DESC);
