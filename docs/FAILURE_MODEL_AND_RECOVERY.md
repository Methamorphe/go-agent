# Failure Model and Recovery Semantics

## Purpose

A durable autonomous runtime must be designed around failure, not around the happy path.

The system will routinely encounter:

- provider timeouts;
- disconnected streams;
- malformed model output;
- commands that hang;
- child agents that fail;
- SQLite busy/disk-full conditions;
- TUI crashes;
- runtime crashes;
- machine restarts;
- partial external side effects;
- stale memory;
- network partitions;
- corrupted or missing artifacts.

This document defines how those failures should be classified and recovered.

---

# 1. Failure domains

Failures belong to explicit domains.

## Presentation failure

Examples:

- terminal closed;
- TUI panic;
- client IPC disconnect;
- renderer bug.

Expected effect:

- client disconnects;
- durable agents continue;
- no canonical state loss.

## Model/provider failure

Examples:

- HTTP timeout;
- rate limit;
- provider 5xx;
- malformed stream;
- request cancelled;
- context too large;
- unknown model.

Expected effect:

- invocation fails/interrupted;
- scheduler may fallback/retry according to policy/effect semantics;
- Agent Process remains recoverable.

## Tool/World failure

Examples:

- command exit code != 0;
- process hangs;
- container dies;
- file permission denied;
- SSH connection lost.

Expected effect:

- action outcome recorded;
- transaction policy decides retry/rollback/escalation.

## Kernel/policy failure

Examples:

- invalid lifecycle transition;
- capability invariant violation;
- budget accounting mismatch;
- reducer inconsistency.

Expected effect:

- fail safely;
- do not execute external mutation;
- surface as internal invariant failure;
- preserve evidence for debugging.

## Storage failure

Examples:

- disk full;
- I/O error;
- corrupted database;
- missing object;
- failed fsync/finalization.

Expected effect:

- enter degraded/read-only/fail-stop behavior depending on severity;
- never continue silently with non-durable canonical mutations.

## Host failure

Examples:

- SIGKILL;
- power loss;
- OS reboot;
- OOM kill.

Expected effect:

- recover from durable boundary;
- classify in-flight work as interrupted/unknown where necessary.

---

# 2. Failure is data

Failures should be structured objects, not only strings.

Conceptual model:

```go
type Failure struct {
    Code          FailureCode
    Domain        FailureDomain
    Message       string
    RetryClass    RetryClass
    Outcome       OutcomeCertainty
    CauseRef      *EventID
    InvocationID  *InvocationID
    WorldActionID *ActionID
    DetailsRef    *ObjectRef
}
```

Important distinctions:

```text
retryable
not_retryable
retry_requires_reconciliation

and

outcome_known_failed
outcome_known_succeeded
outcome_unknown
```

---

# 3. The unknown-outcome problem

One of the most dangerous distributed-system cases:

```text
agent sends external mutation
        ↓
remote system applies it
        ↓
connection dies before response
```

The runtime cannot truthfully mark this `failed`.

It must record:

```text
OutcomeUnknown
```

Then use one of:

- idempotency key lookup;
- read-after-write verification;
- external resource reconciliation;
- human confirmation;
- compensating operation if safe.

Blind retry of an irreversible unknown-outcome action is forbidden.

---

# 4. Invocation recovery

Model invocation lifecycle:

```text
PLANNED
  ↓
STARTED
  ↓
STREAMING
  ↓
COMPLETED | FAILED | CANCELLED | INTERRUPTED
```

On runtime startup, a persisted `STARTED/STREAMING` invocation without terminal outcome is reconciled to `INTERRUPTED` unless a provider-specific resumable protocol proves otherwise.

Policy then decides whether to:

- retry same model;
- fallback model;
- rebuild context and re-invoke;
- ask user;
- fail task.

The logical agent remains intact.

---

# 5. Tool recovery

Tool/action records should track:

```text
requested
authorized
started
process/world identity
last durable known state
outcome
```

For local ephemeral OS processes after daemon crash, process liveness may be unknown.

Initial safe policy:

- do not assume process survived;
- reconcile known PID/process group only when platform-safe;
- classify action interrupted;
- inspect effect/idempotency before retry.

Container/remote Worlds may support stronger reconciliation by stable World action IDs.

---

# 6. Transaction recovery

Transaction states:

```text
OPEN
VERIFYING
READY_TO_COMMIT
COMMITTING
COMMITTED
ROLLING_BACK
ROLLED_BACK
FAILED
NEEDS_RECONCILIATION
```

Crash recovery semantics depend on state.

Examples:

### crash while OPEN

World changes remain isolated; transaction can resume or rollback.

### crash during COMMITTING

May require reconciliation to determine which promotion steps completed.

Commit design should minimize multi-step externally visible promotion.

### irreversible barrier reached

Record each irreversible action separately so recovery never assumes transaction rollback erased external reality.

---

# 7. Process recovery

At startup:

```text
open storage
validate schema/integrity
load active process projections
reconcile interrupted invocations/actions
reconstruct timer/wake scheduler
reconcile Worlds
mark runnable processes READY
resume scheduler
```

No TUI is required.

Recovery should be idempotent: running it twice should not duplicate child processes or external effects.

---

# 8. World recovery levels

Different Worlds offer different guarantees.

## Level 0 — ephemeral

State disappears with process/runtime.

Suitable only for pure/read work or disposable actions.

## Level 1 — reconstructable

Workspace can be recreated from source + known actions.

## Level 2 — snapshotable

World exposes durable checkpoints/snapshots.

## Level 3 — transactional/promotable

World supports isolated mutations and explicit commit/rollback semantics.

Each World advertises capabilities so the kernel does not pretend all Worlds are equally recoverable.

---

# 9. Retry policy

Retry decision matrix should combine:

```text
failure class
effect class
idempotency
outcome certainty
attempt count
deadline/budget
provider/tool health
```

Example:

| Effect | Outcome | Retry |
|---|---|---|
| Read | known failed | usually yes |
| Read | unknown | usually safe |
| Reversible | known failed before effect | yes |
| Reversible | unknown | reconcile first |
| Irreversible | known failed before send | possibly |
| Irreversible | unknown | never blind retry |

Retry loops MUST have bounded attempts/backoff and consume budget.

---

# 10. Circuit breakers / health

Repeated provider/worker failures should not cause thousands of immediate retries across agents.

Maintain health state per dependency:

```text
healthy
suspect
open/unavailable
recovering
```

Scheduler can route elsewhere or delay.

Initial implementation can be simple deterministic exponential backoff + failure counters.

---

# 11. Storage degradation modes

## Transient busy/lock

Bounded retry with busy timeout.

## Disk nearly full

Warn before critical threshold; reject large new artifacts according to policy.

## Disk full / canonical write impossible

Stop new mutating work. Preserve running external actions only long enough to cancel/reconcile safely.

## Corruption/invariant failure

Enter safe diagnostic/read-only mode where possible.

Never auto-delete history to “fix” corruption.

---

# 12. OOM prevention

The architecture should avoid relying on Go OOM recovery because process OOM may be unrecoverable.

Preventive controls:

- bounded buffers;
- streaming I/O;
- object store for large output;
- context budgets;
- child concurrency limits;
- cache size limits;
- TUI viewport virtualization;
- memory metrics/alerts;
- World resource limits.

Synthetic memory-pressure tests should be part of soak suites.

---

# 13. Partial data / truncation

Truncation must be explicit metadata, never silent.

Example tool result:

```yaml
preview_truncated: true
full_object_ref: object://...
bytes_seen: 92838192
preview_bytes: 65536
```

The agent can retrieve/search the full artifact later.

Similarly, if provider output is cut by token limit, distinguish it from normal completion.

---

# 14. Model protocol errors

Malformed tool calls / structured output should be treated as model-level protocol failures.

Possible repair loop:

```text
invalid structured response
   ↓
record validation errors
   ↓
retry/re-prompt within bounded repair budget
   ↓
fail/escalate after N attempts
```

Never pass malformed action parameters to a World just because they came from a trusted model provider.

---

# 15. Child failure semantics

A child failure does not automatically fail parent.

Child result contract can expose:

```text
COMPLETED
FAILED
CANCELLED
PARTIAL
NO_FINDING
```

Parent/harness decides:

- retry child;
- spawn alternative specialist;
- continue with partial evidence;
- escalate;
- fail root task.

Budget and deadline remain enforced.

---

# 16. Startup recovery budget

Startup should not replay years of events synchronously before accepting inspection.

Use:

- current projections;
- snapshots;
- bounded reconciliation set for only non-terminal work;
- lazy historical loading.

If recovery is large, runtime can expose progress while still allowing read-only inspection where safe.

---

# 17. Fault injection

Every major subsystem needs fault injection hooks in tests:

- fail DB write after N events;
- disk-full simulation abstraction;
- disconnect provider after N chunks;
- kill command mid-output;
- fail object finalization;
- panic provider adapter;
- TUI stop-reading;
- timeout World snapshot;
- crash between `Started` and `Completed`;
- crash during transaction commit.

Recovery behavior must be verified, not inferred.

---

# 18. Recovery invariants

After any recoverable crash:

1. no completed canonical event disappears;
2. no mutation is repeated blindly if its previous outcome is unknown;
3. process lineage remains intact;
4. budgets do not magically reset;
5. expired/revoked capabilities do not reappear;
6. transactions do not become committed merely because runtime restarted;
7. TUI history can be reconstructed independently;
8. interrupted work is visible to user/agent.

---

# 19. User-facing failure quality

The system should distinguish:

```text
"Tests failed"
```

from:

```text
"Runtime lost connection to the container while a reversible transaction was open. The transaction remains isolated and needs reconciliation."
```

The second is operational truth, not noise.

Reliability UX should tell users:

- what failed;
- what state is known;
- what may have changed;
- whether retry is safe;
- whether rollback is available;
- what the runtime will do next.

---

# Core invariant

> **The runtime must prefer an explicit `unknown/interrupted` state over pretending certainty it does not possess.**
