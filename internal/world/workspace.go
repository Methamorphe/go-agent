package world

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Methamorphe/go-agent/internal/id"
)

type WorkspaceConfig struct {
	Repository string
	WorkRoot   string
	WorldID    id.WorldID
}

type workspaceMetadata struct {
	Repository              string `json:"repository"`
	GitCommonDir            string `json:"git_common_dir"`
	Worktree                string `json:"worktree"`
	OriginalHead            string `json:"original_head"`
	BaseCommit              string `json:"base_commit"`
	BaseTree                string `json:"base_tree"`
	DirtyPatchSHA256        string `json:"dirty_patch_sha256"`
	UntrackedManifestSHA256 string `json:"untracked_manifest_sha256"`
	IgnoredPolicy           string `json:"ignored_policy"`
	RefPrefix               string `json:"ref_prefix"`
}

type promotionMetadata struct {
	TargetCommit string `json:"target_commit"`
	SourceCommit string `json:"source_commit"`
	MergeCommit  string `json:"merge_commit"`
}

type promotionLease struct {
	WorldID     id.WorldID     `json:"world_id"`
	OperationID id.OperationID `json:"operation_id"`
}

type WorkspaceWorld struct {
	id       id.WorldID
	meta     workspaceMetadata
	local    *LocalWorld
	branchRef BranchRef
}

func NewWorkspaceWorld(ctx context.Context, cfg WorkspaceConfig) (*WorkspaceWorld, error) {
	if cfg.WorldID == "" {
		return nil, fmt.Errorf("workspace world id is required")
	}
	if strings.TrimSpace(cfg.Repository) == "" {
		return nil, fmt.Errorf("workspace repository is required")
	}

	repository, err := gitOutput(ctx, cfg.Repository, nil, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve git repository: %w", err)
	}
	repoRoot, err := filepath.Abs(strings.TrimSpace(string(repository)))
	if err != nil {
		return nil, fmt.Errorf("normalize repository root: %w", err)
	}

	commonOut, err := gitOutput(ctx, repoRoot, nil, nil, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("resolve git common directory: %w", err)
	}
	commonDir := strings.TrimSpace(string(commonOut))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repoRoot, commonDir)
	}
	commonDir, err = filepath.Abs(filepath.Clean(commonDir))
	if err != nil {
		return nil, fmt.Errorf("normalize git common directory: %w", err)
	}

	headOut, err := gitOutput(ctx, repoRoot, nil, nil, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("workspace requires a repository with HEAD: %w", err)
	}
	originalHead := strings.TrimSpace(string(headOut))

	baseCommit, baseTree, err := captureGitSnapshot(ctx, repoRoot, originalHead, "go-agent workspace base")
	if err != nil {
		return nil, fmt.Errorf("capture workspace base: %w", err)
	}
	dirtyHash, err := commandHash(ctx, repoRoot, "diff", "--binary", "HEAD", "--", ".")
	if err != nil {
		return nil, fmt.Errorf("hash dirty workspace: %w", err)
	}
	untrackedHash, err := commandHash(ctx, repoRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("hash untracked manifest: %w", err)
	}

	refPrefix := "refs/go-agent/transactions/" + sanitizeGitRefToken(cfg.WorldID.String())
	if _, err := gitOutput(ctx, repoRoot, nil, nil, "update-ref", refPrefix+"/base", baseCommit); err != nil {
		return nil, fmt.Errorf("retain workspace base: %w", err)
	}

	workRoot := cfg.WorkRoot
	if strings.TrimSpace(workRoot) == "" {
		workRoot = filepath.Join(commonDir, "go-agent", "workspaces")
	}
	workRoot, err = filepath.Abs(filepath.Clean(workRoot))
	if err != nil {
		return nil, fmt.Errorf("normalize workspace root: %w", err)
	}
	if err := os.MkdirAll(workRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace root: %w", err)
	}
	worktree := filepath.Join(workRoot, sanitizeGitRefToken(cfg.WorldID.String()))
	if _, err := os.Stat(worktree); err == nil {
		return nil, fmt.Errorf("workspace path already exists: %s", worktree)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect workspace path: %w", err)
	}
	if _, err := gitOutput(ctx, repoRoot, nil, nil, "worktree", "add", "--detach", worktree, baseCommit); err != nil {
		return nil, fmt.Errorf("create isolated worktree: %w", err)
	}

	local, err := NewLocalWorld(worktree)
	if err != nil {
		_ = removeWorkspaceWorktree(ctx, repoRoot, worktree)
		return nil, err
	}
	meta := workspaceMetadata{
		Repository:              repoRoot,
		GitCommonDir:            commonDir,
		Worktree:                worktree,
		OriginalHead:            originalHead,
		BaseCommit:              baseCommit,
		BaseTree:                baseTree,
		DirtyPatchSHA256:        dirtyHash,
		UntrackedManifestSHA256: untrackedHash,
		IgnoredPolicy:           "exclude",
		RefPrefix:               refPrefix,
	}
	metadata, err := json.Marshal(meta)
	if err != nil {
		_ = removeWorkspaceWorktree(ctx, repoRoot, worktree)
		return nil, err
	}
	return &WorkspaceWorld{
		id:    cfg.WorldID,
		meta:  meta,
		local: local,
		branchRef: BranchRef{
			WorldID:      cfg.WorldID,
			Type:         TypeWorkspace,
			BaseIdentity: baseTree,
			Ref:          worktree,
			Metadata:     metadata,
		},
	}, nil
}

func ReopenWorkspaceWorld(ref BranchRef) (*WorkspaceWorld, error) {
	if ref.Type != TypeWorkspace || ref.WorldID == "" || len(ref.Metadata) == 0 {
		return nil, fmt.Errorf("invalid workspace branch reference")
	}
	var meta workspaceMetadata
	if err := json.Unmarshal(ref.Metadata, &meta); err != nil {
		return nil, fmt.Errorf("decode workspace branch reference: %w", err)
	}
	if meta.Repository == "" || meta.Worktree == "" || meta.BaseCommit == "" || meta.BaseTree == "" {
		return nil, fmt.Errorf("workspace branch reference is incomplete")
	}
	local, err := NewLocalWorld(meta.Worktree)
	if err != nil {
		return nil, fmt.Errorf("reopen isolated worktree: %w", err)
	}
	return &WorkspaceWorld{id: ref.WorldID, meta: meta, local: local, branchRef: ref}, nil
}

func (w *WorkspaceWorld) Profile() Profile {
	return Profile{
		Name:             "workspace",
		Type:             TypeWorkspace,
		EnforcementLevel: EnforcementHostMediated,
		Filesystem: FilesystemGuarantees{
			WorldRelativeRoot: true,
			MutationIsolation: true,
			GitAware:          true,
		},
		Network:          NetworkFullOutbound,
		Snapshot:         SnapshotGuarantees{Supported: true, CrashDurable: true},
		Fork:             ForkGuarantees{Supported: true, CrashDurable: true},
		Promotion:        PromotionGuarantees{Supported: true, ThreeWay: true, Reconciliation: true},
		ProfileVersion:   1,
		SupportsStreaming:       true,
		SupportsCancellation:    true,
		SupportsProcessTreeKill: true,
	}
}

func (w *WorkspaceWorld) Execute(ctx context.Context, action Action) (Result, error) {
	return w.local.Execute(ctx, action)
}

func (w *WorkspaceWorld) BranchRef() BranchRef { return w.branchRef }

func (w *WorkspaceWorld) PreparePromotion(ctx context.Context, operationID id.OperationID) (PromotionPlan, error) {
	if operationID == "" {
		return PromotionPlan{}, fmt.Errorf("promotion operation id is required")
	}
	if err := w.acquirePromotionLease(operationID); err != nil {
		return PromotionPlan{}, err
	}
	prepared := false
	defer func() {
		if !prepared {
			_ = w.releasePromotionLease(operationID)
		}
	}()

	targetCommit, targetTree, err := captureGitSnapshot(ctx, w.meta.Repository, w.meta.BaseCommit, "go-agent promotion target")
	if err != nil {
		return PromotionPlan{}, fmt.Errorf("snapshot promotion target: %w", err)
	}
	sourceCommit, sourceTree, err := captureGitSnapshot(ctx, w.meta.Worktree, w.meta.BaseCommit, "go-agent promotion source")
	if err != nil {
		return PromotionPlan{}, fmt.Errorf("snapshot promotion source: %w", err)
	}
	if _, err := gitOutput(ctx, w.meta.Repository, nil, nil, "update-ref", w.meta.RefPrefix+"/operations/"+sanitizeGitRefToken(operationID.String())+"/target", targetCommit); err != nil {
		return PromotionPlan{}, fmt.Errorf("retain promotion target: %w", err)
	}
	if _, err := gitOutput(ctx, w.meta.Repository, nil, nil, "update-ref", w.meta.RefPrefix+"/operations/"+sanitizeGitRefToken(operationID.String())+"/source", sourceCommit); err != nil {
		return PromotionPlan{}, fmt.Errorf("retain promotion source: %w", err)
	}

	mergeOut, err := gitOutput(ctx, w.meta.Repository, nil, nil, "merge-tree", "--write-tree", targetCommit, sourceCommit)
	if err != nil {
		return PromotionPlan{}, fmt.Errorf("%w: %v", ErrPromotionConflict, err)
	}
	mergedTree := firstOutputLine(mergeOut)
	if mergedTree == "" {
		return PromotionPlan{}, fmt.Errorf("merge-tree returned no merged tree")
	}
	mergeCommit, err := createGitCommit(ctx, w.meta.Repository, mergedTree, []string{targetCommit, sourceCommit}, "go-agent promotion merge")
	if err != nil {
		return PromotionPlan{}, fmt.Errorf("create promotion merge commit: %w", err)
	}
	if _, err := gitOutput(ctx, w.meta.Repository, nil, nil, "update-ref", w.meta.RefPrefix+"/operations/"+sanitizeGitRefToken(operationID.String())+"/merge", mergeCommit); err != nil {
		return PromotionPlan{}, fmt.Errorf("retain promotion merge: %w", err)
	}

	meta, err := json.Marshal(promotionMetadata{TargetCommit: targetCommit, SourceCommit: sourceCommit, MergeCommit: mergeCommit})
	if err != nil {
		return PromotionPlan{}, err
	}
	prepared = true
	return PromotionPlan{
		OperationID:   operationID,
		BaseIdentity:  w.meta.BaseTree,
		TargetBefore:  targetTree,
		Source:        sourceTree,
		Merged:        mergedTree,
		TargetChanged: targetTree != w.meta.BaseTree,
		Metadata:      meta,
	}, nil
}

func (w *WorkspaceWorld) ApplyPromotion(ctx context.Context, plan PromotionPlan) error {
	meta, err := w.validatePromotionPlan(plan)
	if err != nil {
		return err
	}
	if err := w.requirePromotionLease(plan.OperationID); err != nil {
		return err
	}
	_, currentTree, err := captureGitSnapshot(ctx, w.meta.Repository, w.meta.BaseCommit, "go-agent promotion apply preflight")
	if err != nil {
		return fmt.Errorf("snapshot promotion target before apply: %w", err)
	}
	if currentTree != plan.TargetBefore {
		_ = w.releasePromotionLease(plan.OperationID)
		return fmt.Errorf("%w: target changed after prepare", ErrPromotionConflict)
	}

	patch, err := gitOutput(ctx, w.meta.Repository, nil, nil, "diff", "--binary", meta.TargetCommit, meta.MergeCommit)
	if err != nil {
		return fmt.Errorf("compute promotion patch: %w", err)
	}
	if len(bytes.TrimSpace(patch)) > 0 {
		if _, err := gitOutput(ctx, w.meta.Repository, nil, bytes.NewReader(patch), "apply", "--whitespace=nowarn", "-"); err != nil {
			return fmt.Errorf("apply promotion patch: %w", err)
		}
	}
	_, appliedTree, err := captureGitSnapshot(ctx, w.meta.Repository, w.meta.BaseCommit, "go-agent promotion apply verification")
	if err != nil {
		return fmt.Errorf("snapshot target after apply: %w", err)
	}
	if appliedTree != plan.Merged {
		return fmt.Errorf("%w: target tree %s, expected %s", ErrPromotionUncertain, appliedTree, plan.Merged)
	}
	return nil
}

func (w *WorkspaceWorld) VerifyPromotion(ctx context.Context, plan PromotionPlan) error {
	if _, err := w.validatePromotionPlan(plan); err != nil {
		return err
	}
	_, tree, err := captureGitSnapshot(ctx, w.meta.Repository, w.meta.BaseCommit, "go-agent promotion final verification")
	if err != nil {
		return err
	}
	if tree != plan.Merged {
		return fmt.Errorf("%w: promoted tree %s, expected %s", ErrPromotionUncertain, tree, plan.Merged)
	}
	return w.releasePromotionLease(plan.OperationID)
}

func (w *WorkspaceWorld) ReconcilePromotion(ctx context.Context, plan PromotionPlan) (PromotionStatus, error) {
	if _, err := w.validatePromotionPlan(plan); err != nil {
		return PromotionUnknown, err
	}
	_, tree, err := captureGitSnapshot(ctx, w.meta.Repository, w.meta.BaseCommit, "go-agent promotion reconciliation")
	if err != nil {
		return PromotionUnknown, err
	}
	switch tree {
	case plan.Merged:
		if err := w.releasePromotionLease(plan.OperationID); err != nil {
			return PromotionUnknown, err
		}
		return PromotionApplied, nil
	case plan.TargetBefore:
		if err := w.releasePromotionLease(plan.OperationID); err != nil {
			return PromotionUnknown, err
		}
		return PromotionNotApplied, nil
	default:
		return PromotionUnknown, nil
	}
}

func (w *WorkspaceWorld) Rollback(ctx context.Context) error {
	if err := w.releaseWorldPromotionLease(); err != nil {
		return err
	}
	return removeWorkspaceWorktree(ctx, w.meta.Repository, w.meta.Worktree)
}

func (w *WorkspaceWorld) Finalize(ctx context.Context) error {
	if err := w.releaseWorldPromotionLease(); err != nil {
		return err
	}
	return removeWorkspaceWorktree(ctx, w.meta.Repository, w.meta.Worktree)
}

func (w *WorkspaceWorld) Close(context.Context) error { return nil }

func (w *WorkspaceWorld) validatePromotionPlan(plan PromotionPlan) (promotionMetadata, error) {
	if plan.OperationID == "" || plan.BaseIdentity != w.meta.BaseTree || plan.TargetBefore == "" || plan.Merged == "" {
		return promotionMetadata{}, fmt.Errorf("promotion plan is incompatible with workspace branch")
	}
	var meta promotionMetadata
	if err := json.Unmarshal(plan.Metadata, &meta); err != nil {
		return promotionMetadata{}, fmt.Errorf("decode promotion plan: %w", err)
	}
	if meta.TargetCommit == "" || meta.SourceCommit == "" || meta.MergeCommit == "" {
		return promotionMetadata{}, fmt.Errorf("promotion plan metadata is incomplete")
	}
	return meta, nil
}

func (w *WorkspaceWorld) promotionLeasePath() string {
	return filepath.Join(w.meta.GitCommonDir, "go-agent", "promotion.lock")
}

func (w *WorkspaceWorld) acquirePromotionLease(operationID id.OperationID) error {
	path := w.promotionLeasePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create promotion lease directory: %w", err)
	}
	lease := promotionLease{WorldID: w.id, OperationID: operationID}
	body, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, writeErr := file.Write(body); writeErr != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return fmt.Errorf("write promotion lease: %w", writeErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			_ = os.Remove(path)
			return fmt.Errorf("close promotion lease: %w", closeErr)
		}
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("acquire promotion lease: %w", err)
	}
	existing, readErr := readPromotionLease(path)
	if readErr != nil {
		return fmt.Errorf("read promotion lease: %w", readErr)
	}
	if existing.WorldID == w.id && existing.OperationID == operationID {
		return nil
	}
	return fmt.Errorf("%w: promotion target already leased", ErrPromotionConflict)
}

func (w *WorkspaceWorld) requirePromotionLease(operationID id.OperationID) error {
	lease, err := readPromotionLease(w.promotionLeasePath())
	if err != nil {
		return fmt.Errorf("promotion lease unavailable: %w", err)
	}
	if lease.WorldID != w.id || lease.OperationID != operationID {
		return fmt.Errorf("%w: promotion lease owner changed", ErrPromotionConflict)
	}
	return nil
}

func (w *WorkspaceWorld) releasePromotionLease(operationID id.OperationID) error {
	path := w.promotionLeasePath()
	lease, err := readPromotionLease(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if lease.WorldID != w.id || lease.OperationID != operationID {
		return fmt.Errorf("%w: refusing to release another promotion lease", ErrPromotionConflict)
	}
	return os.Remove(path)
}

func (w *WorkspaceWorld) releaseWorldPromotionLease() error {
	path := w.promotionLeasePath()
	lease, err := readPromotionLease(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if lease.WorldID != w.id {
		return nil
	}
	return os.Remove(path)
}

func readPromotionLease(path string) (promotionLease, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return promotionLease{}, err
	}
	var lease promotionLease
	if err := json.Unmarshal(body, &lease); err != nil {
		return promotionLease{}, err
	}
	if lease.WorldID == "" || lease.OperationID == "" {
		return promotionLease{}, fmt.Errorf("promotion lease is incomplete")
	}
	return lease, nil
}

func captureGitSnapshot(ctx context.Context, directory, parent, message string) (string, string, error) {
	indexDir, err := os.MkdirTemp("", "go-agent-git-index-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(indexDir)
	indexPath := filepath.Join(indexDir, "index")
	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := gitOutput(ctx, directory, env, nil, "read-tree", parent); err != nil {
		return "", "", err
	}
	if _, err := gitOutput(ctx, directory, env, nil, "add", "-A"); err != nil {
		return "", "", err
	}
	treeOut, err := gitOutput(ctx, directory, env, nil, "write-tree")
	if err != nil {
		return "", "", err
	}
	tree := strings.TrimSpace(string(treeOut))
	commit, err := createGitCommitWithEnv(ctx, directory, tree, []string{parent}, message, env)
	if err != nil {
		return "", "", err
	}
	return commit, tree, nil
}

func createGitCommit(ctx context.Context, directory, tree string, parents []string, message string) (string, error) {
	return createGitCommitWithEnv(ctx, directory, tree, parents, message, nil)
}

func createGitCommitWithEnv(ctx context.Context, directory, tree string, parents []string, message string, extraEnv []string) (string, error) {
	args := []string{"commit-tree", tree}
	for _, parent := range parents {
		args = append(args, "-p", parent)
	}
	env := append([]string{}, extraEnv...)
	env = append(env,
		"GIT_AUTHOR_NAME=go-agent",
		"GIT_AUTHOR_EMAIL=go-agent@local",
		"GIT_COMMITTER_NAME=go-agent",
		"GIT_COMMITTER_EMAIL=go-agent@local",
	)
	out, err := gitOutput(ctx, directory, env, strings.NewReader(message+"\n"), args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func commandHash(ctx context.Context, directory string, args ...string) (string, error) {
	out, err := gitOutput(ctx, directory, nil, nil, args...)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(out)
	return hex.EncodeToString(sum[:]), nil
}

func gitOutput(ctx context.Context, directory string, extraEnv []string, stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = stdin
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func removeWorkspaceWorktree(ctx context.Context, repository, worktree string) error {
	if _, err := os.Stat(worktree); errors.Is(err, os.ErrNotExist) {
		_, _ = gitOutput(ctx, repository, nil, nil, "worktree", "prune")
		return nil
	} else if err != nil {
		return err
	}
	if _, err := gitOutput(ctx, repository, nil, nil, "worktree", "remove", "--force", worktree); err != nil {
		return err
	}
	return nil
}

func sanitizeGitRefToken(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "world"
	}
	return b.String()
}

func firstOutputLine(body []byte) string {
	line := strings.TrimSpace(string(body))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}
