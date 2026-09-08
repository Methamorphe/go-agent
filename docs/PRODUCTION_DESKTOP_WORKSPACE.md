# Production Desktop Workspace — G13

## Purpose

G13 includes a first-class **desktop GUI** in addition to the production TUI.

The desktop application is intended to be the richest everyday Agent GO experience: modern, calm, extremely responsive, visually polished and capable of exposing multi-Agent execution, plans, diffs, transactions, forks, MMU state and approvals without turning the product into a heavy IDE shell.

The desktop application is **not the Agent runtime**.

> **The GUI is a replaceable client of the durable Go daemon. Closing or crashing the GUI must never terminate Agent work.**

The same canonical runtime must remain usable through CLI, TUI, desktop GUI and future IDE/web clients.

---

# 1. Product position

G13 ships two complementary interactive clients:

```text
CLI / TUI                 Desktop GUI
──────────                ───────────
power-user                primary rich workspace
SSH/tmux                  visual multi-Agent cockpit
keyboard-first            keyboard + mouse
minimal footprint         richer plans/diffs/graphs
remote/headless friendly  local developer experience
        \                    /
         \                  /
          └── same daemon ─┘
```

Neither client owns canonical Agent state.

The GUI may provide a richer presentation than the TUI, but it must never gain private semantics that cannot be represented through the runtime protocol.

---

# 2. Architecture invariant

Preferred architecture:

```text
┌─────────────────────────────┐
│      Desktop Workspace      │
│ presentation + interaction  │
└──────────────┬──────────────┘
               │
       versioned local IPC
               │
┌──────────────▼──────────────┐
│        go-agent daemon      │
│ canonical runtime/state     │
│ Agents / MMU / Worlds       │
│ Transactions / Scheduler    │
└──────────────┬──────────────┘
               │
         durable storage
```

Hard rules:

- no canonical Agent state in GUI memory;
- no model/provider execution owned by the GUI process;
- no direct filesystem mutation that bypasses World/Authority/Transaction semantics;
- no GUI-only approval path that bypasses the kernel;
- reconnect through projections/cursors rather than replaying all history;
- the daemon continues when every GUI window is closed;
- GUI upgrades/restarts do not require Agent restart.

---

# 3. Technology selection gate

The framework is selected empirically against the budgets in this document.

Preferred candidate stack:

```text
UI       Vue 3 + TypeScript + Vite
Shell    Tauri-class lightweight native shell
Runtime  existing Go daemon, separate process
IPC      existing versioned daemon protocol
```

Wails/native alternatives may be benchmarked.

## Default exclusions

The production GUI must **not default to Electron or another bundled-Chromium + Node runtime architecture**.

A technology that embeds a large browser/runtime stack is rejected unless it demonstrably satisfies the same package, RAM, startup and idle-CPU budgets as the lightweight candidates.

Use the operating-system WebView or genuinely native rendering where practical rather than shipping an entire browser engine with the application.

Framework convenience is not allowed to override product performance budgets.

---

# 4. Hard footprint budgets

Initial G13 engineering budgets on release builds:

```text
Application package / installer
  macOS / Linux target             <= 35 MB
  Windows target                   <= 45 MB
  hard ceiling                     <= 60 MB
  OS-provided WebView runtime      excluded from app package measurement

GUI idle RSS after stabilization
  target                           <= 90 MB
  hard ceiling                     <= 150 MB

Idle CPU while attached
  target                           <= 0.5% typical
  hard sustained ceiling           <= 1% without active work/animation

Cold launch → usable shell
  p50                              < 350 ms target
  p95                              < 700 ms

Warm launch / reopen
  p95                              < 300 ms

Local attach to active Agent
  normal session p95               < 250 ms
  synthetic 100k-block p95         < 500 ms target
```

Budgets may be recalibrated only from measured release builds and must not be relaxed merely to accommodate a heavier framework.

A regression beyond a hard ceiling blocks G13 release unless documented as a platform-specific external constraint.

---

# 5. Rendering and interaction budgets

The interface must feel immediate even while Agents and tools are streaming heavily.

```text
input → visible feedback p95        < 50 ms
UI main-thread long task            no routine task > 16 ms
60 Hz frame budget                  16.7 ms
120 Hz-friendly frame budget        8.3 ms where supported
stream presentation                 coalesced ~20–30 Hz
normal pane resize p95              < 50 ms
command palette open p95            < 100 ms
local navigation/tab switch p95     < 100 ms
```

Rules:

- never render one UI node per token;
- never parse the entire transcript on a streaming update;
- never synchronously read SQLite/storage from a render path;
- never rebuild the whole Agent tree because one leaf changed;
- never animate continuously while idle;
- use incremental projections and dirty-region/component updates;
- batch/coalesce high-frequency progress and stream events;
- expensive syntax highlighting/diff computation must be asynchronous and cancellable;
- avoid layout thrashing and unnecessary DOM depth;
- use GPU/compositor-friendly transforms for motion where applicable.

---

# 6. Long-session boundedness

The desktop GUI inherits the TUI invariant:

> **UI cost scales with what is visible and active, not with how old the Agent is.**

Required architecture:

```text
durable transcript / artifacts
          │
          ▼
paginated projection API
          │
          ▼
bounded viewport + LRU cache
          │
          ▼
virtualized renderer
```

The GUI must not hold 100,000 historical blocks as live components.

Large code output, logs, tool stdout, images and artifacts are referenced/paged rather than eagerly loaded.

Scrolling, resizing, new streaming tokens and theme changes must not trigger work proportional to total session history.

---

# 7. Modern visual language

The GUI must look current without becoming decorative or noisy.

## Visual principles

- calm, high-information-density developer workspace;
- restrained surfaces and borders;
- strong typography hierarchy rather than excessive cards;
- semantic color, never rainbow status noise;
- dark and light themes designed separately, not mechanically inverted;
- compact default density with an optional comfortable density;
- clear focus, hover, selected, warning, blocked and destructive states;
- subtle depth only where it communicates layering;
- no gratuitous glassmorphism, giant gradients, excessive shadows or oversized empty space;
- no fake terminal aesthetic for information better represented visually;
- code/diff surfaces remain monospace; surrounding product UI uses a highly legible system/UI font stack;
- icons remain consistent, sparse and paired with text where ambiguity is possible.

## Layout rules

- one dominant task surface at a time;
- sidebars are collapsible/resizable;
- inspectors open contextually rather than permanently consuming space;
- progressive disclosure for MMU, authority, scheduler and forensic details;
- conversation remains central during ordinary pair-programming;
- plan/diff/fork comparison may become the central workspace when relevant;
- avoid nested panels inside nested panels unless there is a real information hierarchy.

---

# 8. Motion rules

Animation exists to explain state changes, not decorate the product.

Default timing guidance:

```text
micro feedback                  ~80–120 ms
panel/tab transitions           ~120–180 ms
larger spatial transition       <= 220 ms
continuous idle animation       forbidden except minimal active-work indicators
```

Hard rules:

- no animation may delay an action becoming usable;
- no blocking entrance animations;
- streaming text is not animated character-by-character by the GUI;
- respect reduced-motion OS/user preference;
- use opacity/transform over layout-heavy animation;
- maintain responsiveness under concurrent tool output.

---

# 9. Keyboard and mouse parity

Desktop richness must not destroy terminal-grade efficiency.

Every frequent operation should be reachable without the mouse:

- focus Agent;
- switch workspace surface;
- open command palette;
- send / steer / queue follow-up;
- approve/deny;
- open plan;
- next/previous diff hunk;
- inspect transaction;
- inspect fork;
- search history;
- open artifact;
- cancel/pause/resume according to allowed runtime semantics.

Mouse interactions add discoverability and precision, especially for plans, diffs, graphs and split panes.

Shortcuts are configurable and conflict-aware.

---

# 10. Primary workspace surfaces

The GUI should make the existing runtime primitives feel like one coherent product:

```text
Project / Agent switcher
Agent cockpit / task tree
Conversation workspace
Composer
Plan workspace
Diff / change review
Integrated terminal/log preview
Tests / diagnostics
Transaction inspector
Fork comparison
Context / MMU inspector
Authority / approval inspector
Model / scheduler inspector
History tree + search + bookmarks
Artifacts viewer
Notifications / blocked work queue
```

The user should not need to understand kernel internals for ordinary work, but the internals should be inspectable when needed.

---

# 11. Multi-Agent UX

The desktop client is the richest multi-Agent supervisory surface.

Each Agent row/card can expose compactly:

```text
state
task
model
World
elapsed time
budget/cost
active action
blocked reason
transaction/fork status
```

Required behavior:

- switch focus instantly without suspending sibling Agents;
- steer one Agent while other Agents continue;
- queue a follow-up explicitly rather than ambiguously interrupting;
- filter/collapse large Agent trees;
- show blocked/approval-needed Agents without noisy global modals;
- surface child results/evidence without injecting entire child transcripts into the parent conversation.

---

# 12. Plan and diff UX

Plan and code review are first-class desktop workspaces.

## Plan

- hierarchical steps;
- dependencies and status;
- inline comments;
- rewrite selected step;
- approve selected/all steps where policy allows;
- explicit transition from PLAN to ACT;
- keyboard and mouse navigation.

## Diff

- unified and side-by-side modes where useful;
- virtualized large diffs;
- file tree with changed-file status;
- inline comments on hunks/lines;
- diagnostics/tests linked to affected changes;
- semantic distinction between speculative WorkspaceWorld state and promoted target state;
- no direct write path bypassing Agent Transaction semantics.

A large editor framework must not be bundled merely to display diffs if a materially lighter component satisfies the interaction requirements.

---

# 13. G7 transaction-native desktop UX

Agent Transactions are visible, not hidden behind a generic spinner.

The GUI exposes:

```text
CREATING
OPEN
VERIFYING
READY_TO_COMMIT
COMMITTING
ROLLING_BACK
COMMITTED
ROLLED_BACK
NEEDS_RECONCILIATION
```

Requirements:

- verification evidence is inspectable;
- commit/rollback/reconcile actions appear only when semantically valid;
- `NEEDS_RECONCILIATION` is visually unmistakable and never presented as success;
- known vs unknown effect outcome is explicit;
- speculative and promoted files are visually distinct;
- approval UI includes effect/authority context without inventing authority itself.

---

# 14. G8 fork-native desktop UX

Once G8 exists, desktop should make parallel futures intuitive:

```text
                BASE
                 │
           ┌─────┴─────┐
           │           │
        Fork A       Fork B
        tests ✓      tests ✓
        418 ms       291 ms
        +14/-8       +9/-4
           │           │
           └─────┬─────┘
                 ▼
             Promote B
```

Required comparison dimensions may include:

- diff;
- tests;
- benchmarks;
- evaluator score;
- model/cost usage;
- changed files;
- diagnostics;
- branch-local evidence/context.

The UI never implies that observed history was rewritten.

---

# 15. Accessibility and platform quality

G13 desktop is not complete without:

- WCAG-conscious contrast and focus states;
- keyboard-only core workflow;
- screen-reader labels for actionable controls where the shell supports them;
- reduced-motion support;
- zoom/text scaling without broken layout;
- Unicode-safe rendering;
- macOS/Linux/Windows release testing;
- correct native clipboard, file picker, notifications and window behavior;
- sensible behavior on HiDPI and multi-monitor setups.

Platform-specific polish is preferred over pretending every OS is identical.

---

# 16. Asset and dependency discipline

Application weight is treated as a first-class regression metric.

Rules:

- every production dependency needs a concrete product purpose;
- tree-shake and code-split noncritical surfaces;
- lazy-load heavy inspectors/viewers;
- no icon-font megabundles;
- no duplicate Markdown/highlighter/editor engines;
- no large analytics/telemetry SDK when a small native/client implementation is adequate;
- no bundled language servers unless explicitly required by a future feature;
- no shipping source maps/debug symbols inside normal release packages unless platform packaging requires them;
- fonts should normally use system stacks rather than bundled font families;
- release CI reports package size and fails on unjustified budget regression.

A new dependency that adds several megabytes must justify that cost in its review.

---

# 17. Startup discipline

Startup performs only what is required to show an interactive shell and connect to the daemon.

Do not eagerly initialize:

- all syntax grammars;
- every inspector;
- complete history;
- every Agent projection;
- diff engines for unopened diffs;
- graph layout engines;
- optional integrations/plugins.

Preferred sequence:

```text
window/shell visible
      ↓
daemon handshake
      ↓
active project + focused Agent summary
      ↓
latest conversation viewport
      ↓
subscribe live cursor
      ↓
lazy-load secondary surfaces
```

---

# 18. Failure behavior

Mandatory scenarios:

- kill GUI during model generation: Agent continues;
- kill GUI during command execution: World action continues according to runtime cancellation semantics, not window lifetime;
- kill GUI during transaction: transaction remains durable/reconcilable;
- restart GUI: current projection appears without full history replay;
- frozen/slow GUI: daemon queues remain bounded and can disconnect/recover client;
- corrupted GUI preference state: canonical Agent state is unaffected;
- unavailable optional viewer: user can still inspect underlying artifact/reference through another client/headless path.

---

# 19. Performance test suite

G13 requires release-build automated/semiautomated measurements, not development-server impressions.

Minimum benchmark matrix:

```text
cold start
warm start
idle RSS / CPU
input-to-paint
streaming response
10 simultaneous tool progress streams
3+ active child Agents
large diff
100k conversation blocks
multi-GB referenced artifacts
resize/split-pane stress
theme switch
GUI kill + reconnect
```

Metrics are captured per supported platform and tracked over time.

---

# 20. Killer demonstrations

G13 desktop cannot close until all of the following are credible:

1. install a release build whose package stays inside the footprint budget;
2. launch cold and become usable within the startup budget;
3. attach to a 100,000-block session without loading/rendering complete history;
4. supervise at least three Agents concurrently while keeping UI interactions fluid;
5. comment/rewrite a plan, execute, inspect tests/diff and complete a G7 transaction;
6. force `NEEDS_RECONCILIATION` and prove the GUI never presents a false successful state;
7. kill the GUI mid-task and reattach while runtime work continued;
8. perform the core workflow keyboard-only;
9. demonstrate reduced-motion and light/dark themes without layout/performance regression;
10. show package/RAM/startup metrics from macOS, Linux and Windows release builds.

---

# 21. Completion criteria

```text
desktop GUI is a replaceable daemon client
modern polished dark + light UI
strict visual-system/design-token implementation
keyboard-first + excellent mouse UX
package hard ceiling <= 60 MB
idle RSS hard ceiling <= 150 MB
cold-start p95 < 700 ms
bounded virtualized 100k-block history
60 Hz frame budget respected for normal interactions
no bundled Chromium/Node architecture by default
multi-Agent cockpit
plan workspace
diff review
G7 transaction UX
G8 fork comparison when available
MMU/context inspection
authority/approval inspection
model/scheduler visibility
macOS/Linux/Windows release matrix
GUI crash never kills Agent work
headless/TUI parity for canonical operations
```

---

# Core invariant

> **The desktop app should feel closer to a small native developer tool than to a browser IDE wrapped in a window. Visual richness is allowed; architectural and runtime bloat are not.**
