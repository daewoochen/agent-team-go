package memory

import (
	"fmt"
	"strings"
)

func BuildContext(entries []Entry, limit int) string {
	if len(entries) == 0 || limit == 0 {
		return ""
	}
	if limit < 0 || limit > len(entries) {
		limit = len(entries)
	}
	entries = entries[len(entries)-limit:]

	lines := []string{"Previous team memory:"}
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("- [%s] %s", entry.Status, entry.Task))
		if strings.TrimSpace(entry.Summary) != "" {
			lines = append(lines, fmt.Sprintf("  Summary: %s", firstLine(entry.Summary)))
		}
	}
	return strings.Join(lines, "\n")
}

func firstLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}
