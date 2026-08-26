# Cognitive Scheduler Architecture

## Status

**A0 architecture contract — ACCEPTED semantic baseline. Routing weights/models remain empirical.**

The Cognitive Scheduler treats language models and agent strategies as heterogeneous compute resources rather than permanent agent identities.

Core rule:

> **An Agent Process owns durable intent/state. A model is a temporary execution resource selected for one cognitive task/invocation.**

---

# 1. Why a scheduler is needed

Binding one agent to one model for its whole lifetime causes avoidable problems:

- cheap extraction uses an expensive frontier model;
- provider outages stall durable processes;
- context-size requirements change over a task;
- local/private data may require local inference;
- recursive agents can multiply cost unexpectedly;
- queue/load conditions change latency;
- different models specialize differently;
- model APIs are deprecated or rate-limited.

A scheduler centralizes these decisions under explicit budgets and policies.

---

# 2. Scheduling unit: Cognitive Task

Do not route an entire Agent Process as one indivisible job.

```go
type CognitiveTask struct {
    ID              CognitiveTaskID
    AgentID         AgentID
    Kind            CognitiveTaskKind
    InputProfile    InputProfile
    Requirements    ModelRequirements
    Objective       SchedulingObjective
    Budget          TaskBudget
    Deadline        *time.Time
    Privacy         PrivacyPolicy
    RiskClass       RiskClass
    RetryPolicy     RetryPolicy
    VerificationRef *VerificationPolicyID
}
```

Candidate task kinds:

```text
classification
extraction
summarization
code-generation
code-review
architecture-reasoning
planning
retrieval-query-generation
memory-consolidation
conflict-resolution
evaluation
conversation-response
vision
```

Task kind is advisory metadata; hard requirements are separate.

---

# 3. Model profile

Each registered model/backend has a versioned profile.

```go
type ModelProfile struct {
    ModelID            ModelID
    ProviderID         ProviderID
    ContextWindow      int
    MaxOutputTokens    int
    Capabilities       ModelCapabilitySet
    Locality           Locality
    DataPolicy         DataPolicy
    InputCost          MoneyRate
    OutputCost         MoneyRate
    ObservedLatency    LatencyProfile
    Reliability        ReliabilityProfile
    Quality            QualityProfile
    Concurrency        ConcurrencyProfile
    Health             HealthState
    ProfileVersion     uint32
}
```

Capabilities:

```text
tool-calling
structured-output
vision
reasoning controls
streaming
large-context
prompt-cache support
specific modalities
```

---

# 4. Runtime telemetry profile

Static marketing/model metadata is insufficient.

Track rolling observed metrics:

```text
TTFT p50/p95
output tokens/sec
request latency p50/p95
rate-limit rate
provider error rate
timeout rate
tool-call parse success
structured-output validity
cost per successful task
```

Metrics are keyed by model/backend and, where useful, task class/context bucket.

Use bounded windows/EWMA/histograms rather than storing every observation in hot memory.

---

# 5. Quality profile

Quality is not one global score.

```text
code generation
review
long-context synthesis
small extraction
tool-use reliability
planning
memory consolidation
```

v0 quality comes from curated configuration/benchmark results.

Later quality can be updated from verified task outcomes.

Do not infer "quality" merely from user-visible model brand/size.

---

# 6. Hard filtering before scoring

Scheduler first eliminates impossible candidates.

Hard filters:

```text
required context capacity
required tool/structured-output capability
privacy/local-only constraints
provider/model allow/deny policy
region/data policy
health = unavailable
budget cannot afford minimum request
risk policy forbids backend
required modality unsupported
deadline impossible under hard bound if configured
```

A soft score never rescues a hard-ineligible model.

---

# 7. Candidate Pareto set

After hard filtering, remove obviously dominated options where practical.

Example model A is worse/equal than B on all relevant dimensions:

```text
quality <= B
expected cost >= B
latency >= B
reliability <= B
```

Then A need not be considered for that task unless a diversity/fallback reason exists.

This keeps routing explainable.

---

# 8. Scheduling objective

The scheduler maximizes expected utility subject to hard constraints.

Conceptual form:

```text
U(model, task) =
    wq * expected_quality
  - wc * expected_cost
  - wl * expected_latency
  - wr * expected_failure_risk
  - wv * expected_variance
```

Weights come from task/user policy profiles such as:

```text
quality-first
balanced
cost-first
latency-first
local-only
```

The exact scalar score is replaceable and versioned.

---

# 9. Latency is load-sensitive

Do not use one static latency number.

Expected latency can depend on:

```text
prompt/context length
expected output length
current backend queue/load
provider health
model throughput
request priority
prompt-cache hit opportunity
```

v0 may begin with observed p50/p95 by size bucket plus current health/load.

The interface must allow a stronger serving-aware estimator later.

---

# 10. Cost estimate

Before routing, estimate:

```text
input token count
reserved output tokens
provider/tool-call overhead
cache pricing if known
```

Then reserve budget pessimistically enough to prevent oversubscription.

After completion:

```text
actual cost reconciles reservation
unused reservation released
```

Unknown pricing/backend cost can be represented explicitly rather than assumed zero.

---

# 11. Quality floor

Some tasks should not trade unlimited quality for cost.

Example:

```yaml
objective: balanced
minimum_quality_class: strong-code-review
```

Hard/soft policy can define:

```text
minimum expected quality
minimum verified tool reliability
minimum context capacity
```

This prevents a cost-first router from assigning a tiny model to a high-risk architecture review merely because it is cheap.

---

# 12. Risk-aware routing

Risk class influences routing.

For high-risk reasoning:

- prefer models with stronger verified quality/reliability;
- optionally require independent verification/reviewer;
- avoid experimental backends unless explicitly allowed;
- retain deterministic authority checks regardless of model strength.

No model is trusted enough to bypass kernel security.

---

# 13. Deterministic execution is also a candidate

Not every cognitive-looking task needs an LLM.

Scheduler/harness may select:

```text
parser
compiler
search index
static analyzer
test runner
rule engine
cached verified result
```

Example:

```text
"Does this Go project compile?"
→ run `go test`/build, not ask a frontier model to guess.
```

The scheduler's conceptual resource pool is broader than models, although v0 implementation can keep deterministic task routing in the harness.

---

# 14. Routing decision

Persist an explainable decision object:

```go
type RoutingDecision struct {
    ID              RoutingDecisionID
    TaskID          CognitiveTaskID
    CandidateRefs   []ModelProfileRef
    Rejected        []CandidateRejection
    Selected        ModelProfileRef
    ObjectiveRef    SchedulingObjectiveID
    EstimatedCost   Money
    EstimatedLatency time.Duration
    ScoreComponents ScoreBreakdown
    PolicyVersion   uint32
    CreatedAt       time.Time
}
```

This answers:

> Why did this child use model X instead of Y?

---

# 15. Fallback

Fallback policy is defined before invocation.

```text
primary model
   ↓ transient failure/rate limit
fallback candidate set
   ↓ re-check hard constraints + remaining budget/deadline
route next
```

Important:

- provider thread state is non-canonical;
- Cognitive MMU reconstructs request for fallback model;
- failed partial output is not silently concatenated as trusted final state;
- tool/action effects from previous attempt must be reconciled before retry.

---

# 16. Retry vs reroute

Distinguish:

```text
retry same backend
reroute same cognitive task
re-plan task
```

Examples:

```text
HTTP 503 → reroute/retry according to policy
invalid structured output → one constrained repair/retry or alternate model
context overflow → MMU rebuild/model with larger context
repeated reasoning failure → stronger model/re-plan
```

All remain bounded.

---

# 17. Hedging

For latency-sensitive high-value requests, scheduler MAY hedge:

```text
start primary
if TTFT > threshold
start secondary
first acceptable verified response wins
cancel loser
```

Hedging spends extra money and must be explicitly enabled by objective/budget policy.

Never hedge state-changing tool execution itself unless effects are safely isolated/idempotent.

---

# 18. Speculative multi-model reasoning

For high-uncertainty/high-value decisions:

```text
Task
 ├─ Model A proposal
 ├─ Model B proposal
 └─ evaluator/verifier
```

Admission requirements:

```text
expected value justifies cost
budget reserved before fan-out
side effects deferred/isolated
comparison criterion defined
```

Do not make "ask 5 models" the default agent strategy.

---

# 19. Scheduler and subagents

Scheduler can decide between:

```text
one stronger model invocation
vs
several cheaper specialist child processes
```

But topology decisions are separate from model routing semantics.

A Team Planner proposes decomposition; Agent Economy/Scheduler checks:

- budget;
- expected parallelism benefit;
- authority delegation;
- latency objective;
- model availability.

---

# 20. Budget hierarchy

Root budget conservation is mandatory.

```text
root reservation pool
      ↓
parent task
      ↓
child cognitive tasks
```

A scheduler never treats provider quotas as budget authority.

Reservations include:

```text
money
tokens
concurrency slots
optional local GPU slots
wall-clock deadline
```

---

# 21. Local inference

Local/OpenAI-compatible models are first-class profiles.

Potential signals:

```text
zero marginal API price
local GPU availability
VRAM/model residency
queue depth
context limit
tokens/sec
privacy advantage
energy/compute policy later
```

"Free" local inference still consumes scarce compute/time and should be scheduled/accounted.

---

# 22. Model switching and cognitive continuity

Switching model must not change Agent identity.

```text
Agent A
 invocation 1 → local model
 invocation 2 → frontier model
 invocation 3 → cheap cloud model
```

Continuity is supplied by:

```text
Agent Process state
Cognitive MMU working set
Epistemic Memory
Event Ledger
```

not provider hidden session state.

---

# 23. Prompt-cache awareness

Where providers expose stable prompt-cache semantics, scheduler/MMU MAY consider expected cache benefit.

Do not compromise context correctness to maximize cache hits.

Cache economics are an optimization signal only.

---

# 24. Provider health state machine

```text
Healthy
Degraded
RateLimited
Unavailable
Recovering
```

Health is based on bounded recent telemetry and explicit provider errors.

Circuit breaker behavior:

```text
repeated transient failures
→ Degraded/Unavailable
→ stop flooding backend
→ periodic bounded probes
→ Recovering
→ Healthy
```

Durable agents should not all stampede a recovering provider.

---

# 25. Fairness

One recursive root must not monopolize all model/provider slots.

Scheduler enforces:

```text
global concurrency
per-root concurrency
per-provider concurrency
priority/fair-share
reserved interactive capacity later
```

Use bounded queues and admission control.

No unlimited pending model request list.

---

# 26. Learned routing: later only

v0 is deterministic/rule/score-based.

Learned routing requires enough verified telemetry.

Lifecycle:

```text
candidate routing policy
→ offline replay
→ shadow decision
→ canary
→ compare baseline
→ promote/rollback
```

A learned router cannot weaken privacy/capability/hard risk constraints.

This integrates with Verified Continual Improvement.

---

# 27. Outcome feedback

Useful scheduler learning signal comes from objective outcomes where possible:

```text
tests passed
verification result
user accepted/rejected
review defects found
structured output validity
task completed within budget
```

Avoid circular evaluation where the same model both solves and declares itself correct.

---

# 28. Cold start

New model profile has little telemetry.

Policy options:

```text
use configured priors
restrict to low-risk tasks
shadow route first
exploration budget
```

Do not immediately route critical tasks to an unknown backend because its advertised price is low.

---

# 29. Scheduler failure semantics

## No eligible candidate

Return:

```text
NoEligibleModel(reason set)
```

Harness may:

- compact context;
- request more budget;
- wait for provider;
- switch task strategy;
- ask user.

## Scheduler storage/telemetry unavailable

Use safe configured static profile snapshot if available; otherwise fail explicitly.

## Budget exhausted

Do not silently use an unaccounted local/cloud model.

---

# 30. Performance architecture

Scheduler hot path uses:

```text
in-memory bounded model registry/profile cache
atomic/cheap health snapshots
precomputed capability indexes
bounded candidate count
no historical scan
```

Routing latency should be negligible relative to model invocation.

Historical metrics aggregate asynchronously.

---

# 31. Required tests

```text
SCH-001 hard privacy constraint filters otherwise best model
SCH-002 insufficient context candidate cannot win soft score
SCH-003 budget reservation prevents concurrent overspend
SCH-004 fallback reconstructs task without provider thread dependency
SCH-005 rate-limited provider circuit breaker prevents request storm
SCH-006 high-risk task cannot route to profile below required quality policy
SCH-007 local model queue pressure affects latency estimate/routing
SCH-008 routing decision is deterministic for fixed profiles/policy/telemetry snapshot
SCH-009 failed hedge loser is cancelled and cannot leak effects
SCH-010 one root cannot consume all configured global slots
SCH-011 learned/shadow router cannot override hard constraints
SCH-012 no eligible candidate returns structured failure, not arbitrary default model
SCH-013 actual cost reconciles reservation exactly once
SCH-014 model switch preserves Agent Process identity/state
SCH-015 100 registered profiles remain cheap to filter/score without history scan
```

---

# 32. Benchmarks

Scenario matrix:

```text
cheap extraction
large-context synthesis
high-risk code review
local-only private task
provider outage
rate-limit storm
local GPU queue saturation
tight latency deadline
cost-constrained recursive team
model deprecation/fallback
```

Measure:

```text
success/verification rate
cost per successful task
TTFT/end-to-end latency
fallback recovery
routing overhead
budget violations (target zero)
provider overload amplification (target zero)
```

---

# 33. Innovation opportunity

The scheduler becomes more than "choose model by price" through the combination:

```text
provider-independent Agent identity
+ per-task cognitive scheduling
+ hard privacy/context/risk filtering
+ dynamic load/latency telemetry
+ hierarchical budget reservations
+ local/cloud heterogeneous resources
+ deterministic + model execution choices
+ bounded hedging/speculation
+ outcome-driven learned policy promotion
+ causal routing observability
```

This is closer to an OS scheduler for heterogeneous intelligence than a static model dropdown.

---

# Accepted A0 decisions

1. Routing happens per Cognitive Task/invocation, not per lifetime Agent Process.
2. Hard eligibility filtering precedes any utility score.
3. Latency uses runtime/load-aware observations, not only static model metadata.
4. Budget is reserved before execution/fan-out and reconciled afterward.
5. v0 routing is deterministic/rule-based; learned routing is deferred and evaluated like software.
6. Provider fallback is possible because provider session state is non-canonical.
7. High-risk tasks can impose quality/reliability floors.
8. Local inference is a first-class schedulable resource, not "free unlimited capacity".
9. Hedging/speculation are budgeted opt-in strategies, never defaults.
10. Scheduler enforces fairness/global/per-root concurrency with bounded queues.
11. Routing decisions are versioned and explainable.
12. No model/router can override hard privacy/authority/security constraints.

## Still empirical before G6

- utility weights;
- task classification heuristics;
- latency estimator sophistication;
- initial quality priors;
- hedging thresholds;
- learned-routing algorithm.

These are replaceable policies, not kernel semantic dependencies.

> **The agent is persistent. The intelligence resource is scheduled.**
