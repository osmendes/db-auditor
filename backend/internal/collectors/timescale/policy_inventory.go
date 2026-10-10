package timescale

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// PolicyInventoryResult holds CAGG/jobs/policies for one database.
type PolicyInventoryResult struct {
	Status               CollectionStatus           `json:"status"`
	Message              string                     `json:"message,omitempty"`
	ContinuousAggregates []ContinuousAggregateFacts `json:"continuous_aggregates,omitempty"`
	Jobs                 []JobFacts                 `json:"jobs,omitempty"`
	Policies             []PolicyFacts              `json:"policies,omitempty"`
}

// CollectPolicyInventory collects CAGGs, jobs and policies when Timescale is present and compatible.
func CollectPolicyInventory(ctx context.Context, conn *pgx.Conn, scope config.Scope) (PolicyInventoryResult, error) {
	version, err := CollectVersion(ctx, conn)
	if err != nil {
		return PolicyInventoryResult{Status: StatusError, Message: err.Error()}, err
	}
	if version == nil {
		return PolicyInventoryResult{
			Status:  StatusSkippedMissing,
			Message: "timescaledb extension not installed",
		}, nil
	}
	if !version.Compatible {
		return PolicyInventoryResult{
			Status:  StatusSkippedUnsupported,
			Message: version.CompatibilityNote,
		}, nil
	}

	caggs, err := CollectContinuousAggregates(ctx, conn, scope)
	if err != nil {
		return PolicyInventoryResult{Status: StatusError, Message: err.Error()}, err
	}
	jobs, err := CollectJobs(ctx, conn, scope)
	if err != nil {
		return PolicyInventoryResult{Status: StatusError, Message: err.Error()}, err
	}
	policies, err := CollectPolicies(ctx, conn, scope)
	if err != nil {
		return PolicyInventoryResult{Status: StatusError, Message: err.Error()}, err
	}

	return PolicyInventoryResult{
		Status:               StatusOK,
		ContinuousAggregates: caggs,
		Jobs:                 jobs,
		Policies:             policies,
	}, nil
}
