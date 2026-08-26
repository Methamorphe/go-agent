# Requirements

## Status

Design baseline derived from the initial product discussion. These requirements are expected to evolve as prototypes validate or invalidate assumptions.

The final product name is **TBD**.

---

# 1. Product requirements

## R-P01 — Not a Prime clone

The product MUST NOT be positioned or architected as a line-for-line Go rewrite of Prime Agent.

It SHOULD retain useful Prime-style ideas (persistent programmable state, recursive delegation, long-running work) while introducing a lower-level durable runtime model.

## R-P02 — Useful first as a developer agent

The first usable application SHOULD target developer/coding workflows because they provide objective verification signals:

- build;
- tests;
- lint;
- Git diff;
- benchmark;
- static analysis.

The kernel MUST remain domain-independent enough for research, DevOps and other autonomous agents later.

## R-P03 — Single-binary core

The core CLI/runtime SHOULD ship as a Go binary with minimal host dependencies.

Optional execution Worlds MAY require Docker, Python, SSH clients or other external runtimes.

## R-P04 — Local-first

The product MUST work locally without requiring a hosted control plane.

Distributed/remote execution MUST remain a later extension rather than a prerequisite.

## R-P05 — Inspectable autonomy

Users MUST be able to inspect active agents, children, budgets, actions and current state.

Autonomous behavior MUST NOT become an opaque background black box.

---

# 2. Agent-process requirements

## R-A01 — Durable identity

Every Agent Process MUST have a stable identifier independent from model provider sessions and OS process lifetime.

## R-A02 — Durable lifecycle

Agent Process lifecycle MUST be persisted.

A process MUST be resumable after runtime restart.

## R-A03 — Parent/child graph

Recursive agents MUST preserve parent/child relationships and lineage.

## R-A04 — Cancellation

Parent cancellation SHOULD propagate to children unless explicitly detached by a future policy.

Cancellation state MUST be durable, not only in a Go `context.Context`.

## R-A05 — Waiting/sleeping

The architecture MUST support agents that are inactive while waiting for time, a condition, a child or approval without requiring their full cognitive state in memory.

---

# 3. LLM/provider requirements

## R-L01 — Provider independence

Canonical agent state MUST NOT depend on proprietary provider thread/session objects.

## R-L02 — Streaming

Providers SHOULD support incremental model events/tokens.

## R-L03 — Cancellation/timeouts

Every model invocation MUST support cancellation and deadline enforcement where the provider permits it.

## R-L04 — Usage accounting

Token/cost metadata SHOULD be recorded whenever available.

## R-L05 — Local/OpenAI-compatible endpoints

The runtime SHOULD support OpenAI-compatible endpoints so local inference stacks such as vLLM/Ollama-like services can participate.

## R-L06 — Model switching

An Agent Process SHOULD be able to change model/provider between invocations without losing logical identity.

---

# 4. Tool and syscall requirements

## R-T01 — Kernel mediation

State-changing actions MUST pass through the runtime/kernel boundary.

## R-T02 — Stable syscall semantics

The kernel SHOULD expose a compact provider-independent action vocabulary.

## R-T03 — Native tools

Initial developer tools SHOULD include:

- filesystem read/list;
- controlled filesystem write;
- command execution;
- search/grep;
- Git inspection and eventually mutation.

## R-T04 — External tools

The architecture SHOULD support external tools through MCP and/or process/RPC adapters without granting them implicit unrestricted authority.

## R-T05 — Persistent Python execution

A persistent Python/IPython-like execution environment SHOULD be supported as an optional World because it is highly valuable for RLM-style data/context manipulation.

Python MUST NOT be required as the host kernel language.

---

# 5. Context requirements

## R-C01 — Bounded working context

The active LLM context MUST remain bounded by an explicit budget regardless of agent lifetime.

## R-C02 — Durable context objects

Information evicted from the prompt MUST remain retrievable when policy/storage permits it.

## R-C03 — Context trace

Every model invocation SHOULD expose what durable pages/memories were materialized and why.

## R-C04 — Explicit recall

The first context system MUST provide explicit retrieval/`recall` before attempting fully automatic context faults.

## R-C05 — Structured compaction

Compaction SHOULD preserve links between summaries and original evidence/history.

---

# 6. Memory requirements

## R-M01 — Scope

Memory MUST support scope (session/project/user/world/etc.) to prevent unrelated memories leaking into every task.

## R-M02 — Provenance

Important durable beliefs SHOULD retain evidence references.

## R-M03 — Freshness

Memory SHOULD distinguish fresh, stale, contested and invalidated information.

## R-M04 — Contradictions

New evidence MUST NOT silently overwrite conflicting durable knowledge without retaining history.

## R-M05 — Dependency/invalidation

Later versions SHOULD propagate staleness through belief dependency edges.

---

# 7. Subagent requirements

## R-S01 — Separate process state

A child MUST own independent logical process/context state.

## R-S02 — Authority subset

Child authority MUST be a subset of delegable parent authority.

## R-S03 — Budget subset

Child resource allocations MUST be bounded by parent/global budgets.

## R-S04 — Parallelism

Independent children SHOULD be able to run concurrently.

## R-S05 — Evidence-oriented results

Child reports SHOULD support structured findings and evidence references rather than only free-form text.

## R-S06 — Peer communication

Later versions SHOULD permit bounded peer-to-peer negotiation/challenge protocols.

---

# 8. Security requirements

## R-SEC01 — Least privilege

Agents and Worlds MUST default to the minimum authority needed.

## R-SEC02 — No prompt-only security

Critical permissions MUST be enforced outside the model.

## R-SEC03 — Secret isolation

Secrets SHOULD be injected directly into authorized tools/world processes without entering model-visible plaintext when possible.

## R-SEC04 — Network control

Worlds SHOULD support network deny/allow policies.

## R-SEC05 — Prompt injection containment

Untrusted content MAY influence reasoning but MUST NOT create new capabilities.

## R-SEC06 — Immutable root intent

The original bound user intent MUST NOT be silently rewritten by an agent or self-improvement system.

## R-SEC07 — Audit trail

Security-relevant grant/deny decisions MUST be recorded.

---

# 9. Effect and transaction requirements

## R-E01 — Effect declaration

Actions MUST declare effect metadata before execution.

## R-E02 — Irreversible boundary

Irreversible effects MUST NOT be speculatively executed as if they were reversible.

## R-E03 — Transaction support

Reversible work SHOULD be executable in an isolated transaction/world with commit/rollback semantics.

## R-E04 — Verification gates

Transactions/forks SHOULD support explicit verification before promotion.

## R-E05 — Compensating actions

External operations that cannot be rolled back but can be compensated SHOULD declare compensation semantics separately.

---

# 10. Fork requirements

## R-F01 — Cognitive + environmental fork

A fork SHOULD eventually clone both relevant agent state and World state.

## R-F02 — Isolation

Sibling forks MUST NOT mutate each other’s isolated World state.

## R-F03 — Evaluation

The runtime MUST record why one branch was promoted over another.

## R-F04 — Objective verification first

Tests/benchmarks/policies SHOULD be preferred over pure LLM judgement when available.

---

# 11. Scheduling requirements

## R-SCH01 — Explicit model policy

Each process/task SHOULD have a model-routing policy rather than a hard-coded model identity.

## R-SCH02 — Cost/latency/quality

Routing SHOULD consider cost, latency and expected quality.

## R-SCH03 — Privacy/locality

Routing MUST honor local-only/private-data constraints.

## R-SCH04 — Provider failure

The scheduler SHOULD support fallback where policy allows.

## R-SCH05 — Historical learning

Later versions MAY learn routing preferences from observed outcomes, but rule-based scheduling is preferred initially.

---

# 12. Persistence requirements

## R-D01 — Event ledger

Meaningful process transitions MUST be persistable as append-only events.

## R-D02 — Snapshots

The runtime SHOULD support snapshots to avoid replaying unbounded event histories during resume.

## R-D03 — Crash recovery

Unexpected process termination MUST NOT corrupt canonical logical state.

## R-D04 — Large artifacts

Large tool/model outputs SHOULD be stored outside the hot relational event rows and referenced by content/object IDs.

---

# 13. Observability requirements

## R-O01 — Process tree

The product SHOULD show parent/child/fork relationships.

## R-O02 — Causal trace

Users SHOULD be able to trace an action back to its intent, evidence, process and authorization.

## R-O03 — Resource metrics

Per-process and aggregate cost/tokens/time SHOULD be visible.

## R-O04 — Context inspector

The system SHOULD make context packing observable.

## R-O05 — Replay

Later versions SHOULD support replay/forking from historical checkpoints.

---

# 14. Self-improvement requirements

## R-I01 — Versioned improvements

Skills/prompts/routing/context policies modified by the system MUST be versioned.

## R-I02 — Evaluation before promotion

Permanent automatic promotion SHOULD require evidence that the candidate improves an objective or accepted metric.

## R-I03 — Rollback

Promoted cognitive artifacts MUST remain revertible.

## R-I04 — No authority expansion

Self-improvement MUST NEVER grant new capabilities or weaken immutable security policy.

---

# 15. Non-functional requirements

## Performance

- kernel overhead should remain negligible relative to model/tool latency;
- concurrency should scale to many waiting/streaming child processes;
- durable agents should not require memory proportional to lifetime history.

## Reliability

- storage transitions should be transactional;
- reducers should be deterministic;
- retries must respect effect/idempotency metadata;
- crash recovery is a core feature, not a later add-on.

## Portability

Primary development targets:

- macOS;
- Linux;
- Windows where practical.

World isolation capabilities may differ by platform.

## Maintainability

- small interfaces;
- standard library preferred when adequate;
- avoid premature framework abstractions;
- kernel semantics documented with invariants;
- tests for policy/security/state transitions without requiring an LLM.

## UX

The eventual TUI SHOULD be:

- fast;
- keyboard-first;
- clear about parallel agent activity;
- explicit about approvals/security boundaries;
- able to attach/detach from durable sessions;
- able to inspect reasoning artifacts without dumping raw internal noise by default.

---

# 16. MVP definition

The smallest architecture-valid MVP is not “chat with a model”.

It is:

```text
1. create durable Agent Process
2. bind immutable Intent
3. invoke one model through provider abstraction
4. execute filesystem/process actions through kernel
5. record events
6. enforce minimal capabilities/effects
7. persist context artifacts
8. terminate runtime completely
9. resume process
10. continue task correctly
```

A stronger developer MVP adds:

```text
11. recursive child process
12. explicit child budget
13. bounded context/recall
14. inspectable process tree
```

At that point, the project already demonstrates a runtime identity distinct from a conventional chat-agent CLI.
