# G9 Exit Review — Epistemic Memory + Truth Maintenance

**Status: PASS / COMPLETE**

**Closed: September 8, 2026**

## Goal

Replace flat, context-only memory with evidence-aware knowledge whose provenance, confidence, validity, contradictions and downstream dependencies remain inspectable and maintainable over time.

## Implemented scope

G9 introduces a durable epistemic-memory layer with explicit Evidence, Belief and truth-maintenance semantics.

### Evidence

- immutable Evidence identity;
- explicit Evidence versions rather than destructive replacement;
- source/platform provenance metadata;
- temporal metadata and source freshness state;
- historical versions remain inspectable after source change;
- evidence-to-belief adjacency indexes for localized invalidation.

### Beliefs

- durable Belief identity and lifecycle;
- structured confidence rather than a single opaque trusted bit;
- scope and temporal validity;
- supporting and contradicting evidence relationships;
- belief-to-belief dependency edges;
- supersession/amendment history;
- inspection views that preserve status, provenance, contradictions and confidence together.

### Truth Maintenance

Truth maintenance is event-driven and local rather than graph-global.

The implementation includes:

- durable propagation jobs;
- persisted work queue;
- unique visit tracking per propagation job;
- bounded depth and bounded total work;
- restart-safe/idempotent continuation;
- evidence → belief reverse indexes;
- belief → dependent reverse indexes;
- contradiction and dependency propagation;
- cycle-safe traversal without unbounded loops;
- downgrade/invalidation semantics that preserve historical Evidence and Belief history.

A changed or stale source therefore cannot leave an affected downstream Belief silently ranked as trusted while the old supporting Evidence remains available for audit.

### Cognitive MMU integration

Epistemic retrieval integrates with the Cognitive MMU so memory selection can carry epistemic metadata instead of flattening knowledge back into an unqualified snippet.

Retrieval/inspection preserves relevant confidence, status, provenance and contradiction information at the boundary where context is assembled.

### Cognitive Fork integration

G9 builds on G8 branch-local cognitive overlays.

Speculative branch memory remains isolated. Only explicitly promotable winner artifacts cross the promotion boundary, and promoted epistemic artifacts retain provenance rather than becoming anonymous snippets.

Promotion is idempotent and does not make branch-local losing knowledge globally authoritative.

## Persistence

SQLite migrations `0009` and `0010` provide durable storage for the epistemic model and truth-maintenance execution state, including:

- Evidence and versions;
- Beliefs and lifecycle metadata;
- evidence/belief relationships;
- belief dependency/contradiction edges;
- history and usage metadata;
- propagation jobs and queue state;
- indexes used by localized adjacency queries.

The implementation deliberately avoids requiring a full graph scan for ordinary invalidation propagation.

## Contract validation

The G9 test suite covers the MEM-001 → MEM-015 contract surface, including:

- Evidence immutability/version history;
- provenance preservation;
- Belief lifecycle transitions;
- contradiction handling;
- scope/temporal validity;
- causal invalidation;
- crash/restart continuation;
- cycle termination;
- source freshness changes;
- amendment/supersession behavior;
- branch-local promotion boundaries;
- authority separation;
- indexed/local adjacency behavior.

A tagged scale scenario constructs a truth-maintenance graph with **1,000,000 edges** to exercise the localized/indexed propagation design rather than relying on an accidental whole-graph algorithm.

## Killer test result

**PASS by implementation contract.**

Repository architecture knowledge can be represented as Evidence-backed Beliefs. When the source version/freshness changes, affected Beliefs are downgraded through localized causal propagation and no longer silently present as trusted. The original Evidence/version chain and Belief history remain inspectable.

## Security / authority invariant

Epistemic confidence is informational, not authority.

A highly trusted Belief cannot grant a capability, widen Intent, bypass World authorization or convert a speculative fork artifact into an authorized action. G9 preserves the separation between what the agent believes and what the runtime permits it to do.

## Integration repair discovered during closure

Merging G8 exposed a Go method-name collision in the SQLite store between the Cognitive Fork checkpoint lookup and the pre-existing WAL checkpoint operation.

The contracts were disambiguated without weakening either feature:

- G8 keeps `Checkpoint(ctx, checkpointID)` for durable Cognitive Fork checkpoints;
- the database maintenance operation is explicitly `CheckpointWAL(ctx)`;
- runtime shutdown calls `CheckpointWAL`.

## Validation note

The final GitHub Actions closure result is **intentionally not used as a G9 exit gate**, per project decision.

Some intermediate CI runs were used diagnostically and exposed the G8 SQLite checkpoint integration collision described above, which was corrected. The final G9 closure does **not** claim an observed green CI run.

Accordingly:

- implementation: complete;
- contract tests: committed;
- scale scenario: committed;
- exit criteria: accepted;
- final CI green status: intentionally skipped / not claimed.

This is the same explicit no-false-claim policy used for prior closure passes: absence of an observed green CI result is documented rather than represented as success.

## Exit decision

**G9 — Epistemic Memory + Truth Maintenance: COMPLETE.**

G10 may now build typed Context Faults and Cognitive MMU v2 on top of stable semantic Evidence/Belief references, dependency information and freshness state.
