# Execution Edit Safety — Checkpoint, Fork, Restore and Merge

## Status

**A0 architecture contract — ACCEPTED conservative baseline.**

This document closes an important gap in `TRANSACTIONS_AND_COGNITIVE_FORKS.md`: a checkpoint/fork/restore/merge is not automatically safe merely because state is persistable.

An agent runtime may already have:

- authorized actions;
- dispatched external requests;
- in-flight tools;
- results that future reasoning depends on;
- irreversible effects;
- child tasks operating from related state.

Execution edits must preserve those causal obligations.

Core rule:

> **A runtime never rewrites history. Restore/fork/merge create new successor timelines from durable state under explicit safety checks.**

---

# 1. Execution edit vocabulary

## Checkpoint

Capture a durable recoverable execution frontier.

## Fork

Create one or more new branch timelines from a checkpoint.

## Restore

Create a new successor timeline that continues from an older checkpoint.

Restore does **not** delete events that happened after that checkpoint.

## Merge

Promote selected compatible state/artifacts from a branch into another timeline using a common base.

Merge does **not** concatenate transcripts or pretend two process histories were one.

---

# 2. Operation lifecycle matters

Every effectful operation has a durable lifecycle:

```text
Proposed
→ Authorized
→ Dispatched
→ Completed
   | FailedBeforeEffect
   | OutcomeUnknown
   | CancelledBeforeDispatch
```

Once an operation is `Dispatched`, an execution edit cannot assume it never happened.

Especially:

```text
Authorized != executed
Dispatched != known success
network error != known failure
```

This composes with the global `OutcomeUnknown` rule.

---

# 3. Execution Frontier

A checkpoint stores an **Execution Frontier** describing the exact causal state being captured.

```go
type ExecutionFrontier struct {
    AgentID             AgentID
    ProcessVersion      uint64
    LedgerPosition      uint64
    InvocationBoundary  *InvocationID
    CompletedOps        []OperationID
    InFlightOps         []OperationID
    UnknownOutcomeOps   []OperationID
    PendingResults      []ResultDependency
    ChildDependencies   []ChildDependency
    TransactionRefs     []TransactionID
    WorldSnapshotRef    *WorldSnapshotRef
}
```

Large lists may be object-backed/compactly indexed.

---

# 4. Quiescent vs non-quiescent checkpoint

## Quiescent checkpoint

No operation that can affect branch semantics is currently in flight.

Preferred v0 state.

## Non-quiescent checkpoint

Some tool/model/child operations remain outstanding.

This is fundamentally harder because a fork/restore must decide who owns future results and how to avoid duplicated effects.

---

# 5. Accepted v0 checkpoint policy

**v0 forkable/mutation-capable checkpoints are conservative and quiescent.**

Requirements:

```text
agent is at model/tool orchestration boundary
no in-flight state-changing World operation
no OutcomeUnknown mutation unresolved
no commit/rollback currently in progress
no pending irreversible action already dispatched
World snapshot/base ref finalized
process state + policy versions persisted
```

Read-only background operations MAY be allowed later, but v0 can wait/cancel/reconcile them before producing a forkable checkpoint.

This intentionally trades some latency for understandable semantics.

---

# 6. Checkpoint classes

```text
ObservationCheckpoint
  usable for replay/debug inspection
  may include in-flight metadata

ResumeCheckpoint
  sufficient to resume same timeline safely

ForkableCheckpoint
  meets quiescent branch-safety requirements

CommittableCheckpoint
  forkable + exact World/base compatibility data required for promotion
```

Do not expose one generic "checkpoint" flag with ambiguous guarantees.

---

# 7. Operation ownership

An asynchronous result has one logical owner timeline.

If operation O was dispatched before a fork and not completed:

v0 does not duplicate O into both children.

Instead fork waits until O is:

```text
completed
failed-before-effect
cancelled safely
or reconciled
```

Later advanced execution-edit checking may transfer/shared-observe some operations, but ownership must remain explicit.

---

# 8. Exactly-once is not assumed

External systems rarely provide generic exactly-once semantics.

The runtime therefore relies on:

```text
operation IDs
idempotency keys when backend supports them
outcome certainty
reconciliation
commit barriers
```

Never claim generic exactly-once tool execution.

Internal ledger transitions can be exactly-once logically via transaction/version checks; external effects cannot be generalized that way.

---

# 9. Restore semantics

Suppose timeline:

```text
checkpoint C1
   ↓
A
   ↓
B
   ↓
C
```

Restore to C1 creates:

```text
original timeline: C1 → A → B → C
                     \
                      restored branch R → ...
```

It does not truncate the original ledger.

This preserves:

- auditability;
- external effects that already occurred;
- causal history;
- time-travel debugging.

---

# 10. Restore safety checks

Before restore/fork from checkpoint:

1. validate checkpoint integrity;
2. identify effects after checkpoint on source timeline;
3. ensure new branch World is isolated from those later effects;
4. ensure no external irreversible effect is "undone" conceptually;
5. mark branch knowledge `as-of checkpoint`;
6. revalidate current policy/capability constraints before new actions.

Historical authority does not guarantee current authority.

---

# 11. World snapshot identity

A branchable World reference must capture enough stable identity to detect divergence.

Examples:

```text
Git base commit + dirty workspace manifest/hash
OCI image/layer digest + overlay ref
DB snapshot ID/LSN where supported
remote workspace immutable snapshot ID
```

A plain directory path is not a sufficient checkpoint identity.

---

# 12. Fork safety

Fork requires:

```text
valid ForkableCheckpoint
budget reservation
child authority subset
branch cognitive overlay
isolated World if mutations possible
unique branch AgentID
unique operation namespace
```

Operation IDs/idempotency namespaces must not collide across branches in a way that causes an external service to treat distinct speculative actions as one accidental retry.

---

# 13. Shared read-only state

Branches MAY share immutable/reference-counted state:

```text
object blobs
source snapshots
base Context Pages
base beliefs
read-only indexes
```

They MUST NOT share mutable branch overlays without explicit synchronization/semantics.

Copy-on-write is the preferred model.

---

# 14. Required-result preservation

A branch continuation may depend on prior results.

Example:

```text
test result R
   ↓
plan decision P
   ↓
fork checkpoint C
```

Checkpoint must either:

- include/ref R as completed dependency;
- or omit P/state that depends on unavailable R.

Never preserve a derived decision while silently discarding the result required to justify it.

This is a key invariant for safe execution edits.

---

# 15. Cognitive dependency frontier

Checkpoint records key durable cognitive refs:

```text
active TaskIntent
active Plan version
Context Manifest/ref set
Belief versions used by unresolved decisions
Verification results
Acceptance-criterion state
```

Not every token is part of the frontier.

The goal is to retain the durable inputs necessary to reconstruct coherent continuation.

---

# 16. Merge is selective

Merge is never:

```text
append branch transcript to parent
```

Candidate merge payload:

```go
type MergeProposal struct {
    BaseCheckpoint       CheckpointID
    SourceBranch         AgentID
    TargetAgent          AgentID
    WorldDiffRef         *ObjectRef
    ArtifactRefs         []ObjectRef
    BeliefPromotions     []BeliefID
    DecisionPromotions   []PageID
    VerificationRefs     []VerificationID
}
```

Every component has its own compatibility/promotion rules.

---

# 17. Three-way World merge

For mutable repository/workspace state:

```text
BASE
├─ TARGET current
└─ SOURCE branch
```

Promotion computes compatibility relative to BASE.

If target changed overlapping state:

```text
MergeConflict
```

Then:

- rebase branch;
- resolve in isolated World;
- re-verify;
- or abandon.

Never overwrite target based solely on branch completion time.

---

# 18. Cognitive merge

Branch cognitive artifacts have categories.

## Safe-ish to promote after validation

```text
verified test result
artifact reference
source-backed factual belief
accepted design decision
```

## Not promoted automatically

```text
speculative hypothesis
branch-only temporary plan
losing alternative assumptions
raw chain-of-work transcript
unverified memory consolidation
```

Cognitive merge is whitelist/selective, not union.

---

# 19. Belief merge semantics

Promoting branch belief:

1. preserve original branch provenance;
2. map branch World/source refs to promoted target versions;
3. detect target contradiction/supersession;
4. run Epistemic Memory status rules;
5. publish only to allowed scope.

A branch belief does not become globally `Active` merely because the branch won a benchmark.

---

# 20. Commit protocol baseline

For a transaction/fork promotion:

```text
PREPARE
  freeze proposal parameters/diff hashes
  verify target/base compatibility
  verify authorization still valid
  verify acceptance checks current
  acquire short promotion lease/lock

APPLY
  perform World-specific promotion with operation ID
  persist intermediate durable state

VERIFY_APPLY
  validate resulting target hashes/state

FINALIZE
  mark committed/promoted
  publish selected cognitive artifacts
  release branch resources/budget
```

If crash occurs during APPLY:

```text
NEEDS_RECONCILIATION
```

not blind re-apply.

---

# 21. Promotion lease

A short target-scoped lease prevents concurrent commits from both validating the same base then racing promotion.

Lease is not a long-lived global lock.

Scope example:

```text
world target + base revision / workspace
```

On expiry/crash, durable operation state determines reconciliation.

---

# 22. Multi-resource edits

No generic distributed ACID claim.

If a task spans:

```text
filesystem
DB
remote API
GitHub publish
```

runtime uses phases:

```text
reversible isolated preparation
→ deterministic verification
→ internal promotion
→ external effect barriers
→ compensation/reconciliation if needed
```

Each resource documents its guarantee.

---

# 23. Unsafe edit conditions

Fork/restore/merge is blocked when any applies:

```text
unresolved OutcomeUnknown relevant mutation
commit/rollback already in critical apply phase
required result missing from frontier
World snapshot integrity unavailable
base compatibility cannot be proven
branch authority/policy cannot be revalidated
pending irreversible effect ownership ambiguous
```

Return structured reason rather than forcing edit.

---

# 24. Advanced non-quiescent edits — deferred research

A future runtime may permit checkpoint/fork while operations are in flight by computing exact obligations:

```text
which dispatched effects must be preserved
which results must remain deliverable
which operations belong to which branch
which continuations are safe
```

This is valuable but high-complexity.

A0 decision:

> **Do not make non-quiescent execution edits a requirement for G8. Prove conservative quiescent edits first.**

The protocol/data model should keep enough operation/frontier metadata so a stronger checker can be added later without redesigning history.

---

# 25. Required tests

```text
EDIT-001 restore never deletes original post-checkpoint events
EDIT-002 fork blocked with unresolved state-changing OutcomeUnknown
EDIT-003 forkable checkpoint cannot contain in-flight mutation in v0
EDIT-004 completed result required by active decision remains referenced in frontier
EDIT-005 sibling branches use distinct operation namespaces
EDIT-006 branch World mutation cannot affect source/target before promotion
EDIT-007 target base divergence causes MergeConflict
EDIT-008 merge promotes selected cognitive artifacts only
EDIT-009 crash during APPLY enters reconciliation, never blind duplicate commit
EDIT-010 historical capability at checkpoint does not bypass current authorization on restored branch
EDIT-011 irreversible effect after checkpoint is not conceptually undone by restore
EDIT-012 promotion lease prevents two commits from concurrently promoting same stale base
EDIT-013 branch belief promotion preserves provenance/source mapping
EDIT-014 large shared base uses COW/reference sharing rather than full-copy history
EDIT-015 unsupported World snapshot guarantee blocks mutation-capable fork
```

---

# 26. Observability

Debugger should show:

```text
checkpoint C42
  process version 918
  ledger seq 14082
  world base git:abc123
  in-flight mutations: 0
  unknown outcomes: 0
  required results: [test://r91]
  class: ForkableCheckpoint

fork group F8
  branch A → discarded
  branch B → promoted
  target base changed? false
  promotion verification: pass
```

---

# 27. Innovation opportunity

The strong abstraction is **safe execution editing**:

```text
durable execution frontier
+ effect/outcome-aware operations
+ quiescent safety class
+ copy-on-write cognitive + World state
+ required-result preservation
+ restore-as-new-timeline
+ selective three-way merge
+ promotion lease
+ reconciliation on uncertain commit
```

This turns checkpoint/fork/restore/merge from UI conveniences into kernel-governed transformations.

---

# Accepted A0 decisions

1. Restore/fork never rewrite or truncate Event Ledger history.
2. v0 mutation-capable fork checkpoints are quiescent.
3. `OutcomeUnknown` blocks unsafe execution edits until reconciliation.
4. A checkpoint records required-result/cognitive frontier, not only process version.
5. Restore revalidates current authority/policy before new effects.
6. World merge is three-way relative to explicit base identity.
7. Cognitive merge is selective; transcripts/speculative assumptions are not unioned.
8. Commit uses prepare/apply/verify/finalize phases plus short promotion lease.
9. Crash during uncertain promotion enters reconciliation.
10. Generic exactly-once external effects and generic distributed ACID are explicitly not promised.
11. Advanced non-quiescent execution editing is deferred but data model supports future research.

> **Time travel can change what the agent does next. It cannot change what the world already observed.**
