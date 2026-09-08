package memory_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/memory"
	"github.com/Methamorphe/go-agent/internal/storage/sqlite"
)

type harness struct {
	ctx   context.Context
	path  string
	store *sqlite.Store
	clock *clock.FakeClock
	ids   *id.Generator
	svc   *memory.Service
	actor id.AgentID
}

func newHarness(t *testing.T, cfg memory.Config) *harness {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	return openHarness(t, path, cfg)
}

func openHarness(t *testing.T, path string, cfg memory.Config) *harness {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	fake := clock.NewFakeClock(now)
	store, err := sqlite.Open(ctx, sqlite.Config{Path: path, Clock: fake})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	ids := id.NewGenerator()
	svc, err := memory.New(store, nil, ids, fake, memory.AllowAllPolicy{}, cfg)
	if err != nil {
		store.Close()
		t.Fatalf("new memory service: %v", err)
	}
	return &harness{ctx: ctx, path: path, store: store, clock: fake, ids: ids, svc: svc, actor: id.AgentID("agent-test")}
}

func (h *harness) close(t *testing.T) {
	t.Helper()
	if h.store != nil {
		if err := h.store.Close(); err != nil {
			t.Fatalf("close sqlite: %v", err)
		}
		h.store = nil
	}
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (h *harness) evidence(t *testing.T, source, version string, provenance memory.ProvenanceClass, kind memory.EvidenceKind) memory.Evidence {
	t.Helper()
	value, err := h.svc.RecordEvidence(h.ctx, memory.EvidenceInput{
		ActorID: h.actor,
		Kind: kind,
		Scope: memory.Scope{Kind: memory.ScopeProject, Ref: "repo"},
		SourceRef: source,
		ContentHash: digest(source + ":" + version),
		ObservedAt: h.clock.Now(),
		SourceVersion: version,
		Provenance: provenance,
		TrustClass: memory.TrustAuthenticated,
		Sensitivity: memory.SensitivityInternal,
	})
	if err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	return value
}

func (h *harness) belief(t *testing.T, proposition string, status memory.BeliefStatus, evidence ...memory.EvidenceLinkInput) memory.Belief {
	t.Helper()
	value, err := h.svc.CreateBelief(h.ctx, memory.BeliefInput{
		ActorID: h.actor,
		Proposition: proposition,
		Scope: memory.Scope{Kind: memory.ScopeProject, Ref: "repo"},
		Status: status,
		Evidence: evidence,
	})
	if err != nil {
		t.Fatalf("create belief: %v", err)
	}
	return value
}

func containsProvenance(values []memory.ProvenanceClass, want memory.ProvenanceClass) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
