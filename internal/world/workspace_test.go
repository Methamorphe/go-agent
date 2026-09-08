package world

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
)

func TestWorkspaceWorldCapturesDirtyAndUntrackedBase(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"tracked.txt": "base\n", ".gitignore": "*.ignored\n"})
	writeTestFile(t, repo, "tracked.txt", "dirty\n")
	writeTestFile(t, repo, "untracked.txt", "included\n")
	writeTestFile(t, repo, "secret.ignored", "excluded\n")

	workspace, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_capture")})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Rollback(ctx)

	if got := normalizeTestEOL(readWorldFile(t, workspace, "tracked.txt")); got != "dirty\n" {
		t.Fatalf("tracked dirty base=%q", got)
	}
	if got := normalizeTestEOL(readWorldFile(t, workspace, "untracked.txt")); got != "included\n" {
		t.Fatalf("untracked base=%q", got)
	}
	_, err = workspace.Execute(ctx, Action{Kind: "fs.read_file", Resource: "secret.ignored", Effect: CanonicalEffect("fs.read_file")})
	if err == nil {
		t.Fatal("ignored file leaked into isolated base")
	}
	if workspace.Profile().EnforcementLevel != EnforcementHostMediated || !workspace.Profile().Promotion.ThreeWay {
		t.Fatalf("workspace profile=%+v", workspace.Profile())
	}
}

func TestWorkspaceWorldThreeWayPromotionPreservesNonOverlappingTargetChange(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"target.txt": "base-target\n", "source.txt": "base-source\n"})
	workspace, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_merge")})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close(ctx)

	writeWorldFile(t, workspace, "source.txt", "source-change\n")
	writeTestFile(t, repo, "target.txt", "target-change\n")

	plan, err := workspace.PreparePromotion(ctx, id.OperationID("op_merge"))
	if err != nil {
		t.Fatal(err)
	}
	if !plan.TargetChanged {
		t.Fatal("target divergence was not detected")
	}
	if err := workspace.ApplyPromotion(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if err := workspace.VerifyPromotion(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Finalize(ctx); err != nil {
		t.Fatal(err)
	}
	if got := normalizeTestEOL(readTestFile(t, repo, "target.txt")); got != "target-change\n" {
		t.Fatalf("target change lost: %q", got)
	}
	if got := normalizeTestEOL(readTestFile(t, repo, "source.txt")); got != "source-change\n" {
		t.Fatalf("source promotion missing: %q", got)
	}
}

func TestWorkspaceWorldConflictingTargetDivergenceBlocksPromotion(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"same.txt": "base\n"})
	workspace, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_conflict")})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Rollback(ctx)

	writeWorldFile(t, workspace, "same.txt", "source\n")
	writeTestFile(t, repo, "same.txt", "target\n")
	_, err = workspace.PreparePromotion(ctx, id.OperationID("op_conflict"))
	if !errors.Is(err, ErrPromotionConflict) {
		t.Fatalf("prepare err=%v, want promotion conflict", err)
	}
	if got := normalizeTestEOL(readTestFile(t, repo, "same.txt")); got != "target\n" {
		t.Fatalf("conflict mutated target: %q", got)
	}
}

func TestWorkspaceWorldReconcileDetectsAppliedPromotion(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"file.txt": "base\n"})
	workspace, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_reconcile")})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Finalize(ctx)

	writeWorldFile(t, workspace, "file.txt", "promoted\n")
	plan, err := workspace.PreparePromotion(ctx, id.OperationID("op_reconcile"))
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.ApplyPromotion(ctx, plan); err != nil {
		t.Fatal(err)
	}
	status, err := workspace.ReconcilePromotion(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if status != PromotionApplied {
		t.Fatalf("reconcile status=%s, want %s", status, PromotionApplied)
	}
}

func TestWorkspaceWorldRollbackLeavesTargetExact(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepository(t, map[string]string{"tracked.txt": "base\n"})
	writeTestFile(t, repo, "tracked.txt", "dirty-before\n")
	writeTestFile(t, repo, "untracked.txt", "untracked-before\n")
	workspace, err := NewWorkspaceWorld(ctx, WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_rollback")})
	if err != nil {
		t.Fatal(err)
	}
	writeWorldFile(t, workspace, "tracked.txt", "speculative\n")
	writeWorldFile(t, workspace, "new.txt", "speculative-new\n")
	if err := workspace.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, repo, "tracked.txt"); got != "dirty-before\n" {
		t.Fatalf("rollback changed tracked target: %q", got)
	}
	if got := readTestFile(t, repo, "untracked.txt"); got != "untracked-before\n" {
		t.Fatalf("rollback changed untracked target: %q", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("speculative file leaked to target: %v", err)
	}
}

func initGitRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "G7 Test")
	runTestGit(t, repo, "config", "user.email", "g7@example.invalid")
	for path, body := range files {
		writeTestFile(t, repo, path, body)
	}
	runTestGit(t, repo, "add", "-A")
	runTestGit(t, repo, "commit", "-m", "base")
	return repo
}

func runTestGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func writeWorldFile(t *testing.T, workspace *WorkspaceWorld, path, body string) {
	t.Helper()
	params, err := json.Marshal(map[string]string{"content": body})
	if err != nil {
		t.Fatal(err)
	}
	_, err = workspace.Execute(context.Background(), Action{Kind: "fs.write_file", Resource: path, Params: params, Effect: CanonicalEffect("fs.write_file")})
	if err != nil {
		t.Fatal(err)
	}
}

func readWorldFile(t *testing.T, workspace *WorkspaceWorld, path string) string {
	t.Helper()
	result, err := workspace.Execute(context.Background(), Action{Kind: "fs.read_file", Resource: path, Effect: CanonicalEffect("fs.read_file")})
	if err != nil {
		t.Fatal(err)
	}
	return string(result.Data)
}

func writeTestFile(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, root, path string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func normalizeTestEOL(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}
