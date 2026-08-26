# Verified Continual Improvement

## Status

**A0 architecture contract — ACCEPTED extension semantics; implementation deferred to G12.**

The runtime may eventually improve prompts, skills, routing policies, context policies and specialist profiles from experience.

Core rule:

> **Self-improvement is versioned experimentation, not mutable personality.**

Nothing learned by the agent may silently expand authority, weaken immutable policy or rewrite historical behavior.

---

# 1. Cognitive artifacts

Versioned artifact families:

```text
PromptArtifact
SkillArtifact
AgentProfileArtifact
RoutingPolicyArtifact
ContextPolicyArtifact
MemoryPolicyArtifact
EvaluatorArtifact
```

Each artifact has stable logical identity + immutable versions.

```go
type CognitiveArtifactVersion struct {
    ArtifactID      ArtifactID
    Version         uint32
    Kind            CognitiveArtifactKind
    ContentRef      ObjectRef
    ParentVersion   *uint32
    OriginRef       ObjectRef
    HypothesisRef   *ObjectRef
    Status          ArtifactStatus
    CreatedAt       time.Time
}
```

---

# 2. Lifecycle

```text
Draft
→ Candidate
→ Evaluating
→ Shadow
→ Canary
→ Promoted
   ├─→ Deprecated
   └─→ RolledBack

Candidate/Evaluating
→ Rejected
```

Not every artifact needs every stage, but permanent automatic promotion cannot jump directly from one successful anecdote to `Promoted`.

---

# 3. Hypothesis first

Candidate creation records:

```text
observed failure/opportunity
proposed change
expected benefit
expected risks
scope/task classes affected
baseline version
```

Example:

```yaml
hypothesis: "For Go race debugging, run `go test -race` before source-wide review"
expected:
  lower_tokens: true
  success_rate_improvement: true
scope: coding/go/concurrency-debug
```

This makes refinement testable.

---

# 4. Evidence corpus

Evaluation uses versioned tasks/episodes with known/observable outcomes where possible.

Sources:

```text
historical replay-safe episodes
synthetic benchmarks
curated regression cases
shadow live tasks
user-approved evaluation corpus
```

Sensitive user/project data remains scope/privacy constrained.

---

# 5. Baseline comparison

Promotion requires comparison to explicit baseline.

Metrics can include:

```text
task verification success
cost
latency
number of tool calls
context faults
security-policy violations
user correction rate
regression suite failures
```

A candidate winning one metric while causing safety regressions is rejected.

---

# 6. Hard non-regression gates

Automatic promotion is blocked by:

```text
new policy/security violation
capability expansion requirement not explicitly approved
higher irreversible-effect rate outside policy
loss of deterministic verification
replay incompatibility
critical regression corpus failure
```

Security gates are hard constraints, not weighted utility.

---

# 7. Authority non-expansion

Cognitive artifacts may specify **required** capabilities.

They cannot grant them.

Example:

```text
new deploy skill requires infrastructure.staging-mutate
```

If process lacks capability:

```text
skill unavailable / needs explicit grant
```

Self-improvement can never modify Capability Grants, root Intent, approval policy or Effect classification floor.

---

# 8. Memory interaction

A repeated successful episode may propose procedural memory/skill.

Flow:

```text
episodes
→ candidate procedure
→ preserve episode provenance
→ evaluate
→ promote artifact
```

Do not continually overwrite one mutable "best practices" text blob.

---

# 9. Shadow evaluation

In Shadow mode:

```text
baseline controls real task
candidate makes parallel non-effecting decisions
runtime compares predictions/actions/routing/context selections
```

Candidate cannot produce external mutations.

Useful for routing/context policies.

---

# 10. Canary

Canary applies promoted-like candidate to bounded eligible low-risk traffic/tasks.

Requires:

```text
explicit sample cap
kill switch
baseline fallback
monitoring
rollback version
```

No uncontrolled global rollout.

---

# 11. Statistical humility

Do not claim improvement from tiny sample sizes.

Evaluation record stores:

```text
N tasks
win/loss/tie
metric distributions
confidence/uncertainty metadata
known dataset bias
```

Exact statistical method can evolve, but raw outcomes remain accessible.

---

# 12. Reproducibility

Every model invocation/task should eventually record exact refs for active cognitive artifacts:

```text
prompt@v4
reviewer-profile@v2
routing-policy@v6
context-policy@v3
skill-set manifest hash
```

This enables historical replay/comparison.

---

# 13. Rollback

Rollback selects prior artifact version for future use.

It does not rewrite historical runs.

```text
v8 promoted
regression detected
→ v8 RolledBack
→ v7 active again
```

Historical tasks remain attributable to v8.

---

# 14. Kill switches

Runtime/system policy can disable:

```text
one candidate/version
artifact family auto-promotion
learned routing
learned context ranking
all self-improvement
```

Kill switch itself is not model-controlled.

---

# 15. Evaluator independence

Avoid same-model circularity:

```text
candidate model proposes change
same exact model says change is better
→ weak evidence
```

Prefer:

```text
deterministic task verifier
held-out outcomes
independent evaluator
user acceptance
```

LLM-as-judge can supplement, not dominate, when objective checks exist.

---

# 16. Context/routing policy improvement

These are especially suitable for data-driven iteration because metrics are observable.

Examples:

```text
Context policy candidate:
  lower fault rate at same token budget

Routing candidate:
  same verification success at 30% lower cost
```

Lifecycle still follows shadow/canary/promotion.

---

# 17. Prompt/skill improvement

More subjective and distribution-sensitive.

Require:

```text
scope-specific evaluation
regression suite
behavioral diff
security/adversarial tests
```

Do not globally promote a coding prompt improvement based on one repository.

---

# 18. Artifact scope

Versions have scope:

```text
global product default
user
project/repository
task class
model family
World type
```

A project-specific learned procedure does not become global automatically.

---

# 19. Candidate contamination

Evaluation must prevent candidate from training/evaluating on the exact same live task in a way that makes result meaningless.

Persist evaluation provenance and corpus membership.

Future benchmark management can detect leakage/reuse.

---

# 20. Required tests

```text
IMP-001 candidate cannot expand capability grants
IMP-002 candidate cannot modify root Intent/policy floor
IMP-003 promotion records baseline/candidate versions and eval refs
IMP-004 rollback changes future active version without rewriting history
IMP-005 shadow candidate cannot execute mutating effects
IMP-006 hard security regression blocks promotion despite quality gain
IMP-007 project-scoped artifact cannot silently become global
IMP-008 historical invocation retains exact artifact manifest
IMP-009 candidate with insufficient eval evidence cannot auto-promote
IMP-010 kill switch prevents new candidate use immediately for future scheduling
```

---

# Accepted A0 decisions

1. Cognitive artifacts are immutable/versioned.
2. Refinement begins with explicit hypothesis and baseline.
3. Permanent auto-promotion requires evaluation; one successful task is insufficient.
4. Shadow/canary are first-class deployment states.
5. Security/authority non-regression is a hard gate.
6. Self-improvement cannot grant capabilities or modify root Intent/effect floor.
7. Rollback changes future active version only.
8. Invocations record exact cognitive artifact versions for reproducibility.
9. Scope prevents local/project learning from becoming global silently.
10. Learned routing/context policies follow the same version/evaluation lifecycle.

> **The system may improve its strategy. It may not quietly redefine its mandate.**
