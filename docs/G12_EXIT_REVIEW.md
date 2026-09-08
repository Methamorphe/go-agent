# G12 Exit Review — Verified Continual Improvement

Status: **PASS pending final CI gate**

## Scope delivered

G12 implements versioned, evaluated and reversible cognitive-artifact improvement without allowing learned state to redefine runtime authority.

### Versioned cognitive artifacts

The runtime now supports immutable versions for:

- prompts;
- skills;
- agent profiles;
- routing policies;
- context policies;
- memory policies;
- evaluators.

Every version carries a stable artifact identity, content/origin references, explicit scope, required-capability requirements, lifecycle state and creation timestamp. Candidates additionally require an explicit hypothesis and parent baseline.

### Verified lifecycle

The implemented lifecycle is:

```text
PROMOTED baseline
→ CANDIDATE
→ EVALUATING
→ SHADOW
→ CANARY
→ PROMOTED
```

Hard failures move a candidate to `REJECTED`. Superseded promoted versions become `DEPRECATED`; rollback marks the regressing version `ROLLED_BACK` and restores a previously promoted version for future resolution.

Promotion requires a matching baseline/candidate evaluation, a minimum number of unique held-out task outcomes, optional independent-evaluator enforcement, no baseline success regression, and all hard non-regression gates to pass.

Duplicate task references are rejected so one observation cannot be counted repeatedly as independent evidence.

### Authority and safety

Cognitive artifacts can declare required capabilities but never grant capabilities. Candidate use checks requirements against the process's existing runtime grants.

`root_intent`, `effect_floor` and `capability_grant` are deliberately not valid cognitive-artifact kinds. Learned state therefore has no representation capable of rewriting those authority primitives.

Shadow candidates are read/non-effecting only. Mutating effects are rejected by runtime semantics rather than prompt convention.

Canary candidates have a durable hard sample cap. Reservation is atomic in SQLite, preventing concurrent callers from exceeding the declared rollout bound.

Security/policy regressions, critical regressions, deterministic-verification loss, replay incompatibility and irreversible-effect regressions are hard gates. They reject the candidate regardless of quality gain.

### Reproducibility and rollback

Every captured invocation manifest stores the exact artifact-version mapping plus a deterministic manifest hash. Manifest rows are immutable.

Rollback changes only the future active version. Historical invocation manifests remain attributable to the version that actually produced the historical run.

### Kill switches

Durable runtime-controlled kill switches support:

- all continual improvement;
- one artifact family;
- one exact artifact version.

The switch is consulted at candidate use, lifecycle advancement, promotion and future active resolution, so a disabled artifact stops being scheduled immediately.

### Durability

Migration `0013_verified_continual_improvement.sql` persists:

- immutable artifact versions;
- active-version pointers;
- evaluation records and raw outcomes;
- promotion records;
- rollback history;
- exact invocation manifests;
- canary usage counters;
- runtime kill-switch state.

Promotion and rollback are transactional. The restart test closes and reopens SQLite and verifies active version, promotion evidence, invocation manifest and kill-switch state.

## Required IMP tests

G12 implements the complete A0 test contract:

1. `IMP-001` candidate cannot expand capability grants;
2. `IMP-002` candidate cannot modify root Intent/policy floor;
3. `IMP-003` promotion records baseline/candidate versions and evaluation refs;
4. `IMP-004` rollback changes future active version without rewriting history;
5. `IMP-005` shadow candidate cannot execute mutating effects;
6. `IMP-006` hard security regression blocks promotion despite quality gain;
7. `IMP-007` project-scoped artifact cannot silently become global;
8. `IMP-008` historical invocation retains exact artifact manifest;
9. `IMP-009` insufficient evaluation evidence cannot auto-promote;
10. `IMP-010` kill switch prevents new candidate use immediately.

An additional killer test proves canary sample caps are hard bounds.

## Exit invariants

- self-improvement is immutable versioned experimentation, never mutable personality;
- every candidate starts from an explicit hypothesis and baseline;
- local/project scope cannot silently widen;
- evaluation provenance and raw outcomes remain durable;
- evaluation task duplication cannot inflate evidence count;
- capability requirements never become capability grants;
- root Intent, effect floor and capability grants remain outside the artifact model;
- shadow is non-mutating;
- canary rollout is bounded and durable;
- security/verification regressions are hard blockers;
- promotion is baseline-relative and durable;
- rollback only affects future resolution;
- historical artifact attribution is immutable;
- kill switches take effect on future scheduling immediately.

## CI gate

The closure gate is the repository matrix:

```text
go test ./...                                      required Linux/macOS/Windows
go test -race ./...                                required
go test -tags soak ./internal/soaktest -run '^$'  required Linux compile gate
go vet ./...                                       required Linux/macOS/Windows
go build ./cmd/go-agent ./cmd/go-agentctl          required Linux/macOS/Windows
```

This document should be changed from `PASS pending final CI gate` to `PASS` only after the G12 head passes the authoritative GitHub Actions gate.
