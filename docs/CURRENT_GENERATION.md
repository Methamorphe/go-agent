# Current Implementation Generation

**Updated: September 8, 2026**

```text
A0  COMPLETE
G0  COMPLETE
G1  COMPLETE
G2  COMPLETE
G3  COMPLETE
G4  COMPLETE
G5  COMPLETE
G6  COMPLETE
G7  COMPLETE
G8  COMPLETE
G9  COMPLETE
G10 COMPLETE
G11 READY TO IMPLEMENT
```

## Current generation

**G11 — Adaptive Teams + Agent Negotiation: READY TO IMPLEMENT.**

G10 is closed and merged. G11 is now the active implementation generation.

The implementation contract is prepared in `G11_IMPLEMENTATION_PLAN.md` and defines:

- durable Team proposal/decomposition;
- scheduler admission and atomic bounded resource reservation;
- temporary specialist profiles without authority expansion;
- typed durable claim/challenge/evidence/counterexample/revision/concession/decision/escalation protocol;
- deterministic convergence conditions;
- hard round/deadline/message/evidence/resource bounds;
- explicit no-progress and escalation outcomes;
- G8 fork/world transaction and selective-promotion integration;
- G9 Evidence/Belief and truth-maintenance integration;
- G10 semantic cognitive references and Context Fault paging integration;
- crash-safe recovery and replay idempotency;
- TEAM-001+ and NEG-001+ contract test families;
- an end-to-end race-condition dispute killer demonstration.

## Previous generation

**G10 — Context Faults + Cognitive MMU v2: COMPLETE.**

G10 turns missing cognitive state into explicit, typed, bounded runtime paging events while preserving provider independence and the authority boundaries established by earlier generations.

G10 closure includes stable semantic references, six typed Context Faults, finite context leases, bounded dependency/evidence/freshness paging, fault-storm protection, durable append-only lifecycle observability, next-manifest handoff, G9 semantic belief references, historical evidence inspection, cross-platform contract tests and a green final CI matrix.

See:

- `G10_EXIT_REVIEW.md`;
- `CONTEXT_FAULTS_AND_COGNITIVE_PAGING.md`;
- `G11_IMPLEMENTATION_PLAN.md`.

## G11 killer demonstration

Reviewer finds a race condition in an implementer's isolated candidate, implementer disputes it, reviewer supplies a reproducer/evidence reference, and the team must either revise and converge or escalate deterministically within finite rounds, deadline and resource budgets. Only a selected/verified branch may be promoted.

## G11 implementation order

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

## Long-duration / scale validation inherited from G10

```text
G10 contract tests             COMPLETE
durable lifecycle tests       COMPLETE
tagged soak compilation       COMPLETE
fault storm bounds            COMPLETE
final CI closure gate         GREEN
1h reference run              PENDING
8h reference run              PENDING
24h reference run             PENDING
```

Long-duration empirical calibration remains separate from semantic generation completion unless explicitly promoted to a mandatory exit gate.
