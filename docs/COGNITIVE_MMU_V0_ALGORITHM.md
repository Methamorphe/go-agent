# Cognitive MMU v0 — Working-Set and Recall Algorithm

## Purpose

The Cognitive MMU should begin with deterministic, observable behavior.

v0 does **not** attempt magical automatic retrieval during model inference. It provides:

- durable Context Pages;
- explicit `recall()`;
- deterministic page ranking;
- hard token budgets;
- structured compaction hooks;
- complete context manifests.

The goal is to prove bounded long-session cognition before adding learned ranking or automatic Context Faults.

---

# 1. Invocation context budget

Every model invocation receives:

```text
model_max_context
reserved_output_tokens
provider/system overhead estimate
safety margin
```

Compute:

```text
input_budget = model_max_context
             - reserved_output_tokens
             - provider_overhead
             - safety_margin
```

The working-set builder is forbidden from producing a request estimated above `input_budget`.

If mandatory pinned content alone exceeds budget, invocation fails with a structured `ContextBudgetImpossible` error rather than truncating critical intent/security text silently.

---

# 2. Context classes

Pack in explicit classes.

## Tier 0 — mandatory/pinned

Examples:

- system/kernel-visible harness instructions;
- immutable root intent summary;
- current delegated task;
- critical security constraints relevant to agent-visible behavior;
- current transaction/fork identity if needed;
- current acceptance criteria.

Tier 0 is size-controlled at creation time.

## Tier 1 — active state

- current plan/task phase;
- latest user interaction;
- unresolved errors;
- active tool/child summaries;
- recent decision state.

## Tier 2 — explicitly recalled/referenced

Pages returned by `recall` or symbolic task references.

## Tier 3 — relevant durable pages

Automatically selected using deterministic v0 ranking.

## Tier 4 — recent conversational continuity

Recent finalized message blocks needed for natural continuity, subject to remaining budget.

Tool schemas/provider protocol overhead are separately accounted for.

---

# 3. Page metadata

Minimal v0 metadata:

```go
type ContextPageMeta struct {
    ID             PageID
    Type           PageType
    Scope          Scope
    SourceRef      *SourceRef
    ObjectRef      ObjectRef
    TokenEstimate  int
    Importance     float32
    Confidence     float32
    CreatedAt      time.Time
    LastAccessedAt time.Time
    PinnedUntil    *time.Time
    SupersededBy   *PageID
}
```

Later:

- dependencies;
- contradiction graph;
- learned reuse metrics;
- embeddings/vector index.

v0 can introduce embeddings only as one optional retrieval signal, not as correctness dependency.

---

# 4. Page types

Start with bounded useful types:

```text
UserMessage
AssistantSummary
SourceExcerpt
FileSummary
ToolResultSummary
Decision
Plan
ChildReport
ErrorTrace
Documentation
RecallResult
```

Avoid dozens of page types before observed need.

---

# 5. Scope filtering

Before ranking, filter candidate pages by allowed scope.

Possible scope hierarchy:

```text
process
root-task
project
workspace
user       later/sensitive
world
```

A child does not automatically retrieve every parent/private page. Memory/context scope and authority/policy determine visibility.

Security filtering happens **before** semantic ranking.

---

# 6. v0 retrieval query

`recall()` accepts typed query:

```go
type RecallQuery struct {
    Text        string
    Scope       []Scope
    Types       []PageType
    Limit       int
    MaxTokens   int
    ExplicitIDs []PageID
}
```

Explicit IDs/references rank highest if visible and valid.

Return:

```text
selected page refs
scores/reasons
unresolved refs
budget used
```

The model can then continue with these pages pinned for next invocation/limited lifetime.

---

# 7. Deterministic ranking v0

Do not pretend ranking is mathematically optimal.

Use a simple explainable score after hard filters:

```text
score =
    w_explicit   * explicit_reference
  + w_task       * lexical_or_semantic_task_relevance
  + w_recency    * recency_decay
  + w_importance * importance
  + w_confidence * confidence
  + w_type       * type_priority
```

Possible initial normalized weights are configuration, not product truth.

Important rules:

- explicit ref dominates;
- pinned pages bypass ranking;
- invalidated/superseded pages filtered or heavily downgraded;
- confidence does not override direct relevance;
- same source may be diversity-limited to avoid 20 near-duplicate chunks.

Every selected page records score components.

---

# 8. Semantic relevance v0

To avoid requiring embedding infrastructure in G4, start with one of:

- lexical BM25/FTS-style metadata/content index;
- simple token/term overlap;
- optional embedding adapter when configured.

SQLite FTS can be evaluated as local-first baseline.

The architecture must allow a later hybrid retriever:

```text
lexical
+
embedding
+
graph refs
+
causal proximity
```

without changing Context Page identity.

---

# 9. Packing algorithm

High-level deterministic algorithm:

```text
1. compute hard input budget
2. reserve tool/syscall schema budget
3. load Tier 0 mandatory pages
4. fail if Tier 0 cannot fit
5. add Tier 1 active-state pages by fixed priority
6. add explicit recall pages by requested priority
7. rank Tier 3 candidates
8. greedily pack while respecting:
     - remaining tokens
     - per-source diversity
     - page validity/scope
9. add recent conversation continuity if useful/budget remains
10. final conservative token re-estimate
11. evict lowest non-mandatory pages if estimate exceeds budget
12. emit ContextManifest
```

A more advanced knapsack/optimization algorithm can be evaluated later only if greedy packing demonstrably hurts quality.

---

# 10. Context manifest

Every invocation persists/references a manifest:

```yaml
invocation_id: inv_123
model: ...
input_budget: 60000
estimated_used: 43122
reserved_output: 8000
pages:
  - id: intent_...
    tier: 0
    reason: mandatory_root_intent
    tokens: 900
  - id: page_auth_service
    tier: 3
    score: 0.91
    reasons:
      task_relevance: 0.98
      recency: 0.60
      importance: 0.80
excluded:
  - id: page_old_log
    reason: budget_or_low_rank
```

Large manifest lists can be object-backed.

This is vital for debugging “why did it forget X?”.

---

# 11. Token estimates

Need model/provider-aware estimation interface:

```go
type TokenEstimator interface {
    EstimateText(model ModelID, text string) int
    EstimateRequestOverhead(model ModelID, requestShape RequestShape) int
}
```

v0 may use conservative generic estimators when exact tokenizer unavailable.

Rule:

> prefer under-utilizing context slightly over provider rejection from overflow.

Cache estimates keyed by object hash + estimator/model family with bounded cache/persisted metadata.

---

# 12. Large pages

If one page is larger than practical working-set budget, do not truncate invisibly.

Options:

- page segmentation/chunks;
- generated summary page with evidence link;
- explicit range retrieval;
- object-level search/query through Python/tool without loading all content.

Page metadata can reference parent object/source.

---

# 13. Recent conversation

Do not treat “last N messages” as whole memory strategy.

A bounded recent-window can preserve dialogue continuity, but old messages should become durable blocks/pages and summaries.

Possible v0 rule:

```text
always consider latest K logical turns
subject to Tier 0–3 budget priority
```

If a recent tool output is huge, only summary/preview page enters context.

---

# 14. Structured compaction v0

Trigger when raw conversational/tool pages exceed active-history policy.

Compaction pipeline:

```text
source pages
   ↓
extract durable facts/decisions/artifacts
   ↓
create summary page
   ↓
summary references source page IDs/object refs
   ↓
source pages become cold/non-default candidates
```

Compaction does not delete originals.

A summary generated by LLM is evidence-derived but not treated as perfect verified fact.

---

# 15. Recall lifecycle

```text
model requests recall(query)
      ↓
RecallRequested event
      ↓
scope/security filter
      ↓
retrieve/rank
      ↓
RecallResolved(selected refs, manifest)
      ↓
selected pages pinned for next invocation or bounded pin lease
```

Pin duration might be:

```text
next invocation only
next N invocations
until task phase changes
```

v0 default: next invocation plus explicit active references.

---

# 16. Anti-thrashing

Even explicit recall can loop.

Track per-task/invocation-chain:

```text
recent recall queries
page sets returned
unresolved refs
```

If same query/page set repeats without progress N times, return structured `RecallLoopDetected` and require alternate strategy/harness decision.

Future automatic Context Faults must enforce even stricter loop budgets.

---

# 17. Cache architecture

Hot caches:

- page metadata;
- token estimates;
- optional retrieval index handles.

All caches bounded by entries/bytes.

Page bodies loaded on demand and released after packing/invocation unless pinned by bounded cache policy.

Do not keep all repository chunks in Go heap because metadata exists in SQLite/object store.

---

# 18. Performance objective

Context building cost should scale with indexed candidate retrieval + selected pages, not total historical bytes.

Targets after benchmark calibration:

```text
10k–100k page metadata corpus
→ retrieve top candidate IDs using index
→ load only candidate/selected bodies
```

Never iterate/read every historical object for each invocation.

---

# 19. v0 quality metrics

Track:

```text
input tokens used/budget
mandatory tokens
retrieved page count
recall count
context-build latency
page cache hit rate
fault/recall loops
selected page reuse
provider overflow failures (target zero)
```

For eval tasks also track whether known-required evidence was selected.

---

# 20. Long-session benchmark

Synthetic setup:

- 100k historical pages;
- only 5–20 relevant to current task;
- several old critical constraints;
- model context 32k/64k.

Validate:

- packing stays within budget;
- retrieval does not scan full content;
- critical explicit refs recalled;
- process heap does not scale with full corpus;
- manifest explains selection.

---

# 21. Path toward Context Faults

v0:

```text
explicit recall between invocations
```

v1/v2 research:

```text
symbolic references
runtime detects unresolved semantic object
→ controlled fault
→ page-in
→ new/resumed inference step
```

Do not depend on provider-level mid-stream interruption/resumption initially.

The provider API ecosystem can change without affecting `recall()` semantics.

---

# Required tests

```text
MMU-001 hard budget never exceeded
MMU-002 mandatory overflow fails explicitly
MMU-003 invalidated/superseded page not selected as normal trusted page
MMU-004 explicit visible PageID outranks generic search result
MMU-005 inaccessible scope page never returned
MMU-006 100k-page corpus does not materialize all bodies
MMU-007 token cache bounded
MMU-008 repeated recall loop detected
MMU-009 summary retains source references
MMU-010 manifest deterministically explains v0 selection for fixed inputs
```

---

# v0 principle

> **Prefer a simple context manager whose mistakes are visible over a sophisticated retriever whose behavior cannot be explained.**
