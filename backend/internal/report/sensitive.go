package report

const redactionMask = "[oculto]"

// RedactSensitive removes fields from the rendered copy only. Source snapshots,
// job provenance, counts and artifact hash remain in the control database.
// The section helpers below do the same work one section at a time for the
// streaming worker; this method applies all of them to a whole document.
func (d *Document) RedactSensitive(level string) {
	databases := len(d.Databases)
	d.Databases = redactDatabases(level, d.Databases)
	d.Tables = redactTables(level, d.Tables)
	d.Findings = redactFindings(level, d.Findings)
	d.redactOverview(level, databases)
}

func redactsSensitive(level string) bool { return level == "identifiers" || level == "strict" }

// redactOverview redacts the fields outside the database, table and finding
// sections. databases is the database count before redaction: the strict
// copy keeps it as the total when no total was loaded.
func (d *Document) redactOverview(level string, databases int) {
	if !redactsSensitive(level) {
		d.RedactionLevel = "none"
		return
	}
	d.RedactionLevel = level
	d.Environment, d.RequestedBy = redactionMask, redactionMask
	d.DatabaseFilter, d.SchemaFilter, d.TableFilter = redactionMask, redactionMask, redactionMask
	for i := range d.CoverageNotes {
		d.CoverageNotes[i] = "Detalhes de cobertura omitidos neste nível de redação."
	}
	for i := range d.Regressions {
		d.Regressions[i].Object = redactionMask
	}
	if level == "strict" {
		if d.TotalDatabases == 0 {
			d.TotalDatabases = databases
		}
		// The strict copy keeps counts and provenance, but removes object lists,
		// storage evolution and baseline details that could reveal topology.
		d.Growth = nil
		d.Baseline = nil
		d.Regressions = nil
	}
}

func redactDatabases(level string, databases []Database) []Database {
	if !redactsSensitive(level) {
		return databases
	}
	if level == "strict" {
		return nil
	}
	for i := range databases {
		databases[i].Name = redactionMask
	}
	return databases
}

func redactTables(level string, tables []Table) []Table {
	if !redactsSensitive(level) {
		return tables
	}
	if level == "strict" {
		return nil
	}
	for i := range tables {
		tables[i].Database, tables[i].Schema, tables[i].Name = redactionMask, redactionMask, redactionMask
	}
	return tables
}

func redactFindings(level string, findings []Finding) []Finding {
	if !redactsSensitive(level) {
		return findings
	}
	for i := range findings {
		findings[i].Database, findings[i].Schema, findings[i].Object = redactionMask, redactionMask, redactionMask
		findings[i].Evidence = "[evidência omitida]"
		findings[i].Title = redactionMask
		findings[i].Summary = "[resumo omitido]"
		findings[i].Recommendation = "[texto original omitido]"
	}
	return findings
}
