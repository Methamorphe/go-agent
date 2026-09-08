# Desktop GUI / Native Agent Workspace

## Status

**Planned implementation generation: G14**

The desktop GUI is the native graphical companion to the completed G13 terminal workspace. It must expose the same durable Agent runtime rather than create a second execution engine or a GUI-owned session model.

The target is a **fast, modern, highly fluid, lightweight cross-platform desktop application built with Wails**.

The product should feel closer to a purpose-built native agent workspace than to a web dashboard wrapped in a window.

---

## Product thesis

The project already has the difficult runtime properties: durable Agent Processes, bounded context, recursive Agents, typed Worlds, transactions, forks, epistemic memory, scheduling, authority, verified improvement and a replaceable TUI client.

G14 turns those capabilities into a polished desktop experience for users who do not want to live entirely in a terminal.

The GUI must preserve one central rule:

> **TUI, GUI and headless clients are projections of the same daemon-owned Agent state.**

A user must be able to start work in the TUI, detach, open the GUI, continue the same durable session, close the GUI, and later reattach without changing Agent identity or execution semantics.

---

## Inspiration

The interaction model should learn from modern agent products such as Hermes Desktop, Goose Desktop and current coding-agent workspaces without cloning their visual identity.

Useful patterns include:

- chat-first central workspace;
- left navigation for projects/sessions/Agents;
- right-side contextual preview/inspector rail;
- live structured tool activity rather than raw log spam;
- persistent multi-session navigation;
- command palette and keyboard-first shortcuts even in a GUI;
- drag-and-drop attachments;
- editable queued follow-ups while the Agent is busy;
- plan/diff review integrated into the main workflow;
- previews that do not force the user to leave the conversation;
- clear visual distinction between running, waiting, blocked, reviewing, failed and reconciliation-required states;
- first-class subagent activity without flooding the main transcript;
- session continuity across graphical, terminal and headless surfaces.

The UI should remain recognizably this project's own product, centered on its durable runtime primitives rather than becoming a generic chat application.

---

## Technical direction

### Desktop shell

Use **Wails** so the application keeps the Go-native architecture and uses the operating system WebView instead of bundling Chromium.

Target Wails v3 during G14 because its desktop API, multi-window model, system integration and Go/JS bridge fit the intended architecture. While v3 remains pre-stable, the repository must pin a tested version rather than track `latest`; moving to a stable v3 release should be preferred once available. Wails v2 remains an acceptable fallback only if a release-blocking v3 issue is demonstrated.

The GUI must not start another copy of the Agent kernel in a way that creates competing canonical state. It attaches to or supervises the same durable local runtime boundary used by the TUI/control protocol.

### Frontend

Preferred baseline:

```text
Wails
Go backend / bindings
Vue 3
TypeScript
Vite
CSS variables + design tokens
Web Animations API / CSS transitions first
```

Large UI/animation frameworks should not be added by default. A small dependency is acceptable only when it measurably improves interaction quality without compromising startup, bundle size or memory goals.

The frontend should use virtualized rendering for long transcripts, trees, logs and diffs. Canonical history remains in the daemon/object store.

---

## Core workspace layout

The default desktop layout should use three composable zones:

```text
┌─────────────────────────────────────────────────────────────────────────┐
│ titlebar / workspace / model-route / status / command palette          │
├───────────────┬───────────────────────────────────────┬─────────────────┤
│               │                                       │                 │
│ Projects      │ Conversation / Agent activity         │ Context rail    │
│ Sessions      │                                       │                 │
│ Agent tree    │ Streaming response                    │ Preview         │
│               │ Tool activity                         │ Diff            │
│               │ Plans / approvals                     │ Inspector       │
│               │                                       │ Artifacts       │
├───────────────┴───────────────────────────────────────┴─────────────────┤
│ composer / queued follow-ups / mode / attachments / run controls       │
└─────────────────────────────────────────────────────────────────────────┘
```

All rails must be collapsible. Medium and narrow layouts should progressively collapse inspectors into drawers/tabs rather than simply shrinking every panel.

---

## UX surfaces

### Conversation

- high-quality streaming Markdown;
- incremental rendering without rebuilding the whole transcript per token;
- structured tool cards with progressive state;
- collapsible verbose outputs;
- copy, retry, branch/fork and inspect actions where semantically valid;
- inline file/image/artifact references;
- search within session;
- jump-to-prompt timeline for long sessions;
- explicit resume/reconnect state after daemon/client interruption.

### Composer

- multiline editing;
- drag-and-drop files;
- command palette/slash actions;
- ASK / PLAN / ACT / REVIEW / OBSERVE mode selection;
- model route visibility without forcing manual model selection;
- queued follow-ups while an Agent is busy;
- edit/reorder/delete queued messages before dispatch;
- immediate stop/steer controls;
- keyboard shortcuts discoverable from the UI.

### Agent cockpit

- root/child Agent tree;
- live state indicators;
- per-Agent current task summary;
- bounded recent activity preview;
- focus switching without interrupting siblings;
- budget/spend indicators;
- team formation and negotiation visualization;
- suspend/resume/cancel actions through runtime APIs only.

### Review workflow

- plan steps as interactive review units;
- side-by-side or unified diffs;
- file tree + changed-file counts;
- hunk-level comments/feedback;
- transaction verify/prepare/commit/rollback/reconcile controls;
- explicit uncertain-effect state;
- fork candidates and evaluation comparison;
- winner rationale and promotion state.

### Contextual preview rail

The right rail should host tabs such as:

- file preview;
- rendered Markdown;
- source code;
- image/artifact preview;
- browser/dev-server preview where permitted;
- terminal/process output;
- diff;
- context/MMU inspector;
- epistemic evidence/belief inspector;
- authority/Intent inspector;
- scheduler/model/budget inspector;
- transaction/fork/team/improvement inspector.

Opening a preview must not replace the current conversation.

### Settings and onboarding

- fast first-run path;
- provider/model configuration;
- local/remote runtime selection when supported;
- themes and density;
- font sizing;
- animation/reduced-motion preference;
- keymap customization;
- runtime diagnostics;
- MCP/tool configuration when exposed by the kernel;
- no secret material rendered into logs or normal debug surfaces.

---

## Visual direction

The target is modern but restrained:

- native-feeling frameless window where platform behavior remains correct;
- generous spacing and strong information hierarchy;
- rounded surfaces used selectively rather than everywhere;
- subtle depth, translucency and blur only where cheap and readable;
- excellent dark mode and first-class light mode;
- semantic status colors driven by tokens;
- crisp typography and code rendering;
- compact dense mode for technical users;
- no permanent visual noise from inactive runtime internals.

Animations should communicate state and spatial continuity rather than decorate the UI.

Preferred animation behavior:

- 120–180 ms micro-interactions;
- 180–260 ms panel/drawer transitions;
- transform/opacity animations where possible;
- spring-like motion only for a few high-value interactions;
- no layout-thrashing token-by-token animations;
- animated streaming caret/activity indicators kept cheap;
- skeleton/progressive loading instead of blocking spinners;
- `prefers-reduced-motion` honored everywhere.

A user should be able to feel that the interface is alive while profiler traces remain boring.

---

## Performance budgets

Performance is a product requirement, not late polish.

Initial engineering targets for a release build on representative modern hardware:

```text
warm first interactive frame           < 300 ms target
cold usable window                     < 700 ms target
normal UI interaction response         < 50 ms
animation cadence                      60 fps target
routine frame budget                   < 16.7 ms
large-history scroll                   no full-history materialization
idle GUI CPU                           approximately zero
idle GUI memory                        bounded and measured
100k-block session                     bounded local projection/cache
stream updates                         coalesced; never one render per token
binary/install size                    continuously measured in CI
```

The exact startup/RAM/size thresholds become release gates only after baseline measurements on macOS, Windows and Linux. G14 is not complete until those baselines exist and regressions are automated.

The implementation must prefer:

- native WebView instead of bundled browser runtime;
- code splitting for non-core inspectors/settings;
- lazy rendering for heavy previews;
- transcript and diff virtualization;
- bounded caches;
- coalesced event delivery;
- incremental Markdown parsing/rendering;
- thumbnail/preview generation off the UI hot path;
- no hidden polling loops when daemon events can drive updates.

---

## Architecture invariants

G14 must preserve all G0–G13 invariants and additionally prove:

1. **Daemon ownership** — GUI state is disposable projection state; canonical Agent state remains durable in the runtime.
2. **Surface parity** — meaningful GUI mutations have equivalent runtime/control semantics and do not exist only as frontend state.
3. **Detach safety** — closing/crashing the GUI does not cancel Agent work unless explicitly requested.
4. **Bounded projection** — GUI memory and render cost do not scale linearly with lifetime session history.
5. **No authority minting** — a graphical approval cannot bypass current Intent/capability/effect checks.
6. **Truthful uncertainty** — reconciliation-required and unknown external outcomes are never rendered as success.
7. **Hidden reasoning stays hidden** — private model chain-of-thought is never surfaced by richer graphical inspectors.
8. **Backpressure** — slow WebView/rendering cannot create an unbounded daemon-to-GUI event queue.
9. **Cross-surface continuity** — TUI and GUI can attach sequentially to the same durable work without semantic drift.
10. **Accessibility** — keyboard operation, reduced motion, contrast and scalable text are product requirements.

---

## G14 candidate deliverables

- `go-agent-gui` Wails desktop application;
- reusable GUI projection API over the local control/runtime protocol;
- same session/project/Agent identities as TUI/headless clients;
- modern responsive three-zone workspace;
- virtualized conversation and activity views;
- streaming Markdown/tool cards;
- project/session navigation;
- multi-Agent cockpit;
- plan and diff review;
- transaction/fork/reconciliation UX;
- preview/artifact rail;
- runtime inspectors;
- command palette and comprehensive shortcuts;
- themes, density and accessibility controls;
- native notifications for meaningful waiting/completion states;
- crash/reconnect/resync handling;
- macOS/Windows/Linux packaging;
- signing/notarization path documented;
- UI unit/component tests;
- Go integration tests around GUI-facing APIs;
- end-to-end smoke tests;
- startup/memory/bundle/frame-time benchmark gates.

---

## Killer demonstration

1. Start a durable coding task in the G13 TUI.
2. Detach while the Agent continues running.
3. Open the Wails GUI and attach to the same Agent/session.
4. Watch subagents and tool activity update live.
5. Open a generated file and diff in the right preview rail without leaving chat.
6. Queue and reorder follow-up instructions while work continues.
7. Review a transaction/fork result and promote/commit it through the GUI.
8. Close the GUI during continued work.
9. Reopen either TUI or GUI and prove Agent identity, history, authority, budgets and uncertainty state are unchanged.
10. Repeat against a synthetic 100k-block history and demonstrate bounded memory/render behavior.

---

## Exit criteria

G14 is complete only when the GUI is not merely functional but **pleasant to use under continuous real work**.

Required closure evidence includes:

```text
Wails desktop shell                         PASS
macOS / Windows / Linux build               PASS
TUI ↔ GUI durable session continuity        PASS
bounded 100k-history projection             PASS
streaming without per-token rerender storm  PASS
plan/diff/transaction/fork UX               PASS
multi-Agent cockpit                         PASS
preview/inspector rail                      PASS
keyboard-first + accessibility              PASS
reduced-motion support                      PASS
crash/reconnect/resync                      PASS
no hidden-reasoning projection              PASS
runtime authority parity                    PASS
startup/memory/size measurements            PASS
frame-time/scroll benchmark gates           PASS
```

The bar is intentionally higher than "a Wails window that can chat". The generation exists to make the runtime feel like a premium, fast agent workspace while keeping the deployment and resource profile much closer to a native Go application than to a heavyweight Electron product.
