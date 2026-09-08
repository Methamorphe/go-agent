# G13 Exit Review — Production TUI / Interactive Agent Workspace

Status: **PASS**

## Scope delivered

G13 turns the durable go-agent runtime into a production fullscreen terminal workspace while preserving the architectural rule that the TUI is a replaceable client and never owns canonical Agent state.

### Production terminal client

A dedicated `go-agent-tui` binary now provides a Bubble Tea v2 fullscreen workspace with:

- ASK, PLAN, ACT, REVIEW and OBSERVE interaction modes;
- adaptive wide/medium/narrow layouts;
- keyboard-first navigation and command palette;
- optional mouse-wheel history navigation through Bubble Tea cell-motion mouse mode;
- dark and light themes;
- JSON customization for theme, refresh cadence, inspector cadence, viewport/cache bounds, FPS, keymaps and command aliases;
- Unicode-safe composer/navigation behavior and terminal resize handling;
- separate steering and queued follow-up paths for active Agents.

The client can be killed or detached without cancelling runtime work. Reattachment rebuilds its visible state from the daemon projection instead of treating terminal state as canonical.

### Bounded conversation and history

Workspace projection is cursor-based and bounded:

- attach returns only a bounded viewport and process-tree projection;
- older history is paged on demand;
- refresh is cursor-based and returns `Behind` when a client must resynchronize instead of accumulating an unbounded presentation queue;
- the local presentation cache has a hard maximum and retains only the required live/history window;
- referenced large artifacts remain object references rather than being materialized into the hot TUI model;
- model-private reasoning is not projected into visible conversation blocks.

A synthetic 100,000-block benchmark is part of CI and proves that session age does not force full-history rendering.

### Multi-Agent cockpit

The workspace exposes durable root/child process state without binding Agent lifetime to the terminal process. The cockpit supports:

- process-tree focus switching;
- current Agent status and blocked state visibility;
- steering the focused Agent;
- separate queued follow-ups;
- suspend/resume controls through daemon APIs;
- bounded tree projection suitable for parallel Agent supervision.

### Plan and diff review

PLAN and REVIEW are first-class presentation surfaces:

- projected plan artifacts are decoded into selectable steps;
- projected diff artifacts are decoded into files/hunks and change counts;
- review focus is keyboard navigable;
- plan-step and diff-file feedback is sent back to the focused Agent with durable references instead of mutating canonical state in the TUI;
- PLAN → ACT remains an explicit user-visible mode transition while runtime authority continues to be enforced independently.

### G7 transaction workflow

G13 makes the durable G7 transaction workflow operable through the same control protocol used by headless clients.

The TUI/control path supports:

```text
verify
prepare
commit
rollback
reconcile
resolve uncertain effect
```

The daemon-side transaction operator reopens the isolated `WorkspaceWorld`, delegates state transitions to `transaction.Manager`, and revalidates the current Agent lifecycle/root Intent at the PREPARE/COMMIT guard points.

A cancelled/failed Agent or invalid WorkspaceWorld promotion target fails closed. UI commands cannot mint capabilities or bypass World/Intent/Effect enforcement.

The end-to-end transaction test proves that a speculative write does not leak into the target repository, verification succeeds in isolation, prepare produces a durable promotion plan, and commit promotes the verified result. A second test proves cancellation after verification blocks promotion.

`NEEDS_RECONCILIATION` remains visibly uncertain and is never rendered as a false successful commit or rollback.

### G8 fork comparison

Forks are a native inspector surface. The TUI presents:

- fork groups and lifecycle state;
- candidate branches;
- budget/token spend;
- evaluation summaries;
- selected/discarded state;
- winner and selection rationale.

This makes objective G8 winner selection legible without importing speculative branch state into the canonical target.

### Context, scheduler and authority visibility

Dedicated inspectors expose runtime-owned summaries for:

- Cognitive MMU/context page count, estimated token occupancy, recall leases, unresolved Context Faults and latest Context Manifest;
- scheduler/model routing, provider, profile version and durable budget consumption;
- root Intent identity/version/goal and authority projection;
- teams/negotiation state;
- verified continual-improvement state;
- transactions and forks.

The authority surface explicitly states that UI approval never grants runtime capability.

### Headless parity and platform support

All meaningful mutations are performed through daemon/control APIs. The TUI does not directly edit durable process state, transaction records, scheduler state, memory or authority.

The CI matrix builds and tests:

```text
./cmd/go-agent
./cmd/go-agentctl
./cmd/go-agent-tui
```

on Linux, macOS and Windows.

## G13 killer conditions

The implemented test/benchmark suite covers the G13 closure conditions:

1. detaching/killing the TUI does not mutate or cancel Agent runtime work;
2. 100,000-block history is bounded to the configured viewport/cache and benchmarked in CI;
3. process focus, steering and queued follow-ups remain separate runtime operations;
4. plan/diff review feedback targets concrete projected artifacts/steps/files;
5. `NEEDS_RECONCILIATION` is rendered as uncertainty, never false success;
6. G8 fork candidates and the selected winner are visibly comparable;
7. core navigation, mode switching, review, transaction commands and detach have keyboard paths;
8. Unicode and adaptive resize behavior are covered across narrow, medium and wide terminal widths;
9. private chain-of-thought fields are never projected as visible reasoning;
10. G7 promotion revalidates current process policy and fails closed after cancellation.

## Performance gate

GitHub Actions run `34238313757` passed the authoritative G13 implementation head `d395f925f96deeebdcc04e4cf93c1d0da70d704b` across Linux, macOS and Windows, including the race detector, vet, the TUI binary build and the Linux bounded TUI benchmark gate.

Representative Linux benchmark results on the final code head family are:

```text
BenchmarkG13Bound100KHistory       ~0.08 ms/op
BenchmarkG13NormalViewportRedraw   ~2.75 ms/op
```

Both are well below the initial 50 ms engineering target for local viewport work. The 100k benchmark performs one bounded retention operation and does not retain the full synthetic history in the TUI model.

## CI gate

```text
go test ./...                                                        PASS Linux/macOS/Windows
go test -race ./...                                                  PASS Linux
go test -tags=soak -run '^$' ./internal/mmu ./internal/storage/sqlite PASS Linux compile gate
go test -run '^$' -bench '^BenchmarkG13' -benchmem -benchtime=250ms ./internal/tui
                                                                       PASS Linux
go vet ./...                                                         PASS Linux/macOS/Windows
go build ./cmd/go-agent ./cmd/go-agentctl ./cmd/go-agent-tui         PASS Linux/macOS/Windows
```

## Exit invariants

- canonical Agent state remains daemon-owned;
- presentation queues/history/cache remain bounded;
- TUI detach/crash cannot cancel durable Agent work by itself;
- hidden model reasoning is never displayed;
- PLAN/REVIEW UX cannot bypass capabilities, Intent or Effect enforcement;
- transaction commit remains verified and policy-guarded;
- uncertain transaction outcomes remain explicit;
- fork comparison is observational until runtime promotion semantics authorize a winner;
- all important mutating operations retain headless/control-protocol parity;
- Linux/macOS/Windows builds and tests remain green;
- the race detector remains green;
- 100k-history and normal-redraw performance gates remain in CI.

## Result

**G13 is COMPLETE.**

The next implementation generation is **G14 — Distributed Worlds / Workers**.
