# Security, Authority, Intent and Effects

## Security thesis

An autonomous agent should never be trusted to enforce its own permissions purely through prompting.

Security policy belongs in the runtime.

The project therefore separates:

```text
What the model wants to do
            │
            ▼
What the runtime allows it to do
```

The second must be authoritative.

## 1. Capability-based authority

Every Agent Process owns an explicit capability set.

Examples:

```text
filesystem.read("./src/**")
filesystem.write("./src/auth/**")
git.read
process.exec(["go", "git", "rg"])
network.connect("api.github.com")
secret.read("staging-api-key")
world.create("oci")
agent.spawn
```

The runtime checks capabilities before an action reaches a World.

### Why capabilities

Capabilities are easier to reason about than broad roles such as:

```text
admin
trusted-agent
coding-agent
```

A reviewer may be allowed to inspect a repository without being able to mutate it. A test agent may run tests but have no network access. A research agent may access the internet while having no filesystem write permission.

## 2. Authority tree

Authority flows downward from the user/root authority.

```text
User Authority
      │
      ▼
 Main Agent
   ├─ Backend Child
   ├─ Reviewer Child
   └─ Research Child
```

### Fundamental invariant

> **A child may never receive authority the parent does not possess.**

If the parent owns:

```text
FS read/write ./project
Network github.com
Git local write
```

it may delegate:

```text
Reviewer:
  FS read ./project
  no network
  no write
```

but cannot delegate:

```text
AWS production admin
```

unless that authority already exists upstream and policy explicitly allows delegation.

## 3. Capability leasing

Capabilities should optionally expire.

```yaml
capability:
  filesystem.write: ./src/auth/**
lease:
  until: task-complete
  max_duration: 15m
  revocable: true
```

This is particularly useful for child agents and temporary elevated operations.

Leases may expire on:

- child completion;
- timeout;
- transaction end;
- parent cancellation;
- explicit revocation;
- user disconnect/policy change.

## 4. Intent Lock

Capabilities answer:

> What is this process technically allowed to do?

They do not fully answer:

> Is this action relevant to what the user asked?

The project therefore proposes an immutable **Intent** object bound near process creation.

Example:

```yaml
intent_id: intent_123
goal: "Fix the authentication refresh bug"
allowed_effect_domains:
  - repository
  - tests
forbidden_domains:
  - production-deploy
  - outbound-communication
  - billing
acceptance_criteria:
  - failing test reproduced
  - fix implemented
  - auth tests pass
```

The agent may refine plans, but it should not silently rewrite the root user intent.

## 5. Intent-Based Authority

Before executing a sensitive action, the kernel can ask two separate questions:

```text
1. Does the process have the capability?
2. Is the action justified by the bound intent?
```

Example:

```text
Intent: fix local authentication bug
Action: upload .env to arbitrary server

Capability: network may exist
Intent relationship: none
→ deny / escalate
```

This provides another barrier against prompt injection and task drift.

### Important caveat

Intent matching cannot rely solely on another unconstrained LLM. It should combine:

- explicit allowed/forbidden domains;
- action metadata;
- typed effect rules;
- path/network/resource policies;
- optional semantic review for ambiguous cases.

## 6. Effect System

Every action declares its effect semantics.

Proposed initial classes:

```go
type EffectClass int

const (
    EffectPure EffectClass = iota
    EffectRead
    EffectReversible
    EffectCompensatable
    EffectIrreversible
)
```

### Examples

| Action | Effect |
|---|---|
| Parse in-memory data | Pure |
| Read file | Read |
| Query read-only DB | Read |
| Modify file in snapshotted world | Reversible |
| Create git commit | Reversible |
| Start disposable container | Compensatable/Reversible |
| Create cloud resource | Compensatable |
| Send email/message | Irreversible |
| Publish externally | Irreversible |
| Transfer money | Irreversible |

The exact classification may depend on the World and available rollback mechanisms.

## 7. Why effects matter

Effect metadata lets the kernel decide:

```text
Can this action run speculatively?
Can it be retried safely?
Can parallel branches both perform it?
Can it be rolled back?
Does it require a transaction?
Does it require user approval?
```

This is more powerful than a generic “dangerous command” flag.

## 8. Risk policy

An optional risk score can sit above effect classes.

Example:

```text
read repository        risk 0
run tests              risk 1
modify source          risk 2
install dependency     risk 3
push branch            risk 6
deploy staging         risk 7
deploy production      risk 9
irreversible deletion  risk 10
```

Possible policy:

```text
0–2  autonomous
3–5  isolated/transactional
6–8  explicit approval or trusted policy
9–10 prohibited by default
```

Risk should be configurable by environment and user policy rather than hard-coded globally.

## 9. Approval barriers

Irreversible actions should create explicit barriers.

```text
Agent proposes action
        │
        ▼
Effect = irreversible
        │
        ▼
Approval policy
   ├─ pre-approved intent scope → execute
   ├─ human approval required → wait
   └─ forbidden → deny
```

An approval should bind to a precise action or constrained action family, not become a permanent blanket permission.

## 10. Secret handling

Secrets should not simply be injected into the LLM context.

Prefer opaque references:

```text
secret://github/token
```

A tool/world can receive the secret directly if authorized without revealing its plaintext to the model.

Example:

```text
LLM requests:
  git.push(remote=origin, credential=secret://github/token)

Kernel:
  verifies authority
  injects credential into isolated process
  records use without recording secret value
```

## 11. Network policy

Network access should be scoped by World/capability:

```text
none
allow-list domains
allow-list IP/service
proxy-only
full outbound
```

Child agents should not automatically inherit unrestricted internet access.

## 12. Filesystem policy

Filesystem capabilities should support:

```text
read path glob
write path glob
create-only
no-delete
temporary workspace only
host filesystem denied
```

Container Worlds can strengthen these guarantees with mounts and namespaces.

## 13. Process execution policy

Instead of giving every agent arbitrary shell access, execution policy may define:

```text
allowed binaries
allowed cwd
max runtime
max output size
network mode
environment whitelist
secret bindings
resource limits
```

A raw shell can still exist for trusted developer mode, but it should be a conscious policy choice.

## 14. Prompt injection model

Prompt injection should be treated as inevitable untrusted input, not as a rare model failure.

Content obtained from:

- web pages;
- repository files;
- logs;
- emails;
- issues;
- tool output;

must never automatically grant authority.

The architectural rule is simple:

> **Information can influence reasoning; information cannot create capabilities.**

Only the authority graph can grant capabilities.

## 15. Agent Transactions

Actions with reversible effects can execute inside a transaction.

Conceptual API:

```text
begin transaction
  modify files
  install dependencies
  run migration in isolated DB
  run tests
verify
  ├─ success → commit
  └─ failure → rollback
```

Potential implementation mechanisms:

- overlay filesystem;
- git worktree;
- container snapshot;
- database transaction/snapshot;
- isolated environment variables;
- deferred external actions.

## 16. Agent transaction properties

Rather than blindly copying database ACID, define agent-specific guarantees.

Desired properties:

### Atomicity where possible

A set of reversible changes becomes visible together or is discarded.

### Isolation

Speculative branches should not corrupt each other or the parent world.

### Explicit effects

The transaction knows which actions cannot truly be rolled back.

### Verification before promotion

A commit may require tests/policies/evaluators.

### Auditability

The ledger records every staged and promoted effect.

## 17. Irreversible-effect boundary

A transaction cannot magically undo an email already sent or a public deployment already observed externally.

Therefore irreversible actions should be deferred until after verification whenever possible:

```text
transactional work
      ↓
verification
      ↓
commit internal state
      ↓
irreversible effect barrier
      ↓
approval/policy
      ↓
perform external action
```

## 18. Security observability

For each action, the system should be able to show:

```text
requested by: agent/child-17
root intent: fix auth bug
capability used: filesystem.write ./src/auth/**
effect: reversible
risk: 2
world: oci/world-31
transaction: tx-82
approved by: policy:auto-low-risk
result: success
```

## 19. Security defaults

Recommended defaults:

- least privilege;
- no secret plaintext in model context;
- no unrestricted child inheritance;
- no irreversible speculative effects;
- network off unless required;
- filesystem writes isolated where practical;
- every mutation event-recorded;
- explicit budget limits;
- production authority absent by default.

## Core invariant

> **The model proposes. The kernel authorizes. The World executes. The Ledger records.**

That separation should remain true even in highly autonomous modes.
