package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestMEM014TruthMaintenanceAdjacencyUsesIndexes(t *testing.T) {
	store, err := Open(context.Background(), Config{Path: filepath.Join(t.TempDir(), "memory-index.db")})
	if err != nil { t.Fatal(err) }
	defer store.Close()

	assertIndexedPlan(t, store, `EXPLAIN QUERY PLAN
SELECT to_belief_id FROM memory_belief_edges
WHERE from_belief_id = ? AND relation = ?`, "belief-root", "derived_from")

	assertIndexedPlan(t, store, `EXPLAIN QUERY PLAN
SELECT belief_id FROM memory_belief_evidence
WHERE evidence_id = ?`, "evidence-root")
}

func assertIndexedPlan(t *testing.T, store *Store, query string, args ...any) {
	t.Helper()
	rows, err := store.db.QueryContext(context.Background(), query, args...)
	if err != nil { t.Fatal(err) }
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil { t.Fatal(err) }
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil { t.Fatal(err) }
	joined := strings.ToUpper(strings.Join(details, " | "))
	if strings.Contains(joined, "SCAN MEMORY_BELIEF_EDGES") || strings.Contains(joined, "SCAN MEMORY_BELIEF_EVIDENCE") {
		t.Fatalf("truth-maintenance lookup regressed to full scan: %s", joined)
	}
	if !strings.Contains(joined, "SEARCH") && !strings.Contains(joined, "INDEX") {
		t.Fatalf("expected indexed adjacency search, got: %s", joined)
	}
}
