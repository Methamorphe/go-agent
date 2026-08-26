# Intent-Based Authority Engine

## Status

**A0 architecture contract — ACCEPTED semantic baseline.**

This document refines the capability model into a complete authorization pipeline.

Capabilities answer:

> **Can this Agent Process technically perform this class of action?**

Intent-Based Authority additionally asks:

> **Is this specific action legitimately connected to the user-authorized task, under the current risk/effect policy?**

The two checks are independent and cumulative.

---

# 1. Security thesis

A broad capability is sometimes necessary for useful work.

Example:

```text
filesystem.write ./project/**
network.connect github.com
```

But capability possession alone should not imply:

```text
any write is relevant
any network request is justified
any GitHub mutation is authorized
```

Therefore:

```text
Capability = maximum technical authority
Intent     = purpose/effect boundary for current task
```

Authorization requires both.

---

# 2. Root Intent Envelope

The root user request becomes a versioned runtime object.

```go
type IntentEnvelope struct {
    ID                 IntentID
    Version            uint32
    RootAgentID        AgentID
    GoalRef            ObjectRef
    ResourceScopes     []IntentResourceScope
    AllowedDomains     EffectDomainSet
    ForbiddenDomains   EffectDomainSet
    AcceptanceCriteria []Criterion
    Constraints        []IntentConstraint
    RiskPolicyRef      RiskPolicyID
    CreatedBy          ActorRef
    CreatedAt          time.Time
}
```

This object is not merely a copy of chat text. It is the durable execution boundary derived from user input and explicit product policy.

The original user text remains evidence/reference.

---

# 3. Intent amendments

"Immutable intent" means **agents cannot silently rewrite it**.

Users can change their request.

That creates:

```text
Intent v1
   ↓ user amendment
Intent v2
```

Historical actions remain associated with the exact intent version that authorized them.

An agent cannot create `Intent v2` on behalf of the user unless a specific product policy explicitly grants such authority—which v0 does not.

---

# 4. Effect-domain taxonomy

Actions declare effect domains independent from their low-level capability.

Initial domain hierarchy:

```text
workspace
  ├─ read
  ├─ mutate
  └─ delete

process
  ├─ execute
  └─ persistent-change

network
  ├─ retrieve
  └─ transmit

source-control
  ├─ inspect
  ├─ local-mutate
  ├─ remote-publish
  └─ destructive-remote

external-communication
  ├─ draft
  └─ send

infrastructure
  ├─ inspect
  ├─ staging-mutate
  └─ production-mutate

secrets
  ├─ use
  └─ reveal

financial
  └─ transact
```

The exact taxonomy is extensible/versioned.

Parent domains contain narrower children only when explicitly defined by schema—not by string prefix guessing.

---

# 5. Intent resource scopes

Intent constrains resources separately from capabilities.

Example:

```yaml
goal: fix authentication bug
resources:
  repository: project-A
  paths:
    - /src/**
    - /tests/**
forbidden:
  - production
  - external-communication.send
```

A capability may allow multiple projects, but the current Intent Envelope can narrow execution to one.

---

# 6. Delegated Intent

A child receives a Task Intent derived from its parent.

```go
type TaskIntent struct {
    ID             TaskIntentID
    RootIntentID   IntentID
    ParentTaskID   *TaskIntentID
    GoalRef        ObjectRef
    ResourceScopes []IntentResourceScope
    AllowedDomains EffectDomainSet
    Constraints    []IntentConstraint
    Criteria       []Criterion
}
```

Hard rule:

```text
AllowedDomains(child)  ⊆ AllowedDomains(parent)
ResourceScopes(child)  ⊆ ResourceScopes(parent)
Forbidden(child)       ⊇ Forbidden(parent)
```

Natural-language goal narrowing is not trusted as a formal security proof; typed domains/resources provide the enforceable subset relation.

---

# 7. Purpose-Carrying Actions

Every non-trivial action request carries a structural purpose reference.

```go
type ActionPurpose struct {
    TaskIntentID       TaskIntentID
    PlanStepID         *PlanStepID
    CriterionIDs       []CriterionID
    RationaleRef       *ObjectRef
    ExpectedOutcomeRef *ObjectRef
}
```

Example:

```text
Action: run `go test ./auth/...`
Purpose:
  task = fix-auth-refresh
  criterion = auth-tests-pass
  plan-step = reproduce-failure
```

This does not prove that the model's rationale is semantically correct.

It creates an auditable causal chain and lets deterministic policy reject orphan actions that have no active task/plan relationship.

---

# 8. Proof-carrying action concept

The system can treat the authorization context as a lightweight **Action Proof**.

```go
type ActionProof struct {
    AgentID          AgentID
    IntentID         IntentID
    IntentVersion    uint32
    TaskIntentID     TaskIntentID
    CapabilityGrants []GrantID
    Purpose          ActionPurpose
    Effect           EffectDescriptor
    Risk             RiskAssessment
    PolicyVersion    PolicyVersion
    ApprovalID       *ApprovalID
}
```

The proof is not cryptographic proof of semantic intent.

It is a complete machine-checkable record of the authority/policy facts used to decide the action.

This becomes part of causal observability and replay.

---

# 9. Authorization pipeline

Canonical order:

```text
1. validate process lifecycle
2. validate action schema
3. derive/verify static + World-specific EffectDescriptor
4. validate capability grants + leases
5. validate hard Intent domains
6. validate Intent resource scopes
7. validate task/purpose references
8. evaluate risk policy
9. evaluate transaction/fork restrictions
10. semantic relevance gate if configured/needed
11. approval gate
12. confirm World can enforce required constraints
13. reserve budget/resources
14. persist authorization decision / operation ID
15. execute World action
```

A later check cannot repair failure of an earlier hard-security check.

---

# 10. Deterministic intent checks

These are authoritative.

Examples:

```text
forbidden effect domain → DENY
resource outside intent scope → DENY
child domain broader than parent → DENY
expired intent version/task → DENY
missing active task purpose → DENY or NEEDS_NARROWER_ACTION
irreversible effect disallowed by intent → DENY
```

No model evaluator can override these.

---

# 11. Semantic relevance gate

Some relationships cannot be proven from typed fields alone.

Example:

```text
Goal: fix auth bug
Action: edit generic cache implementation
```

This might be relevant or task drift.

Semantic evaluation MAY inspect:

- goal;
- active plan;
- action target;
- rationale;
- current evidence;
- expected outcome.

But semantic evaluation is deliberately **non-authoritative**.

It can return:

```text
Relevant
Uncertain
LikelyIrrelevant
```

It cannot create a capability or override a hard denial.

---

# 12. Risk-sensitive semantic policy

Accepted v0 baseline:

## Low-risk reversible actions

If all deterministic intent/capability checks pass:

```text
semantic Relevant  → allow
semantic Uncertain → allow or safer-world requirement according to policy
LikelyIrrelevant   → ask agent to narrow/justify, then deny/escalate if unresolved
```

## Medium-risk actions

```text
Uncertain/LikelyIrrelevant → NeedsApproval or deny
```

## High-risk / irreversible actions

Semantic relevance alone is never sufficient.

Require explicit pre-authorization or scoped user approval in addition to deterministic checks.

This limits the security role of probabilistic model judgement.

---

# 13. Intent drift

Intent drift is a runtime condition, not only a model-behavior concept.

Signals:

```text
orphan actions with no current plan step
repeated edits outside active task resource neighborhood
new effect domains not previously used by task
plan expansion without progress on acceptance criteria
child goals increasingly distant from parent goal
high action count with no criterion/evidence progression
```

Response:

```text
IntentDriftSuspected
   ↓
request re-plan / justification
   ↓
possibly narrow authority
   ↓
possibly require approval/user decision
```

Do not automatically terminate useful exploration from one heuristic signal.

---

# 14. Acceptance criteria as execution anchors

Criteria should influence more than final completion.

Example:

```text
C1 failing test reproduced
C2 fix implemented
C3 auth tests pass
```

Plan/action nodes can point to criteria.

The runtime can detect:

```text
50 actions executed
no criterion progressed
```

and trigger plan review/budget policy.

This ties autonomy to measurable user outcomes.

---

# 15. Approval model

Approval is action-scoped/versioned.

```go
type Approval struct {
    ID             ApprovalID
    Actor          ActorRef
    IntentID       IntentID
    ActionSelector ActionSelector
    EffectSelector EffectSelector
    ResourceScope  ResourceSelector
    MaxUses        uint32
    ValidUntil     *time.Time
    Status         ApprovalStatus
}
```

An approval is consumed/checked at execution time.

Examples:

```text
allow one git push to feature/auth
allow staging deploy for transaction tx_42
allow send of exact drafted email object
```

Avoid vague "always allow everything" prompts in the normal flow.

---

# 16. TOCTOU protection

Authorization and execution must not drift apart.

Possible race:

```text
authorize file write to resolved path A
symlink/path changes
execute now targets B
```

World/action adapters must bind authorization to stable resolved target identity where possible.

Likewise:

```text
approved object hash
must equal executed/published object hash
```

For sensitive actions, action parameters are immutable after authorization; changing them creates a new ActionID and requires re-authorization.

---

# 17. Policy versioning

Historical authorization stores exact versions of:

```text
Intent
Capability grant set
Effect schema
Risk policy
Semantic evaluator policy/model if used
Approval
World enforcement profile
```

Replay/debugging can reconstruct why an action was allowed then even if current policy changed.

---

# 18. Prompt injection boundary

Untrusted content may propose a plan or rationale.

It cannot directly mutate:

```text
Intent Envelope
Capability grants
Approval records
Risk policy
Effect classification floor
```

Memory derived from untrusted content also cannot upgrade those objects.

This composes with Epistemic Memory provenance non-amplification.

---

# 19. Effect classification floor

Tools/Worlds provide a minimum effect declaration.

The model may request a **stricter** classification but never a weaker one.

Example:

```text
World adapter: remote git push = Irreversible/ExternalVisible
Model claims: Reversible
→ kernel uses Irreversible
```

Dynamic World guarantees may strengthen reversibility only through trusted adapter state.

---

# 20. Safe-world redirection

Authorization may return:

```text
NeedsSaferWorld
```

Example:

```text
Action: modify repository
Capability: valid
Intent: valid
Current LocalWorld: direct-host mutation risk too high
Policy: transaction required
```

The runtime can propose:

```text
create isolated World / transaction
re-run action there
```

This is better than binary allow/deny when risk can be structurally reduced.

---

# 21. Authorization result

```go
type AuthorizationResult interface { isAuthorizationResult() }

Allowed(ActionProof)
Denied(ReasonCode)
NeedsApproval(ApprovalSpec)
NeedsSaferWorld(WorldRequirements)
NeedsNarrowerAction(NarrowingHint)
NeedsReplan(DriftDiagnostic)
```

Agent/harness receives typed outcomes, not policy prose.

---

# 22. Performance

Authorization is in the hot path and must be cheap for common actions.

Design:

```text
compiled typed selectors
bounded effective-grant cache
intent domain bitsets/typed IDs
resource normalization cache
policy versions in cache keys
semantic gate only when required
```

No network LLM call for every file read/write.

Common deterministic checks should be micro/millisecond-scale and independent of conversation length.

---

# 23. Failure semantics

## Semantic evaluator unavailable

Low-risk policy MAY fall back to deterministic checks if configured.

Medium/high-risk ambiguous action → NeedsApproval/deny, not silent allow.

## Policy storage unavailable

Fail closed for state-changing actions.

## Approval race/expiry

Re-check approval at operation start.

## Capability revoked after authorization but before execution

Operation-start lease semantics from capability model apply; emergency revocation can separately cancel active operation.

## Action parameters mutate after proof

Proof invalid; create/re-authorize new ActionID.

---

# 24. Required tests

```text
INT-001 capability exists but forbidden intent domain still denies
INT-002 intent relevance cannot create missing capability
INT-003 child task cannot broaden parent domain/resource scope
INT-004 action parameter change invalidates prior authorization proof
INT-005 model cannot downgrade trusted effect classification
INT-006 semantic evaluator outage cannot auto-allow high-risk action
INT-007 exact approval cannot authorize sibling/different ActionID outside selector
INT-008 prompt-injected content cannot amend Intent
INT-009 action proof records exact policy/grant/intent versions
INT-010 path/resource TOCTOU adapter test cannot escape authorized target
INT-011 orphan risky action triggers narrow/replan/approval path
INT-012 NeedsSaferWorld redirects mutation to isolated transactional World
INT-013 root user amendment creates new intent version and old actions remain attributable
INT-014 memory statement about prior permission cannot substitute active approval
INT-015 100k-history session does not increase deterministic authorization latency materially
```

---

# 25. Innovation opportunity

The strongest idea is not a semantic classifier saying whether an action "sounds relevant".

It is the combination:

```text
immutable/versioned user Intent Envelope
+ capability authority
+ typed effect domains
+ scoped delegated task intents
+ purpose-carrying actions
+ risk-sensitive semantic gate
+ exact scoped approvals
+ safe-world redirection
+ versioned Action Proof
+ intent-drift observability
```

This creates something close to **proof-carrying agency**: every meaningful action travels with a machine-inspectable causal/authority envelope.

---

# Accepted A0 decisions

1. Capability and intent are separate mandatory authorization dimensions.
2. Typed domain/resource rules are authoritative; semantic relevance is secondary and cannot override denials.
3. Child typed intent boundaries monotonically narrow.
4. Every non-trivial action carries task/purpose references.
5. High-risk/irreversible actions require explicit policy/approval beyond semantic relevance.
6. Approved parameters become immutable for that authorization; mutation requires re-authorization.
7. Intent amendments are user/versioned events, never silent agent rewrites.
8. The kernel can respond `NeedsSaferWorld` instead of allowing unsafe direct execution.
9. Authorization decisions persist an Action Proof with exact versions.
10. Memory/history cannot substitute for current capability/approval.

> **The model explains why. The kernel proves what authority actually exists.**
