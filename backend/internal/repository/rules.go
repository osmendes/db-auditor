package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
	"github.com/mayconmendes-qc/db-auditor/internal/capabilities"
)

// EnsureRuleCatalog only inserts new immutable versions. Existing catalog
// rows are never overwritten when code or policies evolve.
func (s *Store) EnsureRuleCatalog(ctx context.Context) error {
	for _, rule := range analyzer.Catalog() {
		refs, _ := json.Marshal(rule.References)
		params, _ := json.Marshal(rule.DefaultParameters)
		_, err := s.pool.Exec(ctx, `INSERT INTO rule_catalog
(rule_id,rule_version,category,confidence,impact,risk,recommendation,validation,references_json,default_parameters)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb)
ON CONFLICT (rule_id,rule_version) DO NOTHING`, rule.ID, rule.Version, rule.Category, rule.Confidence,
			rule.Impact, rule.Risk, rule.Recommendation, rule.Validation, refs, params)
		if err != nil {
			return fmt.Errorf("seed rule %s: %w", rule.ID, err)
		}
		var category, impact, risk, recommendation, validation string
		var confidence float64
		var storedRefs, storedParams []byte
		err = s.pool.QueryRow(ctx, `SELECT category,confidence,impact,risk,recommendation,validation,references_json,default_parameters
FROM rule_catalog WHERE rule_id=$1 AND rule_version=$2`, rule.ID, rule.Version).Scan(
			&category, &confidence, &impact, &risk, &recommendation, &validation, &storedRefs, &storedParams)
		if err != nil {
			return fmt.Errorf("read rule %s: %w", rule.ID, err)
		}
		if category != rule.Category || confidence != rule.Confidence || impact != rule.Impact || risk != rule.Risk || recommendation != rule.Recommendation || validation != rule.Validation || !sameJSON(storedRefs, refs) || !sameJSON(storedParams, params) {
			return fmt.Errorf("rule %s version %s changed; create a new version instead", rule.ID, rule.Version)
		}
	}
	return nil
}

func sameJSON(a, b []byte) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

func (s *Store) ListRulePolicies(ctx context.Context, environmentID string) ([]analyzer.RulePolicy, error) {
	rows, err := s.pool.Query(ctx, `SELECT environment_id::text,schema_name,rule_id,enabled,parameters
FROM rule_policy WHERE environment_id=$1::uuid ORDER BY schema_name,rule_id`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []analyzer.RulePolicy{}
	for rows.Next() {
		var p analyzer.RulePolicy
		var raw []byte
		if err := rows.Scan(&p.EnvironmentID, &p.SchemaName, &p.RuleID, &p.Enabled, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &p.Parameters); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) EffectiveRules(ctx context.Context, environmentID, schema string) ([]analyzer.EffectiveRule, error) {
	capabilitiesForEnvironment, err := s.GetEnvironmentCapabilities(ctx, environmentID, false)
	if err != nil {
		return nil, err
	}
	if capabilitiesForEnvironment == nil {
		return nil, pgx.ErrNoRows
	}
	engine := capabilitiesForEnvironment.Engine
	timescaleObserved := false
	for _, item := range capabilitiesForEnvironment.Items {
		if item.Name == "timescale" && item.Applicable {
			timescaleObserved = true
		}
	}
	policies, err := s.ListRulePolicies(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	all := analyzer.EffectiveCatalog(analyzer.SnapshotFacts{EnvironmentID: environmentID, RulePolicies: policies}, schema)
	out := make([]analyzer.EffectiveRule, 0, len(all))
	for _, rule := range all {
		if capabilities.RuleApplicable(engine, rule.ID, timescaleObserved) {
			out = append(out, rule)
		}
	}
	return out, nil
}

func (s *Store) SetRulePolicy(ctx context.Context, p analyzer.RulePolicy) error {
	capabilitiesForEnvironment, err := s.GetEnvironmentCapabilities(ctx, p.EnvironmentID, false)
	if err != nil {
		return err
	}
	if capabilitiesForEnvironment == nil {
		return pgx.ErrNoRows
	}
	engine := capabilitiesForEnvironment.Engine
	timescaleObserved := false
	for _, item := range capabilitiesForEnvironment.Items {
		if item.Name == "timescale" && item.Applicable {
			timescaleObserved = true
		}
	}
	if !capabilities.RuleApplicable(engine, p.RuleID, timescaleObserved) {
		return fmt.Errorf("rule not applicable to %s", engine)
	}
	valid := false
	var defaults map[string]any
	for _, rule := range analyzer.Catalog() {
		if rule.ID == p.RuleID {
			valid = true
			defaults = rule.DefaultParameters
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown rule_id")
	}
	for k, v := range p.Parameters {
		defaultValue, ok := defaults[k]
		if !ok {
			return fmt.Errorf("unknown rule parameter %s", k)
		}
		switch defaultValue.(type) {
		case bool:
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("invalid boolean parameter %s", k)
			}
		case string:
			value, ok := v.(string)
			if !ok || (value != "snake_case" && value != "lowercase" && value != "none") {
				return fmt.Errorf("invalid convention %s", k)
			}
		default:
			n, ok := v.(float64)
			if !ok || n <= 0 || n > 1e12 {
				return fmt.Errorf("invalid numeric parameter %s", k)
			}
			if k == "min_dead_ratio" && n > 1 {
				return fmt.Errorf("invalid ratio %s", k)
			}
		}
	}
	raw, err := json.Marshal(p.Parameters)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO rule_policy(environment_id,schema_name,rule_id,enabled,parameters)
VALUES ($1::uuid,$2,$3,$4,$5::jsonb)
ON CONFLICT(environment_id,schema_name,rule_id) DO UPDATE SET enabled=EXCLUDED.enabled,
parameters=EXCLUDED.parameters,updated_at=now()`, p.EnvironmentID, p.SchemaName, p.RuleID, p.Enabled, raw)
	return err
}
