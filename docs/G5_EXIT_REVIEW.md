# G5 Exit Review — Recursive Agent Processes

## Status

**G5 — Recursive Agent Processes: COMPLETE candidate.**

Implementation branch: `g5-recursive-agent-processes`  
Pull request: **#2 — `feat(g5): implement Recursive Agent Processes`**

G5 implements the recursive orchestration contracts accepted during A0 without turning child agents into prompt-local helpers or goroutine-owned state.

---

## Delivered runtime primitives

### Durable delegated Agent Processes

- dedicated `SpawnID`, `BudgetReservationID`, `MessageID` and `WaitID` identities;
- child creation through `process.Service.CreateDelegatedChild`;
- independent immutable child Task Intent;
- parent/root lineage and depth preserved by the existing durable Agent Process model;
- no permanent goroutine per waiting/sleeping child.

### Spawn validate → reserve → create

`orchestration.Service.Spawn` enforces the accepted order:

1. validate parent state;
2. validate delegated authority subset;
3. validate available hierarchical budget;
4. enforce depth/fan-out/deadline/result-contract bounds;
5. atomically persist spawn + reserve parent budget;
6. only then create the child Agent Process;
7. bind the child account and enqueue it for admission.

A child cannot become READY before its budget reservation exists durably.

### Hierarchical authority and budgets

Each orchestration account persists:

- delegated capability grants;
- available budget;
- reserved budget;
- root orchestration policy.

Child authority can only narrow parent authority. Child budget can only be carved from currently available parent budget. Settlement is idempotent and returns unused reservation to the parent.

### Result / evidence contract

Each spawn durably stores:

- `ResultContract`;
- optional deadline;
- bounded inline result size;
- whether evidence references are mandatory.

Settlement rejects:

- non-terminal child results;
- result summaries above the delegated bound;
- missing evidence when evidence is required;
- usage exceeding the reservation.

Large artifacts remain object references instead of being copied into hot orchestration state.

### Bounded durable messaging

G5 adds durable per-agent mailboxes with:

- stable idempotent `MessageID`;
- parent/child and sibling communication only within the same root tree;
- message classes for request/response/evidence/status/result/cancel;
- bounded message count and aggregate bytes;
- explicit backpressure;
- object references for large payloads;
- prioritized terminal/result delivery;
- explicit consume semantics.

### Durable parent waits

Supported wait modes:

```text
ONE
ALL
FIRST_SUCCESS
QUORUM
```

with failure policies:

```text
FAIL_FAST
COLLECT_ALL
BEST_EFFORT
```

Wait objects and wait edges are persisted in SQLite. Parent process state uses the existing durable `WAITING_CHILD` process state and can be reconstructed after runtime restart.

`ChildTerminal` / `SettleAndNotify` reconciles durable waits and wakes eligible parents without requiring an in-memory waiter goroutine.

### Wait-cycle prevention

Before a new durable wait edge is committed, G5 performs a bounded traversal of the persisted wait graph. A proposed cycle is rejected with structured conflict semantics.

### Cancellation tree

`CancelTree` walks persisted descendants and requests cancellation descendants-first, then the selected subtree root. Repeated cancellation is idempotent because terminal/already-cancelled processes are skipped.

### Fan-out, depth, parallelism and fairness

Root policy persists and enforces:

- maximum total descendants;
- maximum children per node;
- maximum recursive depth;
- maximum active children per root;
- mailbox limits.

Admission is persisted and uses deterministic round-robin selection across roots while respecting each root's `MaxParallelism`, preventing one recursive storm from monopolizing all admitted child slots.

### Duplicate work suppression

A stable task key permits opt-in reuse of a previously completed, settled child result.

Reuse occurs only when explicitly requested and the previous child is durably COMPLETED with a structured result. `IndependentVerification` explicitly disables reuse so redundant verification remains possible.

### Runtime control integration

The daemon constructs the orchestration service at startup and exposes local control messages for:

```text
orchestration.bootstrap
orchestration.spawn
orchestration.wait
orchestration.send
orchestration.mailbox
orchestration.consume
orchestration.settle
orchestration.child_terminal
orchestration.cancel_tree
orchestration.admit
```

The terminal/client remains a disposable caller; canonical orchestration state stays in SQLite/Object Store/process ledger state.

---

## Persistence model

Migration `0005_recursive_orchestration.sql` adds durable tables for:

- orchestration accounts;
- spawn reservations;
- bounded messages;
- waits and wait edges;
- structured child results;
- admission queue state.

Budget reservation and spawn recording occur in one SQLite transaction. Rejection releases the reservation transactionally. Settlement atomically releases unused reservation and records actual usage.

---

## Validation matrix

G5 includes direct tests for the A0 recursive-orchestration invariants:

| Contract | Validation |
|---|---|
| ORCH-001 | child authority cannot exceed parent authority |
| ORCH-002 | budget is reserved before child READY |
| ORCH-003 | 1,000 sleeping children do not imply 1,000 resident goroutines |
| ORCH-004 | stable `MessageID` makes duplicate delivery idempotent |
| ORCH-005 | mailbox count/bytes are bounded with explicit backpressure |
| ORCH-006 | wait cycle is rejected before graph commit |
| ORCH-007 | cancellation propagates through descendants and is idempotent |
| ORCH-008 | large child result is returned by object reference + bounded summary |
| ORCH-009 | fan-out limit is deterministic |
| ORCH-010 | durable parent wait survives SQLite close/reopen |
| ORCH-011 | fail-fast wait wakes parent on terminal child failure |
| ORCH-014 | fair admission prevents one root from starving another |
| ORCH-015 | duplicate completed work is reused only when explicitly allowed |

Additional G5 tests verify:

- evidence-required result contracts;
- max inline result enforcement;
- persisted `MaxParallelism` enforcement and slot reuse;
- the full G5 killer scenario with three child investigators, runtime-store reopen, structured evidence results and parent wake-up only after the wait condition is satisfied.

### Contract tests intentionally belonging to later generations

The A0 document also names:

- **ORCH-012** bounded negotiation rounds → implemented in **G11 — Adaptive Teams + Agent Negotiation**;
- **ORCH-013** selective child knowledge promotion → implemented in **G9 — Epistemic Memory + Truth Maintenance**.

They are intentionally not pulled forward into G5 because doing so would collapse the accepted generation boundaries.

---

## CI validation

Primary code-validation run:

```text
GitHub Actions run 33214615854
```

The run executes:

```text
go test ./...
go vet ./...
go build ./cmd/go-agent ./cmd/go-agentctl
go test -race ./...
```

across Ubuntu, macOS and Windows for the platform job matrix, with the race detector on Ubuntu.

The final G5 merge must only occur after the current branch head is green.

---

## Killer scenario

`TestG5KillerThreeInvestigatorsSurviveRestartAndReturnStructuredEvidence` proves the generation goal:

1. root creates three bounded delegated investigators;
2. each child receives separate Task Intent, authority and budget;
3. root enters a durable `WAITING_CHILD` wait on all three;
4. SQLite is closed and reopened;
5. the wait and children are reconstructed from durable state;
6. children complete and settle bounded structured reports with evidence references;
7. terminal reconciliation wakes the parent only when the wait condition is satisfied;
8. no child transcript is imported into parent hot context as the result contract uses summary/artifact/evidence references.

---

## G5 result

The implementation satisfies the G5 roadmap deliverables:

```text
spawn validate/reserve/create
child Task Intent + authority subset
result/evidence contract
bounded durable messaging
parent durable waits
cancellation tree
wait-cycle detection
fan-out/depth/parallelism/fairness controls
hierarchical budget reservation + settlement
explicit duplicate-work reuse
runtime control integration
restart-safe killer scenario
```

Once the final CI head is green, **G5 is COMPLETE and G6 — Cognitive Scheduler v0 is READY.**
