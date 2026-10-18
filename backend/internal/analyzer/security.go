package analyzer

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// SecurityAnalyzer flags SECURITY DEFINER functions and broad privileges for human review.
// It never recommends REVOKE or DROP automatically.
type SecurityAnalyzer struct{}

func (SecurityAnalyzer) Name() string { return "security" }

// Privileges considered broadly powerful when granted to PUBLIC or non-owner roles.
var excessivePrivileges = map[string]bool{
	"ALL":        true,
	"SUPERUSER":  true,
	"CREATE":     true,
	"TRUNCATE":   true,
	"REFERENCES": true,
	"TRIGGER":    true,
	"EXECUTE":    true, // on sensitive functions still review
	"INSERT":     true,
	"UPDATE":     true,
	"DELETE":     true,
}

func (SecurityAnalyzer) Analyze(_ context.Context, facts SnapshotFacts) ([]Finding, error) {
	out := make([]Finding, 0)

	// SECURITY DEFINER functions — list for review only
	for _, fn := range facts.Functions {
		if !fn.IsSecurityDefiner {
			continue
		}
		key := fmt.Sprintf("%s.%s.%s", fn.Database, fn.Schema, fn.FunctionName)
		if fn.IdentityArgs != "" {
			key = key + "(" + fn.IdentityArgs + ")"
		}
		title := fmt.Sprintf("SECURITY DEFINER function: %s", key)
		f := Finding{
			EnvironmentID: facts.EnvironmentID,
			AuditRunID:    facts.AuditRunID,
			FindingType:   "security.security_definer",
			Severity:      SeverityMedium,
			Status:        StatusOpen,
			Title:         title,
			Summary:       "Function runs with owner privileges (SECURITY DEFINER). Confirm search_path and privilege need; no automatic change is applied.",
			ObjectType:    "function",
			ObjectKey:     key,
			DatabaseName:  fn.Database,
			SchemaName:    fn.Schema,
			ObjectName:    fn.FunctionName,
			Evidence: map[string]any{
				"is_security_definer": true,
				"owner":               fn.Owner,
				"language":            fn.Language,
				"identity_arguments":  fn.IdentityArgs,
				"safety_note":         "Review only — never ALTER or DROP automatically",
			},
			DedupKey: DedupKey("security.security_definer", key, title),
		}
		out = append(out, f)
	}

	for _, role := range facts.Roles {
		if !role.Current {
			continue
		}
		if role.PrivilegeCheckFailed {
			key := fmt.Sprintf("%s.auditor:%s", role.Database, role.RoleName)
			title := "Auditor privilege check incomplete: " + role.RoleName
			out = append(out, Finding{
				EnvironmentID: facts.EnvironmentID, AuditRunID: facts.AuditRunID,
				FindingType: "security.auditor_privilege_unknown", Severity: SeverityMedium, Status: StatusOpen,
				Title: title, Summary: "The privilege query failed. This is not evidence that the role is read-only.",
				ObjectType: "role", ObjectKey: key, DatabaseName: role.Database, ObjectName: role.RoleName,
				DedupKey: DedupKey("security.auditor_privilege_unknown", key, title),
			})
			continue
		}
		if role.CanWrite || role.Superuser || role.CreateRole || role.CreateDB {
			key := fmt.Sprintf("%s.auditor:%s", role.Database, role.RoleName)
			title := "Auditor connection is not read-only: " + role.RoleName
			out = append(out, Finding{
				EnvironmentID: facts.EnvironmentID, AuditRunID: facts.AuditRunID,
				FindingType: "security.auditor_not_readonly", Severity: SeverityHigh, Status: StatusOpen,
				Title: title, Summary: "The target role used by the auditor can write or holds a privileged attribute. Collection stays SELECT-only.",
				ObjectType: "role", ObjectKey: key, DatabaseName: role.Database, ObjectName: role.RoleName,
				Evidence: map[string]any{"superuser": role.Superuser, "can_write": role.CanWrite, "createrole": role.CreateRole},
				DedupKey: DedupKey("security.auditor_not_readonly", key, title),
			})
		}
	}

	// Superuser / powerful roles. Every login role carries its attributes
	// (#126); the auditor's own role also comes from the privilege check, so
	// one finding per database and role merges both sources.
	powerful := map[string]int{}
	for _, role := range facts.Roles {
		if !role.Current && role.Login {
			expired := role.ValidUntil != nil && !role.CollectedAt.IsZero() && role.ValidUntil.Before(role.CollectedAt)
			if expired || role.PossiblyInactive {
				reason := "Nenhuma sessão ativa foi vista nas coletas disponíveis por pelo menos 30 dias; a amostragem não comprova ausência de uso entre coletas."
				if expired {
					reason = "A validade da conta terminou; confirme se ela deve permanecer cadastrada."
				}
				key := fmt.Sprintf("%s.role:%s", role.Database, role.RoleName)
				out = append(out, Finding{EnvironmentID: facts.EnvironmentID, AuditRunID: facts.AuditRunID,
					FindingType: "security.account_inactive_review", Severity: SeverityLow, Status: StatusOpen,
					Title:      "Revisar conta possivelmente inativa: " + role.RoleName,
					Summary:    reason + " Consulte a equipe responsável antes de desativar qualquer acesso.",
					ObjectType: "role", ObjectKey: key, DatabaseName: role.Database, ObjectName: role.RoleName,
					Evidence: map[string]any{"valid_until": role.ValidUntil, "sampled_active": role.SampledActive, "sampled_inactive_window": role.PossiblyInactive, "expired": expired},
					DedupKey: DedupKey("security.account_inactive_review", key, "review"),
				})
			}
		}
		if !role.Superuser && !role.BypassRLS && !role.Replication && !role.CreateRole && len(role.Inherited) == 0 {
			continue
		}
		key := fmt.Sprintf("%s.role:%s", role.Database, role.RoleName)
		if at, ok := powerful[key]; ok {
			mergePowerfulRole(&out[at], role, facts.ServerVersion)
			continue
		}
		powerful[key] = len(out)
		title := fmt.Sprintf("Powerful role: %s", role.RoleName)
		f := Finding{
			EnvironmentID: facts.EnvironmentID,
			AuditRunID:    facts.AuditRunID,
			FindingType:   "security.powerful_role",
			Status:        StatusOpen,
			Title:         title,
			Summary:       "Role has elevated attributes (superuser, bypass RLS, replication or CREATEROLE), directly or through membership in a role that has them or in a predefined role that reaches server files, programs or every table. Confirm least-privilege need.",
			ObjectType:    "role",
			ObjectKey:     key,
			DatabaseName:  role.Database,
			ObjectName:    role.RoleName,
			Evidence: map[string]any{
				"superuser":   false,
				"bypass_rls":  false,
				"replication": false,
				"createrole":  false,
				"login":       false,
				"safety_note": "Never ALTER ROLE or REVOKE automatically",
			},
			DedupKey: DedupKey("security.powerful_role", key, title),
		}
		mergePowerfulRole(&f, role, facts.ServerVersion)
		out = append(out, f)
	}

	// Excessive grants (PUBLIC or ALL on application objects)
	for _, g := range facts.Grants {
		priv := strings.ToUpper(strings.TrimSpace(g.Privilege))
		grantee := strings.ToUpper(strings.TrimSpace(g.Grantee))
		isPublic := grantee == "PUBLIC" || grantee == ""
		isExcessive := excessivePrivileges[priv] || priv == "ALL"
		if !isPublic && !isExcessive {
			continue
		}
		// Skip routine SELECT-only grants to specific roles
		if !isPublic && priv == "SELECT" {
			continue
		}
		key := fmt.Sprintf("%s.%s.%s:%s:%s", g.Database, g.Schema, g.ObjectName, g.Grantee, priv)
		title := fmt.Sprintf("Privilege review: %s on %s to %s", priv, g.ObjectName, g.Grantee)
		sev := SeverityLow
		if isPublic && (priv == "ALL" || priv == "INSERT" || priv == "UPDATE" || priv == "DELETE") {
			sev = SeverityMedium
		}
		f := Finding{
			EnvironmentID: facts.EnvironmentID,
			AuditRunID:    facts.AuditRunID,
			FindingType:   "security.excessive_privilege",
			Severity:      sev,
			Status:        StatusOpen,
			Title:         title,
			Summary:       "Broad privilege detected for review. Confirm business need; auditor never issues REVOKE.",
			ObjectType:    g.ObjectType,
			ObjectKey:     key,
			DatabaseName:  g.Database,
			SchemaName:    g.Schema,
			ObjectName:    g.ObjectName,
			Evidence: map[string]any{
				"grantee":     g.Grantee,
				"privilege":   priv,
				"grantable":   g.Grantable,
				"object_type": g.ObjectType,
				"is_public":   isPublic,
				"safety_note": "Review only — never REVOKE automatically",
			},
			DedupKey: DedupKey("security.excessive_privilege", key, title),
		}
		out = append(out, f)
	}

	return out, nil
}

// powerfulPrivilegeHigh lists inherited privileges that rate the role high.
// CREATEROLE alone (direct or inherited) is medium on PostgreSQL 16+, where it
// only administers the roles it creates; before 16 it grants itself any
// non-superuser role (pg_execute_server_program included), so it is high
// there (see createRoleGrantsAnyRole).
var powerfulPrivilegeHigh = map[string]bool{
	"superuser": true, "bypass_rls": true, "replication": true,
	"pg_execute_server_program": true, "pg_read_server_files": true, "pg_write_server_files": true,
	"pg_read_all_data": true, "pg_write_all_data": true,
}

// createRoleGrantsAnyRole reports whether serverVersion (the collected
// server_version, such as "15.8" or "16.4 (Debian ...)") is older than
// PostgreSQL 16. An unknown version is not assumed old.
func createRoleGrantsAnyRole(serverVersion string) bool {
	digits := strings.TrimLeftFunc(serverVersion, unicode.IsSpace)
	end := strings.IndexFunc(digits, func(r rune) bool { return !unicode.IsDigit(r) })
	if end >= 0 {
		digits = digits[:end]
	}
	major, err := strconv.Atoi(digits)
	return err == nil && major > 0 && major < 16
}

// mergePowerfulRole folds one source of a role's power into its finding:
// own attributes, and inherited ones with the role they come from (#126).
func mergePowerfulRole(f *Finding, role RoleFact, serverVersion string) {
	evidence := f.Evidence
	for name, set := range map[string]bool{"superuser": role.Superuser, "bypass_rls": role.BypassRLS, "replication": role.Replication, "createrole": role.CreateRole, "login": role.Login} {
		if set {
			evidence[name] = true
		}
	}
	inherited, _ := evidence["inherited"].([]map[string]string)
	seen := map[string]bool{}
	for _, item := range inherited {
		seen[item["privilege"]+":"+item["source"]] = true
	}
	for _, entry := range role.Inherited {
		privilege, source, ok := strings.Cut(entry, ":")
		if !ok || seen[entry] {
			continue
		}
		seen[entry] = true
		inherited = append(inherited, map[string]string{"privilege": privilege, "source": source})
	}
	sort.Slice(inherited, func(i, j int) bool {
		if inherited[i]["privilege"] != inherited[j]["privilege"] {
			return inherited[i]["privilege"] < inherited[j]["privilege"]
		}
		return inherited[i]["source"] < inherited[j]["source"]
	})
	sources := []string{}
	high := evidence["superuser"] == true || evidence["bypass_rls"] == true || evidence["replication"] == true
	for _, item := range inherited {
		if !slices.Contains(sources, item["source"]) {
			sources = append(sources, item["source"])
		}
		high = high || powerfulPrivilegeHigh[item["privilege"]]
	}
	sort.Strings(sources)
	if createRoleGrantsAnyRole(serverVersion) && (evidence["createrole"] == true || hasPrivilege(inherited, "createrole")) {
		evidence["createrole_grants_any_role"] = true
		high = true
	}
	if len(inherited) > 0 {
		evidence["inherited"] = inherited
		evidence["source_roles"] = sources
	}
	f.Severity = SeverityMedium
	if high {
		f.Severity = SeverityHigh
	}
}

func hasPrivilege(inherited []map[string]string, privilege string) bool {
	for _, item := range inherited {
		if item["privilege"] == privilege {
			return true
		}
	}
	return false
}
