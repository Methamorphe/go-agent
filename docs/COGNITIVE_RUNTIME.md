# Cognitive Runtime: Context Virtualization and Memory

## Thesis

The LLM context window should be treated as a **scarce working set**, not as the canonical memory of the agent.

This document defines the proposed cognitive runtime around that idea.

## 1. Cognitive MMU

The Cognitive MMU is a runtime component responsible for deciding what information should be present in the model’s active context.

Analogy:

```text
Computer system                    Agent runtime
---------------                    -------------
CPU cache / RAM                    active LLM context
virtual memory                     context address space
pages                              context pages
page-in                            retrieve into prompt
page-out                           evict from prompt
page fault                         context fault
working set                        task-relevant knowledge
persistent disk                    event/memory/object stores
```

The analogy is useful, but the implementation is semantic rather than byte-addressed.

## 2. Context Page

A Context Page is the smallest independently managed unit of cognitive context.

Possible page types:

```text
source-code
file-summary
conversation-segment
decision
belief
tool-result
log-window
plan
child-report
API-schema
error-trace
documentation
world-state
```

Example metadata:

```yaml
id: ctx_01J...
type: source-code
source: repo://src/auth/service.go
scope: project
importance: 0.82
confidence: 1.0
size_tokens: 1450
last_accessed: ...
pinned: false
dirty: false
dependencies:
  - ctx_...
```

Pages may contain raw content, compressed content, summaries or references to external blobs.

## 3. Working set

Before a model invocation, the runtime constructs a working set:

```text
System/harness instructions
Current intent
Current task state
Pinned context
Relevant context pages
Relevant beliefs
Recent causal events
Required tool/syscall schemas
Reserved output budget
```

The working set is constrained by a token budget and model-specific context limits.

### Page ranking signals

Potential signals include:

- semantic relevance to current task;
- recency;
- explicit references from current plan;
- causal proximity;
- dependency edges;
- confidence;
- importance;
- source freshness;
- frequency of successful reuse;
- child-agent report priority;
- user pinning.

No single embedding similarity score should determine context.

## 4. Context Fault

A **Context Fault** occurs when the running agent requires knowledge that is not currently materialized in its working context.

Conceptual flow:

```text
Model references missing knowledge
            │
            ▼
      Context Fault
            │
            ▼
      Cognitive MMU
            │
    resolve references / query
            │
            ▼
     load relevant pages
            │
            ▼
    resume / new invocation
```

A context fault may be explicit:

```text
recall("previous auth migration decision")
```

or runtime-assisted when a symbolic reference is dereferenced:

```text
@decision/auth-session-strategy
```

The first implementation should probably favor explicit `recall()` semantics before attempting opaque automatic interruption/resumption of provider inference.

## 5. Page-in and page-out

### Page-in

Triggered by:

- `recall()` syscall;
- task transition;
- reference dependency;
- child report;
- verification failure;
- scheduler/context planner.

### Page-out

Candidates are evicted when:

- relevance decreases;
- their information is represented by a higher-level summary;
- the active model’s context budget changes;
- a new task supersedes the previous working set.

Eviction removes a page from the prompt, **not from durable memory**.

## 6. Pinning

Some context must remain hot for a period:

- immutable user intent;
- current acceptance criteria;
- critical safety constraints;
- active plan fragment;
- current transaction identity.

Pinned data still needs size limits; pinning everything defeats virtualization.

## 7. Context compaction

Instead of destructive “summarize the whole conversation and hope”, compaction can become structured.

Example:

```text
raw tool events
    ↓
extract durable facts
    ↓
beliefs / decisions / artifacts
    ↓
produce summary page
    ↓
evict raw pages from hot context
```

Raw history remains accessible through the Event Ledger.

This allows the agent to later revisit original evidence if a summary is questioned.

## 8. Epistemic Memory

The long-term memory system should store **beliefs**, not only text snippets.

A belief has semantics:

```go
type Belief struct {
    ID           BeliefID
    Statement    string
    Confidence   float64
    Scope        Scope
    Status       BeliefStatus
    Provenance   []EvidenceRef
    DependsOn    []BeliefID
    Contradicts  []BeliefID
    ValidFrom    time.Time
    ValidUntil   *time.Time
    LastVerified *time.Time
}
```

Example:

```yaml
statement: "Authentication sessions are stored in Redis"
confidence: 0.91
scope: repo:example/backend
provenance:
  - file: src/auth/session.go#L42-L90
  - file: docker-compose.yml
status: active
contradicts:
  - belief: planned-postgres-migration
```

## 9. Memory categories

A useful taxonomy discussed for the project:

### Working memory

Short-lived state required by the current task.

### Episodic memory

What happened in a specific prior session/task.

Example:

> The previous PostgreSQL migration failed because extension X was unavailable.

### Semantic memory

Facts about a project/world.

Example:

> This repository uses Go workspaces.

### Procedural memory

Reusable processes and skills.

Example:

> Release procedure: run A, then B, then C.

### Preference/project policy memory

Persistent project conventions that influence future work.

These should have explicit scope rather than being globally injected.

## 10. Provenance

Every durable belief should ideally point to evidence.

Evidence may include:

```text
file/version/line range
Git commit
tool output
API result
user message
test result
child-agent report
web source
runtime observation
```

This enables the runtime to distinguish:

```text
user assertion
vs
agent inference
vs
verified runtime fact
```

## 11. Confidence

Confidence is not meant to be a magical universal probability. It is metadata used for ranking, review and invalidation policies.

Possible sources of confidence:

- direct deterministic observation;
- test/verification evidence;
- independent sources agreeing;
- model inference without verification;
- stale evidence;
- conflicting evidence.

The runtime may lower confidence when dependencies become stale.

## 12. Causal invalidation / Truth Maintenance

This is a major innovation target.

Suppose:

```text
Belief A: API uses JWT
    │
    ├── Belief B: refresh-token endpoint is required
    └── Decision C: do not create server sessions
```

Then a repository change establishes:

```text
Belief A is no longer true: API migrated to server sessions
```

The system should not leave B and C silently active.

Instead:

```text
A invalidated
  ↓
B → needs review
C → needs review
```

This forms a knowledge dependency graph.

Potential statuses:

```text
active
contested
stale
invalidated
needs_review
superseded
```

## 13. Contradictions

When new evidence contradicts old memory, do not immediately overwrite history.

Preserve both claims:

```text
Belief old
Belief new
Contradiction edge
Evidence for each
Resolution state
```

A resolver may:

- compare timestamps;
- inspect source authority;
- run a verification task;
- ask a specialist child;
- ask the user if ambiguity is irreducible.

## 14. Object Store vs Memory Store

Not all external state is “memory”.

Large datasets should live as addressable objects:

```text
Object Store
  ├─ parsed logs
  ├─ ASTs
  ├─ test outputs
  ├─ search result sets
  ├─ generated files
  └─ binary artifacts
```

Memory can then reference objects rather than copying them into text.

Example:

```text
belief: "failure cluster #3 is caused by timeout"
evidence: object://analysis/log-cluster-3
```

## 15. Python and external computation

A persistent Python World is extremely useful for manipulating object-store data:

```python
logs = objects.load("object://logs/run-42")
errors = [x for x in logs if x.level == "ERROR"]
summary = aggregate(errors)
```

Only `summary` or selected rows may need to enter the LLM context.

This preserves the strongest Prime-style RLM capability without coupling the whole kernel to Python.

## 16. Context observability

Every model invocation should be inspectable:

```text
Invocation #932
model: ...
context budget: 64k
used: 41k

pages:
  intent                 1.2k pinned
  current plan           0.8k pinned
  auth/service.go        3.1k relevance .94
  auth decision          1.4k dependency
  test failure           2.2k recent
  child report           1.1k priority
  ...
```

This will be essential to debug why the agent forgot something or reached the wrong conclusion.

## 17. Avoiding a naive vector-memory trap

Embeddings are useful retrieval signals but should not become the whole memory architecture.

A robust retrieval decision can combine:

```text
semantic similarity
+
explicit dependency graph
+
causal history
+
scope
+
recency/freshness
+
confidence
+
task references
```

## 18. First implementation strategy

Do not attempt the perfect Cognitive MMU immediately.

### v0

- persisted context objects/pages;
- explicit `recall(query)`;
- simple ranking;
- token budget packing;
- pinned intent/current task;
- model invocation trace.

### v1

- page dependencies;
- structured summaries;
- epistemic beliefs;
- provenance;
- stale/contradiction tracking.

### v2

- causal invalidation;
- learned page-ranking policy;
- automated context faults;
- model-specific working-set optimization.

## Core objective

The agent should eventually be able to work for days without its context window growing with the duration of the task.

Its **knowledge may grow**, but its **active context remains bounded**.
