# World Action and Effect Protocol

## Purpose

The World boundary is where probabilistic agent decisions become observable operations.

It must therefore be one of the most explicit contracts in the system.

Core rule:

> **The model proposes an action. The kernel validates authority/intent/effects/budget. The World executes only an authorized immutable action request.**

---

# 1. World responsibilities

A World provides controlled execution against an environment.

It is responsible for implementing environment-specific mechanics such as:

- filesystem operations;
- process execution;
- network policy enforcement where supported;
- resource limits;
- snapshot/fork/rollback capabilities;
- action reconciliation;
- secret binding into execution;
- streamed outputs.

A World does **not** decide user intent, child delegation or LLM policy.

---

# 2. World capability descriptor

Each World instance exposes immutable/slow-changing abilities.

Conceptual descriptor:

```go
type WorldCapabilities struct {
    ExecuteProcesses   bool
    ReadFilesystem     bool
    WriteFilesystem    bool
    NetworkPolicy      NetworkPolicyLevel
    ResourceLimits     bool
    SecretsBinding     bool
    Snapshot           bool
    Fork               bool
    Rollback           bool
    DurableIdentity    bool
    ReconcileActions   bool
}
```

Do not infer `Fork=true` just because a World can copy files somehow. Guarantees must be documented by adapter.

---

# 3. Action identity

Every execution attempt has stable IDs:

```text
ActionID            logical authorized action
AttemptID           one concrete execution attempt
AgentID
WorldID
TransactionID? / ForkID?
```

`ActionID` remains stable during reconciliation/idempotent retry logic where semantics allow.

A second non-idempotent retry may require a new ActionID linked to original rather than pretending it is same effect.

---

# 4. Action descriptor

Candidate action envelope:

```go
type Action struct {
    ID             ActionID
    AgentID        AgentID
    WorldID        WorldID
    Kind           ActionKind
    ParametersRef  ObjectRefOrInline
    Effect         EffectDescriptor
    RequiredCaps   []CapabilityRequirement
    IntentBinding  IntentRelation
    Deadline       time.Time
    OutputPolicy   OutputPolicy
    IdempotencyKey *string
    TransactionID  *TransactionID
}
```

The kernel creates/finalizes this immutable authorized action descriptor.

The World must not accept arbitrary unvalidated model JSON directly.

---

# 5. Action kinds

Initial developer World kinds should be typed, not an everything-string tool API.

Candidate v0:

```text
FS.Read
FS.List
FS.Stat
FS.Write
FS.Create
FS.Remove       later/high risk
Process.Exec
Git.Read        may initially be Process.Exec with stronger wrapper later
```

Raw shell can be represented as `Process.Exec` with shell binary only if policy explicitly permits it.

Prefer argv execution over shell concatenation for deterministic argument boundaries.

---

# 6. Effect descriptor

The descriptor separates **class** from additional properties.

```go
type EffectDescriptor struct {
    Class              EffectClass
    Idempotency        IdempotencyClass
    Retryability       RetryClass
    ExternalVisibility VisibilityClass
    Compensation       *CompensationDescriptor
    Domains            []EffectDomain
    RequiresSecret     bool
    CostBearing        bool
}
```

Core classes:

```text
Pure
Read
Reversible
Compensatable
Irreversible
```

---

# 7. Static vs dynamic effect semantics

A tool/action definition establishes the **minimum safe effect**.

Runtime/World context may strengthen or refine it, but must not silently weaken it.

Example:

```text
FS.Write minimum class: mutation

inside isolated COW transaction:
  effective class → Reversible

on direct host workspace without snapshot:
  effective class may remain Reversible only if exact previous content is captured durably before write;
  otherwise guarantees must be weaker/explicit.
```

Another example:

```text
HTTP POST generic minimum: potentially Irreversible
```

A specialized adapter with idempotency/compensation can refine metadata.

The model cannot choose its own effect class.

---

# 8. Effect domains

Effect domains help Intent policy reason deterministically.

Initial domain taxonomy might include:

```text
workspace.read
workspace.mutate
process.execute
network.read
network.mutate
external.communication
source_control.local
source_control.remote
secrets.use
infrastructure.mutate
database.read
database.mutate
billing
```

Use typed constants/structured resources, not arbitrary free-text domains for security decisions.

---

# 9. Authorization pipeline

Recommended order:

```text
1. parse/validate typed action parameters
2. resolve World and capability support
3. derive effective effect descriptor
4. check process lifecycle/budget/deadline
5. check required capabilities
6. check capability lease validity
7. check intent/domain constraints
8. check transaction/fork effect rules
9. evaluate risk/approval policy
10. reserve resource/cost where needed
11. persist authorization/start preconditions
12. execute in World
```

A denial before step 11 never reaches World.

Exact event boundaries can combine steps transactionally.

---

# 10. World execution interface

Avoid returning huge payloads as in-memory structs.

Conceptual asynchronous/streaming contract:

```go
type World interface {
    Describe(ctx context.Context) (WorldDescriptor, error)
    Execute(ctx context.Context, req AuthorizedAction, sink ActionSink) ActionOutcome
    Reconcile(ctx context.Context, action ActionID) (ReconciliationResult, error)
}
```

`ActionSink` may expose streaming destinations for stdout/stderr/artifacts rather than channels with unlimited accumulation.

Snapshot/Fork are optional interfaces/capabilities rather than forcing every World to implement fake methods.

Example:

```go
type SnapshotWorld interface {
    Snapshot(ctx context.Context, req SnapshotRequest) (SnapshotRef, error)
}
```

This is preferable to one giant interface where unsupported methods return `ErrNotSupported` everywhere.

---

# 11. Action outcome

Outcome needs separate execution result and certainty.

```go
type ActionOutcome struct {
    Status       OutcomeStatus
    Certainty    OutcomeCertainty
    ExitCode     *int
    ResultRef    *ObjectRef
    StdoutRef    *ObjectRef
    StderrRef    *ObjectRef
    EffectRef    *EffectEvidenceRef
    Failure      *FailureRef
}
```

Certainty:

```text
KnownSucceeded
KnownFailed
Unknown
```

A non-zero command exit can be `KnownFailed` as task/tool result while the process execution itself successfully occurred.

Need separate concepts in implementation to avoid ambiguity.

---

# 12. Output policy

Authorized action includes output limits:

```text
max_runtime
max_preview_bytes
max_total_bytes?       # optional hard quota
persist_full_stdout
persist_full_stderr
compression
line/tail behavior
```

Flow:

```text
pipe
 ├── streaming object sink
 └── bounded preview sink
```

If hard output quota is exceeded, policy may:

- stop capturing preview only;
- continue compressed artifact;
- terminate process if storage/resource budget says so.

Never default to unbounded `bytes.Buffer`.

---

# 13. Process execution

`Process.Exec` input should prefer:

```text
executable
argv[]
cwd
env allowlist/additions
stdin object/ref optional
timeout
resource policy
```

Avoid one shell command string unless action explicitly requests shell semantics.

Secret env values are bound by kernel/World from opaque refs and must be redacted in event/output where possible.

---

# 14. Filesystem mutation protocol

For reversible host writes, kernel/World must define rollback evidence.

Example write:

```text
read/capture prior metadata/content/hash
        ↓
finalize rollback object
        ↓
persist action authorization + rollback reference
        ↓
write atomically where possible (temp + rename)
        ↓
record resulting hash/metadata
```

For transaction Worlds, overlay snapshot may provide rollback instead.

Do not label direct mutation Reversible without recoverable prior state.

---

# 15. Network policy

World descriptor/policy can expose levels:

```text
None
LoopbackOnly
DomainAllowlist
ServiceAllowlist
ProxyMediated
Unrestricted
```

Kernel capabilities and World enforcement both matter:

```text
capability says may contact github.com
AND
World network policy must permit it
```

If World cannot technically enforce a requested restriction, descriptor must say so; kernel may refuse to run sensitive action there.

---

# 16. Secret binding

Action refers to:

```text
SecretRef
```

World receives materialized secret through a secure execution-specific channel:

- env var;
- file descriptor/temp credential file with strict permissions;
- process stdin;
- provider SDK parameter not exposed to model.

Events/logs store reference/name/use metadata, never plaintext.

If tool echoes secret, redaction is defense-in-depth but cannot guarantee complete protection. Prefer adapters that avoid exposing secret to arbitrary shell where possible.

---

# 17. Resource limits

World action policy can constrain:

```text
wall time
CPU quota
memory
process count
file size/output
disk quota
network
```

Guarantees vary by World:

- OCI can enforce strongly;
- LocalWorld may only enforce timeout/process management partially.

Capability descriptor must distinguish hard enforcement from advisory limits where necessary.

---

# 18. Reconciliation

Worlds supporting external durable identities can reconcile interrupted actions.

Examples:

```text
OCI container ID
remote job ID
Kubernetes Job UID
cloud operation/idempotency key
```

Local ephemeral process reconciliation is weaker.

`Reconcile(ActionID)` can return:

```text
StillRunning
KnownSucceeded
KnownFailed
Unknown
NotFound
```

The kernel records reconciliation fact before deciding retry.

---

# 19. World lifecycle

Candidate states:

```text
CREATING
READY
BUSY?          # avoid if World can run concurrent actions; use leases instead
DEGRADED
LOST
DESTROYING
DESTROYED
```

Concurrency ability belongs in descriptor/policy rather than globally assuming one action at a time.

---

# 20. World isolation levels

For user-visible/security reasoning, define coarse levels:

```text
HostDirect
WorkspaceRestricted
ProcessSandboxed
ContainerIsolated
RemoteIsolated
```

Do not equate labels with absolute security; descriptor should enumerate actual guarantees.

---

# 21. Transaction interaction

Inside transaction:

- action carries TransactionID;
- transaction manager validates effects;
- World mutation must target transaction-specific isolated state;
- external irreversible actions are deferred/barriered where possible;
- outcome contributes staged effect ledger.

World does not decide transaction promotion itself unless it implements the promotion primitive requested by transaction manager.

---

# 22. Fork interaction

Fork branch obtains distinct World or branch/snapshot reference.

Branch action must never accidentally resolve base parent's direct World binding when fork requires isolation.

WorldID/BranchID should therefore be explicit in authorized action, not inferred from ambient global current workspace.

---

# 23. Required tests

```text
W-001 denied action never invokes World mock
W-002 model-supplied effect classification cannot weaken action effect
W-003 1 GB stdout streams with bounded heap
W-004 process timeout kills managed process tree according to platform contract
W-005 secret plaintext absent from event/action serialization
W-006 unsupported World snapshot/fork rejected before work
W-007 crash-after-external-send can produce OutcomeUnknown
W-008 retry policy requires reconciliation for unknown irreversible outcome
W-009 direct reversible FS write has durable rollback evidence before mutation
W-010 fork branch uses isolated World binding
```

---

# v0 implementation guidance

Start with a deliberately narrow `LocalWorld`:

```text
FS.Read
FS.List
FS.Stat
Process.Exec
```

Then controlled writes.

Do not expose an unrestricted shell as the first abstraction merely because it is easy. The architecture benefits from typed operations and measurable effects from day one.
