# G6 Exit Review — Cognitive Scheduler v0

## Status

**G6 — Cognitive Scheduler v0: COMPLETE.**

Implementation branch: `feat/g6-cognitive-scheduler-finish`  
Pull request: **#4 — `feat(g6): complete Cognitive Scheduler v0`**

G6 stops binding an Agent Process to a single model. Model selection is now a deterministic runtime scheduling decision over explicit task requirements, model profiles, health, budget, locality/privacy and bounded concurrency.

---

## Delivered runtime primitives

### Model registry and profiles

The scheduler owns bounded model Profiles with provider/model identity, context/output limits, capabilities, locality/data policy, cost, latency, reliability, task-quality estimates, concurrency and profile version.

### Cognitive Task descriptor

Every scheduled invocation is represented by a `CognitiveTask` carrying stable Agent/Root identity, task kind, estimated input/output tokens, hard requirements, objective, available budget, privacy policy and risk class.

Model identity never becomes Agent identity.

### Deterministic hard eligibility + scoring

Routing first rejects candidates that violate hard constraints: context, output capacity, capabilities, locality/privacy, provider/model allow/deny policy, health/circuit state, quality/reliability floor, finite budget, deadline and experimental policy.

Only hard-eligible candidates enter deterministic utility scoring across quality, cost, latency, failure risk and load. Stable tie-breaking keeps identical inputs deterministic.

### Provider health and fallback

Runtime telemetry tracks bounded health state, queue depth, latency and transient failures. Circuit states prevent repeated routing into unavailable/rate-limited backends. Fallback excludes already-failed candidates and rebuilds the invocation from the durable Cognitive Task/MMU working context rather than relying on provider-local conversation state.

### Root budget reservation and exact settlement

Scheduler budget accounting is now behind `scheduler.BudgetStore`.

The daemon uses the SQLite `Store` implementation, backed by migration `0006_cognitive_scheduler.sql`. Root limits, spent resources, active reservations and settled reservations survive daemon/database reopen.

Reservation is guarded transactionally against overspend. Settlement records actual usage exactly once; release returns unused reservation without fabricating spend. A second settlement is rejected.

This closes the restart hole where an in-memory scheduler budget could otherwise have reset after daemon restart.

### Bounded concurrency and fairness

Global, per-root and per-provider/model slot limits are enforced before invocation. Root capacity cannot be consumed without bound by a single Agent tree. Slot leases release exactly once.

### Runtime/daemon configuration

The daemon supports opt-in scheduler configuration with:

- OpenAI and OpenAI-compatible backends;
- configurable provider/model profiles;
- global/per-root/per-provider slots;
- default root budget;
- reserved output tokens;
- capability, context, quality, reliability, locality/privacy and cost metadata.

If `scheduler.enabled` is false, legacy G4 behavior remains unchanged.

See `COGNITIVE_SCHEDULER_V0.md`.

### MMU + invocation integration

G4's Cognitive MMU remains responsible for bounded context construction. G6 routes each model invocation over that bounded working set. Routing decisions are durably ordered with model invocation events and can change between steps without changing Agent identity.

---

## Validation matrix

| Contract | Validation |
|---|---|
| SCH-001 | privacy/local-only is a hard filter |
| SCH-002 | context capacity is a hard filter |
| SCH-003 | concurrent reservation cannot overspend root budget |
| SCH-004 | fallback rebuilds from Cognitive Task, not provider thread state |
| SCH-005 | circuit breaker prevents provider failure storms |
| SCH-006 | high-risk tasks enforce a quality floor |
| SCH-007 | queue pressure can change routing |
| SCH-008 | routing decisions are deterministic for identical inputs |
| SCH-009 | hedged loser is cancelled |
| SCH-010 | per-root slots preserve fair capacity |
| SCH-011 | shadow/advisory routing cannot bypass hard constraints |
| SCH-012 | no eligible model returns structured failure |
| SCH-013 | actual cost settles exactly once |
| SCH-014 | model switching preserves durable Agent identity |
| SCH-015 | 100-profile routing does not depend on conversation history |

Additional integration validation proves:

- routed provider fallback emits durable routing/invocation events in causal order;
- daemon configuration constructs multiple providers/profiles;
- SQLite + Process Ledger + Cognitive MMU + scheduler + fake provider execute end-to-end;
- active scheduler reservations survive SQLite close/reopen;
- already-spent scheduler budget survives a second reopen;
- overspend remains rejected after reopen;
- settlement after reopen preserves exact-once semantics;
- quality-first/cost-first/latency-first behavior is compared against strongest/cheapest/fastest static baselines.

---

## CI validation

GitHub Actions run `34160212580` on head `ba26110afe7cadb96832d8ab2df357e3bcd6d949` passed:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

The platform jobs passed `go test ./...`, `go vet ./...`, soak-scenario compilation where applicable, and `go build ./cmd/go-agent ./cmd/go-agentctl`. The race job passed `go test -race ./...`.

---

## Evaluation

The deterministic v0 router is evaluated against static strongest/cheapest/fastest choices using a controlled synthetic profile matrix. Objective-specific routing selects the expected baseline under quality-first, cost-first and latency-first objectives while balanced routing preserves an explicit tradeoff rather than hard-coding one provider.

No learned router is used in v0. Shadow/advisory proposals remain unable to weaken hard eligibility constraints.

---

## G6 result

**PASS.**

G6 satisfies its roadmap deliverables:

```text
model registry / Profile
Cognitive Task descriptor
hard eligibility filtering
deterministic cost/latency/quality/load scoring
provider health + circuit breaker
fallback
privacy/locality constraints
global/per-root/provider slots
routing decisions + metrics
durable root budget reservation/settlement
MMU + invocation integration
daemon/config integration
restart-safe scheduler budget accounting
baseline evaluation
cross-platform CI + race
```

**G6 is COMPLETE. G7 — Workspace/OCI World + Agent Transactions is READY.**
