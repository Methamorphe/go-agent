//go:build soak

package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMEM014MillionEdgeNeighborhoodAvoidsFullGraphScan(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Config{Path: filepath.Join(t.TempDir(), "memory-million.db"), MaxOpenConns: 1})
	if err != nil { t.Fatal(err) }
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil { t.Fatal(err) }

	started := time.Now()
	_, err = store.db.ExecContext(ctx, `
WITH digits(d) AS (VALUES(0),(1),(2),(3),(4),(5),(6),(7),(8),(9)),
numbers(n) AS (
  SELECT a.d + 10*b.d + 100*c.d + 1000*d.d + 10000*e.d + 100000*f.d
  FROM digits a CROSS JOIN digits b CROSS JOIN digits c CROSS JOIN digits d CROSS JOIN digits e CROSS JOIN digits f
)
INSERT INTO memory_belief_edges(from_belief_id, to_belief_id, relation, created_at)
SELECT CASE WHEN n = 424242 THEN 'root-belief' ELSE printf('source-%06d', n) END,
       printf('target-%06d', n), 'derived_from', '2026-09-08T12:00:00Z'
FROM numbers`)
	if err != nil { t.Fatal(err) }
	t.Logf("inserted 1,000,000 synthetic edges in %s", time.Since(started))

	var count int
	if err := store.db.QueryRowContext(ctx, `
SELECT count(*) FROM memory_belief_edges
WHERE from_belief_id = ? AND relation = ?`, "root-belief", "derived_from").Scan(&count); err != nil { t.Fatal(err) }
	if count != 1 { t.Fatalf("affected neighborhood count=%d, want 1", count) }

	rows, err := store.db.QueryContext(ctx, `EXPLAIN QUERY PLAN
SELECT to_belief_id FROM memory_belief_edges
WHERE from_belief_id = ? AND relation = ?`, "root-belief", "derived_from")
	if err != nil { t.Fatal(err) }
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil { t.Fatal(err) }
		details = append(details, detail)
	}
	plan := strings.ToUpper(strings.Join(details, " | "))
	if strings.Contains(plan, "SCAN MEMORY_BELIEF_EDGES") || !strings.Contains(plan, "INDEX") {
		t.Fatalf("million-edge lookup did not use adjacency index: %s", plan)
	}
}
