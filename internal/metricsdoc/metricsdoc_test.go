package metricsdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintAcceptsWellFormedDocument(t *testing.T) {
	content := `# Metrics

## Metrics ledger

| window | realized vs MTM | artifact (commit) | costs | date |
|---|---|---|---|---|
| 2025-04-01 -> 2026-09-17 (6 quarters) | realized +110742; MTM not recorded | abs-10d walk-forward recipe (AGENTS.md) | commission 0.05% | 2026-09-16 |
| 2026-06-19 -> 2026-09-17 (90d preflight) | MTM +79453; realized not recorded | ensemble_model.json @ 722353e | zero | 2026-09-17 |
| not recorded | +75028; realized vs MTM not recorded | not recorded | not recorded | not recorded |
| not recorded | +63212; realized vs MTM not recorded | not recorded | not recorded | not recorded |
| 2026-06-19 -> 2026-09-17 (holdout) | realized +56379; MTM not recorded | news-aware ensemble_model.json @ 277ebf7 | commission 0.05% | 2026-09-17 |
| 2026-06-19 -> 2026-09-17 (holdout) | realized +40102; MTM not recorded | no-news control @ 277ebf7 | commission 0.05% | 2026-09-17 |
| 2026-06-19 -> 2026-09-17 (90d preflight) | MTM +25699; realized not recorded | ensemble_model.json @ 277ebf7 | zero | 2026-09-17 |
`
	if issues := Lint(content); len(issues) != 0 {
		t.Fatalf("Lint() issues = %v, want none", issues)
	}
}

func TestLintDetectsMissingSection(t *testing.T) {
	content := "# Metrics\n\nNo ledger here.\n"
	issues := Lint(content)
	if len(issues) == 0 {
		t.Fatal("Lint() returned no issues for a document without the ledger section")
	}
	if !contains(issues, `missing "## Metrics ledger" section`) {
		t.Fatalf("Lint() issues = %v, want missing-section issue", issues)
	}
}

func TestLintDetectsMissingNumber(t *testing.T) {
	content := `## Metrics ledger

| window | realized vs MTM | artifact (commit) | costs | date |
|---|---|---|---|---|
| 2025-04-01 -> 2026-09-17 (6 quarters) | realized +110742; MTM not recorded | abs-10d walk-forward recipe (AGENTS.md) | commission 0.05% | 2026-09-16 |
| 2026-06-19 -> 2026-09-17 (90d preflight) | MTM +79453; realized not recorded | ensemble_model.json @ 722353e | zero | 2026-09-17 |
| not recorded | +75028; realized vs MTM not recorded | not recorded | not recorded | not recorded |
| not recorded | +63212; realized vs MTM not recorded | not recorded | not recorded | not recorded |
| 2026-06-19 -> 2026-09-17 (holdout) | realized +56379; MTM not recorded | news-aware ensemble_model.json @ 277ebf7 | commission 0.05% | 2026-09-17 |
| 2026-06-19 -> 2026-09-17 (holdout) | realized +40102; MTM not recorded | no-news control @ 277ebf7 | commission 0.05% | 2026-09-17 |
| 2026-06-19 -> 2026-09-17 (90d preflight) | MTM not recorded; realized not recorded | ensemble_model.json @ 277ebf7 | zero | 2026-09-17 |
`
	issues := Lint(content)
	if !contains(issues, "ledger is missing required number 25699") {
		t.Fatalf("Lint() issues = %v, want missing-number issue for 25699", issues)
	}
}

func TestLintDetectsMalformedRow(t *testing.T) {
	content := `## Metrics ledger

| window | realized vs MTM | artifact (commit) | costs | date |
|---|---|---|---|---|
| 2025-04-01 -> 2026-09-17 | realized +110742 | short row |
| not recorded | +75028 | not recorded | not recorded | not recorded |
| not recorded | +63212 | not recorded | not recorded | not recorded |
| not recorded | +56379 | not recorded | not recorded | not recorded |
| not recorded | +40102 | not recorded | not recorded | not recorded |
| not recorded | +79453 | not recorded | not recorded | not recorded |
| not recorded | +25699 | not recorded | not recorded | not recorded |
`
	issues := Lint(content)
	if !contains(issues, "ledger row has 3 columns, want 5: | 2025-04-01 -> 2026-09-17 | realized +110742 | short row |") {
		t.Fatalf("Lint() issues = %v, want malformed-row issue", issues)
	}
}

func TestLintDetectsWrongHeaderOrder(t *testing.T) {
	content := `## Metrics ledger

| date | costs | artifact (commit) | realized vs MTM | window |
|---|---|---|---|---|
| 2026-09-16 | commission 0.05% | recipe | realized +110742 | 6 quarters |
| 2026-09-17 | zero | model @ 722353e | MTM +79453 | 90d preflight |
| 2026-09-17 | zero | model @ 277ebf7 | MTM +25699 | 90d preflight |
| 2026-09-17 | costs | control @ 277ebf7 | realized +56379 | holdout |
| 2026-09-17 | costs | control @ 277ebf7 | realized +40102 | holdout |
| 2026-09-17 | costs | not recorded | +75028 | not recorded |
| 2026-09-17 | costs | not recorded | +63212 | not recorded |
`
	issues := Lint(content)
	if !contains(issues, `ledger header column 1 = "date", want "window"`) {
		t.Fatalf("Lint() issues = %v, want wrong-header-order issue", issues)
	}
}

func TestRepositoryMetricsDocumentPassesLint(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "metrics.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if issues := Lint(string(content)); len(issues) != 0 {
		t.Fatalf("Lint(%s) issues = %v, want none", path, issues)
	}
}

func contains(issues []string, want string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, want) {
			return true
		}
	}
	return false
}
