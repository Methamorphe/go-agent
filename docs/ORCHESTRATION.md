# Orchestration, Subagents and Cognitive Scheduling

## Thesis

Subagents should be treated as **managed intelligent processes**, not merely additional prompts.

The runtime should be able to create, supervise, budget, route, pause, resume and terminate them just like an operating system manages processes — while preserving agent-specific semantics such as context, authority and model selection.

## 1. Child agent contract

Creating a child should produce a concrete contract:

```yaml
task: "Review the authentication subsystem for race conditions"
intent_parent: intent_123
authority:
  filesystem:
    read: ./src/auth/**
    write: none
  network: none
budget:
  max_cost: 0.40
  max_wall_time: 10m
  max_children: 2
model_policy:
  objective: quality
world:
  mode: inherited-readonly
result_contract:
  format: findings
  evidence_required: true
```

A child receives only what it needs.

## 2. Recursive delegation

Children may create descendants if they own `agent.spawn` and enough remaining budget/authority.

```text
Lead Agent
  │
  ├─ Backend Agent
  │    └─ SQL Specialist
  │
  ├─ Test Agent
  │
  └─ Security Agent
       ├─ Auth Reviewer
       └─ Dependency Reviewer
```

The authority/budget tree must always remain bounded.

## 3. Adaptive Team Formation

The agent hierarchy should not be hard-coded for every task.

A lead can analyze a problem and request a temporary team:

```text
Task: diagnose intermittent API latency
               │
               ▼
          Lead Agent
               │
      task decomposition
               │
      ┌────────┼────────┐
      ▼        ▼        ▼
     SQL     Runtime   Network
      │
      ▼
 Query-plan specialist
```

Another task may create a completely different topology.

The **workflow itself becomes generative**.

### Guardrail

Team formation must remain subject to:

- child-count limit;
- cost budget;
- wall-clock budget;
- parallelism limit;
- capability-delegation rules;
- scheduler policy.

## 4. Agent messaging

Agents should be able to exchange structured messages without routing every interaction through the user or root agent.

Possible message types:

```text
request
response
evidence
proposal
challenge
counterexample
status
result
cancel
escalation
```

A message should carry references to evidence/objects rather than copy huge context payloads unnecessarily.

## 5. Agent Negotiation

A potentially differentiated feature is explicit peer negotiation.

Example:

```text
Implementer
  "This cache is thread-safe."
        │
        ▼
Reviewer
  "I found a race when invalidation overlaps refresh."
        │
        ▼
Implementer
  "Provide reproducer."
        │
        ▼
Reviewer
  evidence://test/race-42
        │
        ▼
Implementer
  "Confirmed. Updating implementation."
```

Instead of treating a reviewer as a one-shot critic, the runtime can support a finite disagreement protocol.

### Escalation

If peers do not converge within limits:

```text
peer disagreement
      ↓
stronger evaluator / parent agent
      ↓
resolve or request user decision
```

## 6. Evidence-aware communication

Agent reports should be able to distinguish:

```text
claim
hypothesis
verified finding
uncertainty
artifact
```

A child returning:

> “There is probably a race.”

is weaker than:

```yaml
finding: "Concurrent refresh can publish stale state"
confidence: 0.96
evidence:
  - test://race-reproducer-42
  - file://src/cache.go#L81-L120
verification:
  command: go test -race ./...
  status: failed-before-fix
```

This feeds naturally into Epistemic Memory.

## 7. Agent Economy

Every process participates in a resource economy.

Resource dimensions:

```text
money
tokens
wall time
compute time
child count
parallelism
tool calls
external API quota
world/storage usage
```

The parent can allocate part of its remaining budget to children.

### Budget conservation

If a parent owns €2 of remaining model budget, it cannot spawn five children each with an independent €2 budget.

The runtime should support reservations and release unused budget.

## 8. Cognitive Scheduler

The scheduler chooses **how intelligence is allocated**.

Traditional model selection:

```text
User config → always use model X
```

Cognitive scheduling:

```text
Task characteristics
      │
      ▼
Scheduler
      │
      ├─ local small model
      ├─ cheap cloud model
      ├─ frontier model
      ├─ deterministic tool/process
      └─ specialist child strategy
```

## 9. Scheduling signals

Candidate signals:

- task category;
- expected complexity;
- required context size;
- latency objective;
- quality objective;
- budget remaining;
- privacy/data locality;
- provider availability;
- historical model success on similar tasks;
- tool-use reliability;
- code-language specialization;
- reasoning depth requirement.

## 10. Scheduling objective

Conceptually:

```text
maximize expected task utility
subject to:
  cost budget
  latency deadline
  authority constraints
  privacy constraints
```

A simplified score might combine:

```text
expected_quality
- cost_penalty
- latency_penalty
- failure_risk
```

The score itself should remain replaceable; early versions can use rules before learning routing policies.

## 11. Model as compute resource

The system should think of models the way a heterogeneous compute scheduler thinks of CPUs/GPUs:

```text
small local model
  cheap / private / fast / weaker

mid-tier cloud model
  moderate cost / good general ability

frontier reasoning model
  expensive / slower / strong
```

Tasks such as simple extraction, summarization or classification may not justify frontier-model cost.

## 12. Model fallback and migration

Since process state is provider-independent, an agent should be able to continue after:

- rate limit;
- provider outage;
- model deprecation;
- budget exhaustion;
- context requirement change.

The scheduler can reconstruct a working set for another model instead of depending on one provider’s hidden thread state.

## 13. Speculative model execution

For high-value decisions, the scheduler may run multiple candidate reasoners:

```text
Problem
  ├─ Model A → proposal A
  ├─ Model B → proposal B
  └─ Model C → critique
              ↓
          evaluator
```

This must be budget-aware; it should not become the default for every trivial task.

## 14. Agent Fork / Cognitive Fork

Forking is stronger than spawning a child.

A child receives a new task/context contract.

A **fork** copies a recoverable state point so alternative futures can be explored.

```text
checkpoint #921
      │
      ├─ fork A → implementation strategy A
      └─ fork B → implementation strategy B
```

A full fork may include:

- Agent Process state;
- context working set references;
- epistemic state;
- world snapshot;
- filesystem state;
- database/container state where supported;
- budget allocation.

## 15. Speculative execution

Example coding workflow:

```text
             current state
                  │
             checkpoint
                  │
          ┌───────┴───────┐
          ▼               ▼
        fork A          fork B
     simple cache     sharded cache
          │               │
       tests/bench      tests/bench
          │               │
          └───────┬───────┘
                  ▼
              evaluator
                  │
                B wins
                  │
           promote/commit B
```

The discarded branch remains inspectable in the ledger if policy allows.

## 16. Evaluators

A fork should not be selected solely by another LLM saying “B looks better”.

Prefer objective or mixed verification:

```text
tests
benchmarks
linters
type checker
security policy
acceptance criteria
LLM critique
user preference
```

The evaluator should record why a branch won.

## 17. Verified Continual Improvement

The system may learn from repeated work, but changes should enter a lifecycle.

Candidate targets:

- procedural skill;
- harness instruction;
- memory rule;
- child-agent profile;
- routing policy;
- context ranking heuristic.

Lifecycle:

```text
experience
  ↓
refinement hypothesis
  ↓
candidate version
  ↓
evaluation against baseline
  ↓
promote / reject
```

Metadata:

```yaml
candidate: auth-debug-skill@v7
origin: session/928
hypothesis: "run token-expiry reproducer before source audit"
baseline_success: 0.71
candidate_success: 0.86
evaluations: 24
status: promoted
rollback_to: v6
```

## 18. Avoiding self-improvement drift

Self-improvement must not grant new authority or alter immutable root policy.

A learned skill can improve **how** a task is done; it cannot silently change **what** the user authorized.

## 19. Long-running goals

For multi-hour/day tasks, the process should be able to enter states such as:

```text
RUNNING
WAITING_CONDITION
SCHEDULED
SLEEPING
SUSPENDED
```

Wake triggers may later include:

- time;
- child completion;
- external event;
- changed repository state;
- human approval;
- resource availability.

Durability means these waits survive process restart.

## 20. Observing an agent team

A future TUI could show:

```text
root  Fix flaky integration tests              RUNNING
├─ a1 Analyze CI history                       DONE
├─ a2 Reproduce locally                        RUNNING
│  └─ a5 Inspect DB timing                     WAITING_TOOL
├─ a3 Review networking                        DONE
└─ a4 Test alternate retry strategies          RUNNING
   ├─ fork A exponential                       VERIFYING
   └─ fork B jittered                          VERIFYING

Budget: €0.83 / €2.00
Tokens: 182k
Wall: 08:41
```

This is much more useful than a linear transcript for understanding parallel autonomous work.

## Core objective

Orchestration should make the system behave less like “one very long conversation” and more like a **temporary organization of bounded intelligent processes** working toward a shared intent.
