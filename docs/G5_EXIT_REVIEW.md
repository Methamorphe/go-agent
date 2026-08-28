# G5 Exit Review — Recursive Agent Processes

## Status

**G5 — Recursive Agent Processes: COMPLETE.**

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

Each orchestration account persists delegated capability grants, available budget, reserved budget and root orchestration policy.

Child authority can only narrow parent authority. Child budget can only be carved from currently available parent budget. Settlement is idempotent and returns unused reservation to the parent.

### Result / evidence contract

Each spawn durably stores its `ResultContract` and optional deadline. Settlement rejects non-terminal child results, oversized inline summaries, missing evidence when required, and usage above the reservation. Large artifacts remain Object Store references.

### Bounded durable messaging

G5 adds durable per-agent mailboxes with stable idempotent `MessageID`, same-root parent/child or sibling communication, typed message classes, bounded count/bytes, explicit backpressure, object refs for large payloads, terminal-result priority and explicit consume semantics.

### Durable parent waits

Supported wait modes are `ONE`, `ALL`, `FIRST_SUCCESS` and `QUORUM`, with `FAIL_FAST`, `COLLECT_ALL` and `BEST_EFFORT` failure policies.

Wait objects and wait edges are persisted in SQLite. Parent process state uses the existing durable `WAITING_CHILD` state and survives restart. `ChildTerminal` / `SettleAndNotify` reconciles waits and wakes eligible parents without one waiter goroutine per child.

### Wait-cycle prevention

Before a new durable wait edge is committed, G5 performs a bounded traversal of the persisted wait graph. A proposed cycle is rejected with structured conflict semantics.

### Cancellation tree

`CancelTree` walks persisted descendants and requests cancellation descendants-first, then the selected subtree root. Repeated cancellation is idempotent.

### Fan-out, depth, parallelism and fairness

Root policy persists and enforces maximum total descendants, maximum children per node, recursive depth, active parallelism and mailbox limits. Admission is persisted and round-robins roots while respecting each root's `MaxParallelism`.

### Duplicate-work suppression

A stable task key permits opt-in reuse of a previously completed, settled child result. `IndependentVerification` explicitly disables reuse.

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

Canonical orchestration state stays in SQLite/Object Store/process ledger state, never in the terminal client.

---

## Persistence model

Migration `0005_recursive_orchestration.sql` adds durable orchestration accounts, spawn reservations, mailboxes, waits/wait edges, structured child results and admission state.

Budget reservation and spawn recording occur in one SQLite transaction. Rejection releases reservation transactionally. Settlement releases unused reservation and records actual usage.

---

## Validation matrix

| Contract | Validation |
|---|---|
| ORCH-001 | child authority cannot exceed parent authority |
| ORCH-002 | budget is reserved before child READY |
| ORCH-003 | 1,000 sleeping children do not imply 1,000 resident goroutines |
| ORCH-004 | stable `MessageID` makes duplicate delivery idempotent |
| ORCH-005 | mailbox count/bytes are bounded with explicit backpressure |
| ORCH-006 | wait cycle is rejected before graph commit |
| ORCH-007 | cancellation propagates through descendants and is idempotent |
| ORCH-008 | large child result is returned by object ref + bounded summary |
| ORCH-009 | fan-out limit is deterministic |
| ORCH-010 | durable parent wait survives SQLite close/reopen |
| ORCH-011 | fail-fast wait wakes parent on terminal child failure |
| ORCH-014 | fair admission prevents one root from starving another |
| ORCH-015 | duplicate completed work is reused only when explicitly allowed |

Additional tests verify evidence-required result contracts, inline-result bounds, persisted `MaxParallelism`, slot reuse, and the complete three-investigator restart scenario.

The A0 catalog's **ORCH-012** negotiation test remains assigned to **G11**, and **ORCH-013** selective epistemic promotion remains assigned to **G9**. They are intentionally not pulled into G5 because that would violate the accepted generation boundaries.

---

## CI validation

GitHub Actions run `33214615854` passed:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

The platform jobs passed `go test ./...`, `go vet ./...` and `go build ./cmd/go-agent ./cmd/go-agentctl`; the race job passed `go test -race ./...`.

---

## Killer scenario

`TestG5KillerThreeInvestigatorsSurviveRestartAndReturnStructuredEvidence` proves the generation goal:

1. root creates three bounded delegated investigators;
2. each gets separate Task Intent, authority, budget and result contract;
3. root enters durable `WAITING_CHILD` on all three;
4. SQLite is closed and reopened;
5. wait and child state are reconstructed;
6. children complete and settle bounded reports with evidence refs;
7. parent wakes only when the wait condition is satisfied;
8. child transcripts are not copied into parent hot context.

---

## G5 result

**PASS.**

G5 satisfies its roadmap deliverables:

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

**G5 is COMPLETE. G6 — Cognitive Scheduler v0 is READY.**
