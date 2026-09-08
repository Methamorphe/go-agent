# G14 — Wails v3 Desktop GUI / Agent Workspace

## Decision

The desktop GUI is built on **Wails v3**, intentionally, even while Wails v3 remains in beta.

This is a deliberate product and architecture choice rather than a temporary experiment. The project prefers to build directly on the Wails generation intended for the long-term desktop client instead of shipping on Wails v2 and planning an avoidable migration later.

The beta status therefore changes the engineering gates, not the framework choice.

## Goal

Deliver a modern, fluid, lightweight desktop Agent workspace that exposes the same durable runtime as the production TUI while keeping canonical Agent state in the daemon.

The GUI must feel like a first-class desktop product rather than a web app wrapped in a window:

- fast startup;
- low idle memory;
- small distributable footprint;
- immediate input response;
- smooth streaming;
- tasteful, purposeful animations;
- dense but readable information architecture;
- keyboard-first power-user workflows with excellent mouse support;
- native-feeling macOS, Windows and Linux integration;
- zero duplication of kernel semantics in the frontend.

## Product inspiration

The interaction model should learn from mature agentic workspaces such as Hermes and Goose without cloning their branding or layouts.

Patterns to retain:

- central conversation/work stream;
- durable session navigation;
- side navigation for agents, sessions and workspace context;
- right-side contextual inspector for files, diffs, plans, artifacts and runtime state;
- visible tool/action activity without overwhelming the conversation;
- editable queued instructions/follow-ups;
- command palette and keyboard shortcuts;
- fast switching between Agent processes and branches;
- in-place plan/diff approval and targeted feedback;
- transparent runtime state, transaction state and reconciliation state;
- excellent streaming behavior and long-history virtualization.

## Architecture

```text
Wails v3 Desktop Client
        │
        │ local control protocol / typed bridge
        ▼
Durable go-agent Runtime Daemon
        │
        ├─ Agent Processes
        ├─ Event Ledger
        ├─ MMU / Epistemic Memory
        ├─ Scheduler
        ├─ Worlds / Transactions / Forks
        └─ Object Store
```

The desktop application is a replaceable presentation client exactly like the TUI.

Closing, reloading or crashing the GUI must not terminate durable Agent work unless the user explicitly requests process cancellation through the runtime API.

## Wails v3 policy

Wails v3 is the required framework.

Because it is beta, G14 must additionally include:

- exact Wails v3 version pinning;
- reproducible toolchain lock/documentation;
- explicit upgrade procedure;
- smoke tests around bindings/events/windows most likely to change during beta;
- dependency upgrade review before every Wails bump;
- no reliance on undocumented internals when a supported API exists;
- adapter boundaries around Wails-specific services so UI domain logic remains framework-independent;
- CI builds on macOS, Windows and Linux;
- rollback path to the previous pinned Wails v3 beta if an upstream update regresses the application.

No Wails v2 compatibility layer or planned v2 fallback is required.

## UX system

### Main layout

Adaptive desktop layout:

```text
┌──────────────┬───────────────────────────────┬──────────────────────┐
│ Navigation   │ Agent conversation / activity │ Context inspector    │
│              │                               │                      │
│ Sessions     │ streaming response            │ plan / diff / files  │
│ Agents       │ tool calls                    │ runtime inspectors    │
│ Workspace    │ user composer                 │ artifacts / branches │
└──────────────┴───────────────────────────────┴──────────────────────┘
```

Panels must be collapsible and resizeable. The center workspace always receives priority when space is constrained.

### Interaction modes

Preserve semantic parity with the TUI:

- ASK;
- PLAN;
- ACT;
- REVIEW;
- OBSERVE.

The mode should affect available actions and visual emphasis without producing completely different applications.

### Command palette

A universal command palette should expose navigation and runtime actions with fuzzy search and shortcut hints.

Examples:

- switch Agent;
- switch session;
- open file;
- inspect current transaction;
- inspect fork candidates;
- suspend/resume Agent;
- change interaction mode;
- toggle sidebars;
- open settings;
- execute safe runtime commands.

## Visual language

The target is restrained modernity rather than decorative animation.

Principles:

- crisp typography;
- strong hierarchy;
- generous but efficient spacing;
- subtle depth and surfaces;
- coherent dark and light themes;
- excellent code/diff legibility;
- semantic status colors;
- minimal chrome;
- no gratuitous gradients or permanently moving backgrounds;
- animation communicates state changes rather than hiding latency.

## Motion system

Animations must be short, interruptible and GPU-friendly.

Use motion for:

- panel opening/closing;
- inspector transitions;
- new streamed activity arriving;
- Agent/session switching;
- command palette entry/exit;
- expand/collapse groups;
- transaction/fork state transitions;
- optimistic acknowledgement of safe local interactions.

Avoid animation for continuous token-by-token layout shifts where it harms readability.

Honor reduced-motion system preferences.

Target motion durations should generally remain within roughly 100–220 ms for common UI transitions, with longer transitions reserved for meaningful workspace changes.

## Streaming and responsiveness

The GUI must not re-render the full conversation for each streamed token.

Required behavior:

- coalesced streaming updates;
- bounded UI event queues;
- long-history virtualization;
- paginated historical loading;
- lazy inspector rendering;
- large artifacts represented by references until opened;
- cancellable expensive previews;
- no unbounded frontend cache;
- background client work must yield to input/scroll/render responsiveness.

## Desktop-native behavior

Where supported through Wails v3, provide:

- native menus where appropriate;
- native file/folder dialogs;
- OS theme detection;
- window state persistence;
- deep-link/session opening hooks where useful;
- native notifications for meaningful background Agent events;
- sensible macOS titlebar/window behavior;
- proper Windows window controls;
- Linux desktop integration where available.

These features must remain presentation conveniences and never become required for runtime correctness.

## Functional surfaces

### Conversation / activity stream

- user and Agent messages;
- streaming model output;
- compact tool activity blocks;
- expandable tool details;
- referenced artifacts;
- queued follow-ups;
- steering messages;
- visible cancellation/suspend state;
- no private chain-of-thought rendering.

### Multi-Agent cockpit

- process tree;
- Agent status and current task;
- focus switching;
- child Agent activity summary;
- team membership;
- budgets/spend summary;
- suspend/resume/cancel through runtime APIs.

### Plans and review

- structured plan steps;
- progress states;
- step-level feedback;
- file/hunk diff review;
- accept/reject/comment workflows routed through existing runtime semantics.

### Transactions and reconciliation

Expose G7 semantics explicitly:

```text
verify
prepare
commit
rollback
reconcile
resolve uncertain effect
```

`NEEDS_RECONCILIATION` must remain visibly distinct from success and failure.

### Forks

- branch candidates;
- evaluation scores;
- spend;
- branch status;
- selected winner;
- rationale;
- diff comparison where applicable.

### Runtime inspectors

Provide modern visual inspectors for:

- Context/MMU and Context Faults;
- epistemic beliefs/evidence;
- model/provider routing;
- scheduler and budgets;
- Intent/authority;
- transactions;
- cognitive forks;
- adaptive teams/negotiation;
- verified continual improvement.

Inspectors must request bounded projections from the runtime rather than loading canonical history into the frontend.

## Performance budget

Performance is a product feature and a G14 exit requirement.

Initial engineering budgets:

- warm interactive window available as quickly as practical, target `< 1 s` on a representative modern developer machine;
- idle client CPU effectively negligible;
- interaction-to-paint target `< 16 ms` for ordinary local UI actions when no expensive OS/WebView operation is involved;
- normal panel transition sustains visually smooth rendering at display refresh rate;
- ordinary streamed updates never block scrolling/input;
- 100k-message synthetic history remains bounded through virtualization;
- frontend memory must plateau under long-session/history stress rather than grow linearly;
- distributable size and idle RAM are tracked in CI/release benchmarks and treated as regression metrics.

Targets are engineering budgets, not permission to fake measurements. Exit review records actual macOS/Windows/Linux measurements and the representative hardware used.

## Frontend architecture requirements

The frontend must remain modular and testable.

Suggested boundaries:

```text
frontend/
  app/
  features/
    conversation/
    agents/
    plans/
    review/
    transactions/
    forks/
    inspectors/
    settings/
  components/
  design-system/
  state/
  runtime-client/
```

Rules:

- generated Wails bindings stay at the edge;
- runtime DTOs are converted into frontend view models;
- presentation state is separated from durable runtime state;
- domain operations go through a small runtime-client abstraction;
- no direct SQLite access from the frontend;
- no duplicated authorization/effect logic in TypeScript/JavaScript;
- frontend approval never mints capabilities;
- client-side optimistic state must reconcile against daemon truth.

## Design-system requirements

Create a small internal design system rather than accumulating one-off components.

At minimum:

- typography scale;
- spacing scale;
- radius/elevation tokens;
- semantic colors;
- motion tokens;
- focus states;
- buttons;
- inputs/composer;
- menus/popovers;
- tabs;
- tree/list rows;
- badges/status indicators;
- panels/cards;
- code blocks;
- diff primitives;
- skeleton/loading states;
- toasts/notifications;
- accessible dialogs.

## Accessibility

Required:

- full keyboard navigation for core flows;
- visible focus states;
- reasonable contrast in dark/light modes;
- reduced-motion support;
- scalable text without catastrophic layout breakage;
- semantic labels for interactive controls exposed through the WebView accessibility tree.

## Required invariants

- canonical Agent state remains in the daemon;
- GUI detach/crash does not cancel durable Agent work;
- all meaningful mutations retain headless/control-protocol parity;
- GUI cannot mint authority or weaken effect policy;
- uncertain external outcomes remain uncertain until reconciliation;
- frontend caches and queues are bounded;
- long history is virtualized;
- large payloads are streamed/referenced rather than fully buffered;
- hidden model reasoning is never exposed accidentally;
- Wails-specific failures cannot corrupt durable runtime state;
- GUI and TUI can attach to the same durable sessions and observe consistent canonical state.

## Testing and quality gates

G14 closes only after:

- unit tests for frontend state/view-model logic;
- component tests for high-value interaction surfaces;
- runtime-client contract tests;
- Wails binding/event smoke tests;
- end-to-end happy-path desktop workflow;
- GUI detach/crash while Agent continues running;
- daemon restart followed by GUI resync;
- slow-client/backpressure scenarios;
- 100k-history virtualization test;
- long streaming stress test;
- reduced-motion and keyboard-navigation checks;
- macOS build + smoke;
- Windows build + smoke;
- Linux build + smoke;
- measured startup/RAM/binary-size/render responsiveness results in the exit review.

## Initial killer demonstration

Start a durable coding Agent, interact with it from the Wails v3 GUI, watch multiple child Agents and tool actions stream live, inspect and review a generated diff, detach/quit the GUI while work continues, reopen the GUI, reattach to the same Agent, review a transaction/fork state and complete the workflow without loss of canonical state.

## Exit definition

G14 is complete when the Wails v3 desktop workspace is genuinely pleasant to use for daily Agent work, remains lightweight and responsive under long sessions, preserves every daemon/kernel invariant established through G13, and has cross-platform evidence for its performance and crash/reconnect behavior.
