package world

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
)

func TestG8ReleaseWorkspaceBranchRemovesWorktreeRefsAndPromotionLease(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"candidate.txt": "base\n"})
	snapshot, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_g8_cleanup_base")})
	if err != nil {
		t.Fatal(err)
	}
	defer ReleaseWorkspaceBranch(ctx, snapshot.BranchRef())

	branch, err := ForkWorkspaceWorld(ctx, snapshot.BranchRef(), id.WorldID("wld_g8_cleanup_branch"), "")
	if err != nil {
		t.Fatal(err)
	}
	writeWorldFile(t, branch, "candidate.txt", "winner\n")
	if _, err := branch.PreparePromotion(ctx, id.OperationID("op_g8_cleanup")); err != nil {
		t.Fatal(err)
	}

	ref := branch.BranchRef()
	var meta workspaceMetadata
	if err := json.Unmarshal(ref.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(meta.Worktree); err != nil {
		t.Fatalf("fork worktree missing before cleanup: %v", err)
	}
	if _, err := os.Stat(branch.promotionLeasePath()); err != nil {
		t.Fatalf("promotion lease missing before cleanup: %v", err)
	}
	refsBefore, err := gitOutput(ctx, repo, nil, nil, "for-each-ref", "--format=%(refname)", meta.RefPrefix+"/")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(refsBefore)) == "" {
		t.Fatal("expected retained G8 refs before cleanup")
	}

	if err := ReleaseWorkspaceBranch(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(meta.Worktree); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fork worktree survived cleanup: %v", err)
	}
	if _, err := os.Stat(branch.promotionLeasePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("promotion lease survived cleanup: %v", err)
	}
	refsAfter, err := gitOutput(ctx, repo, nil, nil, "for-each-ref", "--format=%(refname)", meta.RefPrefix+"/")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(refsAfter)) != "" {
		t.Fatalf("synthetic refs survived cleanup: %s", refsAfter)
	}
	if err := ReleaseWorkspaceBranch(ctx, ref); err != nil {
		t.Fatalf("cleanup is not idempotent: %v", err)
	}
}
