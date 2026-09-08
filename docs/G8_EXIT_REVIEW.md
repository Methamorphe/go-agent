# G8 Exit Review — Cognitive Fork / Safe Execution Editing

## Status

**G8 — Cognitive Fork / Safe Execution Editing: COMPLETE.**

Implementation branch: `g8-cognitive-fork-safe-execution-editing`

G8 turns checkpoint/fork/restore from UI-style time travel into kernel-governed execution editing. Alternative futures are created as new durable Agent timelines with isolated WorkspaceWorld state, bounded budgets, branch-local cognitive overlays and explicit objective evaluation. Promotion reuses G7's three-way, lease-protected transaction protocol and never rewrites source history.

---

## Delivered runtime primitives

### Quiescent forkable checkpoints

G8 records a durable checkpoint only when mutation-capable execution editing is safe:

- source process is at a READY/SUSPENDED orchestration boundary;
- no active process lease/wait/sleep obligation is present;
- no model invocation or tool action remains in flight;
- no active/non-terminal Agent Transaction remains;
- no unresolved `OUTCOME_UNKNOWN` transaction effect remains;
- every declared required result is present in the durable completed-result frontier;
- an immutable WorkspaceWorld base identity is available;
- current Intent/capability authority is captured with the checkpoint.

Unsupported mutation-capable World backends fail closed rather than advertising fork guarantees they cannot prove.

### Execution Frontier

The checkpoint contains an explicit causal frontier rather than only a process version:

```text
AgentID
process version
ledger sequence
last invocation boundary
completed result refs
in-flight invocation/action refs
unknown-outcome action refs
active transaction refs
required-result dependencies
child dependencies
context refs
immutable World snapshot/base ref
```

This preserves the information needed to reject unsafe execution edits and prevents derived decisions from silently surviving without their required evidence.

### Restore as a new timeline

Restore never truncates the source Event Ledger.

`ForkTimelineFromState` creates a new `AgentID` from the checkpointed process state, preserves root/parent lineage and causation, and starts a new successor process timeline. Events that happened after the checkpoint on the source process remain immutable and inspectable.

### Isolated branch Agent + World state

Each G8 fork receives:

- a unique `ForkID`;
- a unique branch `AgentID`;
- a unique `WorldID`;
- a detached Git worktree created from the checkpoint's exact retained base commit/tree;
- an operation/action namespace including the fork identity;
- a branch-local cognitive overlay;
- an independent scheduler budget reservation created before the branch is admitted.

Sibling worktrees share immutable Git objects/base history while mutable worktree state remains isolated. Ten-fork COW validation checks that forks share the same common Git object store/base commit without reusing mutable worktrees.

### Historical ∩ current authority

A checkpoint never becomes a capability resurrection token.

At fork creation G8 recomputes effective authority from both the checkpoint and the current authority provider:

```text
capabilities       = checkpoint ∩ current
authorized domains = checkpoint ∩ current
forbidden domains  = checkpoint ∪ current
acceptance criteria = checkpoint ∪ current
Intent version     = current
```

Requested branch capabilities must be valid delegations of both historical and current grants. Revoked capabilities/domains therefore remain revoked on restore/fork.

Speculative branches are additionally wrapped in `SpeculativeTransactionalWorld`, which mechanically denies irreversible effects before they reach the inner World.

### Independent branch budgets

Budget is reserved before branch Agent/World creation completes.

Each branch persists:

- reservation ID;
- branch limit;
- actual branch spend;
- settlement state.

A branch cannot spend beyond its reservation. Cleanup/finalization settles actual usage exactly once; losing branches release unused reserved capacity.

### Branch-local cognitive overlay

G8 persists branch-local context/memory/decision entries separately from the checkpoint base.

Promotion is whitelist-based:

- only entries explicitly marked promotable are candidates;
- promotable memory requires provenance;
- branch-only hypotheses/non-promotable entries are never unioned into the parent;
- each promoted artifact uses a deterministic idempotent promotion key;
- promoted state is persisted per artifact.

Raw speculative thought/transcript union is not a merge strategy.

### Objective evaluator

Each fork stores structured metrics, evidence reference, eligibility, score and reason.

Selection is deterministic:

1. hard correctness floor;
2. weighted correctness/quality/latency/cost/token objective;
3. stable `ForkID` tie-break.

The winning `ForkID` and human-readable selection reason are persisted on the fork group before promotion.

### Promotion through G7 transactions

Only the selected eligible winner may begin promotion.

Promotion verifies that the supplied branch World matches the winner's `WorldID` and checkpoint base identity, then delegates to the G7 transaction protocol:

```text
BEGIN
→ VERIFY
→ PREPARE
→ APPLY
→ VERIFY_APPLY
→ FINALIZE
```

G7 therefore continues to provide:

- three-way target/base/source merge semantics;
- target divergence detection;
- target-scoped promotion lease;
- current commit guard hooks;
- uncertain APPLY reconciliation rather than blind replay.

G8 marks a branch/group promoted only after the underlying transaction is durably `COMMITTED`.

### Cleanup and retention

Cleanup is idempotent and handles both winners and losers.

For each Workspace fork it releases:

- any promotion lease still owned by the fork;
- detached worktree if still present;
- all synthetic Git refs below the fork ref prefix, including retained promotion operation refs;
- unused branch budget reservation through exact-once settlement.

This also works after successful G7 finalization, when the winner worktree may already be gone but Git refs still need explicit release.

Heavy fork/group state can be retained for a configured window, then purged only after Worlds are gone and budgets are settled. Checkpoint purge releases its retained Workspace snapshot/reference after child fork resources have been finalized.

---

## Killer demonstration

`TestG8KillerDemoTwoSolutionsBenchmarkPromoteOnlyWinner` implements the roadmap demonstration:

```text
checkpoint exact base
├─ fork A → writes slow candidate
└─ fork B → writes fast candidate

parent target remains unchanged
        ↓
objective benchmark/evidence
        ↓
select B and persist reason
        ↓
G7 verify/prepare/commit B only
        ↓
target contains fast candidate
        ↓
whitelist cognitive promotion
        ↓
cleanup A + B + reservations + retained resources
```

The test asserts distinct Agent/World/reservation identities, no pre-promotion mutation leakage, deterministic winner selection, winner-only World promotion, non-promotable cognitive exclusion and resource/budget cleanup.

---

## Execution-edit contract coverage

| Contract | G8 validation |
|---|---|
| EDIT-001 | restore/fork creates a new Agent timeline; source process state/history is not truncated |
| EDIT-002 | unresolved transaction `OUTCOME_UNKNOWN` blocks forkable checkpoint creation |
| EDIT-003 | in-flight model/tool operations block forkable checkpoint creation |
| EDIT-004 | declared required result must exist in the durable completed-result frontier |
| EDIT-005 | sibling actions use fork-namespaced action IDs |
| EDIT-006 | sibling Workspace mutations stay isolated from source/target before promotion |
| EDIT-007 | G7 three-way promotion rejects unsafe overlapping target divergence |
| EDIT-008 | cognitive promotion is explicit whitelist-only and provenance-aware |
| EDIT-009 | G7 uncertain APPLY state remains reconciliation-driven, never blind duplicate commit |
| EDIT-010 | fork authority is historical ∩ current; revoked rights cannot be resurrected |
| EDIT-011 | restore creates a successor timeline and never claims later external history was undone |
| EDIT-012 | G7 target-scoped promotion lease remains the promotion serialization primitive |
| EDIT-013 | promotable memory requires provenance and promotion preserves source/provenance metadata |
| EDIT-014 | ten Workspace forks share immutable Git base/object storage rather than deep-copying history |
| EDIT-015 | unsupported mutation-capable World type is rejected at checkpoint/fork boundary |

## Cognitive-fork contract coverage

| Contract | G8 validation |
|---|---|
| FK-001 | sibling Workspace mutations are isolated |
| FK-002 | branch cognitive writes are stored in branch overlay state |
| FK-003 | requested/current branch authority cannot exceed checkpoint authority |
| FK-004 | scheduler reservation is acquired before branch creation succeeds |
| FK-005 | losing branch World changes never reach target and cleanup removes its resources |
| FK-006 | objective evidence, score and persisted winner reason are first-class state |
| FK-007 | ten forks use shared Git object/base identity with distinct mutable worktrees |
| FK-008 | G7 base/target divergence rules gate winner promotion |
| FK-009 | speculative irreversible effects are denied before inner World execution |
| FK-010 | cleanup settles budget once and releases worktree, promotion lease and synthetic refs idempotently |

---

## Persistence

SQLite migration `0008_cognitive_forks.sql` persists:

- immutable execution checkpoints and process-state integrity hashes;
- Execution Frontier JSON;
- authority snapshot JSON;
- fork groups and persisted winner/selection reason;
- branch Agent/World identity;
- budget reservation/limit/spend/settlement state;
- objective evaluation/evidence;
- cognitive overlay and per-entry promotion state.

The in-memory store mirrors the same G8 contract for deterministic unit tests.

---

## Validation execution note

The G8 deterministic/unit/integration validation suite is committed on the implementation branch, including the killer demonstration, SQLite persistence tests, execution-frontier tests, speculative-effect tests, Workspace fork/COW tests, authority-intersection tests and full cleanup tests.

Per explicit project-session instruction, **GitHub Actions CI was intentionally not used as the G8 closure gate in this pass**.

A direct local validation was attempted from the execution sandbox, but the sandbox could not resolve `github.com` while cloning the branch:

```text
fatal: unable to access 'https://github.com/Methamorphe/go-agent.git/':
Could not resolve host: github.com
```

Therefore this review does **not** invent or claim a green `go test ./...`, `go vet ./...`, race or cross-platform run that was not observed. Those execution checks remain appropriate before merging/releasing the branch, but the requested G8 implementation/documentation scope is closed.

---

## Non-claims preserved

G8 deliberately does not claim:

- generic exactly-once semantics for external systems;
- generic distributed ACID across unrelated resources;
- mutation-capable OCI forks while the OCI adapter cannot prove snapshot/fork/promotion guarantees;
- safe arbitrary non-quiescent execution editing;
- that restore erases effects/events observed after the checkpoint;
- automatic union of branch transcripts, hypotheses or unverified memory;
- authority inherited forever from a historical checkpoint.

---

## G8 result

**PASS — implementation scope complete.**

G8 delivers the roadmap primitives:

```text
quiescent forkable/committable checkpoint
explicit Execution Frontier
restore-as-new-Agent-timeline
isolated branch Agent + WorkspaceWorld
copy-on-write/shared immutable Git base
branch cognitive overlay
independent pre-admission budget reservations
fork-unique action namespace
historical ∩ current authority
speculative irreversible-effect denial
objective evidence-backed evaluator
persisted deterministic winner reason
G7 three-way/lease-protected winner promotion
selective provenance-aware cognitive promotion
idempotent World/ref/lease/budget cleanup
retention + purge
SQLite restart-durable G8 state
killer two-solution benchmark/promotion test
```

Time travel can change what the Agent does next. It does not rewrite what the World already observed.
