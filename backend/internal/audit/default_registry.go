package audit

import "github.com/mayconmendes-qc/db-auditor/internal/config"

// NewDefaultRegistry builds the production collector set (live SQL against target DSNs).
// Without AUDITOR_TARGET_DSN_<uuid>, collectors fail with a clear configuration error
// instead of reporting success with 0 rows.
func NewDefaultRegistry() *Registry {
	opts := LiveRegistryOptions{
		Targets: config.LoadTargetDSNs(),
		Scope: config.Scope{
			DatabaseDenylist: []string{"template0", "template1", "postgres"},
			SchemaDenylist:   []string{"pg_catalog", "information_schema"},
		},
		Policy: config.CollectionPolicy{ColumnStatsEnabled: true, ColumnStatsLimit: 5000, WorkloadEnabled: true, WorkloadLimit: 500},
	}
	r := NewLiveRegistry(opts)
	AttachStructuralCollectors(r, opts)
	return r
}
