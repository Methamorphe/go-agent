package world

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
)

func TestG8WorkspaceForkUsesCheckpointSnapshotNotLaterTarget(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"state.txt": "checkpoint\n"})
	snapshot, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_g8_snapshot")})
	if err != nil { t.Fatal(err) }
	defer snapshot.Rollback(ctx)

	writeTestFile(t, repo, "state.txt", "later-target\n")
	branch, err := ForkWorkspaceWorld(ctx, snapshot.BranchRef(), id.WorldID("wld_g8_restore"), "")
	if err != nil { t.Fatal(err) }
	defer branch.Rollback(ctx)
	if got := normalizeTestEOL(readWorldFile(t, branch, "state.txt")); got != "checkpoint\n" { t.Fatalf("fork observed %q want checkpoint snapshot", got) }
	if got := normalizeTestEOL(readTestFile(t, repo, "state.txt")); got != "later-target\n" { t.Fatalf("fork mutated later target: %q", got) }
}

func TestG8SiblingWorkspaceMutationsAreIsolated(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"candidate.txt": "base\n"})
	snapshot, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_g8_sibling_base")})
	if err != nil { t.Fatal(err) }
	defer snapshot.Rollback(ctx)
	a, err := ForkWorkspaceWorld(ctx, snapshot.BranchRef(), id.WorldID("wld_g8_a"), "")
	if err != nil { t.Fatal(err) }
	defer a.Rollback(ctx)
	b, err := ForkWorkspaceWorld(ctx, snapshot.BranchRef(), id.WorldID("wld_g8_b"), "")
	if err != nil { t.Fatal(err) }
	defer b.Rollback(ctx)

	writeWorldFile(t, a, "candidate.txt", "branch-a\n")
	writeWorldFile(t, b, "candidate.txt", "branch-b\n")
	if got := normalizeTestEOL(readWorldFile(t, a, "candidate.txt")); got != "branch-a\n" { t.Fatalf("branch A=%q", got) }
	if got := normalizeTestEOL(readWorldFile(t, b, "candidate.txt")); got != "branch-b\n" { t.Fatalf("branch B=%q", got) }
	if got := normalizeTestEOL(readTestFile(t, repo, "candidate.txt")); got != "base\n" { t.Fatalf("sibling mutation leaked to target: %q", got) }
}

func TestG8TenForksShareGitObjectBaseInsteadOfCopyingHistory(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"large-base.txt": "shared\n"})
	snapshot, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_g8_cow_base")})
	if err != nil { t.Fatal(err) }
	defer snapshot.Rollback(ctx)

	var commonDir, baseCommit string
	worktrees := make(map[string]struct{})
	branches := make([]*WorkspaceWorld, 0, 10)
	defer func() { for _, branch := range branches { _ = branch.Rollback(ctx) } }()
	for i := 0; i < 10; i++ {
		branch, err := ForkWorkspaceWorld(ctx, snapshot.BranchRef(), id.WorldID(fmt.Sprintf("wld_g8_cow_%02d", i)), "")
		if err != nil { t.Fatal(err) }
		branches = append(branches, branch)
		var meta workspaceMetadata
		if err := json.Unmarshal(branch.BranchRef().Metadata, &meta); err != nil { t.Fatal(err) }
		if i == 0 { commonDir, baseCommit = meta.GitCommonDir, meta.BaseCommit }
		if meta.GitCommonDir != commonDir || meta.BaseCommit != baseCommit { t.Fatalf("fork %d duplicated/diverged base metadata: %+v", i, meta) }
		if _, exists := worktrees[meta.Worktree]; exists { t.Fatalf("fork %d reused mutable worktree %s", i, meta.Worktree) }
		worktrees[meta.Worktree] = struct{}{}
		if branch.BranchRef().BaseIdentity != snapshot.BranchRef().BaseIdentity { t.Fatalf("fork %d base identity differs", i) }
	}
	if len(worktrees) != 10 { t.Fatalf("worktree count=%d want 10", len(worktrees)) }
}
