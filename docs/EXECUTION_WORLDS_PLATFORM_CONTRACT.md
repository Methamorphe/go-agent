# Execution Worlds and Platform Contract

## Status

**A0 architecture contract — ACCEPTED semantic baseline. Some adapter mechanics remain prototype-validated.**

A World is the kernel boundary between an Agent Process and an environment in which observable effects occur.

Core rule:

> **A World advertises guarantees. The kernel never assumes isolation, rollback or enforcement that the selected World cannot actually provide.**

---

# 1. Why Worlds exist

Without a World boundary, tools directly operate on:

```text
host filesystem
host processes
network
credentials
Docker
remote SSH
browser
Python runtime
```

Then authorization and transaction semantics become tool-specific.

Worlds normalize execution while preserving different guarantee levels.

---

# 2. Minimal World contract

Keep the interface small.

```go
type World interface {
    ID() WorldID
    Profile(ctx context.Context) (WorldProfile, error)
    Execute(ctx context.Context, req ActionRequest) (ActionHandle, error)
    Inspect(ctx context.Context, req InspectRequest) (InspectResult, error)
    Close(ctx context.Context) error
}
```

Snapshot/fork/promotion are optional capabilities discovered through profile/adapters rather than methods every World must fake.

Possible optional interfaces:

```go
type SnapshotWorld interface { ... }
type ForkableWorld interface { ... }
type PromotableWorld interface { ... }
```

---

# 3. World Profile

```go
type WorldProfile struct {
    Type              WorldType
    Platform          Platform
    Filesystem        FilesystemGuarantees
    Processes         ProcessGuarantees
    Network           NetworkGuarantees
    Secrets           SecretGuarantees
    Snapshot          SnapshotGuarantees
    Fork              ForkGuarantees
    Promotion         PromotionGuarantees
    ResourceLimits    ResourceLimitGuarantees
    EnforcementLevel  EnforcementLevel
    ProfileVersion    uint32
}
```

Policy matches required guarantees against profile.

---

# 4. Enforcement levels

Initial vocabulary:

```text
Advisory
HostMediated
Isolated
StronglyIsolated
```

These are not marketing labels; every level maps to concrete guarantees.

Example:

```text
LocalWorld filesystem path check → HostMediated
OCI mount namespace             → Isolated
remote hardened sandbox         → StronglyIsolated later
```

Do not call LocalWorld a sandbox.

---

# 5. World types

## LocalWorld

Direct same-user host execution under kernel mediation.

Strengths:

- zero external dependency;
- fastest developer iteration;
- direct access to repository/toolchain.

Weaknesses:

- host filesystem/process isolation is limited;
- network restriction may be incomplete cross-platform;
- rollback depends on higher-level workspace mechanism;
- arbitrary shell is broad authority.

Use only under explicit policy.

## WorkspaceWorld

Isolated developer workspace layered over local execution.

v0 target:

```text
Git/versioned base + isolated workspace directory/worktree
```

Provides mutation isolation for repository work without requiring OCI runtime.

## OCIWorld

Container-based execution with stronger:

- filesystem mounts;
- process namespace;
- CPU/memory limits;
- environment isolation;
- network mode controls.

Docker/Podman/containerd adapter details can vary.

## PythonWorld

Persistent Python/IPython-like execution environment.

It is normally hosted inside Local/Workspace/OCI resource policy rather than becoming a privileged bypass.

## SSHWorld

Remote execution with explicit remote identity/resource scope.

## BrowserWorld

Controlled browser environment with domain/network/persistence policy.

## KubernetesWorld

Later remote pod/job/workspace implementation.

---

# 6. Action execution

World never receives raw LLM JSON directly.

Flow:

```text
model/harness proposal
→ typed ActionRequest
→ effect classification
→ capability + Intent-Based Authority
→ World compatibility check
→ durable operation authorization
→ World.Execute
```

`ActionRequest` parameters are immutable after authorization.

---

# 7. Action handle and streams

```go
type ActionHandle interface {
    ID() OperationID
    Result() <-chan ActionResult
    Stdout() io.Reader
    Stderr() io.Reader
    Cancel(ctx context.Context) error
}
```

Exact Go API may differ, but semantics require:

- streaming output;
- bounded runtime buffers;
- durable OperationID;
- cancellation;
- one terminal outcome/reconciliation state.

Do not require collecting stdout/stderr into `[]byte`.

---

# 8. Output policy

```text
process stdout/stderr
      ↓
streaming object writer
      ├─ content-addressed final object
      ├─ bounded tail/preview
      └─ presentation coalescing
```

Configurable limits:

```text
max runtime
max total output bytes
max output rate optional
preview bytes/lines
```

On limit:

```text
truncate presentation only
or terminate action if hard output quota policy
```

Full artifact policy is explicit.

---

# 9. Process-tree ownership

Every started process belongs to one World operation/process group/job object.

Cancellation must target descendants, not only direct child.

Accepted platform direction:

## Unix/macOS/Linux

Use process groups/session mechanics where applicable so runtime can signal the owned group.

## Windows

Use Windows Job Objects for descendant ownership/termination rather than assuming killing the parent kills children.

This behavior must be wrapped behind one process supervisor adapter and tested per platform.

---

# 10. Cancellation ladder

Configurable execution cancellation:

```text
request graceful termination
wait grace interval
force terminate owned process tree
reconcile exit/outcome
```

For an action with externally visible effects, process termination does not imply rollback.

Effect/outcome state remains separate.

---

# 11. Filesystem paths

All filesystem actions use **logical World-relative paths**.

Pipeline:

```text
logical path
→ clean/normalize
→ resolve within World root
→ evaluate symlink policy
→ obtain stable target/parent identity when possible
→ capability/intent authorization
→ execute through World adapter
```

Never authorize an arbitrary host path string then pass a differently resolved path to OS.

---

# 12. Symlink policy

Accepted v0 default:

```text
reads: symlink resolution allowed only if final resolved target remains inside authorized World scope
writes/create/delete: resolve parent/target and reject escape outside scope
symlink creation: separate explicit filesystem operation/capability
```

Sensitive adapters use handle/fd-relative operations where available to reduce TOCTOU races.

Cross-platform canonicalization differences are adapter responsibility.

---

# 13. Network guarantees

World profile states what it can actually enforce:

```text
None
ObserveOnly
DomainProxyAllowlist
NetworkNamespaceRestricted
FullOutbound
```

LocalWorld may not be able to enforce a robust per-process domain allowlist portably without proxying.

If policy requires enforceable network isolation:

```text
LocalWorld → incompatible
NeedsSaferWorld(OCI/proxy-enabled World)
```

---

# 14. Secrets

Secret binding happens below model context.

```text
secret ref
→ authorization
→ World binds via env/file/fd/provider-specific mechanism
→ process/tool receives value
```

Logs/object streams must run secret-redaction policy where practical, but redaction is defense-in-depth; primary protection is avoiding plaintext model exposure.

---

# 15. Resource limits

World profile may support:

```text
wall time
CPU quota
memory limit
process count
file size/storage quota
output quota
network policy
```

OCI/remote Worlds can enforce more strongly than LocalWorld.

Kernel scheduler still accounts resources even when OS hard enforcement is unavailable.

---

# 16. Snapshot capability

A snapshot advertises exact guarantee:

```go
type SnapshotGuarantees struct {
    Supported       bool
    Mutations       SnapshotMutationCoverage
    CrashDurable    bool
    Consistency     SnapshotConsistency
    CostClass       CostClass
}
```

Possible consistency:

```text
MetadataOnly
FilesystemQuiescent
ApplicationCoordinated
```

Never treat a metadata-only checkpoint as rollback-capable World snapshot.

---

# 17. WorkspaceWorld v0 choice

**Accepted baseline:** Git-aware isolated workspace is the first mutation-isolation primitive for coding tasks.

Preferred semantics:

```text
base repository identity
+ captured dirty/untracked manifest where policy permits
→ isolated worktree/workspace
→ branch changes
→ diff/verification
→ three-way promotion relative to base
```

Implementation may combine:

- Git worktree/temporary branch;
- object-backed patch for pre-existing dirty state;
- copy of explicitly included untracked files.

Do not require full repository copy by default.

Exact Git mechanics are prototype-validated before G7, but external semantics are now fixed.

---

# 18. Dirty workspace policy

Creating isolated branch from dirty user workspace is dangerous if semantics are ambiguous.

Checkpoint records:

```text
HEAD/base commit
tracked dirty diff hash/object
included untracked manifest/hash
ignored files policy
```

Branch must reproduce the captured base state.

Files outside inclusion policy are not silently copied.

Promotion must detect if user's original workspace changed after checkpoint.

---

# 19. OCIWorld baseline

OCIWorld should use:

```text
read-only/controlled mounts
explicit workspace mount/overlay
network off by default
resource limits where runtime supports
secret bindings explicit
non-root user by default where practical
no host Docker socket mount by default
```

Mounting `/var/run/docker.sock` or equivalent is effectively broad host authority and must be explicit high-risk capability/policy.

---

# 20. PythonWorld baseline

PythonWorld is a persistent computation service with:

```text
kernel/session ID
working directory scope
resource limits
network inherited from containing World policy
object-store bridge
stdout/stderr streaming
reset/restart operation
```

Python variables are useful ephemeral/durable-ish execution state, but canonical Agent Process correctness must not rely on an unrecoverable Python heap.

Important objects can be persisted through Object Store/evidence refs.

---

# 21. World failure states

```text
Healthy
Degraded
Lost
NeedsReconciliation
Closed
```

If World process/container disappears:

- pure/read tasks may retry if safe;
- mutation transaction reconciles snapshot/effect state;
- Agent Process survives independently.

World loss is not Agent identity loss.

---

# 22. World operation outcome

Action terminal outcome separates transport/process status from effect certainty.

```go
type ActionResult struct {
    ExitStatus       *int
    Completion       CompletionState
    OutcomeCertainty OutcomeCertainty
    EffectRefs       []EffectRecordID
    StdoutRef        *ObjectRef
    StderrRef        *ObjectRef
    ResultRef        *ObjectRef
}
```

Example:

```text
process exited 1
but external API mutation completed
```

must not be collapsed into `Failed` with assumed no effect.

---

# 23. World promotion

Only Worlds advertising compatible promotion semantics participate in transactions/forks.

Promotion adapter provides:

```text
prepare(base,target,diff)
apply(operationID)
verify resulting identity/hash
reconcile(operationID)
```

This feeds `EXECUTION_EDIT_SAFETY.md`.

---

# 24. Required tests

```text
WORLD-001 LocalWorld is never reported as stronger isolation than actual adapter
WORLD-002 path traversal/symlink cannot escape filesystem scope
WORLD-003 Unix owned process tree terminates on force-cancel
WORLD-004 Windows Job Object owned process tree terminates on force-cancel
WORLD-005 multi-GB stdout streams to object storage without proportional heap growth
WORLD-006 client/TUI disconnect cannot block World output drain indefinitely
WORLD-007 network-restricted policy rejects incompatible LocalWorld and requests safer World
WORLD-008 secret(use) reaches process without plaintext entering model-visible request
WORLD-009 WorkspaceWorld sibling mutations stay isolated
WORLD-010 dirty base divergence blocks unsafe promotion
WORLD-011 PythonWorld crash does not destroy canonical Agent identity/state
WORLD-012 OCIWorld defaults do not expose host Docker socket/network unexpectedly
WORLD-013 lost World produces structured reconciliation state for mutation
WORLD-014 effect certainty remains distinct from process exit status
WORLD-015 unsupported snapshot/fork operation fails explicitly
```

---

# Accepted A0 decisions

1. Worlds advertise concrete guarantees; no fake generic sandbox abstraction.
2. LocalWorld is host-mediated, not a secure sandbox.
3. Optional snapshot/fork/promotion capabilities are discovered rather than forced on every World.
4. Action output is streaming/object-backed by default.
5. Process descendants are owned using process groups on Unix-like systems and Job Objects on Windows.
6. Filesystem authority is World-relative with explicit symlink escape prevention.
7. Strong network restrictions require a World capable of enforcing them.
8. WorkspaceWorld using Git-aware isolated state is the first coding mutation-isolation primitive.
9. Dirty workspace state is captured explicitly and base divergence blocks promotion.
10. OCI network is off/restricted by default and host-control sockets are never implicit.
11. Python is a persistent Execution World, not canonical kernel memory.
12. World loss never means Agent identity loss.

## Prototype-validated details before G3/G7

- exact Git worktree/dirty-base capture implementation;
- platform process-tree APIs and edge cases;
- OCI runtime selection/feature detection;
- fd/handle-relative safe filesystem implementation details.

These prototypes validate adapters, not redefine World semantics.

> **A World is a contract of guarantees, not a synonym for “somewhere commands run”.**
