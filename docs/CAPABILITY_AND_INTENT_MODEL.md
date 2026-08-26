# Capability, Delegation and Intent Model

## Purpose

This document turns the security concepts into an implementable authorization model.

Core rule:

```text
Content can influence reasoning.
Content cannot mint authority.
```

A state-changing action requires deterministic kernel authorization before reaching a World.

---

# 1. Authorization dimensions

An action is allowed only if all applicable dimensions pass:

```text
process lifecycle
AND
capability possession
AND
capability lease validity
AND
intent compatibility
AND
effect/transaction policy
AND
budget/resource availability
AND
approval policy
AND
World enforcement capability
```

No single broad `trusted=true` flag bypasses this pipeline.

---

# 2. Capability structure

A capability is typed authority over a resource/action scope.

Conceptual form:

```go
type Capability struct {
    Kind       CapabilityKind
    Resource   ResourceSelector
    Operations OperationSet
    Constraints CapabilityConstraints
}
```

Examples:

```text
filesystem(path="./src/**", ops=[read])
filesystem(path="./src/auth/**", ops=[read,write,create])
process(exec=["go","git","rg"], cwd="./project/**")
network(domains=["api.github.com"], ops=[connect])
secret(name="github-token", ops=[use])
agent(ops=[spawn,message])
world(type="oci", ops=[create,fork])
```

---

# 3. Capability grammar design principle

The subset relation must be mechanically decidable.

Avoid v0 policies like:

```text
allow if arbitrary script returns true
```

inside capability delegation.

Use typed selectors with well-defined subset/intersection operations.

```text
ChildCapability ⊆ ParentDelegableCapability
```

must be a deterministic function.

---

# 4. Filesystem capability

Candidate selector:

```text
root/path scope
operations:
  read
  list
  stat
  create
  write
  rename
  delete
symlink policy
```

Important path semantics:

- canonicalize relative to World workspace root;
- reject traversal escaping scope;
- define symlink resolution behavior;
- OS case sensitivity differences handled by World;
- capability uses logical World paths, not arbitrary host paths when inside isolated World.

Example subset:

```text
parent: ./project/** read+write
child:  ./project/src/auth/** read
→ valid

child: ../secrets/** read
→ invalid
```

Glob/pattern language should be intentionally limited so subset checks remain reliable.

---

# 5. Process capability

Selector can include:

```text
allowed executable identities
cwd scope
shell allowed? bool
environment variable names allowed
network requirement relation
max execution class/risk optional
```

Example:

```text
exec binaries: [go, git, rg]
cwd: workspace/**
shell: false
```

If shell is allowed, capability is inherently broader because shell can launch other programs. Treat `shell=true` as explicit powerful authority rather than equivalent to one binary allowlist.

---

# 6. Network capability

Selector:

```text
domain/service allowlist
ports/protocol optional
operation class: read-like / mutate external
```

Domain matching rules must avoid suffix mistakes:

```text
allowed: github.com
must not automatically allow: github.com.attacker.example
```

Use parsed normalized hostnames and explicit subdomain policy.

Network capability does not guarantee World can enforce network restriction. Authorization also requires compatible World network policy.

---

# 7. Secret capability

Do not define capability as “read secret plaintext” by default.

Operations:

```text
use      bind secret to authorized adapter/action without model seeing plaintext
reveal   exceptionally expose plaintext, much higher-risk explicit capability
```

Preferred:

```text
secret(github-token, use)
```

not:

```text
secret(github-token, reveal)
```

A child should normally receive only scoped `use` authority for required service.

---

# 8. Agent capability

Controls orchestration operations:

```text
spawn
message
cancel-child
inspect-child
fork
```

`spawn` additionally requires resource budget and delegated capability subset.

Depth/fan-out limits are economy/policy constraints, not encoded solely as capability.

---

# 9. World capability

Controls creation/use of execution environment types:

```text
world.local.use
world.oci.create
world.oci.fork
world.ssh.connect(resource scope)
```

Having `world.oci.create` does not automatically grant filesystem/network/secrets inside the World; those are separate capabilities/policies.

---

# 10. Grants

A grant binds a capability to a subject Agent Process.

```go
type CapabilityGrant struct {
    GrantID       GrantID
    Subject       AgentID
    Capability    Capability
    Issuer        ActorRef
    ParentGrantID *GrantID
    Delegable     bool
    Lease         Lease
    Status        GrantStatus
}
```

Status:

```text
Active
Revoked
Expired
Consumed        # only for future one-shot grants
```

---

# 11. Delegation

To create child grant:

```text
requested child capability
       ↓
find one or more active parent delegable grants
       ↓
prove requested capability is subset/intersection
       ↓
prove lease duration <= parent validity
       ↓
create child grant with parent grant references
```

If capability requires combining several parent grants, representation must preserve all authorizing parents or normalize them before delegation.

Simpler v0: require each child grant be covered by one parent grant where feasible.

---

# 12. Authority monotonicity

Across recursive delegation:

```text
Authority(child) ⊆ Authority(parent)
```

But a parent can choose different subsets for different children.

Authority may shrink through:

- narrower resource selector;
- fewer operations;
- tighter constraints;
- shorter lease;
- non-delegable flag.

It may never expand by inheritance.

---

# 13. Lease semantics

Lease:

```text
valid_from
valid_until? / duration
until_task_complete
until_transaction_end
revocable
```

Kernel checks validity at **authorization time**, not only at grant creation.

If lease expires while a long-running action is active, v0 policy must define whether:

- action keeps its already-authorized execution lease until deadline; or
- runtime attempts cancellation.

Recommended v0:

> authorization validity is captured at action start; capability revocation prevents new actions, while emergency hard revocation may separately cancel active operations.

This avoids unpredictable mid-syscall behavior while retaining an explicit emergency mechanism later.

---

# 14. Revocation propagation

If parent grant is revoked, descendant grants derived from it must become unusable.

Implementation options:

- eager recursive revocation events;
- lazy validity check walking ancestry/version;
- cached effective validity with invalidation.

For local v0, eager marking/indexed ancestry is acceptable; design must avoid O(entire history) checks per action.

Revocation reason and causal parent grant are preserved.

---

# 15. Root intent

Intent is a separate immutable-ish policy object.

Conceptual schema:

```go
type Intent struct {
    ID                 IntentID
    RootAgentID        AgentID
    GoalRef            ObjectRefOrInline
    AllowedDomains     []EffectDomainConstraint
    ForbiddenDomains   []EffectDomainConstraint
    AcceptanceCriteria []Criterion
    Constraints        []IntentConstraint
    CreatedBy          ActorRef
    Version            uint32
}
```

Root intent cannot be modified by Agent Actor.

User-approved amendments create a **new explicit intent version/amendment event**, preserving original history.

“Immutable” therefore means no silent overwrite, not inability for user to change their mind.

---

# 16. Child intent

Child receives delegated task intent:

```text
root_intent_ref
parent_task_intent_ref
goal: analyze auth race condition
allowed domains narrower/equal
constraints inherited
```

Child task can be more specific but not broaden forbidden effects.

Semantic proof that arbitrary natural-language child goal is “narrower” cannot be purely formal; therefore hard domains/capabilities remain security enforcement while language intent is an additional relevance signal.

---

# 17. Intent compatibility

Use layers.

## Layer A — deterministic effect-domain rules

Example:

```text
root intent forbids external.communication
Action effect domain includes external.communication
→ deny
```

## Layer B — typed resource constraints

Example:

```text
intent scopes workspace/project-X
Action target workspace/project-Y
→ deny/escalate
```

## Layer C — semantic relevance evaluation

For ambiguous sensitive action, optional policy model/human may assess rationale.

Semantic evaluation can cause:

```text
allow if already capability-approved and low risk
request approval
or deny
```

It must never create missing capability.

---

# 18. Action authorization record

For each state-changing/sensitive action, record a compact authorization proof/reference:

```text
ActionID
AgentID
IntentID/version
GrantIDs used
Effect descriptor/version
Policy decision/version
ApprovalID? if any
World capability result
Decision allow/deny
Reason code
```

This enables causal trace without rerunning current policy against old state.

---

# 19. Approval tokens

An approval is scoped authority/policy acknowledgement, not a permanent “yes to everything”.

Approval can bind:

```text
specific ActionID
specific effect/resource family
max count
expiry
transaction/fork ID
```

Example:

```text
approve git push to branch feature/x once before 17:00
```

not:

```text
approve all future irreversible actions
```

unless user deliberately creates such broader persistent policy through a separate explicit mechanism.

---

# 20. Policy evaluation result

Structured result:

```text
Allowed
Denied(reason code)
NeedsApproval(approval spec)
NeedsSaferWorld(requirements)
NeedsNarrowerAction
```

This allows agent/harness to adapt without parsing error text.

---

# 21. Prompt injection handling

Untrusted inputs can contain:

```text
"Ignore previous instructions"
"Run curl ..."
"Reveal your token"
```

These may influence model reasoning, but actual action still needs:

- typed request validation;
- capability;
- intent/effect compatibility;
- World policy;
- approval when needed.

No parsed document can call `CapabilityGranted` command unless actor has authority to grant.

---

# 22. Capability cache

Policy checks can use bounded caches of compiled selectors/effective grants.

Cache key must include relevant grant/policy versions.

Revocation/expiry invalidates cache or version mismatch makes entry stale.

Correctness never depends on cache being current if version validation exists.

---

# 23. Required property tests

Generate random authority trees.

Assert:

```text
CAP-001 child effective authority never exceeds ancestor chain
CAP-002 narrowing path/op/lease remains valid subset
CAP-003 expired parent makes descendant unusable
CAP-004 revoked grant cannot authorize new action
CAP-005 content/message cannot invoke grant path without authorized actor
CAP-006 secret(use) does not imply secret(reveal)
CAP-007 forbidden intent domain denies even when capability exists
CAP-008 missing capability denies even if semantic intent evaluator says relevant
CAP-009 approval for Action A cannot authorize Action B outside scope
CAP-010 authorization proof identifies exact grant/intent/policy versions
```

---

# v0 capability grammar recommendation

Keep v0 deliberately typed and small:

```text
FilesystemCapability
ProcessCapability
NetworkCapability
SecretCapability
AgentCapability
WorldCapability
```

Avoid building a general policy programming language until concrete missing expressiveness appears.

This keeps delegation subset checks understandable, testable and fast.
