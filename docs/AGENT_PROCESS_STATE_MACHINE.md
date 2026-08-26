# Agent Process State Machine

## Purpose

This document freezes the first precise lifecycle semantics of the core `Agent Process` primitive.

An Agent Process is a durable logical state machine. It is **not** a goroutine, OS process, provider thread, terminal session or one model invocation.

---

# 1. Identity

Every process has immutable identity fields:

```text
AgentID
RootAgentID
ParentAgentID?       # absent only for root
CreatedAt
CreationEventID
LineageDepth
```

`AgentID` never changes if model/provider/World changes.

A fork creates a **new AgentID** with fork lineage to the source checkpoint/process.

---

# 2. Logical states

Initial state model:

```text
NEW
READY
RUNNING
WAITING
SLEEPING
SUSPENDED
COMPLETED
FAILED
CANCELLED
```

`WAITING` carries a reason subtype instead of exploding the main enum:

```text
WAITING_MODEL
WAITING_TOOL
WAITING_CHILD
WAITING_APPROVAL
WAITING_RESOURCE
WAITING_RECONCILIATION
```

The canonical process state does not need to transition to `WAITING_MODEL` for every provider token. Waiting is at operation granularity.

---

# 3. State meanings

## NEW

Created durably but not yet eligible to execute.

Typical work:

- root intent binding;
- initial authority/budget validation;
- initial World/memory scope setup.

`NEW` should be brief.

## READY

Runnable work exists, but no execution lease is currently owned by a runtime worker.

A READY process may wait in scheduler queues without resident worker.

## RUNNING

A runtime execution lease owns the process's current step and may perform cognitive/kernel work.

`RUNNING` does not mean an LLM request is necessarily active.

## WAITING

Process cannot progress until an external or child/resource event occurs.

No dedicated goroutine is required merely to preserve this state.

## SLEEPING

Waiting on durable time/event condition that is explicitly scheduled.

## SUSPENDED

Administratively paused. Scheduler must not activate it until explicit resume.

Distinct from WAITING: a suspended process could otherwise be runnable.

## COMPLETED

Intent/task reached successful terminal outcome according to harness/acceptance semantics.

## FAILED

Terminal failure with structured reason/evidence.

## CANCELLED

Terminal user/parent/policy cancellation completed.

---

# 4. Terminality

Terminal states:

```text
COMPLETED
FAILED
CANCELLED
```

They cannot transition back to READY/RUNNING.

Continuing work after terminality requires:

- new Agent Process;
- fork/checkpoint branch;
- explicit future "reopen as new process" product operation.

This protects historical semantics.

---

# 5. Allowed primary transitions

```text
NEW       → READY
NEW       → FAILED
NEW       → CANCELLED

READY     → RUNNING
READY     → SUSPENDED
READY     → CANCELLED
READY     → FAILED

RUNNING   → READY
RUNNING   → WAITING
RUNNING   → SLEEPING
RUNNING   → SUSPENDED
RUNNING   → COMPLETED
RUNNING   → FAILED
RUNNING   → CANCELLED

WAITING   → READY
WAITING   → SUSPENDED
WAITING   → FAILED
WAITING   → CANCELLED

SLEEPING  → READY
SLEEPING  → SUSPENDED
SLEEPING  → FAILED
SLEEPING  → CANCELLED

SUSPENDED → READY
SUSPENDED → CANCELLED
SUSPENDED → FAILED
```

No other transition is valid in v0.

---

# 6. READY ↔ RUNNING execution lease

The durable state machine must avoid two runtime workers simultaneously acting as the same exclusive process step.

Concept:

```text
READY process version 42
       │
 scheduler attempts activation
       │
 atomic transition:
   READY@42 → RUNNING@43
   execution_lease = lease-X
       │
 worker owns lease-X
```

Lease metadata can include:

```text
LeaseID
RuntimeInstanceID
AcquiredAt
Heartbeat/expiry only if needed
ProcessVersion
```

For single local daemon G1, a simple atomic state/version transition is enough; explicit distributed lease behavior is reserved for G13.

On daemon restart, stale `RUNNING` state is reconciled based on in-flight work records rather than assumed to still have a worker.

---

# 7. Process version

Every accepted canonical transition increments a process stream/version.

```text
version 0  AgentCreated
version 1  IntentBound
version 2  Ready
version 3  Running
...
```

Commands specify expected version when they require exclusive current-state semantics.

Conflict:

```text
expected 10
actual   11
→ concurrency conflict
```

Caller/supervisor reloads and re-evaluates.

---

# 8. Operation state vs process state

Do not overload process lifecycle with every sub-operation.

Separate durable operation entities:

```text
ModelInvocation
WorldAction
Verification
Transaction
Approval
ChildWait
```

Example:

```text
Process: WAITING(reason=WAITING_MODEL, operation=inv-123)
Invocation inv-123: STREAMING
```

This keeps process state compact while preserving detailed recovery.

---

# 9. Cancellation state machine

Cancellation is not instantaneous state mutation if execution may still be running.

Use a durable cancellation request:

```text
CancelRequested
       │
       ├── signal active execution
       │
       ├── cancel pending children according to policy
       │
       └── reconcile actions
               │
               ▼
            CANCELLED
```

Process projection may carry:

```text
cancel_requested: bool
cancel_scope
cancel_event_id
```

A process may temporarily remain RUNNING/WAITING while cancellation is being settled.

Terminal `CANCELLED` means the runtime has finished its defined cancellation/reconciliation work.

---

# 10. Suspension semantics

Suspension means:

- no new model/tool/child work starts;
- active operation policy decides whether current operation is allowed to finish or is cancelled;
- durable state becomes SUSPENDED after reaching safe point.

Initial v0 policy can be simple:

```text
suspend = request cancellation of interruptible current operation, then suspend
```

Later policies may allow graceful “finish current step then suspend”.

---

# 11. Waiting semantics

A WAITING process stores:

```text
WaitingReason
OperationRef / ChildSet / ApprovalRef / ResourceRequest
Since
WakePolicy
```

Wake event must be idempotent.

Example child wait:

```text
wait condition:
  any | all | quorum
children:
  A B C
```

A child result event updates wait condition. When satisfied:

```text
WAITING → READY
```

---

# 12. Sleeping semantics

Sleep record:

```text
SleepID
AgentID
WakeAt?            # time based
WakeEventFilter?   # later event based
CreatedAt
Generation/version
```

Scheduler uses generation/version so stale timers cannot wake a re-slept/cancelled process.

Wake operation checks current process state + sleep ID before transition.

---

# 13. Completion semantics

Kernel does not decide product-level success by itself.

Harness asks to complete with:

```text
CompletionRequest
  summary artifact/ref
  acceptance evidence refs
  unresolved warnings
```

Kernel validates structural rules:

- process not cancelled;
- no mandatory transaction unresolved;
- policy-required approvals/verification satisfied;
- no illegal lifecycle transition.

Harness/domain verification determines whether task acceptance criteria are actually satisfied.

---

# 14. Failure semantics

`FAILED` includes a structured terminal FailureRef.

Failure is appropriate when process cannot continue under current intent/policy/budget.

Examples:

- unrecoverable storage-independent task failure;
- exhausted retries/budget;
- unreconcilable required World;
- explicit harness decision that task is impossible.

Transient operation failures should not immediately terminal-fail the process if recovery/fallback exists.

---

# 15. Parent/child terminal behavior

Child terminal state does not automatically determine parent state.

Parent receives a structured child result/failure notification and harness decides.

Parent cancellation normally propagates cancellation request to non-detached descendants.

Detached children are deferred design; v0 should **not** support arbitrary detached children.

This avoids orphan authority/budget complexity.

---

# 16. Root/child intent

Root process has immutable root Intent.

Child has:

```text
RootIntentRef
DelegatedTaskIntent
```

Child task intent must be compatible/narrower relative to parent/root policy.

Lineage preserves original root intent even after many recursive levels.

---

# 17. Recovery mapping

On daemon startup:

| Persisted state | Recovery action |
|---|---|
| NEW | validate initialization; move READY or fail |
| READY | enqueue runnable |
| RUNNING | reconcile in-flight operation/lease; usually clear stale lease then READY/WAITING/FAILED |
| WAITING | restore wait subscriptions/conditions; no worker required |
| SLEEPING | rebuild wake scheduler |
| SUSPENDED | leave inactive |
| terminal | no execution |

Recovery emits explicit reconciliation events where state changes.

---

# 18. Process snapshot

Snapshot projection should include only reconstructable current state, not huge artifacts.

```text
identity/lineage
version/status
intent refs
active capability refs
budget account refs
current wait/sleep metadata
active operation refs
World binding refs
memory/context scope refs
last checkpoint refs
```

Large conversation/context bodies are referenced, not embedded.

---

# 19. Process commands

Candidate command vocabulary separate from events:

```text
CreateAgent
InitializeAgent
ActivateAgent         # supervisor/internal
YieldAgent
WaitAgent
SleepAgent
SuspendAgent
ResumeAgent
RequestCancel
CompleteAgent
FailAgent
```

Commands are requests that may be rejected. Events are facts that happened.

Do not name commands/events identically if it blurs this distinction.

---

# 20. Required invariant tests

```text
P-SM-001 terminal state cannot return to RUNNING
P-SM-002 only one accepted activation for same expected version
P-SM-003 stale wake cannot revive cancelled/re-slept process
P-SM-004 restart of WAITING process requires no lost in-memory callback
P-SM-005 cancellation request survives restart
P-SM-006 child completion does not implicitly complete parent
P-SM-007 process identity unchanged after model switch
P-SM-008 snapshot + tail reproduces exact status/version/wait metadata
P-SM-009 suspended process is never scheduled
P-SM-010 thousands of SLEEPING processes do not imply thousands of resident goroutines
```

---

# Open points before G1 implementation

- exact naming of lifecycle events;
- whether `NEW` is externally visible or folded into create transaction;
- exact activation lease representation in single-daemon v1;
- cancellation “finish current step” option can remain deferred;
- event-based durable wake can follow time-based wake.

These are narrow implementation choices; the core lifecycle semantics above should remain stable.
