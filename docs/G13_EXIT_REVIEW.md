# G13 Exit Review — Production TUI / Interactive Agent Workspace

Status: **PASS**

## Scope delivered

G13 turns the durable go-agent runtime into a production fullscreen terminal workspace while preserving the architectural rule that the TUI is a replaceable client and never owns canonical Agent state.

### Production terminal client

A dedicated `go-agent-tui` binary provides a Bubble Tea v2 fullscreen workspace with:

- ASK, PLAN, ACT, REVIEW and OBSERVE interaction modes;
- adaptive wide/medium/narrow layouts;
- keyboard-first navigation and command palette;
- optional mouse-wheel history navigation;
- dark and light themes;
- JSON customization for theme, refresh cadence, inspector cadence, viewport/cache bounds, FPS, keymaps and command aliases;
- Unicode-safe composer/navigation behavior and terminal resize handling;
- separate steering and queued follow-up paths for active Agents.

### One-command runtime lifecycle

The final G13 user experience no longer requires a separate terminal for the daemon.

Running:

```text
go-agent-tui
```

or, from the repository:

```text
make run
```

is sufficient to enter the interactive workspace.

The TUI now:

1. probes the configured local control endpoint without mutating runtime state;
2. reuses an already-running compatible local runtime when available;
3. otherwise re-executes its own binary in an internal runtime-daemon mode;
4. starts that runtime as a detached OS process/session;
5. redirects detached runtime diagnostics to `runtime-data/runtime.log`;
6. waits for the control endpoint to become ready with bounded timeout/probe intervals;
7. launches the fullscreen client only after runtime readiness is established.

Unix uses a detached session and Windows uses detached/new-process-group creation flags. The daemon is therefore not a child terminal job whose lifetime is tied to the fullscreen UI.

This preserves the core G13 failure invariant: **closing or crashing the TUI does not cancel the durable runtime or active Agent work.** A later `go-agent-tui` invocation simply probes and reattaches to the existing runtime.

Invalid UI mode/theme/customization options are validated before autostart, so malformed client configuration does not create a daemon as a side effect.

The Makefile now treats the TUI as the normal development entry point:

```text
make run     -> fullscreen TUI + automatic runtime lifecycle
make tui     -> same explicit TUI entry point
make daemon  -> explicit headless daemon for development/ops
make build   -> builds both go-agent and go-agent-tui
```

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

G13 makes the durable G7 transaction workflow operable through the same control protocol used by headless clients:

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

`NEEDS_RECONCILIATION` remains visibly uncertain and is never rendered as a false successful commit or rollback.

### G8 fork comparison

Forks are a native inspector surface. The TUI presents fork groups, candidate branches, budget/token spend, evaluation summaries, selected/discarded state, winner and selection rationale without importing speculative branch state into the canonical target.

### Context, scheduler and authority visibility

Dedicated inspectors expose runtime-owned summaries for:

- Cognitive MMU/context pages, estimated token occupancy, recall leases, unresolved Context Faults and latest Context Manifest;
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
2. the one-command TUI autostarts a detached runtime while retaining daemon/client process separation;
3. 100,000-block history is bounded to the configured viewport/cache and benchmarked in CI;
4. process focus, steering and queued follow-ups remain separate runtime operations;
5. plan/diff review feedback targets concrete projected artifacts/steps/files;
6. `NEEDS_RECONCILIATION` is rendered as uncertainty, never false success;
7. G8 fork candidates and the selected winner are visibly comparable;
8. core navigation, mode switching, review, transaction commands and detach have keyboard paths;
9. Unicode and adaptive resize behavior are covered across narrow, medium and wide terminal widths;
10. private chain-of-thought fields are never projected as visible reasoning;
11. G7 promotion revalidates current process policy and fails closed after cancellation.

## Performance and CI gate

GitHub Actions run `34243114439` passed the final one-command TUI code head `ed63b9bc6b34e558edced8874c1307bff55fae51` across Linux, macOS and Windows, including the race detector, vet, all three binary builds and the Linux bounded TUI benchmark gate.

The immediately preceding launcher/build head `a22d7e9b9dee8b8828ab8f602c53e0b009e2fb14` also passed the full matrix, including the Windows detached-process implementation.

Representative Linux G13 benchmark results remain well below the initial 50 ms local viewport target:

```text
100k-history bounded retention   << 1 ms/op
normal viewport redraw           ~1–3 ms/op
```

CI gate:

```text
go test ./...                                                         PASS Linux/macOS/Windows
go test -race ./...                                                   PASS Linux
go test -tags=soak -run '^$' ./internal/mmu ./internal/storage/sqlite PASS Linux compile gate
go test -run '^$' -bench '^BenchmarkG13' -benchmem -benchtime=250ms ./internal/tui
                                                                        PASS Linux
go vet ./...                                                          PASS Linux/macOS/Windows
go build ./cmd/go-agent ./cmd/go-agentctl ./cmd/go-agent-tui          PASS Linux/macOS/Windows
```

## Exit invariants

- canonical Agent state remains daemon-owned;
- presentation queues/history/cache remain bounded;
- a single TUI command handles runtime discovery/autostart and attachment;
- an autostarted runtime is OS-detached from the TUI process;
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

The terminal product now has a real one-command startup path while keeping the durable daemon independent from presentation lifetime.

The next implementation generation is **G14**.
