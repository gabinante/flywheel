package risk

import "strings"

// DatabaseClassifier performs static analysis on DDL operations.
//
// Classification rules:
//   - DROP TABLE/DATABASE, TRUNCATE, narrowing ALTER (drop column, reduce size) → destructive
//   - ALTER TABLE with type changes, rename → high
//   - ALTER TABLE additive (add column, add index) → reversible
//   - CREATE TABLE/INDEX/VIEW (additive) → low
//   - SELECT, EXPLAIN, SHOW → safe
//
// The classifier inspects Operation.Action (the DDL statement type) and
// Operation.Details for additional context like column changes.
type DatabaseClassifier struct{}

// ddlRule pairs a pattern with its risk level and description.
// Rules are evaluated in order — more specific patterns must come first.
type ddlRule struct {
	pattern     string
	level       RiskLevel
	rule        string
	description string
}

// ddlRules is the ordered list of DDL classification rules.
// Evaluated top-to-bottom; first match wins. Ordering matters:
// more specific patterns (e.g. "add column") precede generic ones (e.g. "alter table").
var ddlRules = []ddlRule{
	// --- Destructive (highest priority) ---
	{"drop table", RiskDestructive, "db.drop_table", "DDL drops an entire table — irreversible data loss"},
	{"drop database", RiskDestructive, "db.drop_database", "DDL drops an entire database — irreversible data loss"},
	{"drop schema", RiskDestructive, "db.drop_schema", "DDL drops an entire schema — irreversible data loss"},
	{"truncate table", RiskDestructive, "db.truncate_table", "DDL truncates table data — irreversible data loss"},
	{"truncate", RiskDestructive, "db.truncate", "DDL truncates table data — irreversible data loss"},
	{"drop column", RiskDestructive, "db.drop_column", "DDL drops a column — irreversible data loss"},
	{"drop index", RiskDestructive, "db.drop_index", "DDL drops an index — may impact query performance irreversibly"},

	// --- High risk ---
	{"alter column type", RiskHigh, "db.alter_column_type", "DDL changes column type — may cause data truncation or cast errors"},
	{"rename table", RiskHigh, "db.rename_table", "DDL renames table — breaks references"},
	{"rename column", RiskHigh, "db.rename_column", "DDL renames column — breaks references"},
	{"alter column rename", RiskHigh, "db.alter_column_rename", "DDL renames column — breaks references"},
	{"alter type", RiskHigh, "db.alter_type", "DDL alters a type — may break existing data"},

	// --- Reversible (specific patterns before generic "alter table") ---
	{"add column", RiskReversible, "db.add_column", "DDL adds a column — reversible with DROP COLUMN"},
	{"add index", RiskReversible, "db.add_index", "DDL adds an index — reversible with DROP INDEX"},
	{"add constraint", RiskReversible, "db.add_constraint", "DDL adds a constraint — reversible with DROP CONSTRAINT"},
	{"create index", RiskReversible, "db.create_index", "DDL creates an index — reversible with DROP INDEX"},
	{"alter table", RiskReversible, "db.alter_table", "DDL alters table structure — review specific changes"},

	// --- Low risk (additive) ---
	{"create table", RiskLow, "db.create_table", "DDL creates a new table — additive, no existing data affected"},
	{"create view", RiskLow, "db.create_view", "DDL creates a new view — additive, no existing data affected"},
	{"create schema", RiskLow, "db.create_schema", "DDL creates a new schema — additive"},
	{"create database", RiskLow, "db.create_database", "DDL creates a new database — additive"},
	{"create extension", RiskLow, "db.create_extension", "DDL creates a new extension — additive"},
	{"create function", RiskLow, "db.create_function", "DDL creates a new function — additive"},
	{"create trigger", RiskLow, "db.create_trigger", "DDL creates a new trigger — additive"},
	{"create sequence", RiskLow, "db.create_sequence", "DDL creates a new sequence — additive"},

	// --- Safe (read-only) ---
	{"select", RiskSafe, "db.select", "Read-only query — no data modification"},
	{"explain", RiskSafe, "db.explain", "Query plan analysis — no data modification"},
	{"show", RiskSafe, "db.show", "Metadata query — no data modification"},
	{"describe", RiskSafe, "db.describe", "Table description — no data modification"},
	{"analyze", RiskSafe, "db.analyze", "Statistics collection — no data modification"},
}

// Classify classifies a database operation by static analysis of the DDL action.
func (c *DatabaseClassifier) Classify(op Operation) (RiskLevel, []RuleCitation) {
	action := strings.ToLower(strings.TrimSpace(op.Action))

	// Check for narrowing operations in details (e.g. column size reduction).
	if isNarrowingAlteration(op.Details) {
		return RiskDestructive, []RuleCitation{{
			Rule:        "db.narrowing_alteration",
			Backend:     BackendDatabase,
			Level:       RiskDestructive,
			Description: "DDL narrows column type/size — may cause data truncation",
		}}
	}

	// Evaluate rules in order — first match wins.
	for _, r := range ddlRules {
		if strings.Contains(action, r.pattern) {
			return r.level, []RuleCitation{{
				Rule:        r.rule,
				Backend:     BackendDatabase,
				Level:       r.level,
				Description: r.description,
			}}
		}
	}

	// Unknown database operation → maximum risk.
	return RiskDestructive, []RuleCitation{{
		Rule:        "db.unknown_operation",
		Backend:     BackendDatabase,
		Level:       RiskDestructive,
		Description: "Unknown database operation '" + op.Action + "' — defaults to maximum risk",
	}}
}

// isNarrowingAlteration checks Details for signs of column narrowing.
// Details keys: "old_type", "new_type", "old_size", "new_size".
func isNarrowingAlteration(details map[string]any) bool {
	if details == nil {
		return false
	}
	oldSize, oldOK := toInt(details["old_size"])
	newSize, newOK := toInt(details["new_size"])
	if oldOK && newOK && newSize < oldSize {
		return true
	}
	// Check "narrowing" flag set by external lint tools (squawk/skeema-lint).
	if narrowing, ok := details["narrowing"].(bool); ok && narrowing {
		return true
	}
	return false
}

// toInt converts a value to int, handling float64 (from JSON) and int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
