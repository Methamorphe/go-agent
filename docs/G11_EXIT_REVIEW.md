# G11 Exit Review — Adaptive Teams + Agent Negotiation

Status: **PENDING FINAL CI GATE**

## Scope delivered

G11 adds a durable runtime layer for temporary specialist teams and finite evidence-oriented disagreement.

### Adaptive team formation

The runtime now supports:

- explicit team proposals and task decomposition;
- temporary specialist profiles;
- mandatory policy/scheduler admission before any spawn;
- unique specialist roles;
- bounded team size, parallelism declaration, wall-clock deadline, negotiation rounds and turns;
- delegation through the existing G5 `Spawn` path so capability subsets, budget conservation, fan-out/depth and result contracts remain authoritative;
- durable team lifecycle including rejection and partial formation failure.

### Agent negotiation

The runtime exposes the bounded protocol:

```text
claim
challenge
evidence
counterexample
revision
concede
escalation
```

The protocol enforces causal `reply_to` links, participant membership, evidence requirements, finite rounds, finite total turns and deadlines.

A concession produces an explicit `CONVERGED` resolution. Exhausted bounds or explicit peer escalation produce a durable terminal escalation targeted at the team lead.

### Durability

Migration `0012_adaptive_teams_negotiation.sql` persists teams, negotiations and ordered turns.

Negotiation turn append is transactional and optimistic-versioned: a stale writer cannot overwrite a competing update.

The restart test closes and reopens SQLite and verifies that negotiation state, version, round, ordered turns and causal reply links are reconstructed exactly.

### Evidence and authority

Evidence is carried as references rather than copied transcripts. Negotiation statements do not grant authority, and specialist creation always re-enters the existing G5 authority/budget enforcement path.

## Killer tests

The G11 tests cover:

1. reviewer claims a race;
2. implementer challenges and asks for a reproducer;
3. reviewer supplies `evidence://race-reproducer-42`;
4. implementer revises its position and accepts remediation;
5. reviewer concedes and the dispute becomes `CONVERGED`;
6. a separate dispute reaches its round bound and escalates deterministically to the lead;
7. scheduler/policy rejection is proven to happen before any child spawn;
8. the negotiation transcript survives SQLite close/reopen.

## Exit invariants

- team topology is generated but bounded;
- team formation has a mandatory admission gate;
- specialists cannot bypass recursive-agent authority or budget conservation;
- peer disagreement is finite by runtime semantics, not prompt convention;
- evidence references and causal reply links survive restart;
- stale concurrent negotiation writes conflict instead of overwriting;
- escalation is explicit, deterministic and durable;
- negotiation cannot mint capabilities or weaken Intent/Effect/World enforcement;
- no unbounded dialogue queue or transcript is introduced.

## Validation gate

G11 is complete only after the final repository CI head passes:

```text
go test ./...
go test -race ./...
go test -tags soak ./internal/soaktest -run '^$'
go vet ./...
go build ./cmd/go-agent ./cmd/go-agentctl
```

The cross-platform GitHub Actions matrix is the authoritative closure gate. Until that final head is green, this document remains `PENDING FINAL CI GATE`.
