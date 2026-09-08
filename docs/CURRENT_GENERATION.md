# Current Implementation Generation

**Updated: September 8, 2026**

```text
A0  COMPLETE
G0  COMPLETE
G1  COMPLETE
G2  COMPLETE
G3  COMPLETE
G4  COMPLETE
G5  COMPLETE
G6  COMPLETE
G7  COMPLETE
G8  READY
```

## Current generation

**G7 — Workspace/OCI World + Agent Transactions: COMPLETE.**

G7 makes speculative mutation isolated and promotion-controlled where the selected World can prove the required guarantees.

The implementation now includes:

- Git-aware `WorkspaceWorld` with explicit tracked-dirty/untracked base capture and ignored-file exclusion;
- detached isolated worktrees without mutating the user's Git index;
- target-divergence detection, three-way promotion and target-scoped promotion lease;
- APPLY verification and reconciliation by durable Git identities rather than blind replay;
- OCI execution with network-off/read-only/cap-drop/no-new-privileges defaults, controlled mounts, resource limits and model-opaque secret binding;
- honest World Profiles that do not claim unsupported OCI snapshot/fork/promotion guarantees;
- optional `TransactionalWorld` semantics rather than fake transaction support on every World;
- `SecureTransactionalWorld` composition with the G3 authority gate;
- durable transaction/effect/verification/audit state in SQLite via `0007_agent_transactions.sql`;
- explicit `begin → execute → verify → prepare → commit / rollback / reconcile` lifecycle;
- durable DISPATCH before World mutation;
- irreversible-effect deferral;
- `OUTCOME_UNKNOWN` → `NEEDS_RECONCILIATION` safety;
- rollback hazard checks that prevent false rollback claims for unresolved or externally visible effects;
- evidence-backed effect reconciliation;
- current-policy/capability commit guard immediately before PREPARE and COMMIT.

Implementation head `da1ce88159bf8f3d51606222fb64413827dd2743` passed GitHub Actions CI run `34203844173`:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

The platform jobs passed `go test ./...`, `go vet ./...` and both binary builds. The race job passed `go test -race ./...`.

Long-duration workflow run `34203844315` also passed on the same implementation head.

See:

- `G7_EXIT_REVIEW.md`;
- `TRANSACTIONS_AND_COGNITIVE_FORKS.md`;
- `EXECUTION_WORLDS_PLATFORM_CONTRACT.md`;
- `EXECUTION_EDIT_SAFETY.md`;
- `LONG_DURATION_BENCHMARKS.md`.

## Long-duration validation

```text
soak harness                IMPLEMENTED
soak compile gate           ENABLED IN CI
latest soak workflow        PASS
1h reference run            PENDING
8h reference run            PENDING
24h reference run           PENDING
```

The historical 1h/8h/24h empirical longevity runs remain separate calibration work and are not a known G7 correctness defect. G7's transaction correctness and crash/reconciliation invariants are covered by deterministic tests and restart/reopen integration tests.

## Next generation

**G8 — Cognitive Fork / Safe Execution Editing: READY.**

G8 can now build quiescent forkable checkpoints, isolated branch Agent/World state, branch-local cognitive overlays, objective branch evaluation and selective promotion on top of the transaction/promotion substrate proven by G7.
