# G3 Exit Review — Worlds + Authority + Effect System

Status: **PASS**

Date: August 28, 2026

## Scope delivered

G3 establishes the formal execution-security boundary defined in A0:

- typed World `Action`, `Result`, `Profile` and effect contracts;
- `LocalWorld` filesystem and process execution implementation;
- workspace-root confinement with symlink-aware existing-path resolution;
- Unix process-group cancellation and Windows process-tree termination adapter;
- filesystem read/write, process execution and network capability domains;
- scoped grants and deterministic lease expiry;
- delegation subset validation for domain, scope and lease duration;
- canonical Effect classification with idempotency/retryability traits;
- versioned root Intent policy with allowed/forbidden domains and acceptance-criteria metadata;
- Purpose-Carrying Actions;
- kernel-generated `ActionProof` on authorization success;
- typed authorization decisions;
- `SecureWorld` authorization gate that refuses an action before the underlying World is invoked;
- security-decision sink so authorization outcomes can be causally recorded by the Ledger integration layer.

## Security invariants proved

The deterministic G3 test suite proves:

1. a child cannot acquire a capability absent from its parent;
2. delegated scope cannot be broader than the parent scope;
3. a child lease cannot outlive its parent lease;
4. an expired lease fails deterministically;
5. a read-only agent cannot write even if model/tool parameters claim extra authority;
6. prompt/tool content cannot mint capabilities;
7. an unauthorized action never reaches the underlying World implementation;
8. model-supplied Effect metadata cannot downgrade the kernel's canonical classification;
9. an allowed action produces an Action Proof bound to the Intent version, purpose and capability;
10. each authorization attempt can emit one explicit security decision.

## Effect model

Canonical classes implemented:

```text
Pure
Read
Reversible
Compensatable
Irreversible
```

Initial classifications:

- filesystem read/list → `Read`, idempotent, retryable;
- filesystem write → `Reversible`, idempotent, retryable;
- process execution → `Compensatable`, non-idempotent, non-retryable by default;
- network request foundation → `Irreversible`, non-idempotent, non-retryable by default.

The model cannot lower this classification because authorization recomputes and compares the canonical Effect.

## World / platform behavior

`LocalWorld` exposes filesystem and process actions inside a configured workspace root. Existing resources are resolved through symlinks before the root-boundary check. Process actions use `exec.CommandContext`; Unix commands are placed in their own process group and Windows termination uses tree-aware `taskkill /T /F`.

The `Profile` explicitly advertises cancellation, streaming capability and process-tree cancellation support. G3 defines the boundary; later Worlds (Workspace/OCI/remote workers) can implement the same contract without changing the authority model.

## Authority semantics

Authority is derived from kernel-owned inputs only:

```text
root Intent
+ granted capabilities
+ current lease time
+ canonical action kind/effect
+ purpose/resource
→ authorization decision
→ Action Proof (only on allow)
```

Model output is untrusted action content. It is never interpreted as a grant.

## Validation

Implementation commits on `main`:

```text
15c4443 feat(g3): add world action and effect contracts
2ebdb60 feat(g3): add capability and intent authority
aeee360 feat(g3): add authorized world execution boundary
87386e4 feat(g3): implement local world filesystem and process actions
7707aa2 feat(g3): add unix process-tree cancellation
623a52c feat(g3): add windows process-tree cancellation
fcdebded test(g3): cover world authority killer cases
```

GitHub Actions validation run: `33203489779`.

The generation is considered closed only with the full Ubuntu/macOS/Windows matrix and race detector green.

## G3 decision

**PASS — G3 is complete.**

G4 may now build Cognitive MMU v0 without inventing new execution authority semantics.
