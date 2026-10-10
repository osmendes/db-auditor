package analyzer

// Registry holds named analyzers.
type Registry struct {
	items []Analyzer
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{items: make([]Analyzer, 0)}
}

// Register appends an analyzer.
func (r *Registry) Register(a Analyzer) {
	r.items = append(r.items, a)
}

// All returns registered analyzers in registration order.
func (r *Registry) All() []Analyzer {
	out := make([]Analyzer, len(r.items))
	copy(out, r.items)
	return out
}

// DefaultRegistry returns Sprint 6–9 analyzers.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(StorageAnalyzer{})
	r.Register(IndexAnalyzer{})
	r.Register(StructuralAnalyzer{})
	r.Register(ChunkAnalyzer{})
	r.Register(CAGGAnalyzer{})
	r.Register(PolicyAnalyzer{})
	r.Register(InactivityAnalyzer{})
	r.Register(VacuumAnalyzer{})
	r.Register(PerformanceAnalyzer{})
	r.Register(SecurityAnalyzer{})
	r.Register(DriftAnalyzer{})
	r.Register(P2Analyzer{})
	r.Register(MaintenanceTrendAnalyzer{})
	return r
}
