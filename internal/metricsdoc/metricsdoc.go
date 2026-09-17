package metricsdoc

import (
	"fmt"
	"strings"
)

const ledgerHeader = "## Metrics ledger"

var requiredColumns = []string{"window", "realized vs MTM", "artifact (commit)", "costs", "date"}

// RequiredNumbers are the headline P&L numbers that must always be present in
// the Metrics ledger table. They are migrated historical results; losing one
// silently would break the single-source-of-truth contract.
var RequiredNumbers = []string{"110742", "79453", "75028", "63212", "56379", "40102", "25699"}

// Lint validates the Metrics ledger section of a docs/metrics.md document and
// returns a list of issues. An empty result means the document is well-formed.
func Lint(content string) []string {
	lines := strings.Split(content, "\n")

	headerIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == ledgerHeader {
			headerIdx = i
			break
		}
	}
	if headerIdx == -1 {
		return []string{fmt.Sprintf("missing %q section", ledgerHeader)}
	}

	table := collectTable(lines, headerIdx+1)
	if len(table) < 3 {
		return []string{"ledger table must have a header, separator and at least one data row"}
	}

	var issues []string

	header := splitRow(table[0])
	if len(header) != len(requiredColumns) {
		issues = append(issues, fmt.Sprintf("ledger header has %d columns, want %d: %s", len(header), len(requiredColumns), table[0]))
	} else {
		for i, want := range requiredColumns {
			if header[i] != want {
				issues = append(issues, fmt.Sprintf("ledger header column %d = %q, want %q", i+1, header[i], want))
			}
		}
	}

	if !isSeparatorRow(table[1]) {
		issues = append(issues, "second ledger table row is not a markdown separator")
	}

	var body strings.Builder
	for _, row := range table[2:] {
		cells := splitRow(row)
		if len(header) > 0 && len(cells) != len(header) {
			issues = append(issues, fmt.Sprintf("ledger row has %d columns, want %d: %s", len(cells), len(header), row))
		}
		for i, cell := range cells {
			if strings.TrimSpace(cell) == "" {
				issues = append(issues, fmt.Sprintf("ledger row has an empty cell in column %d: %s", i+1, row))
			}
		}
		body.WriteString(row)
		body.WriteString("\n")
	}

	bodyText := body.String()
	for _, number := range RequiredNumbers {
		if !strings.Contains(bodyText, number) {
			issues = append(issues, fmt.Sprintf("ledger is missing required number %s", number))
		}
	}

	return issues
}

func collectTable(lines []string, start int) []string {
	var table []string
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if len(table) == 0 {
			if strings.HasPrefix(trimmed, "|") {
				table = append(table, trimmed)
			}
			continue
		}
		if trimmed == "" || !strings.HasPrefix(trimmed, "|") {
			break
		}
		table = append(table, trimmed)
	}
	return table
}

func splitRow(row string) []string {
	parts := strings.Split(row, "|")
	var cells []string
	for i, part := range parts {
		cell := strings.TrimSpace(part)
		if cell == "" && (i == 0 || i == len(parts)-1) {
			continue
		}
		cells = append(cells, cell)
	}
	return cells
}

func isSeparatorRow(row string) bool {
	trimmed := strings.TrimSpace(row)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return false
	}
	for _, r := range trimmed {
		switch r {
		case '|', '-', ':', ' ':
		default:
			return false
		}
	}
	// Must contain at least one dash to be a markdown separator.
	return strings.Contains(trimmed, "-")
}
