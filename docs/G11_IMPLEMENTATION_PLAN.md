# G11 — Adaptive Teams + Agent Negotiation

**Status: READY TO IMPLEMENT**

G11 moves the runtime from static multi-agent execution graphs to bounded, inspectable, scheduler-admitted adaptive teams whose members can make claims, challenge them with evidence, revise positions and escalate deterministically.

## Goal

Allow the runtime to form temporary specialist teams when a task benefits from parallel or adversarial reasoning, while preserving all existing authority, budget, world-transaction, memory and context-paging invariants.

The target is not free-form agent chat. Negotiation is a typed runtime protocol with explicit rounds, deadlines, budgets, evidence references and terminal outcomes.

## Foundation already available

G11 builds directly on:

- G6 Cognitive Scheduler admission and resource accounting;
- G7 Workspace/OCI World + Agent Transactions;
- G8 checkpoints, forks, branch-local overlays and selective promotion;
- G9 Evidence/Belief graph and truth maintenance;
- G10 typed cognitive references, Context Faults, leases and bounded paging.

No G11 component may bypass those layers.

---

# 1. Runtime concepts

## Team

A Team is a durable runtime object scoped to one root task or negotiation objective.

Required fields:

```text
TeamID
RootAgentID
Objective
Status
CreatedAt
Deadline
RoundLimit
CurrentRound
BudgetEnvelope
AdmissionState
EscalationPolicy
```

Suggested statuses:

```text
PROPOSED
ADMITTED
ACTIVE
CONVERGED
ESCALATED
FAILED
CANCELLED
EXPIRED
```

## Team member

Members are temporary specialist profiles, not new authority principals.

Each member records:

```text
MemberID
TeamID
AgentID
Role
Specialty
Mandate
ParentAgentID
BudgetSlice
AllowedScopes
CreatedAt
Status
```

Example roles:

```text
implementer
reviewer
tester
researcher
security_reviewer
performance_reviewer
arbiter
```

Specialist profiles may change prompting, tools presented, retrieval policy and evaluation criteria, but they may never widen Intent, capability grants or World permissions.

---

# 2. Team proposal and decomposition

Add a provider-independent proposal contract:

```go
type TeamProposal struct {
    Objective     string
    Rationale     string
    Members       []MemberProposal
    Budget        scheduler.Resources
    Deadline      time.Time
    RoundLimit    int
    SuccessPolicy SuccessPolicy
}
```

A proposal must be evaluated before creating child agents.

Admission checks must include:

- root task still active;
- no authority expansion;
- total requested resources fit scheduler/root budgets;
- role count and team size within configured limits;
- deadline bounded by task/runtime policy;
- no duplicate/unproductive specialist topology;
- current fork/team limits not exceeded.

A rejected proposal remains observable with a typed rejection reason.

---

# 3. Scheduler admission

G11 must use G6 rather than spawning agents directly.

Admission should reserve resources atomically for the team before children begin work.

Suggested limits:

```text
MaxTeamMembers
MaxActiveTeamsPerRoot
MaxNegotiationRounds
MaxClaimsPerRound
MaxEvidenceRefsPerMessage
MaxWallTime
MaxTeamTokens
MaxTeamMoneyMicros
```

Team reservation must be released or settled on every terminal path, including crash recovery, cancellation and expiration.

---

# 4. Negotiation protocol

Negotiation messages are typed durable records, not arbitrary chat transcripts.

## Message kinds

```text
CLAIM
CHALLENGE
EVIDENCE
COUNTEREXAMPLE
REVISION
CONCESSION
DECISION
ESCALATION
```

## Minimal message shape

```go
type NegotiationMessage struct {
    ID            NegotiationMessageID
    TeamID        TeamID
    Round         int
    AuthorAgentID id.AgentID
    Kind          MessageKind
    ClaimID       *ClaimID
    ParentID      *NegotiationMessageID
    Statement     string
    References    []mmu.CognitiveRef
    Confidence    float64
    CreatedAt     time.Time
}
```

Every reference is resolved through G10 paging rules. Missing evidence may generate a Context Fault; it must never be silently injected.

---

# 5. Claim lifecycle

Claims are durable objects with explicit state.

Suggested states:

```text
OPEN
CHALLENGED
SUPPORTED
REFUTED
REVISED
CONCEDED
UNRESOLVED
ACCEPTED
```

A claim must preserve:

- original statement;
- current revision;
- author;
- supporting Evidence/Belief/context refs;
- challenges/counterexamples;
- confidence history;
- final disposition.

Revisions must append history rather than overwrite prior reasoning artifacts.

---

# 6. Convergence rules

Convergence must be deterministic and bounded.

A team can converge when, for example:

- all blocking claims are accepted/conceded/refuted;
- required reviewer roles approve;
- no unresolved challenge above configured severity remains;
- objective-specific checks pass;
- minimum evidence requirements are satisfied.

The implementation must not use "agents stopped disagreeing" as the sole convergence criterion.

---

# 7. Deadlines, rounds and anti-loop protection

Hard invariants:

- finite round limit;
- finite message count per round;
- finite evidence/context paging budget;
- finite wall-clock deadline;
- repeated claim/challenge pairs detected;
- no recursive team creation without scheduler admission;
- no team can extend its own deadline or budget.

Suggested terminal reasons:

```text
CONVERGED
ROUND_LIMIT
DEADLINE
BUDGET_EXHAUSTED
NO_PROGRESS
AUTHORITY_DENIED
MEMBER_FAILED
EVIDENCE_INSUFFICIENT
```

Progress should be explicit, e.g. claim state changed, new evidence introduced, a challenge resolved or an objective check completed.

---

# 8. Escalation

Escalation is a first-class runtime outcome, not an error string.

Escalation may target:

- root agent;
- designated arbiter member;
- human approval boundary;
- deterministic fallback strategy.

Escalation payload should include:

```text
objective
unresolved claims
best-supported positions
blocking evidence gaps
resource usage
round/deadline state
recommended next action
```

The escalation artifact must be durable and replayable.

---

# 9. Integration with G8 forks and transactions

Implementation candidates may work in isolated G8 forks.

Rules:

- each candidate implementation stays in its own transactional World/fork;
- negotiation may inspect results through evidence/context references;
- reviewer disagreement cannot mutate another branch directly;
- final winner selection follows explicit objective/evaluation rules;
- only the selected branch can be promoted;
- cognitive artifacts marked non-promotable remain branch-local.

This enables the G11 killer demo without weakening G8 isolation.

---

# 10. Integration with G9 epistemic memory

Negotiation should consume G9 knowledge without flattening provenance.

A CLAIM may reference Beliefs; an EVIDENCE message should prefer immutable Evidence refs where possible.

If a referenced Belief becomes stale/contested during negotiation:

- its claim support must be re-evaluated;
- dependent claims may move back to OPEN/CHALLENGED;
- original history remains inspectable;
- confidence cannot mint authority.

G9 truth-maintenance notifications should eventually be able to wake active negotiations in a bounded way, but initial G11 may perform this check at round boundaries.

---

# 11. Integration with G10 Context Faults

Negotiation payloads carry semantic refs only.

Examples:

```text
belief://blf_architecture
 evidence://evd_race_reproducer
 ctx://ctx_test_output
 object://sha256:...
```

When a member needs missing/stale/supporting knowledge, G10 handles it through typed faults and finite leases.

Team negotiation must share the root task's paging/resource envelope; it cannot create an independent unlimited context budget.

---

# 12. Persistence

Proposed SQLite migrations:

```text
0012_teams.sql
0013_negotiation.sql
```

Suggested tables:

```text
teams
team_members
team_admissions
team_resource_usage
negotiation_rounds
negotiation_messages
negotiation_claims
negotiation_claim_history
negotiation_escalations
```

Indexes must support:

- active teams by root agent;
- messages by team/round;
- open claims by team;
- claim history by claim;
- unresolved/blocking claims;
- deadline scanning;
- member lookup;
- crash recovery.

No runtime path should require scanning all historical negotiations.

---

# 13. Crash recovery

After restart, the runtime must be able to recover:

- admitted/active team state;
- scheduler reservations;
- current round;
- member identities and statuses;
- claim states;
- unresolved challenges;
- deadline and round limits;
- escalation state.

Replaying a completed negotiation must not duplicate promotions, commits, messages or resource charges.

---

# 14. Observability

Every meaningful transition should be inspectable:

```text
team.proposed
team.admitted
team.rejected
team.started
member.spawned
round.started
claim.created
claim.challenged
evidence.added
claim.revised
claim.resolved
round.completed
team.converged
team.escalated
team.expired
team.failed
```

Provider transcript details remain secondary; canonical runtime state is the typed durable protocol.

---

# 15. Security and authority invariants

Hard requirements:

1. Team creation cannot widen root Intent.
2. Specialist roles cannot mint capabilities.
3. Child agents receive the intersection of current authority and their specialist mandate.
4. Negotiation output cannot directly commit World mutations.
5. Promotion still requires G7/G8 transaction rules.
6. Evidence confidence does not alter authorization.
7. Escalation cannot bypass approval boundaries.
8. Cancellation/revocation propagates to active team members.

---

# 16. Initial package layout

Suggested implementation structure:

```text
internal/team/
    types.go
    manager.go
    admission.go
    members.go
    recovery.go

internal/negotiation/
    types.go
    manager.go
    claims.go
    protocol.go
    convergence.go
    escalation.go

internal/storage/sqlite/
    team_store.go
    negotiation_store.go
    migrations/0012_teams.sql
    migrations/0013_negotiation.sql
```

Prefer narrow interfaces to imports between scheduler, process, fork, memory, MMU and World.

---

# 17. Contract test matrix

Reserve identifiers `TEAM-001+` and `NEG-001+`.

Minimum closure set:

```text
TEAM-001 proposal persists before spawn
TEAM-002 scheduler rejection creates zero child agents
TEAM-003 admission atomically reserves bounded budget
TEAM-004 specialist authority is intersection-only
TEAM-005 team size/deadline limits enforced
TEAM-006 cancellation propagates and releases reservations
TEAM-007 crash recovery resumes active team without duplicate children

NEG-001 claim/challenge/evidence lifecycle is append-only
NEG-002 evidence refs use cognitive references and G10 paging
NEG-003 round limit terminates negotiation deterministically
NEG-004 wall deadline terminates negotiation deterministically
NEG-005 repeated challenge loop triggers NO_PROGRESS
NEG-006 revision preserves previous claim history
NEG-007 stale/contested belief cannot silently remain accepted support
NEG-008 convergence requires objective blocking conditions to clear
NEG-009 escalation preserves unresolved positions and evidence
NEG-010 replay is idempotent
NEG-011 loser branch cannot leak World mutations
NEG-012 only selected winner may promote G8 cognitive/world artifacts
NEG-013 authority revocation during negotiation is revalidated
NEG-014 resource exhaustion cannot be bypassed by creating more members
NEG-015 concurrent message/claim updates are race-safe
```

---

# 18. Killer demonstration

Required end-to-end scenario:

1. Root task proposes an implementer + reviewer team.
2. Scheduler admits both with bounded resources.
3. Implementer creates a candidate in an isolated G8 fork.
4. Reviewer raises a `CLAIM` that the candidate contains a race condition.
5. Implementer sends a `CHALLENGE` disputing the claim.
6. Reviewer supplies a reproducible test/log as immutable evidence/context reference.
7. Missing test context is paged through G10 if needed.
8. Implementer either revises the implementation or provides counterevidence.
9. Reviewer re-evaluates the revised candidate.
10. Team converges deterministically, or reaches round/deadline limit and creates a durable escalation.
11. If converged, only the accepted candidate is eligible for transaction verification/promotion.
12. Full negotiation history, evidence, resource usage and final rationale remain inspectable after restart.

No unlimited dialogue is permitted.

---

# 19. Exit gate

G11 is COMPLETE only when:

- team proposal/admission is durable and bounded;
- specialist members cannot widen authority;
- negotiation protocol is typed and append-only;
- claims/challenges/evidence/revisions are replayable;
- deadlines/rounds/no-progress protections are enforced;
- escalation is durable;
- G8/G9/G10 integration is covered;
- crash/restart idempotency is tested;
- race detector passes;
- standard CI matrix is green;
- tagged soak/scale scenarios compile.

Long-duration 1h/8h/24h empirical runs remain calibration gates separate from semantic completion unless explicitly promoted to mandatory closure criteria.

## Implementation order

Recommended sequence:

```text
G11.1 IDs + typed contracts
G11.2 durable Team store + admission
G11.3 specialist child creation + authority intersection
G11.4 durable negotiation messages + claims
G11.5 bounded rounds/deadlines/no-progress
G11.6 G9/G10 evidence integration
G11.7 G8 fork/evaluation/promotion integration
G11.8 crash recovery + idempotency
G11.9 killer demo + race/CI/soak gates
```

This order intentionally establishes persistence and authority boundaries before adding negotiation behavior.