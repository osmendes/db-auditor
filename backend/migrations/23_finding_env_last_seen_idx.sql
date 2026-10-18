-- PF-06: security/performance pages (ListFindingsCategoryPage) and the
-- run/finding history order one environment by last_seen_at. The global
-- finding_last_seen_idx walks every environment's rows; this one stays inside
-- the environment. Single CONCURRENTLY statement, outside a transaction.
CREATE INDEX CONCURRENTLY IF NOT EXISTS finding_env_last_seen_idx ON finding (environment_id, last_seen_at DESC, (id::text) DESC);
