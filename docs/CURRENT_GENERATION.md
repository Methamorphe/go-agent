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
G11 COMPLETE
G12 COMPLETE
G13 READY
```

## Current generation

**G12 — Verified Continual Improvement: COMPLETE.**

G12 adds controlled self-improvement as immutable, versioned experimentation rather than mutable runtime personality. The system can now evaluate and promote cognitive artifacts while preserving every authority, scope, safety and historical-attribution invariant established in G0–G11.

The implementation now includes:

- immutable versioned prompt, skill, agent-profile, routing-policy, context-policy, memory-policy and evaluator artifacts;
- explicit hypotheses, baseline parentage, content/origin references and scope on every candidate;
- durable lifecycle states for candidate, evaluation, shadow, canary, promotion, rejection, deprecation and rollback;
- held-out evaluation records with raw per-task outcomes, evaluator/corpus provenance and duplicate-task contamination rejection;
- configurable minimum evidence and independent-evaluator requirements before promotion;
- hard non-regression gates for security/policy violations, critical regressions, verification loss, replay incompatibility and irreversible-effect regression;
- capability requirements that are checked against existing runtime grants but can never mint authority;
- deliberate exclusion of root Intent, Effect-floor and capability-grant mutation from the cognitive-artifact model;
- strict scope equality across candidate lineage so project/user/task-local learning cannot silently become global;
- non-mutating Shadow semantics;
- durable Canary sample caps with atomic reservation;
- transactional baseline/candidate promotion and transactional rollback;
- immutable invocation artifact manifests with deterministic hashes for exact historical attribution;
- durable all/family/version kill switches that take effect on future artifact use immediately;
- SQLite persistence through migration `0013_verified_continual_improvement.sql`;
- restart reconstruction of active versions, promotion evidence, invocation manifests and kill-switch state;
- complete `IMP-001` through `IMP-010` contract coverage plus a hard canary-bound killer test.

The G12 killer condition is explicit: a candidate may outperform its baseline on quality or cost, but it cannot promote if it introduces a security/verification regression, needs authority the process does not already possess, widens scope, lacks enough unique evaluation evidence, or has been disabled by a runtime kill switch. Rollback restores a prior promoted version for future use without modifying historical invocation attribution.

See:

- `G12_EXIT_REVIEW.md`;
- `VERIFIED_CONTINUAL_IMPROVEMENT.md`;
- `G11_EXIT_REVIEW.md`.

## Validation note

GitHub Actions run `34224392815` passed the G12 implementation head across Linux, macOS and Windows, including the race detector, vet, builds and Linux soak compile gate.

## Long-duration / scale validation

```text
G12 IMP contract/killer tests   PASS
durable restart replay         PASS
atomic canary bound             PASS
cross-platform CI              PASS
race detector                  PASS
1h reference run               PENDING
8h reference run               PENDING
24h reference run              PENDING
```

Long-duration empirical calibration remains separate from G12 semantic completion.

## Next generation

**G13 — Production TUI / Interactive Agent Workspace: READY.**

G13 can now expose the durable process tree, worlds/transactions/forks, epistemic memory, Context/MMU state, scheduler decisions, adaptive teams/negotiation and verified cognitive-artifact lifecycle through the production terminal workspace without moving canonical state into the UI.
