# Context Faults and Cognitive Paging

## Status

**A0 architecture contract — ACCEPTED semantic baseline.**

This document extends `COGNITIVE_MMU_V0_ALGORITHM.md` and defines how missing knowledge becomes an explicit runtime event rather than an accidental model failure.

The key constraint is deliberate:

> A Context Fault is handled at an agent/model invocation boundary. v0/v1 does not depend on provider-specific mid-token inference suspension/resume.

That keeps the semantic model provider-independent and replayable.

---

# 1. Why Context Faults exist

A long-lived Agent Process cannot keep every historically useful item in the active LLM context.

The runtime therefore needs a first-class answer to:

```text
The process needs knowledge K
but K is not materialized in the current working set.
```

Without a runtime primitive, agents repeatedly:

- re-read files;
- issue ad-hoc searches;
- forget that prior evidence exists;
- duplicate tokens;
- depend on lossy conversation summaries;
- become less reliable as history grows.

The Cognitive MMU turns this into managed paging.

---

# 2. Cognitive address space

The runtime exposes stable semantic references rather than prompt offsets.

Canonical reference families:

```text
ctx://<page-id>          durable Context Page
belief://<belief-id>     Epistemic Memory belief
evidence://<evidence-id> raw/verifiable evidence
object://<object-id>     large immutable object/artifact
event://<event-id>       Event Ledger event
checkpoint://<id>        recoverable process/world point
agent://<agent-id>       another Agent Process
```

The reference is stable even when its materialized text representation changes.

A model does not receive arbitrary storage paths or SQLite IDs as the public cognitive ABI.

---

# 3. Fault classes

Context Faults are typed.

## `ReferenceFault`

The process dereferences a known semantic reference that is not currently materialized.

Example:

```text
belief://auth/session-storage
```

## `RecallFault`

The process asks for knowledge by query rather than ID.

Example:

```text
recall("decision about session rotation")
```

## `EvidenceFault`

A belief/decision is present, but policy requires original evidence before a high-confidence or high-risk action.

## `FreshnessFault`

The referenced knowledge exists but is stale relative to freshness policy or changed source state.

## `DependencyFault`

A selected page/belief depends on another item required to interpret or validate it.

## `RepresentationFault`

The object exists but cannot fit/usefully enter context in its current representation and needs a range, summary, projection or query.

---

# 4. Fault lifecycle

```text
DETECTED
   ↓
VALIDATING
   ├── denied scope/authority → DENIED
   ├── invalid reference      → UNRESOLVED
   └── valid                  → RESOLVING
                                  ↓
                              MATERIALIZING
                                  ↓
                              BUDGETING
                              ├── cannot fit → NEEDS_COMPACTION/PROJECTION
                              └── fits       → RESOLVED
                                                  ↓
                                             PIN/LEASE
                                                  ↓
                                             NEXT INVOCATION
```

Fault resolution is durable enough to explain/replay the next invocation but does not require persisting large page bodies in the Event Ledger.

---

# 5. Provider-independent resume semantics

A Context Fault never assumes that a provider can suspend an inference stream and resume its hidden KV state later.

The normal flow is:

```text
model invocation N
  ↓
model emits recall/reference syscall
  ↓
provider tool-call boundary / invocation ends
  ↓
MMU resolves fault
  ↓
ContextManifest N+1 contains resolved pages
  ↓
model invocation N+1 continues task
```

This works with:

- OpenAI-style tool calls;
- Anthropic-style tool calls;
- local OpenAI-compatible servers;
- future providers;
- deterministic replay.

Provider-specific inference interception can be an optimization later, never a correctness requirement.

---

# 6. Fault request

```go
type ContextFaultRequest struct {
    ID            FaultID
    AgentID       AgentID
    InvocationID  InvocationID
    Kind          FaultKind
    Reference     *CognitiveRef
    Query         *RecallQuery
    Purpose       FaultPurpose
    RequiredBy    []CognitiveRef
    MaxTokens     int
    Urgency       FaultUrgency
}
```

`Purpose` is important for retrieval quality and security:

```text
continue_reasoning
verify_claim
prepare_action
resolve_contradiction
inspect_history
satisfy_dependency
```

---

# 7. Scope and authority

Retrieval is not automatically authorized merely because an object exists.

Resolution pipeline:

```text
reference/query
   ↓
agent memory/context visibility scope
   ↓
project/world/user boundaries
   ↓
secret/sensitive-content policy
   ↓
allowed candidate set
```

A child cannot fault in parent-private pages unless explicitly visible to its delegated scope.

A Context Fault never mints authority.

---

# 8. Page leases

Resolved pages enter the working set through a bounded **Context Lease**.

```go
type ContextLease struct {
    PageID        PageID
    AgentID       AgentID
    Reason        LeaseReason
    ValidForTurns uint16
    ExpiresAt     *time.Time
    HardPin       bool
}
```

Default policy:

```text
explicit recall/reference → next 1 invocation
active unresolved task ref → renewed while referenced
root intent/security      → hard managed pin
```

Pages are not permanently pinned just because they faulted once.

---

# 9. Page granularity decision

**Accepted A0 baseline:** logical semantic units first, bounded by token size second.

Do not use fixed byte/token chunks as the primary abstraction.

Preferred segmentation:

```text
source code      → symbol/function/type/section, then split if large
documentation    → heading/semantic section
conversation     → logical turn/block
tool output      → summary + indexed ranges/windows
logs             → time/error clusters/windows
decisions        → one decision object
beliefs          → one belief object + evidence refs
```

Generic target for text pages:

```text
preferred: ~800–2,000 estimated tokens
hard normal-page ceiling: ~4,000 tokens
```

Objects larger than that use ranges, child pages, summaries or an execution/query World.

These numbers are tuning defaults, not protocol invariants.

---

# 10. Cognitive page hierarchy

A large object can expose a page tree:

```text
object://repo/auth-service
  ├─ page summary
  ├─ page symbol Login
  ├─ page symbol Refresh
  ├─ page symbol Revoke
  └─ page tests
```

The MMU can first load the summary and fault in a narrower child page only if required.

This is analogous to hierarchical page tables only as an intuition; the implementation remains semantic.

---

# 11. Working-set locality

The MMU should exploit three forms of locality.

## Temporal locality

Recently useful pages are likely useful again soon.

## Task locality

Pages referenced by the current task/plan/acceptance criteria have priority.

## Dependency locality

A page's direct evidence/dependency neighbors may become useful together.

The runtime MAY prefetch a small bounded neighbor set when historical measurements show benefit.

Prefetch never overrides hard token budgets.

---

# 12. Fault budget

Every invocation chain has explicit fault limits.

```go
type FaultBudget struct {
    MaxFaultsPerTurn       int
    MaxFaultsPerTaskWindow int
    MaxMaterializedTokens  int
    MaxResolutionTime      time.Duration
    MaxRepeatedSetHits     int
}
```

A model cannot produce an infinite `recall → recall → recall` loop.

Exhaustion returns structured state:

```text
ContextFaultBudgetExceeded
```

The harness then changes strategy, compacts, delegates, asks the user, or fails explicitly.

---

# 13. Fault-storm detection

A fault storm occurs when the process repeatedly swaps pages without making observable task progress.

Signals:

- same query repeated;
- same page set loaded/evicted repeatedly;
- >N faults with no new evidence/action/plan transition;
- working set churn above threshold;
- repeated RepresentationFault for same object.

Response options:

```text
increase summary abstraction
create task-specific synthesis page
spawn focused child
query object externally
stop and surface diagnostic
```

Do not simply increase context size automatically.

---

# 14. Eviction

Eviction removes a page from hot working context only.

Never delete durable knowledge as an eviction side effect.

Eviction score considers:

```text
pin/lease status
active references
importance
task relevance
recency
reload cost
representation size
source diversity
```

A high reload-cost page can receive mild retention bias, but correctness is not dependent on cache residency.

---

# 15. Dirty cognitive state

Context Pages themselves are immutable/versioned references where practical.

When a source changes:

```text
old page remains historically addressable
new page/version created
freshness/index points to new version
belief dependencies may become stale
```

The MMU does not mutate old evidence in place.

This is crucial for replay and Epistemic Memory.

---

# 16. Structured compaction and paging

Compaction is not destructive summarization.

```text
raw pages
   ↓
summary page S
   ↓
S references raw page IDs
   ↓
raw pages become cold
```

If later reasoning requires detail:

```text
summary assertion
   ↓
EvidenceFault / ReferenceFault
   ↓
raw page-in
```

This lets the runtime compress active cognition while preserving auditability.

---

# 17. Retrieval indexes

Accepted v0 architecture:

```text
SQLite metadata + FTS lexical index
+
explicit graph/reference lookup
+
optional embedding adapter
```

Embeddings are an optional ranking signal, not the identity/store of memory.

Candidate retrieval must be bounded before page bodies are loaded.

A 100k-page corpus must not require scanning 100k blobs per invocation.

---

# 18. Context Manifest additions

Each resolved fault appears in the next invocation manifest:

```yaml
faults:
  - id: fault_123
    kind: EvidenceFault
    requested: belief://auth/session-storage
    resolved_pages:
      - ctx://page_91
      - evidence://ev_44
    tokens: 1780
    lease: next_invocation
    reason: verify_claim
```

This answers:

> Why was this information in the model context?

---

# 19. Context fault events

Canonical lifecycle events are coarse:

```text
ContextFaultDetected
ContextFaultDenied
ContextFaultUnresolved
ContextFaultResolved
ContextLeaseGranted
ContextLeaseExpired
ContextFaultLoopDetected
```

Do not event-source every lexical candidate score as separate events. Detailed manifests can be object-backed.

---

# 20. Performance invariants

```text
CF-P01 fault resolution memory is bounded by candidate window + selected bodies
CF-P02 historical corpus size does not determine hot heap size
CF-P03 candidate retrieval is indexed
CF-P04 page body loading is lazy
CF-P05 fault queues are bounded
CF-P06 no one fault can pin arbitrary unbounded content
```

---

# 21. Failure semantics

## Storage unavailable

Fault remains unresolved; no fabricated memory enters context.

## Referenced page corrupted

Mark object integrity failure and surface `ContextObjectCorrupt`.

## Token estimator unavailable

Use conservative fallback or reject if safe budgeting impossible.

## Embedding backend unavailable

Fall back to lexical/graph retrieval if policy allows.

## Page source changed during resolution

Resolve against versioned object; freshness metadata can trigger follow-up FreshnessFault.

---

# 22. Required tests

```text
CF-001 explicit reference resolves only visible page
CF-002 fault never bypasses scope/security filter
CF-003 resolved pages obey next-invocation token budget
CF-004 repeated fault set triggers loop/storm protection
CF-005 100k historical pages do not become resident during one fault
CF-006 provider switch does not change fault semantics
CF-007 compaction summary can fault back to raw evidence
CF-008 stale page triggers freshness path when policy requires
CF-009 inaccessible evidence does not get copied through summary
CF-010 crash after FaultResolved but before invocation can reconstruct same manifest refs
CF-011 oversized object produces RepresentationFault/projection rather than silent truncation
CF-012 page lease expiration removes hot pin without deleting durable object
```

---

# 23. Innovation boundary

The innovation is not merely calling retrieval “virtual memory”.

The stronger architecture is the combination of:

```text
stable cognitive references
+ bounded working set
+ typed context faults
+ context leases
+ hierarchical semantic pages
+ structured compaction with evidence links
+ observable manifests
+ fault-storm control
+ provider-independent resume semantics
```

That combination turns context management into a runtime subsystem with explicit guarantees.

---

# Accepted A0 decisions

1. Context Faults happen at invocation/tool boundaries, not provider-specific mid-token suspension.
2. Explicit `recall`/semantic references are the correctness path; automatic inference-time detection is optional later.
3. Cognitive references are stable and storage-independent.
4. Page granularity is semantic-first, with ~800–2,000 token preferred pages and ~4k normal ceiling as tuning defaults.
5. Fault resolution uses bounded leases rather than permanent pins.
6. Retrieval v0 is lexical/graph first; embeddings are optional.
7. Original evidence remains retrievable after compaction.
8. Fault storms are a first-class bounded failure mode.

> **A Context Fault means “required knowledge is not hot”, never “knowledge was lost”.**
