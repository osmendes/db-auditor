-- PF-06: the findings queue (GET /api/v1/findings) orders one environment by
-- severity rank, confidence, last_seen_at and id. Without this index every
-- page sorts all of the environment's findings (top-N or external merge).
-- The expressions match listFindings in internal/repository/findings.go.
-- Single statement on purpose: CONCURRENTLY runs outside a transaction and
-- does not block writes to finding while the index is built.
CREATE INDEX CONCURRENTLY IF NOT EXISTS finding_queue_order_idx ON finding (
  environment_id,
  (CASE severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END),
  confidence DESC,
  last_seen_at DESC,
  (id::text) DESC
);
