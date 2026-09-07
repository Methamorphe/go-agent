//go:build soak

package sqlite_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/soaktest"
	"github.com/Methamorphe/go-agent/internal/storage"
	"github.com/Methamorphe/go-agent/internal/storage/sqlite"
)

func TestSoakSQLiteDurability(t *testing.T) {
	cfg, err := soaktest.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	maxObjects, err := soaktest.IntEnv("SOAK_SQLITE_MAX_OBJECTS", 100_000)
	if err != nil {
		t.Fatal(err)
	}
	reopenEvery, err := soaktest.IntEnv("SOAK_SQLITE_REOPEN_EVERY", 1_000)
	if err != nil {
		t.Fatal(err)
	}
	checkpointEvery, err := soaktest.IntEnv("SOAK_SQLITE_CHECKPOINT_EVERY", 100)
	if err != nil {
		t.Fatal(err)
	}
	if maxObjects <= 0 || reopenEvery <= 0 || checkpointEvery <= 0 {
		t.Fatal("SQLite soak cardinality/reopen/checkpoint settings must be positive")
	}

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.db")
	openStore := func() *sqlite.Store {
		store, openErr := sqlite.Open(ctx, sqlite.Config{
			Path:         path,
			BusyTimeout:  5 * time.Second,
			MaxOpenConns: 8,
			Clock:        clock.NewSystemClock(),
		})
		if openErr != nil {
			t.Fatalf("open sqlite: %v", openErr)
		}
		return store
	}

	store := openStore()
	defer func() { _ = store.Close() }()

	for i := 0; i < 256; i++ {
		record := soakObjectRecord(i)
		if err := store.RegisterObject(ctx, record); err != nil {
			t.Fatalf("warm-up register object %d: %v", i, err)
		}
		if _, err := store.Object(ctx, record.Ref); err != nil {
			t.Fatalf("warm-up read object %d: %v", i, err)
		}
	}
	if err := store.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}

	baseline := soaktest.SampleRuntimeAfterGC()
	latency := soaktest.NewLatencyWindow(cfg.MaxLatencySamples)
	deadline := time.Now().Add(cfg.Duration)
	nextSample := time.Now().Add(cfg.SampleInterval)
	operations := 0
	created := 256
	reopens := 0
	checkpoints := 1

	for time.Now().Before(deadline) {
		index := operations % maxObjects
		if created < maxObjects {
			index = created
			created++
		}
		record := soakObjectRecord(index)

		started := time.Now()
		if err := store.RegisterObject(ctx, record); err != nil {
			t.Fatalf("register object %d: %v", index, err)
		}
		loaded, err := store.Object(ctx, record.Ref)
		latency.Add(time.Since(started))
		if err != nil {
			t.Fatalf("read object %d: %v", index, err)
		}
		if loaded.Digest != record.Digest || loaded.SizeBytes != record.SizeBytes {
			t.Fatalf("object %d changed across durable roundtrip", index)
		}
		operations++

		if operations%checkpointEvery == 0 {
			if err := store.Checkpoint(ctx); err != nil {
				t.Fatalf("checkpoint at operation %d: %v", operations, err)
			}
			checkpoints++
		}
		if operations%reopenEvery == 0 {
			if err := store.Close(); err != nil {
				t.Fatalf("close before reopen at operation %d: %v", operations, err)
			}
			store = openStore()
			reopens++
			probe := soakObjectRecord(index)
			if _, err := store.Object(ctx, probe.Ref); err != nil {
				t.Fatalf("reopen verification at operation %d: %v", operations, err)
			}
		}

		if time.Now().After(nextSample) {
			sample := soaktest.SampleRuntime()
			stats := store.Stats()
			if stats.InUse != 0 {
				t.Fatalf("database connection remained in-use between operations: %d", stats.InUse)
			}
			t.Logf(
				"sqlite_soak ops=%d created=%d reopens=%d checkpoints=%d heap_alloc=%d heap_inuse=%d heap_objects=%d goroutines=%d gc=%d db_open=%d db_idle=%d db_waits=%d p95=%s max=%s",
				operations,
				created,
				reopens,
				checkpoints,
				sample.HeapAlloc,
				sample.HeapInuse,
				sample.HeapObjects,
				sample.Goroutines,
				sample.NumGC,
				stats.OpenConnections,
				stats.Idle,
				stats.WaitCount,
				latency.P95(),
				latency.Max(),
			)
			nextSample = time.Now().Add(cfg.SampleInterval)
		}
		time.Sleep(cfg.Interval)
	}

	if err := store.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	final := soaktest.SampleRuntimeAfterGC()
	if err := soaktest.ValidateRuntimeGrowth(baseline, final, cfg); err != nil {
		t.Fatal(err)
	}
	if operations == 0 {
		t.Fatal("soak completed without any SQLite operation")
	}
	t.Logf(
		"sqlite_soak_complete duration=%s ops=%d created=%d reopens=%d checkpoints=%d baseline_heap=%d final_heap=%d baseline_goroutines=%d final_goroutines=%d p95=%s max=%s",
		cfg.Duration,
		operations,
		created,
		reopens,
		checkpoints,
		baseline.HeapAlloc,
		final.HeapAlloc,
		baseline.Goroutines,
		final.Goroutines,
		latency.P95(),
		latency.Max(),
	)
}

func soakObjectRecord(index int) storage.ObjectRecord {
	body := []byte(fmt.Sprintf("go-agent-sqlite-soak-%d", index))
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	return storage.ObjectRecord{
		Ref:         "sha256:" + digest,
		Algorithm:   "sha256",
		Digest:      digest,
		SizeBytes:   int64(len(body)),
		MediaType:   "application/octet-stream",
		Compression: "",
		CreatedAt:   time.Unix(int64(index), 0).UTC(),
	}
}
