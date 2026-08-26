# Agent Transactions and Cognitive Forks

## Purpose

Transactions and Cognitive Forks are among the project's strongest differentiators, but they are also dangerous concepts if their guarantees are vague.

This document defines the intended semantics before implementation.

Core idea:

```text
checkpoint/base state
       │
       ├── isolated work
       │      ├── actions
       │      └── verification
       │
       └── promote or discard
```

A transaction focuses on **safe staged mutation**.

A Cognitive Fork focuses on **alternative futures** with separate Agent + World branch state.

---

# 1. Definitions

## Checkpoint

Durable named reference to recoverable logical state:

```text
Agent process version
context/memory references
World snapshot/base reference when supported
transaction/fork metadata
```

A checkpoint is metadata + references, not necessarily a copy of all data.

## Agent Transaction

A bounded isolated work unit where reversible effects are staged, verified and then committed/rolled back.

## Cognitive Fork

A new branch Agent Process created from a checkpoint with isolated cognitive overlays and, when mutations occur, isolated World state.

---

# 2. Transaction state machine

```text
CREATING
   ↓
OPEN
   ↓
VERIFYING
   ├──→ OPEN            verification failed but more work allowed
   ├──→ READY_TO_COMMIT
   └──→ ROLLING_BACK

READY_TO_COMMIT
   ├──→ COMMITTING
   └──→ ROLLING_BACK

COMMITTING
   ├──→ COMMITTED
   └──→ NEEDS_RECONCILIATION

ROLLING_BACK
   ├──→ ROLLED_BACK
   └──→ NEEDS_RECONCILIATION

NEEDS_RECONCILIATION
   ├──→ COMMITTED
   ├──→ ROLLED_BACK
   └──→ FAILED
```

Terminal:

```text
COMMITTED
ROLLED_BACK
FAILED
```

---

# 3. Transaction canonical state

```go
type Transaction struct {
    ID               TransactionID
    AgentID          AgentID
    BaseCheckpoint   CheckpointID
    WorldID          WorldID
    IsolatedWorldRef WorldBranchRef
    State            TransactionState
    Version          uint64
    EffectRefs       []EffectRecordID
    VerificationRefs []VerificationID
    CommitPolicy     CommitPolicy
    CreatedAt        time.Time
}
```

Large diffs/artifacts are refs, not embedded.

---

# 4. Transaction creation requirements

Before OPEN:

1. process is eligible;
2. required World capabilities known;
3. isolated mutation mechanism created;
4. budget/storage reserved;
5. base checkpoint finalized;
6. transaction record persisted.

If isolation cannot be provided, kernel must not claim transactional semantics.

Possible response:

```text
TransactionUnsupported(World lacks snapshot/overlay)
```

or explicit weaker mode in future—never silently.

---

# 5. Effect staging

Every mutation in transaction records:

```text
ActionID
EffectDescriptor
World-local evidence/diff
rollback/compensation metadata
external visibility
outcome certainty
```

Pure/read actions do not need rollback but remain traceable if relevant.

Reversible mutation operates on isolated transaction World where possible.

---

# 6. Irreversible effects

Default rule:

> Irreversible externally visible actions are not executed inside speculative transaction work before verification.

Instead create deferred effect intent:

```text
DeferredAction
  exact action descriptor
  authorization requirements
  approval requirements
```

Flow:

```text
isolated reversible work
       ↓
verification
       ↓
commit/promote internal state
       ↓
irreversible barrier
       ↓
re-authorize if required
       ↓
execute external action
```

If irreversible effect is intrinsically the task, transaction can still stage preparation but cannot promise rollback of that external effect.

---

# 7. Verification

Verification is a first-class object.

```go
type Verification struct {
    ID          VerificationID
    TxID        TransactionID
    Checks      []CheckSpec
    Results     []CheckResultRef
    Status      VerificationStatus
    StartedAt   time.Time
    CompletedAt *time.Time
}
```

Check priority:

1. deterministic tests/build/lint/bench;
2. policy/invariant checks;
3. artifact comparison;
4. model evaluator when objective checks insufficient;
5. user approval for subjective/high-risk promotion.

A model saying “looks good” is not equivalent to tests passing.

---

# 8. Commit semantics

Commit means:

> promote the transaction's defined reversible World/cognitive state to become the parent's selected current state.

It does not mean all external irreversible follow-up effects have happened.

Commit protocol must define per World.

For Git-worktree-like workspace:

```text
verify base has not diverged unexpectedly
compute transaction diff
apply/promote atomically where possible
validate resulting hashes
record promotion
```

For OCI overlay:

- export/apply layer or workspace diff according to adapter semantics.

Multi-resource commit is not magically atomic. If one transaction spans filesystem + DB + remote cloud, either:

- use coordinated supported transaction/compensation semantics;
- split into phases;
- classify as weaker and require reconciliation.

Do not market generic distributed ACID.

---

# 9. Commit conflict

Parent/base may have changed since transaction opened.

Before promotion compare base version/hash.

If diverged:

```text
CommitConflict
```

Options:

- rebase transaction changes into new isolated branch;
- rerun verification;
- ask agent/user;
- abandon transaction.

Never blindly overwrite newer parent World state.

---

# 10. Rollback semantics

Rollback means discard isolated reversible effects and release branch resources.

If transaction performed compensatable/irreversible external effects despite policy, rollback cannot pretend they vanished.

Result may become:

```text
ROLLED_BACK_WITH_EXTERNAL_EFFECTS
```

or `NEEDS_RECONCILIATION` depending severity.

Audit remains permanently.

---

# 11. Crash recovery by state

## OPEN/VERIFYING

Isolated World remains if durable; resume or rollback.

If World disappeared:

- reconstruct if possible;
- otherwise mark failed/needs reconciliation based on effects.

## READY_TO_COMMIT

Safe to resume decision/commit after revalidating base.

## COMMITTING

Highest-risk crash state.

Reconcile promotion steps using durable commit operation IDs/hashes.

Do not rerun commit blindly.

## ROLLING_BACK

Reconcile resource cleanup/compensations.

---

# 12. Cognitive Fork state

Fork creates a new Agent Process identity plus branch metadata:

```go
type Fork struct {
    ID              ForkID
    SourceAgentID   AgentID
    SourceCheckpoint CheckpointID
    BranchAgentID   AgentID
    WorldBranchRef  *WorldBranchRef
    MemoryOverlay   MemoryOverlayRef
    ContextOverlay  ContextOverlayRef
    BudgetReservation BudgetReservationID
    State           ForkState
}
```

Branch Agent has parent/fork lineage but own process lifecycle/version.

---

# 13. Fork state machine

```text
CREATING
  ↓
READY
  ↓
RUNNING
  ↓
COMPLETED | FAILED | CANCELLED
  ↓
EVALUATED
  ├──→ PROMOTED
  └──→ DISCARDED
```

Multiple branches may share one ForkGroup/evaluation group.

---

# 14. Cognitive state copy-on-write

Do not deep-copy all memory/context for every fork.

Branch sees:

```text
base immutable refs
+
branch-local overlay
```

Writes/new beliefs/pages go to overlay.

Promotion defines which branch-local cognitive artifacts become visible to parent/root scope.

Losing branch history can be retained for audit/evaluation while excluded from normal retrieval.

---

# 15. World state copy-on-write

World fork should use cheapest correct native mechanism:

```text
Git worktree/branch
filesystem COW/overlay
container snapshot/layer
DB transaction/snapshot
remote ephemeral workspace
```

The kernel works with abstract `WorldBranchRef`, but adapter documents exact guarantees.

If branch only performs pure/read analysis, separate World fork may be unnecessary; both can read immutable/shared base.

Mutation triggers isolation requirement.

---

# 16. Branch budgets

Fork fan-out reserves budget before branches run.

Example:

```text
parent available: $4, 20 min, 4 model slots

branch A reserve: $1.2, 6 min
branch B reserve: $1.2, 6 min

remaining parent reserve preserved
```

No speculative branch can silently exceed root/global concurrency or spend.

Losers release unused reservations.

---

# 17. Branch evaluator

Evaluation object receives structured branch outputs:

```text
artifacts/diffs
verification results
benchmarks
cost
latency
risk/effects
child findings
```

Prefer objective comparator.

Example score policy:

```text
hard gate: tests pass
hard gate: no security policy regression
primary: benchmark improvement
secondary: complexity/diff size
tertiary: model reviewer
```

Policy version is recorded.

---

# 18. Promotion semantics

Promoting fork means selecting branch state as successor to base.

Potential components:

```text
World diff
selected context/decision pages
verified new beliefs
artifacts
process result
```

Do **not** merge every branch thought/memory into parent automatically.

Cognitive promotion should be selective to avoid contaminating parent memory with speculative assumptions.

A promoted branch can either:

- become the continuing primary Agent Process; or
- commit artifacts back then parent continues.

v0 recommendation for simplicity:

> promote artifacts/World result back to parent transaction while parent Agent identity continues; branch remains child/fork history.

Later full branch-process promotion can be evaluated.

---

# 19. Losing branches

Policy options:

```text
retain metadata + result summary
retain full artifacts for N days
retain if user pins/debug mode
immediately GC heavy isolated World after safe finalization
```

Canonical branch events remain, but huge disposable artifacts may follow retention policy if not required for audit/replay.

Object reachability rules must account for retained checkpoints/evals.

---

# 20. Speculation safety

Actions forbidden by default in speculative branch:

- external communication;
- remote git push;
- production deploy;
- billing/payment;
- destructive external DB mutation;
- secret rotation;
- any irreversible action not explicitly sandboxed/deferred.

The Effect System should enforce this mechanically.

---

# 21. Nested transactions/forks

v0 recommendation:

- allow at most one active transaction per branch/process execution path;
- forbid arbitrary nested transactions initially;
- allow child processes to have their own independent transaction if budget/World isolation permits;
- bound fork depth/fan-out.

Nested semantic complexity can be added after core commit/recovery is proven.

---

# 22. Checkpoint semantics

Checkpoint includes exact references needed to branch/replay:

```text
AgentID + process version
World snapshot/base hash/ref
context/memory base refs
cognitive artifact versions
policy versions
optional current plan/task refs
```

Checkpoint creation should be relatively cheap through references/COW, not serialize entire history.

---

# 23. Required transaction tests

```text
TX-001 failed verification can rollback exact defined workspace state
TX-002 irreversible action deferred before verification
TX-003 commit conflict detected when base changed
TX-004 crash during OPEN recovers resumable/rollback state
TX-005 crash during COMMITTING never produces false committed state
TX-006 unknown external outcome enters reconciliation
TX-007 unsupported World cannot claim transactional guarantee
TX-008 rollback does not erase audit history
TX-009 storage failure before authorization prevents mutation
TX-010 stale transaction cannot commit after cancellation/revocation policy forbids it
```

---

# 24. Required fork tests

```text
FK-001 sibling mutations isolated
FK-002 branch context/memory writes use overlay
FK-003 branch authority never exceeds base
FK-004 budget reserved before fan-out
FK-005 losing branch cannot leak World changes
FK-006 objective evaluator promotion reason persisted
FK-007 10 large forks use COW/reference behavior rather than 10 full history copies
FK-008 parent/base divergence blocks unsafe promotion
FK-009 speculative irreversible action denied
FK-010 cleanup releases World resources and unused budget exactly once
```

---

# 25. First practical implementation path

Do not build generic cross-resource transactions first.

Start developer-specific but architecture-valid:

```text
Git repository/workspace
  ↓
checkpoint = base commit + workspace hash/state
  ↓
isolated Git worktree / OCI workspace
  ↓
agent edits
  ↓
tests/bench
  ↓
compute diff
  ↓
promote diff if base compatible
```

This gives strong concrete semantics and objective verification.

Then OCI overlay/container snapshots can strengthen isolation.

---

# Core principles

> **Speculation without isolation is just concurrent mutation.**

> **Rollback without effect semantics is a promise the runtime cannot keep.**

> **Promotion is a controlled state transition, not “copy whatever the winning agent touched”.**
