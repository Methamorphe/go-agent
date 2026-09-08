# Adaptive Teams + Agent Negotiation

## Status

G11 runtime contract.

## Goal

G11 turns recursive agents into temporary, bounded organizations instead of static graphs. A lead process may propose a team of specialists, but team formation is admitted by runtime policy before any child is created. Disagreement between team members is represented by a finite evidence-oriented protocol rather than an unbounded chat loop.

## Layering

G11 is deliberately built on existing runtime guarantees:

```text
G11 Team / Negotiation
        |
        +-- admission policy / Cognitive Scheduler
        +-- G5 spawn + authority subset + budget conservation
        +-- durable Agent Processes
        +-- G9 evidence references
        +-- SQLite event/state durability
```

G11 does not create a second capability system, child budget ledger or model scheduler.

## Team lifecycle

```text
PROPOSED
   |
   +-- admission denied ----------> REJECTED
   |
   +-- admission accepted --------> ADMITTED
                                      |
                                      +-- all specialists spawned --> ACTIVE
                                      |
                                      +-- partial spawn failure ----> FORMATION_FAILED

ACTIVE -- unresolved bounded dispute --> ESCALATED
```

A proposal contains:

- root and lead Agent IDs;
- one explicit objective;
- temporary specialist profiles;
- delegated authority and budget per specialist;
- result/evidence contract;
- optional deadline;
- bounded member, parallelism, negotiation-round and turn limits.

Roles are unique within a proposal. Empty tasks and zero/negative specialist budgets are rejected before admission.

## Scheduler admission boundary

`TeamAdmitter` is the mandatory admission boundary. `Service.Form` cannot create a specialist before the proposal has an affirmative admission decision.

This keeps policy replaceable: G6 Cognitive Scheduler/capacity policy, a deterministic test policy, or a later learned advisory policy may implement admission, but no policy can bypass the G5 spawn path. Every accepted specialist still goes through G5 authority-subset checks, root budget conservation, depth/fan-out limits and durable admission queues.

A denied proposal becomes `REJECTED` and performs zero spawns.

## Specialist profiles

A specialist profile is ephemeral team structure, not durable new authority. It carries:

```text
role
task intent
delegated capability subset
reserved child budget
result/evidence contract
model objective hint
independent-verification flag
```

The model objective is advisory metadata. Actual invocation routing remains a G6 decision and stable Agent identity is preserved across model changes.

## Negotiation lifecycle

```text
OPEN
  |
  +-- concession / accepted revision --> CONVERGED
  |
  +-- explicit escalation ------------> ESCALATED
  |
  +-- round/turn bound ----------------> ESCALATED
  |
  +-- deadline ------------------------> EXPIRED
```

`EXPIRED` is terminal and still records an escalation turn and target so expiry never disappears as a silent timeout.

The default bounds are:

```text
members: 8
rounds: 4
turns: 32
```

Hard implementation caps are:

```text
members: 16
rounds: 16
turns: 128
```

These bounds are part of the kernel contract, not prompt instructions.

## Turn protocol

Supported turns are:

```text
claim
challenge
evidence
counterexample
revision
concede
escalation
```

Rules are deliberately restrictive:

- the first turn is a root claim;
- later turns reference an existing turn;
- a participant cannot challenge its own claim/revision;
- evidence must answer a challenge and carry at least one evidence reference;
- counterexamples require evidence references;
- revisions answer challenge/evidence/counterexample turns;
- concession records an explicit resolution;
- escalation records an explicit reason and target.

Evidence payloads are references (`evidence://`, `object://`, etc.), not copied transcripts. This preserves G9/G10 provenance and context-boundedness.

## Round semantics

A round advances when a new challenge contests a claim or revision. If accepting another challenge would exceed `MaxRounds`, the runtime appends an escalation turn instead. The model cannot extend the dialogue by asking for another round.

The total transcript is independently bounded by `MaxTurns` and the team deadline.

## Escalation

The deterministic G11 escalation target is the team lead. A later policy may route the escalated dispute to a stronger evaluator or user-facing approval surface, but G11 always records that escalation occurred and why.

Escalation never grants additional capability.

## Durability and concurrency

Migration `0012_adaptive_teams_negotiation.sql` stores:

- current team state;
- current negotiation state/version/round;
- append-only ordered negotiation turns.

Turn append uses optimistic version CAS inside one SQLite transaction. A concurrent stale writer fails rather than overwriting another turn.

Restart reconstruction preserves:

```text
negotiation state
version
round
participant set
resolution/escalation target
turn ordering
reply-to causal links
evidence references
```

## Failure semantics

- admission failure: no child is spawned;
- policy rejection: durable `REJECTED` team;
- specialist spawn failure after earlier members were created: durable `FORMATION_FAILED` with already-created members explicitly recorded; no false atomic-formation claim is made;
- stale negotiation writer: conflict;
- invalid turn transition: invalid argument;
- outsider participant/turn: permission denied;
- deadline/round/turn exhaustion: terminal escalation/expiry rather than infinite dialogue.

## Security invariants

G11 cannot:

- mint authority from role names, claims or evidence;
- exceed G5 delegated child budgets;
- bypass root fan-out/depth constraints;
- promote evidence into capability;
- weaken World/Effect/Intent gates;
- silently continue negotiation beyond runtime bounds.

## Killer demonstration

The contract test uses this sequence:

```text
Reviewer:    claim race exists
Implementer: challenge; request reproducer
Reviewer:    evidence://race-reproducer-42
Implementer: revision; confirms race and remediation
Reviewer:    concede; explicit shared resolution
```

The resulting negotiation is `CONVERGED` and the evidence reference remains in the durable transcript.

A second test exceeds the configured disagreement-round limit and proves deterministic escalation to the lead instead of another peer round.
