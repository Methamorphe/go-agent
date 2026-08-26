# Local Runtime Control Protocol

## Purpose

The durable runtime and terminal client are separate ownership domains.

This document defines the first protocol-level contract for:

```text
TUI / CLI / future IDE client
             │
         local IPC
             │
             ▼
      Runtime Supervisor
```

The protocol is intentionally designed so a month-old session can be attached without shipping or rendering its full history.

---

# 1. Protocol goals

- thin clients;
- reconnectable sessions;
- paginated history;
- bounded live subscriptions;
- explicit control commands;
- version negotiation;
- no large payloads inline by default;
- multi-client-read future;
- platform-independent semantics.

Non-goal in v0: network-exposed remote control API.

---

# 2. Transport

Preferred local transports:

```text
macOS/Linux: Unix domain socket
Windows:     named pipe when practical
fallback:    loopback TCP with local authentication/ownership constraints
```

Transport is below protocol semantics.

A single binary may auto-launch daemon and connect.

---

# 3. Framing

v0 preference:

- explicit length framing;
- versioned envelope;
- JSON payload for debugability/evolution;
- strict maximum frame size;
- large content referenced via object APIs.

Do not rely on newline framing if arbitrary string payloads/large messages make framing/error handling fragile.

Conceptual envelope:

```json
{
  "protocol_version": 1,
  "kind": "request|response|event",
  "type": "conversation.list",
  "id": "msg_...",
  "reply_to": null,
  "agent_id": "agent_...",
  "sequence": 123,
  "payload": {}
}
```

Binary encoding can replace JSON later without changing semantic message types.

---

# 4. Handshake

Client opens transport and sends:

```text
Hello
  protocol_min
  protocol_max
  client_name/version
  capabilities
```

Runtime returns:

```text
Welcome
  selected_protocol_version
  runtime_version
  runtime_instance_id
  feature flags
  storage/runtime health summary
```

If no compatible version exists, disconnect with structured error.

---

# 5. Request/response IDs

Every state-changing or query request has client-generated `RequestID`.

Runtime responses include `reply_to`.

For correctness-sensitive commands, request ID can also provide idempotency for reconnect/retry.

Example:

```text
RequestCancel(id=req-42)
connection drops
retry RequestCancel(id=req-42)
→ same accepted result, no duplicate cancel semantics
```

---

# 6. Core client operations

## Runtime/process discovery

```text
runtime.status
process.list
process.get
process.tree
```

## Session control

```text
process.create
process.attach
process.detach
process.cancel
process.suspend
process.resume
process.checkpoint
```

`detach` is primarily a client subscription operation, not an Agent Process lifecycle transition.

## User interaction

```text
input.submit
approval.respond
```

## Historical conversation

```text
conversation.latest
conversation.before
conversation.after
conversation.get_block
```

## Inspection

```text
events.list
context.get_summary
context.get_invocation_manifest
authority.get
budget.get
world.get
transaction.get
artifact.get_meta
artifact.open/read-range later
```

---

# 7. Subscription model

Client subscribes to derived live topics rather than raw global events.

Candidate subscriptions:

```text
process:<id>:presentation
process:<id>:status
process:<id>:children
runtime:health
approvals:mine
```

Runtime returns subscription ID + current cursor/sequence.

Live events carry a monotonically increasing subscription/presentation cursor where practical.

---

# 8. Presentation stream is reconstructable

Presentation updates are **not canonical storage**.

If client misses updates:

```text
cursor gap detected
       ↓
client requests current projection/latest viewport
       ↓
continues subscription
```

Therefore a presentation queue can coalesce/drop selected low-priority updates under pressure.

Security/audit facts remain canonical elsewhere.

---

# 9. Presentation event types

Candidate derived events:

```text
MessageBlockAdded
MessageBlockPatched
MessageBlockFinalized
ToolCardAdded
ToolCardUpdated
ChildCardAdded
ChildCardUpdated
ProcessStatusChanged
ApprovalPresented
ApprovalResolved
BudgetSummaryChanged
NoticeAdded
RuntimeHealthChanged
```

During model streaming, one message block is patched with coarse chunks; do not emit one durable/client object per token.

---

# 10. Priority classes

Presentation frames include priority:

```text
critical
interactive
normal
telemetry
```

Examples:

- approval request → critical;
- user/assistant visible text → interactive;
- tool progress → normal;
- token-count refresh → telemetry.

Slow-client policy can coalesce/drop telemetry first.

Critical messages should instead cause disconnect/recovery if queue cannot progress rather than silent loss.

---

# 11. Slow-client policy

Each client has a bounded outbound queue measured in frames **and bytes**.

When thresholds are approached:

1. coalesce replaceable updates by entity/block ID;
2. drop stale telemetry frames and increment counter;
3. replace repeated progress updates with newest value;
4. if still overloaded, send/record `ClientFellBehind` if possible and disconnect;
5. client reconnects from current projection/history.

Never grow an unlimited slice waiting for terminal rendering.

---

# 12. Conversation block model

Logical persisted presentation blocks:

```go
type ConversationBlock struct {
    ID          BlockID
    AgentID     AgentID
    Sequence    uint64
    Type        BlockType
    CreatedAt   time.Time
    Finalized   bool
    ContentRef  *ObjectRef
    Preview     string
    RelationRef *EntityRef
}
```

Types:

```text
UserMessage
AssistantMessage
ToolCall
ToolResultSummary
ChildAgentSummary
ArtifactReference
Approval
Notice
Error
Checkpoint
```

Streaming assistant block may have transient patches until finalized response artifact exists.

---

# 13. History pagination

Use cursor-based pagination, not offset over huge history.

Example:

```text
conversation.latest(agent, limit=100)
→ blocks + oldest_cursor + newest_cursor

conversation.before(agent, cursor=oldest_cursor, limit=100)
```

Cursor encodes stable ordering key/sequence, not mutable UI offset.

Maximum page size enforced server-side.

---

# 14. Initial attach

```text
Attach(agentID)
  ↓
ProcessSummary
ProcessTreeSummary
latest N conversation blocks
pending approval/current operation summaries
presentation subscription cursor
```

Target: data proportional to current viewport/active state.

Attach never requires full Event Ledger replay by client.

---

# 15. Artifact access

Large object access must not be shoved into one IPC frame.

v0 options:

- runtime exposes chunk/range streaming requests;
- for trusted same-user local runtime, may expose controlled file/object streaming handle abstraction;
- never reveal arbitrary object-store filesystem paths as protocol authority.

Artifact metadata:

```text
ObjectRef
size
media type
hash
compression
preview/truncation metadata
```

---

# 16. Control authority

Local client identity and process control permissions need a policy boundary even if v0 is single-user.

At minimum:

- local socket/pipe permissions restricted to user;
- daemon rejects unexpected remote interfaces by default;
- approval/control command records ActorRef/UserActor;
- future multi-user/auth can be added without changing Agent capability semantics.

Client control authority is separate from an Agent's action capabilities.

---

# 17. User input semantics

`input.submit` creates a durable interaction/command, not direct mutation of an in-memory transcript.

Fields may include:

```text
agent_id
request_id
text/content object
attachments refs
client timestamp optional
expected interaction mode/version optional
```

Runtime records authoritative receipt timestamp and binds input to process correlation.

---

# 18. Approval semantics

Client receives:

```text
ApprovalPresented
  approval_id
  action summary
  effect/risk
  capability
  intent rationale
  scope of approval
  expiry
```

Client responds:

```text
approval.respond
  approval_id
  decision approve/deny
  optional constrained scope
  request_id
```

Approval token itself is canonical kernel state; TUI card is presentation.

---

# 19. Cancellation/interrupt UX protocol

Distinguish:

```text
input.cancel_draft       local UI only
invocation.interrupt     request to stop current model/tool operation where permitted
process.cancel           durable process cancellation
process.detach           leave process running
```

The protocol should prevent one overloaded `Ctrl+C` meaning four different things invisibly.

---

# 20. Health and metrics stream

Runtime can expose low-rate summaries:

```text
heap/routine optional diagnostics
active agents
active model/tool calls
storage health
provider health
queue pressure warnings
```

High-frequency debug metrics are not pushed to normal TUI unless diagnostics screen subscribes.

---

# 21. Protocol errors

Structured categories:

```text
InvalidRequest
UnsupportedVersion
NotFound
Conflict
Unauthorized
ForbiddenByPolicy
RateLimited
RuntimeDraining
StorageUnavailable
TooLarge
CursorExpired
InternalError
```

Errors include stable code + human message + optional retry metadata.

Do not make clients parse error strings.

---

# 22. Compatibility

Protocol versioning rules:

- additive optional fields preferred;
- unknown event types ignored only when declared safe/optional;
- required feature negotiation for semantic additions;
- daemon/client version mismatch gives explicit compatibility result.

No direct exposure of internal Go struct serialization.

---

# 23. Required tests

```text
IPC-001 TUI kill does not cancel active process
IPC-002 reconnect latest viewport without full history transfer
IPC-003 slow client cannot cause unbounded runtime memory growth
IPC-004 100k-block history pages by cursor correctly
IPC-005 duplicate control request ID is idempotent where specified
IPC-006 oversized frame rejected without daemon crash
IPC-007 malformed client frame isolated to connection
IPC-008 approval response records user actor and exact approval ID
IPC-009 presentation gap recovers via refetch
IPC-010 protocol-version mismatch produces structured refusal
```

---

# v0 decision baseline

Preferred semantic baseline:

```text
local socket/pipe
length-framed versioned envelopes
JSON payloads initially
cursor-based history
bounded per-client presentation queues
large artifact references/streams
runtime-owned projections
```

Exact transport library/encoding optimizations can be benchmarked later without changing this contract.
