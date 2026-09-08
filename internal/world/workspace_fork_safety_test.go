package world

import (
	"context"
	"errors"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
)

func TestG8ForkPromotionRejectsOverlappingTargetDivergence(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"same.txt": "base\n"})
	snapshot, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_g8_div_base")})
	if err != nil { t.Fatal(err) }
	defer snapshot.Rollback(ctx)
	branch, err := ForkWorkspaceWorld(ctx, snapshot.BranchRef(), id.WorldID("wld_g8_div_branch"), "")
	if err != nil { t.Fatal(err) }
	defer branch.Rollback(ctx)
	writeWorldFile(t, branch, "same.txt", "branch\n")
	writeTestFile(t, repo, "same.txt", "target\n")
	if _, err := branch.PreparePromotion(ctx, id.OperationID("op_g8_diverge")); !errors.Is(err, ErrPromotionConflict) { t.Fatalf("prepare err=%v want promotion conflict", err) }
	if got := normalizeTestEOL(readTestFile(t, repo, "same.txt")); got != "target\n" { t.Fatalf("conflicting promotion changed target: %q", got) }
}

func TestG8PromotionLeaseAllowsOnlyOnePreparedForkAtATime(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"a.txt": "base-a\n", "b.txt": "base-b\n"})
	snapshot, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_g8_lease_base")})
	if err != nil { t.Fatal(err) }
	defer snapshot.Rollback(ctx)
	a, err := ForkWorkspaceWorld(ctx, snapshot.BranchRef(), id.WorldID("wld_g8_lease_a"), "")
	if err != nil { t.Fatal(err) }
	defer a.Rollback(ctx)
	b, err := ForkWorkspaceWorld(ctx, snapshot.BranchRef(), id.WorldID("wld_g8_lease_b"), "")
	if err != nil { t.Fatal(err) }
	defer b.Rollback(ctx)
	writeWorldFile(t, a, "a.txt", "from-a\n")
	writeWorldFile(t, b, "b.txt", "from-b\n")
	if _, err := a.PreparePromotion(ctx, id.OperationID("op_g8_lease_a")); err != nil { t.Fatal(err) }
	if _, err := b.PreparePromotion(ctx, id.OperationID("op_g8_lease_b")); !errors.Is(err, ErrPromotionConflict) { t.Fatalf("second prepare err=%v want lease conflict", err) }
	if err := a.Rollback(ctx); err != nil { t.Fatal(err) }
	if _, err := b.PreparePromotion(ctx, id.OperationID("op_g8_lease_b_retry")); err != nil { t.Fatalf("lease was not released after rollback: %v", err) }
}
