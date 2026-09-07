# G1 Exit Review — Durable Agent Process + Event Ledger

## Status

**G1 — Durable Agent Process + Event Ledger: COMPLETE.**

Primary implementation commit:

```text
f0b8628bb5b4f4e0e9e8a7d78b37d975c7f1d67b
feat: implement G1
```

G1 was closed on August 27, 2026. The historical exit-review file was missing even though the roadmap, implementation commits, tests and CI already marked the generation complete. This document repairs that documentation gap without changing G1 semantics.

---

## Delivered

### Durable Agent Process

- stable `AgentID` identity;
- immutable parent/root lineage metadata;
- accepted process lifecycle state machine;
- root Intent persistence;
- optimistic process-version concurrency control;
- durable sleep/wait state foundations;
- runtime activation ownership separated from canonical process state.

### Event Ledger

- append-only meaningful process events;
- global sequence and per-process version ordering;
- causation/correlation metadata;
- pure deterministic reducer;
- versioned snapshots;
- deterministic full replay;
- snapshot + tail reconstruction;
- current durable process projection.

### Durable command semantics

- request-id idempotency receipts;
- atomic event/projection/receipt commits;
- optimistic CAS rejection of stale process versions;
- stale wake rejection;
- durable recovery of processes left `RUNNING` by a dead runtime instance.

### Supervision

- runnable-process activation foundation;
- canonical state independent from goroutine lifetime;
- thousands of waiting/sleeping processes without one permanent goroutine per process.

---

## Validation

The G1 commit sequence added dedicated coverage for:

```text
ledger envelope invariants
durable process-store invariants
hard-kill process recovery
bounded supervisor residency
durable projection/replay benchmarks
durable RUNNING recovery integration
```

Relevant historical commits include:

```text
af1057e  test(g1): cover ledger envelope invariants
2e071f0  test(g1): cover durable process store invariants
4be99b4  test(g1): automate process recovery after hard kill
043d737  test(g1): prove bounded supervisor residency
49935a6  bench(g1): add durable projection and replay benchmarks
a357f75  test(g1): cover durable running recovery integration
7942fa3  docs: mark G1 complete and G2 ready
```

---

## Killer criteria

G1 proves the central durable-process claim without requiring an LLM:

1. create a process;
2. append meaningful state changes;
3. persist ledger/projection/snapshot state;
4. kill/reopen the runtime/storage boundary;
5. reconstruct exact logical state, version and lineage;
6. reject stale mutations deterministically;
7. supervise large waiting populations without permanent per-process workers.

The generation also validated deterministic full replay and snapshot + tail reconstruction.

---

## CI validation

GitHub Actions run `33074043420` passed on August 27, 2026 on integrated G1-era `main` head `576704f81a8e3405fa0ca02878d99b201d12c957`:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

The integrated head included the final Windows named-pipe client fix needed for the cross-platform daemon/client baseline.

Later generations G2–G5 and the current integrated `main` head continue to pass the same cross-platform test/vet/build/race baseline, so no known G1 regression is carried into G6.

---

## G1 result

**PASS.**

G1 established the durable state substrate required by every later generation:

```text
Agent Process identity
+ Event Ledger
+ deterministic reducer
+ projection/snapshot/replay
+ idempotent commands
+ optimistic concurrency
+ crash recovery
+ bounded supervision
```

**G1 is COMPLETE.**
