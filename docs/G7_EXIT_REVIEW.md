# G7 Exit Review — Workspace/OCI World + Agent Transactions

## Status

**G7 — Workspace/OCI World + Agent Transactions: COMPLETE.**

Implementation branch: `g7-workspace-oci-transactions`  
Pull request: **#5 — `feat(g7): Workspace/OCI World + Agent Transactions`**

G7 turns speculative mutation into a kernel-governed operation: mutable work is isolated in a World that advertises concrete guarantees, verification is explicit, promotion is three-way and crash-aware, and uncertain/external effects cannot be hidden behind a false rollback.

---

## Delivered runtime primitives

### Git-aware WorkspaceWorld

`WorkspaceWorld` captures an explicit Git base without mutating the user's index:

- repository identity and `HEAD` parent;
- tracked dirty state;
- included untracked files;
- ignored-file exclusion through Git policy;
- deterministic base tree/commit identity;
- isolated detached worktree under the repository common Git directory.

The captured dirty/untracked state becomes the transaction base rather than being silently discarded or copied ambiguously.

Workspace mutations remain isolated until promotion. Rollback removes the isolated worktree/resources and leaves the target workspace unchanged.

### Three-way promotion and target divergence

Promotion is relative to the captured base:

```text
BASE
├─ TARGET current
└─ SOURCE isolated transaction
```

`PreparePromotion`:

- acquires a short target-scoped promotion lease;
- captures the current target and source identities;
- computes a Git three-way merge tree;
- rejects overlapping conflicts;
- persists exact target/source/merged identities in the promotion plan.

`ApplyPromotion` revalidates the target identity before applying. `VerifyPromotion` proves the resulting target tree. `ReconcilePromotion` distinguishes applied, not-applied and unknown outcomes without blindly replaying APPLY.

Concurrent target divergence is therefore either merged safely, rejected as a conflict or sent to reconciliation; newer target state is never blindly overwritten.

### OCI World

`OCIWorld` provides a Docker/Podman execution adapter with conservative defaults:

- network disabled by default;
- network access requires explicit opt-in;
- read-only root filesystem by default;
- `cap-drop=ALL`;
- `no-new-privileges`;
- optional CPU, memory, PID and wall-time limits;
- controlled bind mounts;
- host-control socket mounts such as Docker/Podman/containerd sockets rejected by default;
- optional non-root user;
- secret refs resolved below model context and mounted as temporary read-only files.

The v0 adapter does **not** claim snapshot/fork/promotion support it cannot prove. Its World Profile explicitly reports those guarantees as unsupported while still advertising the isolation/resource guarantees it actually enforces.

### Transactional World contract

G7 introduces an optional `TransactionalWorld` capability instead of forcing every World to fake transaction semantics.

The contract exposes:

```text
BranchRef
PreparePromotion
ApplyPromotion
VerifyPromotion
ReconcilePromotion
Rollback
Finalize
```

`LocalWorld` remains host-mediated and does not become a sandbox merely because transaction support exists elsewhere.

### G3 authority composition

`SecureTransactionalWorld` composes the existing G3 `SecureWorld` authorization gate with a real `TransactionalWorld`.

A denied action is rejected before it reaches the inner transactional World. Transaction promotion remains a separate kernel operation, with current authority/policy revalidated through the transaction commit guard immediately before PREPARE and again immediately before COMMIT/APPLY.

Historical authority at transaction creation is therefore insufficient to authorize a later promotion after capability/policy revocation.

### Durable Agent Transaction state machine

G7 implements the accepted state machine:

```text
CREATING
  → OPEN
  → VERIFYING
  → READY_TO_COMMIT
  → COMMITTING
  → COMMITTED

OPEN / VERIFYING / READY_TO_COMMIT
  → ROLLING_BACK
  → ROLLED_BACK

uncertain critical state
  → NEEDS_RECONCILIATION
  → COMMITTED | ROLLED_BACK | FAILED/later intervention
```

Only the accepted v0 commit policy, verification-required promotion, is admitted.

### Durable SQLite transaction store

Migration `0007_agent_transactions.sql` persists:

- transaction identity/state/version;
- base checkpoint and isolated World reference;
- prepared promotion plan;
- effect records and outcome certainty;
- first-class verification objects;
- append-only transaction audit events.

State transitions use version checks so stale writers cannot silently advance a transaction. OPEN, READY_TO_COMMIT and COMMITTING transaction state survives SQLite close/reopen.

### Effect lifecycle and unknown outcomes

Every effectful transaction action is recorded before crossing the World boundary:

```text
PREPARED
→ DISPATCHED
→ COMPLETED
 | FAILED_BEFORE_EFFECT
 | OUTCOME_UNKNOWN
```

The `DISPATCHED` record is durable before `World.Execute`.

If persistence fails before DISPATCH, mutation is never attempted. If execution loses certainty after dispatch, the effect becomes `OUTCOME_UNKNOWN` and the transaction immediately enters `NEEDS_RECONCILIATION`.

A transaction with an unresolved dispatched effect cannot claim a clean rollback. Explicit effect reconciliation requires a known outcome plus evidence before the transaction may leave reconciliation when safe.

### Irreversible and externally visible effects

Canonical irreversible effects are deferred before World dispatch during speculative work.

Rollback additionally refuses to claim success when a known-applied compensatable/irreversible effect lacks proven compensation. Audit history remains durable even when isolated World state is discarded.

### Verification and commit protocol

Verification is first-class and objective checks run before promotion. A failed check returns the transaction to `OPEN` for further work or rollback.

Commit follows the accepted safety protocol:

```text
PREPARE
→ APPLY
→ VERIFY_APPLY
→ FINALIZE
```

A crash/error during APPLY or VERIFY_APPLY never creates a false `COMMITTED` state. It enters `NEEDS_RECONCILIATION` and uses durable operation/tree identities to determine what the target actually observed.

---

## Required transaction validation

| Contract | Validation |
|---|---|
| TX-001 | breaking multi-file verification fails and isolated workspace rolls back without mutating the target |
| TX-002 | irreversible action is durably deferred before World execution |
| TX-003 | overlapping target divergence is rejected as a promotion conflict |
| TX-004 | OPEN transaction/base World reference survives SQLite reopen |
| TX-005 | interrupted/failed APPLY never produces false `COMMITTED` state |
| TX-006 | unknown dispatched outcome enters `NEEDS_RECONCILIATION`; false rollback is blocked until evidence-backed resolution |
| TX-007 | World without real mutation-isolation/promotion guarantees returns `TransactionUnsupported` |
| TX-008 | rollback keeps durable transaction/audit events rather than erasing history |
| TX-009 | durable effect-write failure before DISPATCH prevents mutation from reaching the World |
| TX-010 | commit guard revocation after verification/prepare prevents APPLY |

Additional Workspace/World validation proves:

- dirty tracked + untracked base capture;
- ignored files do not leak into the isolated base;
- sibling/target changes remain isolated before promotion;
- non-overlapping target divergence is preserved by three-way promotion;
- promotion reconciliation identifies already-applied target state;
- rollback leaves the defined target state unchanged;
- SecureTransactionalWorld denies before inner World execution;
- OCI defaults disable network and elevated container capabilities;
- OCI network requires explicit opt-in;
- host-control socket mounts are rejected by default;
- secret plaintext is not encoded into the model-visible command arguments;
- OCI Profile never advertises unsupported snapshot/fork/promotion guarantees.

---

## CI validation

Implementation head `da1ce88159bf8f3d51606222fb64413827dd2743` passed GitHub Actions CI run `34203844173`:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

The platform jobs passed `go test ./...`, `go vet ./...`, both binary builds, and the Linux soak-scenario compile gate. The race job passed `go test -race ./...`.

Long-duration workflow run `34203844315` also passed on the same implementation head.

---

## Non-claims preserved

G7 deliberately does not claim:

- generic distributed ACID across filesystem, databases and remote services;
- generic exactly-once external effects;
- rollback of irreversible effects already observed externally;
- OCI snapshot/fork support before an adapter can prove it;
- LocalWorld as a secure sandbox;
- blind replay of an uncertain promotion.

These are architectural safety properties, not missing marketing features.

---

## G7 result

**PASS.**

G7 satisfies the roadmap deliverables:

```text
Git-aware isolated WorkspaceWorld
captured dirty/untracked base policy
target-divergence detection
three-way promotion + promotion lease
promotion apply verification + reconciliation
OCI controlled mounts + network-off default
OCI CPU/memory/PID/time limits
model-opaque explicit secret binding
honest World guarantee Profiles
durable Agent Transaction state machine
begin / execute / verify / prepare / commit / rollback / reconcile
durable effect/outcome lifecycle
irreversible-effect deferral
unknown-outcome reconciliation
current-authority commit guard
SQLite restart-safe transaction/effect/verification/audit state
cross-platform tests/vet/builds + race + soak workflow
```

**G7 is COMPLETE. G8 — Cognitive Fork / Safe Execution Editing is READY.**
