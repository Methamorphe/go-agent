package world

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Methamorphe/go-agent/internal/id"
)

// ForkWorkspaceWorld creates a new isolated workspace from the exact immutable
// base captured by a previous WorkspaceWorld BranchRef. It never snapshots the
// current target repository, so forks cannot accidentally observe later edits.
func ForkWorkspaceWorld(ctx context.Context, base BranchRef, worldID id.WorldID, workRoot string) (*WorkspaceWorld, error) {
	if base.Type != TypeWorkspace || base.BaseIdentity == "" || len(base.Metadata) == 0 {
		return nil, fmt.Errorf("invalid workspace fork base")
	}
	if worldID == "" {
		return nil, fmt.Errorf("workspace fork world id is required")
	}
	var source workspaceMetadata
	if err := json.Unmarshal(base.Metadata, &source); err != nil {
		return nil, fmt.Errorf("decode workspace fork base: %w", err)
	}
	if source.Repository == "" || source.GitCommonDir == "" || source.BaseCommit == "" || source.BaseTree == "" {
		return nil, fmt.Errorf("workspace fork base metadata is incomplete")
	}
	if source.BaseTree != base.BaseIdentity {
		return nil, fmt.Errorf("workspace fork base identity mismatch")
	}
	if strings.TrimSpace(workRoot) == "" {
		workRoot = filepath.Join(source.GitCommonDir, "go-agent", "forks")
	}
	root, err := filepath.Abs(filepath.Clean(workRoot))
	if err != nil {
		return nil, fmt.Errorf("normalize fork root: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create fork root: %w", err)
	}
	worktree := filepath.Join(root, sanitizeGitRefToken(worldID.String()))
	if _, err := os.Stat(worktree); err == nil {
		return nil, fmt.Errorf("workspace fork path already exists: %s", worktree)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if _, err := gitOutput(ctx, source.Repository, nil, nil, "worktree", "add", "--detach", worktree, source.BaseCommit); err != nil {
		return nil, fmt.Errorf("create workspace fork worktree: %w", err)
	}
	local, err := NewLocalWorld(worktree)
	if err != nil {
		_ = removeWorkspaceWorktree(ctx, source.Repository, worktree)
		return nil, err
	}
	refPrefix := "refs/go-agent/forks/" + sanitizeGitRefToken(worldID.String())
	if _, err := gitOutput(ctx, source.Repository, nil, nil, "update-ref", refPrefix+"/base", source.BaseCommit); err != nil {
		_ = removeWorkspaceWorktree(ctx, source.Repository, worktree)
		return nil, fmt.Errorf("retain fork base: %w", err)
	}
	meta := workspaceMetadata{
		Repository:              source.Repository,
		GitCommonDir:            source.GitCommonDir,
		Worktree:                worktree,
		OriginalHead:            source.OriginalHead,
		BaseCommit:              source.BaseCommit,
		BaseTree:                source.BaseTree,
		DirtyPatchSHA256:        source.DirtyPatchSHA256,
		UntrackedManifestSHA256: source.UntrackedManifestSHA256,
		IgnoredPolicy:           source.IgnoredPolicy,
		RefPrefix:               refPrefix,
	}
	body, err := json.Marshal(meta)
	if err != nil {
		_ = removeWorkspaceWorktree(ctx, source.Repository, worktree)
		_ = deleteWorkspaceRefPrefix(ctx, source.Repository, refPrefix)
		return nil, err
	}
	ref := BranchRef{WorldID: worldID, Type: TypeWorkspace, BaseIdentity: source.BaseTree, Ref: worktree, Metadata: body}
	return &WorkspaceWorld{id: worldID, meta: meta, local: local, branchRef: ref}, nil
}

// ReleaseWorkspaceBranch is the idempotent G8 cleanup primitive. It removes
// the branch-owned promotion lease, detached worktree, and every synthetic Git
// ref retained below the branch prefix. It remains correct after a successful
// G7 commit, where Finalize may already have released the lease/worktree while
// promotion refs are still reachable.
func ReleaseWorkspaceBranch(ctx context.Context, ref BranchRef) error {
	if ref.Type != TypeWorkspace || len(ref.Metadata) == 0 {
		return nil
	}
	var meta workspaceMetadata
	if err := json.Unmarshal(ref.Metadata, &meta); err != nil {
		return err
	}
	if meta.Repository == "" {
		return nil
	}
	owner := &WorkspaceWorld{id: ref.WorldID, meta: meta}
	if err := owner.releaseWorldPromotionLease(); err != nil {
		return err
	}
	if meta.Worktree != "" {
		if err := removeWorkspaceWorktree(ctx, meta.Repository, meta.Worktree); err != nil {
			return err
		}
	}
	return deleteWorkspaceRefPrefix(ctx, meta.Repository, meta.RefPrefix)
}

// ReleaseWorkspaceSnapshot releases the checkpoint's retained Workspace
// resources. Child forks remain valid because each child has its own base ref.
func ReleaseWorkspaceSnapshot(ctx context.Context, ref BranchRef) error {
	return ReleaseWorkspaceBranch(ctx, ref)
}

func deleteWorkspaceRefPrefix(ctx context.Context, repository, prefix string) error {
	if strings.TrimSpace(repository) == "" || strings.TrimSpace(prefix) == "" {
		return nil
	}
	out, err := gitOutput(ctx, repository, nil, nil, "for-each-ref", "--format=%(refname)", prefix+"/")
	if err != nil {
		return fmt.Errorf("list workspace refs for cleanup: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		ref := strings.TrimSpace(line)
		if ref == "" || !strings.HasPrefix(ref, prefix+"/") {
			continue
		}
		if _, err := gitOutput(ctx, repository, nil, nil, "update-ref", "-d", ref); err != nil {
			return fmt.Errorf("delete workspace ref %s: %w", ref, err)
		}
	}
	return nil
}
