package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	agenttx "github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/world"
)

func TestG7OpenTransactionSurvivesSQLiteReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.db")
	cfg := Config{Path: path}
	store, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 8, 8, 0, 0, 0, time.UTC)
	tx := agenttx.Transaction{
		ID:             id.TransactionID("tx_open_restart"),
		AgentID:        id.AgentID("agt_open_restart"),
		BaseCheckpoint: id.CheckpointID("chk_open_restart"),
		WorldID:        id.WorldID("wld_open_restart"),
		IsolatedWorldRef: world.BranchRef{
			WorldID:      id.WorldID("wld_open_restart"),
			Type:         world.TypeWorkspace,
			BaseIdentity: "base-tree",
			Ref:          "durable-worktree",
		},
		State:        agenttx.StateOpen,
		Version:      2,
		CommitPolicy: agenttx.CommitRequireVerification,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.Create(ctx, tx, "TransactionOpened", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Get(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != agenttx.StateOpen || got.Version != tx.Version || got.IsolatedWorldRef.BaseIdentity != "base-tree" {
		t.Fatalf("reopened transaction=%+v", got)
	}
}
