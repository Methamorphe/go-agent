//go:build soak

package mmu_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/soaktest"
	"github.com/Methamorphe/go-agent/internal/storage/sqlite"
)

func TestSoakMMUCorpusBounded(t *testing.T) {
	cfg, err := soaktest.ConfigFromEnv()
	if err != nil { t.Fatal(err) }
	corpusPages, err := soaktest.IntEnv("SOAK_MMU_CORPUS_PAGES", 10_000)
	if err != nil { t.Fatal(err) }
	if corpusPages <= 0 { t.Fatal("SOAK_MMU_CORPUS_PAGES must be positive") }

	ctx := context.Background()
	root := t.TempDir()
	store, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(root, "runtime.db"), BusyTimeout: 5 * time.Second, MaxOpenConns: 4, Clock: clock.NewSystemClock()})
	if err != nil { t.Fatal(err) }
	defer store.Close()
	objects, err := objectstore.New(filepath.Join(root, "objects")); if err != nil { t.Fatal(err) }
	sharedObject, err := objects.Put(ctx, strings.NewReader("needle durable memory evidence used by the MMU soak harness")); if err != nil { t.Fatal(err) }

	const agentID id.AgentID = "agt_soak_mmu"
	now := time.Now().UTC()
	for i := 0; i < corpusPages; i++ {
		pageID := id.ContextPageID(fmt.Sprintf("ctx_soak_%032x", i)); searchText := "historical unrelated context"; importance := 0.2
		if i%97 == 0 { searchText = "needle durable memory evidence"; importance = 0.9 }
		meta := mmu.PageMeta{ID: pageID, AgentID: agentID, Type: mmu.PageSourceExcerpt, Scope: mmu.ScopeProject, SourceRef: fmt.Sprintf("soak:%d", i), ObjectRef: sharedObject.Ref, TokenEstimate: 16, Importance: importance, Confidence: 1, CreatedAt: now.Add(-time.Duration(i%3600)*time.Second), LastAccessedAt: now.Add(-time.Duration(i%3600)*time.Second)}
		if err := store.PutContextPage(ctx, meta, searchText); err != nil { t.Fatalf("seed page %d: %v", i, err) }
	}
	if err := store.CheckpointWAL(ctx); err != nil { t.Fatal(err) }

	manager, err := mmu.New(store, objects, id.NewGenerator(), clock.NewSystemClock(), mmu.DefaultConfig()); if err != nil { t.Fatal(err) }
	request := mmu.BuildRequest{AgentID: agentID, Model: "soak-model", Budget: mmu.Budget{ModelMaxContext: 8_192, ReservedOutputTokens: 1_024, ProviderOverhead: 512, SafetyMargin: 256, SchemaTokens: 256, ActiveTokens: 256}, AllowedScopes: []mmu.Scope{mmu.ScopeProject}, Query: "needle durable memory"}
	for i := 0; i < 5; i++ { set, buildErr := manager.Build(ctx, request); if buildErr != nil { t.Fatalf("warm-up build: %v", buildErr) }; assertWorkingSetBudget(t, set) }

	baseline := soaktest.SampleRuntimeAfterGC(); latency := soaktest.NewLatencyWindow(cfg.MaxLatencySamples); deadline := time.Now().Add(cfg.Duration); nextSample := time.Now().Add(cfg.SampleInterval); operations := 0
	for time.Now().Before(deadline) {
		started := time.Now(); set, buildErr := manager.Build(ctx, request); latency.Add(time.Since(started)); if buildErr != nil { t.Fatalf("build %d: %v", operations, buildErr) }; assertWorkingSetBudget(t, set); operations++
		if time.Now().After(nextSample) { sample := soaktest.SampleRuntime(); stats := store.Stats(); t.Logf("mmu_soak ops=%d heap_alloc=%d heap_inuse=%d heap_objects=%d goroutines=%d gc=%d db_open=%d db_in_use=%d p95=%s max=%s", operations, sample.HeapAlloc, sample.HeapInuse, sample.HeapObjects, sample.Goroutines, sample.NumGC, stats.OpenConnections, stats.InUse, latency.P95(), latency.Max()); nextSample = time.Now().Add(cfg.SampleInterval) }
		time.Sleep(cfg.Interval)
	}
	final := soaktest.SampleRuntimeAfterGC(); if err := soaktest.ValidateRuntimeGrowth(baseline, final, cfg); err != nil { t.Fatal(err) }; if operations == 0 { t.Fatal("soak completed without any MMU operation") }
	t.Logf("mmu_soak_complete duration=%s corpus_pages=%d ops=%d baseline_heap=%d final_heap=%d baseline_goroutines=%d final_goroutines=%d p95=%s max=%s", cfg.Duration, corpusPages, operations, baseline.HeapAlloc, final.HeapAlloc, baseline.Goroutines, final.Goroutines, latency.P95(), latency.Max())
}

func assertWorkingSetBudget(t *testing.T, set mmu.WorkingSet) {
	t.Helper(); if set.Manifest.EstimatedUsed > set.Manifest.InputBudget { t.Fatalf("working set escaped hard budget: used=%d budget=%d", set.Manifest.EstimatedUsed, set.Manifest.InputBudget) }
}
