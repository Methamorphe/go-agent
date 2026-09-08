# Current Implementation Generation

**Updated: September 8, 2026**

```text
A0  COMPLETE
G0  COMPLETE
G1  COMPLETE
G2  COMPLETE
G3  COMPLETE
G4  COMPLETE
G5  COMPLETE
G6  COMPLETE
G7  COMPLETE
G8  COMPLETE
G9  COMPLETE
G10 COMPLETE
G11 COMPLETE
G12 COMPLETE
G13 COMPLETE
G14 READY
```

## Current generation

**G13 — Production TUI / Interactive Agent Workspace: COMPLETE.**

G13 turns the durable runtime into a production fullscreen terminal workspace without moving canonical Agent state into the client.

The implementation now includes:

- dedicated `go-agent-tui` binary built on Bubble Tea v2;
- ASK, PLAN, ACT, REVIEW and OBSERVE interaction modes;
- adaptive wide/medium/narrow layouts with Unicode-safe resize behavior;
- keyboard-first navigation, command palette, search and optional mouse-wheel history navigation;
- dark/light themes plus declarative JSON keymap/command/performance customization;
- cursor-based attach/refresh with bounded history pages, bounded local cache and explicit client resynchronization when behind;
- a virtualized 100,000-block history path that never requires the full session to remain hot in the TUI model;
- a durable multi-Agent cockpit with focus switching, steering, queued follow-ups and suspend/resume controls;
- first-class projected plan and diff review surfaces with selectable step/file feedback;
- fully operable G7 transaction commands for verify, prepare, commit, rollback, reconcile and uncertain-effect resolution;
- daemon-side transaction promotion guards that revalidate current Agent lifecycle/root Intent and fail closed after cancellation;
- explicit `NEEDS_RECONCILIATION` presentation that never claims false commit/rollback success;
- G8 fork comparison with candidates, spend, evaluations, selected/discarded state and winner rationale;
- Context/MMU, scheduler/model, authority, teams, transactions, forks and verified-improvement inspectors;
- explicit prevention of private chain-of-thought projection into visible conversation blocks;
- control-protocol/headless parity for meaningful runtime mutations;
- Linux/macOS/Windows test, vet and build coverage for the daemon, CLI and TUI;
- Linux race-detector and bounded G13 performance benchmark gates in CI.

The core architectural invariant remains intact: **the TUI is a replaceable client of the durable runtime and never owns canonical Agent state.** Detaching or killing the terminal client does not cancel Agent work by itself.

See:

- `G13_EXIT_REVIEW.md`;
- `PRODUCTION_TUI_WORKSPACE.md`;
- `G12_EXIT_REVIEW.md`.

## Validation note

GitHub Actions run `34238313757` passed the authoritative G13 code head `d395f925f96deeebdcc04e4cf93c1d0da70d704b` across Linux, macOS and Windows, including `go test ./...`, `go vet ./...`, all three binary builds and the Linux race detector.

The Linux CI gate also compiles the long-duration soak scenarios and executes the bounded G13 TUI benchmarks. Representative final-head-family results are approximately:

```text
100k-history bounded retention   ~0.08 ms/op
normal viewport redraw           ~2.75 ms/op
```

Both remain well below the initial 50 ms local viewport engineering target.

## G13 closure matrix

```text
production fullscreen TUI                    PASS
keyboard-first workflow                      PASS
optional mouse history interaction           PASS
virtualized/bounded conversation history     PASS
plan and diff review                         PASS
multi-Agent cockpit                          PASS
G7 transaction operate/inspect               PASS
G8 fork comparison                           PASS
MMU/context inspector                        PASS
authority inspector                          PASS
model/scheduler visibility                   PASS
themes/keymaps/command palette               PASS
headless/runtime API parity                  PASS
macOS/Linux/Windows CI                       PASS
race detector                                PASS
100k-history responsiveness benchmark        PASS
TUI detach/crash isolation from Agent work   PASS
hidden-reasoning non-projection              PASS
```

Long-duration 1h/8h/24h reference campaigns remain empirical calibration work shared by the project and are not a semantic G13 closure dependency.

## Next generation

**G14 — Distributed Worlds / Workers: READY.**

G14 can now extend the same durable Agent Process and World abstractions across local and remote workers while preserving identity, authority, transaction, object and reconciliation semantics established through G13.
