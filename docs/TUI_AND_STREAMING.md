# TUI, Streaming and Session Attachment Architecture

## Purpose

The terminal experience must stay responsive during long-running sessions.

The UI therefore follows a strict architectural rule:

> **The terminal is a replaceable client of the durable runtime, not the runtime itself.**

This document defines how to avoid the classic failure mode where a chat-oriented terminal application becomes slower as the transcript grows.

---

# 1. Process separation

Preferred production architecture:

```text
┌──────────────────────────┐
│        go-agent TUI      │
│ presentation only        │
└─────────────┬────────────┘
              │ local IPC
              ▼
┌──────────────────────────┐
│     runtime supervisor   │
│                          │
│ Agent Processes          │
│ providers                │
│ tools/worlds             │
│ event ledger             │
│ scheduler                │
└─────────────┬────────────┘
              │
              ▼
         durable storage
```

The project may ship one binary with multiple commands/process modes, but logical ownership stays separate:

```bash
go-agent daemon
go-agent attach <agent-id>
go-agent run ...
```

A convenience `go-agent` command may auto-start/connect to the local daemon.

## Why separation matters

- TUI crashes do not kill agents;
- agent work can continue detached;
- UI can be upgraded/restarted independently;
- reconnect only loads a viewport, not complete history;
- stream consumers can have independent backpressure;
- runtime memory is not tied to terminal widget state.

---

# 2. IPC requirements

The local client protocol should support:

- attach/detach;
- subscribe to process projections;
- subscribe to live output streams;
- fetch historical ranges;
- send user messages;
- approve/deny actions;
- cancel/pause/resume;
- inspect context/authority/budgets;
- reconnect after client loss;
- protocol version negotiation.

Transport options:

```text
Unix domain socket     macOS/Linux
named pipe             Windows
localhost socket       portable fallback
```

Protocol semantics should be transport-independent.

## Encoding

Do not optimize prematurely around a complex serialization framework.

A good v0 is a framed versioned message envelope with JSON payloads and object references for large data.

Example:

```json
{
  "version": 1,
  "type": "process.update",
  "request_id": "...",
  "agent_id": "...",
  "sequence": 1832,
  "payload": {}
}
```

Large content does not travel inline by default:

```json
{
  "object_ref": "object://sha256:...",
  "preview": "first useful lines...",
  "size": 93818273
}
```

A future binary encoding can be introduced without changing logical message semantics.

---

# 3. UI projection model

The TUI should not replay raw ledger events itself to reconstruct canonical runtime state.

The runtime provides derived projections:

```text
ProcessSummary
ProcessTree
ConversationViewport
InvocationStatus
ToolStatus
ApprovalRequest
BudgetSummary
ContextSummary
WorldSummary
```

The TUI keeps only what it needs to render the current screens.

This protects UI code from kernel schema churn and reduces CPU/memory usage.

---

# 4. Historical conversation virtualization

Never hold the entire transcript as rendered widgets.

Model:

```text
Persistent transcript store
           │
           ▼
   paginated history API
           │
           ▼
   bounded viewport cache
           │
           ▼
        renderer
```

The client asks for windows such as:

```text
latest 100 logical message blocks
previous 100 before cursor X
next 100 after cursor Y
```

A logical message block may reference persisted Markdown/text/tool output.

## Scroll behavior

Scrolling upward can fetch older ranges asynchronously.

Rendering should maintain anchor position while loading new blocks.

No operation such as resize, color change or new token should re-render/parsing-process an hour of history.

---

# 5. Incremental rendering

Streaming model output should use one mutable current block rather than append a new widget/node per token.

```text
token chunks
    │
    ▼
coalescer (e.g. max 20–30 UI updates/s)
    │
    ▼
append to active response buffer
    │
    ▼
render visible dirty region
```

When response completes:

- freeze/finalize message block;
- persist canonical response artifact in runtime;
- cache parsed representation only within bounded client cache.

Do not perform full Markdown parsing for every token.

Possible strategy:

- render plain/incremental text while streaming;
- parse/finalize Markdown at coarse intervals or completion;
- cache finalized rendering AST/layout with LRU bounds.

---

# 6. UI event classes

Not every runtime event deserves immediate rendering.

## High priority

- user input echo;
- approval request;
- model visible text;
- critical failure;
- completion/cancellation;
- security denial.

## Medium priority

- tool started/completed;
- child state changed;
- plan/task phase changed.

## Low priority/coalescible

- progress percentage;
- token counters;
- cost counters;
- spinner animation;
- repeated stdout chunks;
- queue metrics.

The local presentation stream can coalesce low-priority updates when the client is slow.

Canonical state remains queryable separately.

---

# 7. Terminal output architecture

The TUI must not directly consume raw stdout from tools.

Correct flow:

```text
Tool process stdout
       │
       ├── object/blob writer
       │
       ├── tail/line framing
       │
       └── UI preview stream
```

The preview stream can use:

- maximum bytes/lines;
- rate limiting;
- line aggregation;
- explicit truncation markers.

Example UI:

```text
$ go test ./...
...
[showing live tail; 18.4 MB persisted]
```

The user can request the artifact or search within it without the TUI storing all 18.4 MB.

---

# 8. Attach semantics

`attach` should be cheap even for a month-old agent.

Sequence:

```text
client connects
  ↓
protocol handshake
  ↓
fetch ProcessSummary + current active states
  ↓
fetch latest conversation viewport
  ↓
subscribe from sequence/cursor
```

It MUST NOT mean:

```text
read every historical event
render every message
reconstruct all context pages
```

The runtime should expose a monotonic presentation sequence or cursor so reconnects can request only missed updates where practical.

---

# 9. Detach semantics

Closing a terminal:

```text
TUI exits
```

must only remove its subscriptions.

It must not imply:

```text
cancel agent
cancel child agents
cancel model invocation
close world
```

Cancellation must be an explicit command.

This separation should be visible in UX:

```text
Ctrl+C        context-dependent interrupt/input cancel
:detach       leave session running
:cancel       cancel current/root process according to scope
```

Exact bindings come later.

---

# 10. Client backpressure

Each connected UI has an independent bounded outbound queue.

If a UI stops reading:

- canonical runtime work continues;
- non-critical presentation updates may be coalesced;
- the client can be marked behind;
- the runtime may disconnect a pathological client;
- reconnect recovers through projections/history cursors.

The runtime must never maintain an unbounded queue because one terminal is frozen in the background.

---

# 11. Multiple clients

The architecture should naturally allow later:

```text
terminal A ─┐
terminal B ─┼── runtime
web UI     ─┤
IDE plugin ─┘
```

This is not an MVP requirement, but avoiding presentation ownership in the kernel makes it possible.

Input arbitration must eventually define whether several clients can send commands concurrently.

Initial rule may be:

- many read subscribers;
- one interaction lease/controller at a time.

---

# 12. Presentation persistence

TUI-only state should generally not pollute the canonical Agent Event Ledger.

Examples not worth canonical events:

- selected tab;
- scroll offset;
- current pane size;
- spinner frame;
- local theme.

Useful persistent client preferences can live in separate configuration/state tables.

---

# 13. Message model

Conversation UI should render typed blocks rather than concatenate one giant text string.

Possible block types:

```text
UserMessage
AssistantMessage
ThinkingStatus       # not hidden reasoning content
ToolCall
ToolResultSummary
ArtifactReference
ChildAgentCard
ApprovalRequest
SystemNotice
Error
Checkpoint
```

Each block has a stable ID and can be fetched independently.

A tool result block should normally contain a summary/preview + object reference, not the entire raw payload.

---

# 14. Responsive architecture constraints

Avoid these patterns:

```text
[]Message containing entire history
[]string with every token ever emitted
reparseAllMarkdown() on every update
one UI event per token
one widget per stdout line forever
sync DB read inside render()
unbounded channel from runtime → TUI
periodic full-process-tree polling
```

Prefer:

```text
bounded viewport
stable block IDs
incremental updates
async data fetch
coalesced stream frames
derived projections
push notifications + cursors
LRU caches
```

---

# 15. TUI crash/restart test

Mandatory scenario:

1. start agent;
2. model/tool work is active;
3. kill TUI process abruptly;
4. runtime and agent remain active;
5. tool/model completes;
6. restart TUI;
7. attach;
8. latest state and messages appear without replaying complete history.

This is a core architecture test, not an edge case.

---

# 16. Long-history benchmark

Seed a synthetic session with:

- 100,000 message/tool blocks;
- multiple GB of referenced artifacts;
- only the latest 100 blocks visible.

Measure:

- attach latency;
- client RSS;
- scroll latency;
- resize latency;
- new-token update latency.

The client should behave based on **viewport size**, not total transcript size.

---

# 17. TUI library choice

A Go TUI library such as Bubble Tea may be evaluated, but the architecture must not depend on any library-specific global model semantics.

Selection criteria:

- incremental updates;
- predictable allocations;
- viewport virtualization;
- mouse optional, keyboard first;
- no requirement to rebuild the entire history per frame;
- good Unicode support;
- testability without a real terminal;
- stable Windows/macOS/Linux behavior.

If a library makes long-history virtualization difficult, the project should change library or build narrower rendering primitives rather than compromise the runtime architecture.

---

# Core invariant

> **UI cost must scale with what is currently visible and active, not with how old the agent is.**
