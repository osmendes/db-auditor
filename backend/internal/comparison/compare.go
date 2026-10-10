package comparison

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/fingerprint"
)

// ObjectItem is a generic comparable inventory object.
type ObjectItem struct {
	ObjectType  string            `json:"object_type"`
	Key         string            `json:"key"` // e.g. schema.table or database
	Name        string            `json:"name"`
	Fingerprint string            `json:"fingerprint,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
}

// CompareSets diffs two sets of objects keyed by ObjectType+Key.
func CompareSets(source, target []ObjectItem) Result {
	srcMap := index(source)
	tgtMap := index(target)
	allKeys := make(map[string]struct{})
	for k := range srcMap {
		allKeys[k] = struct{}{}
	}
	for k := range tgtMap {
		allKeys[k] = struct{}{}
	}
	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	objects := make([]ObjectDiff, 0, len(keys))
	for _, k := range keys {
		s, sok := srcMap[k]
		t, tok := tgtMap[k]
		switch {
		case sok && !tok:
			objects = append(objects, ObjectDiff{
				ObjectType: s.ObjectType, ObjectKey: s.Key, Status: StatusOnlySource,
				SourceName: s.Name, SourceFP: s.Fingerprint,
			})
		case !sok && tok:
			objects = append(objects, ObjectDiff{
				ObjectType: t.ObjectType, ObjectKey: t.Key, Status: StatusOnlyTarget,
				TargetName: t.Name, TargetFP: t.Fingerprint,
			})
		default:
			objects = append(objects, classifyPair(s, t))
		}
	}
	return Result{Summary: summarize(objects), Objects: objects}
}

func index(items []ObjectItem) map[string]ObjectItem {
	m := make(map[string]ObjectItem, len(items))
	for _, it := range items {
		k := compositeKey(it.ObjectType, it.Key)
		m[k] = it
	}
	return m
}

func compositeKey(objectType, key string) string {
	return strings.ToLower(objectType) + "|" + fingerprint.NormalizeName(key)
}

func classifyPair(s, t ObjectItem) ObjectDiff {
	d := ObjectDiff{
		ObjectType: s.ObjectType, ObjectKey: s.Key,
		SourceName: s.Name, TargetName: t.Name,
		SourceFP: s.Fingerprint, TargetFP: t.Fingerprint,
	}
	if s.Fingerprint != "" && t.Fingerprint != "" {
		if s.Fingerprint == t.Fingerprint {
			d.Status = StatusMatch
			return d
		}
		d.Status = StatusDrift
		d.FieldDiffs = append(d.FieldDiffs, FieldDiff{Field: "fingerprint", Source: s.Fingerprint, Target: t.Fingerprint})
		return d
	}
	if s.Fingerprint == "" && t.Fingerprint == "" && len(s.Fields) == 0 && len(t.Fields) == 0 {
		// name-only presence on both sides without structural data
		d.Status = StatusUnknown
		return d
	}
	diffs := diffFields(s.Fields, t.Fields)
	if len(diffs) == 0 {
		d.Status = StatusMatch
		return d
	}
	d.Status = StatusDrift
	d.FieldDiffs = diffs
	return d
}

func diffFields(a, b map[string]string) []FieldDiff {
	if a == nil {
		a = map[string]string{}
	}
	if b == nil {
		b = map[string]string{}
	}
	keys := make(map[string]struct{})
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	out := make([]FieldDiff, 0)
	for k := range keys {
		av, aok := a[k]
		bv, bok := b[k]
		if !aok {
			out = append(out, FieldDiff{Field: k, Target: bv})
			continue
		}
		if !bok {
			out = append(out, FieldDiff{Field: k, Source: av})
			continue
		}
		if av != bv {
			out = append(out, FieldDiff{Field: k, Source: av, Target: bv})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// FilterByStatus returns objects matching any of the given statuses (empty = all).
func FilterByStatus(objects []ObjectDiff, statuses []string) []ObjectDiff {
	if len(statuses) == 0 {
		return objects
	}
	allow := make(map[string]struct{}, len(statuses))
	for _, s := range statuses {
		allow[strings.ToUpper(strings.TrimSpace(s))] = struct{}{}
	}
	out := make([]ObjectDiff, 0)
	for _, o := range objects {
		if _, ok := allow[string(o.Status)]; ok {
			out = append(out, o)
		}
	}
	return out
}

// FilterByObjectType filters by object_type (empty = all).
func FilterByObjectType(objects []ObjectDiff, objectType string) []ObjectDiff {
	if strings.TrimSpace(objectType) == "" {
		return objects
	}
	want := strings.ToLower(objectType)
	out := make([]ObjectDiff, 0)
	for _, o := range objects {
		if strings.ToLower(o.ObjectType) == want {
			out = append(out, o)
		}
	}
	return out
}

// KeyFor builds a stable object key from parts.
func KeyFor(parts ...string) string {
	norm := make([]string, 0, len(parts))
	for _, p := range parts {
		p = fingerprint.NormalizeName(p)
		if p != "" {
			norm = append(norm, p)
		}
	}
	return strings.Join(norm, ".")
}

// StringField formats a value for field maps.
func StringField(v any) string {
	return fmt.Sprint(v)
}
