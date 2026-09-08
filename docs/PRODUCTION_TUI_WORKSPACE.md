# Production TUI / Interactive Agent Workspace

## Purpose

This document defines the product-level terminal experience planned for **G13 — Production TUI / Interactive Agent Workspace**.

The goal is not to build a prettier chat box. The TUI should become a fast, expressive cockpit for supervising durable agents, tools, transactions, forks, context, approvals and parallel work while preserving the kernel invariant established in `TUI_AND_STREAMING.md`:

> **The TUI is a replaceable client of the durable runtime. It never owns canonical Agent state.**

The design should take the strongest interaction ideas from modern coding-agent interfaces while remaining native to go-agent's process/World/MMU/transaction architecture.

---

# 1. Product thesis

The default experience should feel immediate enough for pair programming and powerful enough for supervising long-running autonomous work.

The target is:

```text
conversation speed
+ IDE-grade review
+ multi-agent supervision
+ durable process control
+ inspectable context/security
+ keyboard-first terminal ergonomics
```

The UI should reveal complexity progressively. A new user can type a request and work naturally; an advanced user can inspect every Agent, World, budget, transaction, fork, context page and causal event without leaving the terminal.

---

# 2. Inspiration baseline

G13 should learn from products rather than clone them.

## Claude Code-inspired qualities

- low-friction conversational coding loop;
- explicit plan/act and permission boundaries;
- clear, compact tool/action progress;
- strong keyboard workflow;
- useful command/status-line ergonomics;
- hooks and project-level customization without turning the main interaction into configuration management.

## Codex-inspired qualities

- first-class supervision of multiple agents/tasks;
- parallel work that remains understandable instead of becoming background noise;
- goal + success-criteria oriented execution;
- easy switching between focused pair-programming and higher-level orchestration;
- annotation/review workflows that let the user refine work at the point where it is displayed.

## Grok Build-inspired qualities

- rich fullscreen, mouse-capable TUI;
- dedicated plan viewer;
- review/comment/rewrite of plan steps before mutation;
- clean inline diff review;
- skills/plugins/hooks/MCP/subagent discoverability;
- visually polished presentation without sacrificing terminal speed.

## Pi-inspired qualities

- minimal, composable interaction primitives;
- themes, extensions, custom commands and keybindings;
- model switching during a session;
- tree-structured session/history navigation;
- steering a running agent independently from queued follow-up messages;
- headless/RPC parity so the TUI never becomes the only usable client.

The result must still be recognizably **go-agent**, with durable processes, recursive Agent trees, Cognitive MMU, Worlds, transactions and forks visible as native product concepts rather than hidden implementation details.

---

# 3. Core interaction model

The TUI has five primary interaction modes:

```text
ASK       explain / investigate without intended mutation
PLAN      inspect architecture and proposed work before mutation
ACT       execute normal authorized work
REVIEW    inspect plans, diffs, tests, transactions and candidate promotion
OBSERVE   supervise long-running / parallel Agent work with minimal input noise
```

Mode is visible and keyboard-switchable. It is never security authority by itself: capabilities, Intent, Effect classification and World guarantees remain enforced by the runtime.

---

# 4. Default layout

Wide terminal:

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ project · branch · Agent · model · mode · context · cost · world · tx    │
├──────────────────┬────────────────────────────────────┬────────────────────┤
│ AGENTS / TASKS   │ CONVERSATION / WORK               │ INSPECTOR          │
│                  │                                    │                    │
│ root             │ user / assistant                   │ Plan               │
│ ├─ reviewer      │ tool cards                         │ Diff               │
│ ├─ tests         │ streaming output                   │ Context/MMU        │
│ └─ researcher    │ plan / review blocks               │ Authority          │
│                  │                                    │ Transaction/Fork   │
├──────────────────┴────────────────────────────────────┴────────────────────┤
│ composer / command palette / steering queue                               │
└────────────────────────────────────────────────────────────────────────────┘
```

The layout adapts rather than merely shrinking:

- narrow terminals collapse side panes into overlays/tabs;
- the conversation remains the primary surface;
- mouse is supported but never required;
- every important operation has a keyboard path;
- pane state is client-local and never pollutes the Agent Event Ledger.

---

# 5. Conversation and streaming UX

Conversation is rendered as typed, virtualized blocks rather than an ever-growing string.

Required block types include:

```text
UserMessage
AssistantMessage
ProgressUpdate
Plan
ToolCall
ToolResult
Diff
TestResult
Artifact
Approval
ChildAgent
Transaction
ForkComparison
ContextFault
SecurityDecision
Checkpoint
Error
```

Behavior:

- model text streams smoothly into one mutable block;
- tool output is compact by default with live tail + persisted artifact reference;
- verbose commands collapse automatically while preserving exit status and useful tail;
- completed tool cards are one-line/compact unless expanded;
- Markdown is finalized incrementally without re-parsing the full transcript;
- code blocks support copy, file-path recognition and optional syntax highlighting;
- long history is paginated/virtualized exactly as defined in `TUI_AND_STREAMING.md`.

---

# 6. Composer, steering and follow-ups

The composer must support both normal messages and control of an active Agent.

Baseline semantics:

```text
Enter       send / steer current active Agent at next safe boundary
Alt+Enter   queue a follow-up after current work completes
Esc         close overlay / cancel local editor state
Ctrl+P      command palette
Ctrl+L      model picker
@           fuzzy project file/symbol reference
/           command / skill palette
```

Exact bindings remain configurable and must avoid terminal-host conflicts.

Steering and follow-up queues are visibly distinct so a user always knows whether a message will alter active work or wait for it.

---

# 7. Command palette

A fuzzy command palette is a first-class surface, not a collection of undocumented slash commands.

Examples:

```text
agent: spawn / switch / cancel / pause / resume
model: choose / objective / effort
plan: open / approve / rewrite
world: inspect / switch
transaction: verify / prepare / commit / rollback / reconcile
fork: create / compare / promote / discard
context: inspect / recall / pin
history: search / bookmark / tree / export
ui: theme / layout / keymap
skill: run / inspect
```

Commands expose discoverable descriptions and shortcuts. Slash-command compatibility can map onto the same registry.

---

# 8. Plan workspace

Complex tasks can enter PLAN mode before mutation.

The plan viewer supports:

- hierarchical steps;
- dependency indication;
- predicted files/Worlds/effects when known;
- per-step status;
- inline user comments;
- rewrite one step without regenerating everything;
- approve all or approve selected steps;
- explicit transition from PLAN to ACT.

Plan approval never bypasses runtime authority. It is user intent input, not a capability grant.

---

# 9. Diff and change review

Diff review should be one of the strongest parts of the product.

Required capabilities:

- inline and side-by-side layouts where terminal width permits;
- file tree of changed paths;
- additions/deletions summary;
- syntax-aware hunks;
- jump between changed files/hunks;
- comment on a hunk and send the comment back to the active Agent;
- inspect generated/test files separately;
- compare transaction source vs base vs current target when needed;
- clearly distinguish speculative WorkspaceWorld changes from promoted target changes.

G13 may expose hunk-level user review, but actual promotion semantics stay transaction-level unless the runtime explicitly supports a safe partial-promotion operation.

---

# 10. Agent tree and parallel work

Recursive Agent Processes become visible product primitives.

Each Agent row/card can show:

```text
state
current task
model
world
elapsed time
budget/cost
current tool/action
blocked/waiting reason
unread result/evidence
```

The user can:

- switch focus without stopping other Agents;
- expand/collapse subtrees;
- steer/cancel/pause a selected Agent subject to authority;
- inspect child evidence without importing entire child transcripts;
- see which Agents are running in parallel;
- receive concise completion/error badges without chat spam.

Large trees must be virtualized and filterable.

---

# 11. Model and scheduler UX

The Cognitive Scheduler stays authoritative, but its decisions become understandable.

The header/inspector can show:

- selected model/provider;
- routing objective;
- effort/quality policy;
- context occupancy;
- estimated/actual cost;
- fallback/circuit status;
- privacy/locality constraints.

Users can request a model/objective override only through policies the scheduler allows. The UI never silently weakens hard eligibility constraints.

---

# 12. Transaction UX

G7 transaction state is visible without requiring logs.

Example status progression:

```text
OPEN → VERIFYING → READY_TO_COMMIT → COMMITTING → COMMITTED
                                  ↘ ROLLING_BACK → ROLLED_BACK
                                  ↘ NEEDS_RECONCILIATION
```

The UI provides:

- clear transaction banner/state;
- speculative change count;
- verification checks and results;
- target-divergence/conflict presentation;
- commit / rollback controls when valid;
- prominent reconciliation state when outcome certainty is unknown;
- no wording that implies an irreversible effect was rolled back when it was not.

---

# 13. Cognitive Fork UX

Once G8 exists, forks become a visual comparison workflow rather than hidden runtime IDs.

The TUI should support:

- create fork from eligible checkpoint;
- show branch tree;
- label alternatives;
- view independent progress/cost/status;
- compare diffs, tests, benchmark scores and evaluator results;
- inspect branch-local cognitive changes;
- promote selected winner;
- discard/retain losing branches according to retention policy.

A two-column fork comparison is preferred when width permits.

---

# 14. Context / Cognitive MMU inspector

The TUI exposes enough context state to make model behavior debuggable without dumping hidden reasoning.

Show:

- active context budget and occupancy;
- working-set pages by tier/source;
- pinned/leased pages;
- compaction events;
- recalls;
- Context Faults;
- source/provenance identifiers;
- why an item is hot when that reason is available from runtime metadata.

Users can request recall/pin/unpin through runtime APIs. The TUI never directly edits canonical MMU state.

---

# 15. Authority and approval UX

Security should feel understandable rather than obstructive.

Approval surfaces show:

```text
what action is requested
why the Agent says it needs it
Effect class
scope/path/host/command
World guarantee
current capability/lease
risk/reversibility information
```

Keyboard actions should support deny, approve once, and other runtime-supported scoped grants without presenting fake guarantees.

Security denials remain visible in causal history but are compact in the normal conversation view.

---

# 16. History tree, bookmarks and search

Durable history should be navigable like a timeline/tree, not only scrollback.

Required:

- full-text search over projected conversation blocks;
- jump to checkpoints, transactions, forks, failures and approvals;
- bookmarks/labels;
- tree navigation for forked timelines;
- export/share adapters where policy permits;
- current-live-position shortcut.

The TUI fetches ranges lazily. Searching a month-old Agent cannot require rendering a month of history.

---

# 17. Themes and visual polish

The default theme should look intentionally designed, not like a generic Bubble Tea demo.

Requirements:

- excellent dark default;
- light theme;
- truecolor with 256/16-color fallbacks;
- semantic color tokens rather than hard-coded colors;
- clear typography hierarchy using terminal weight/dim/underline sparingly;
- status glyphs that also have textual meaning;
- user themes via declarative config;
- configurable border density and animation/spinner behavior;
- accessibility: no critical state communicated by color alone.

Optional terminal capabilities such as images or advanced hyperlinks may be detected and used progressively, never required.

---

# 18. Extensibility

G13 should preserve Pi-like customizability without embedding an unsafe in-process plugin free-for-all into the kernel.

Extension points may include:

- themes;
- keymaps;
- commands;
- prompt templates;
- skills;
- inspector panels/widgets;
- status-line segments;
- external/RPC extensions;
- MCP-backed commands where authority permits.

All extensions remain clients of explicit runtime APIs. An extension cannot mint authority, bypass Effect classification or read secrets merely because it can render UI.

---

# 19. Headless parity

Everything important in the TUI must correspond to an explicit runtime/client protocol operation.

The same system remains usable through:

```text
go-agentctl
JSON/event streaming
RPC/IPC clients
future IDE client
future web client
```

No critical Agent operation may exist only as an untestable TUI callback.

---

# 20. Performance budgets

The TUI is a realtime client and receives explicit product performance gates.

Reference targets on a normal developer machine:

```text
keypress → visible update p95          < 50 ms
stream coalescing/render target        20–30 updates/s
idle CPU while attached                near-zero / < 1% typical
100k-block local session attach        < 500 ms target
resize of normal viewport p95          < 50 ms
history memory                         bounded by viewport/cache, not age
```

These are initial engineering targets and may be calibrated empirically, but regressions must be benchmarked rather than accepted subjectively.

No full-history Markdown parse, full process-tree polling or unbounded presentation queue is allowed.

---

# 21. Terminal/platform matrix

G13 must be tested on:

```text
macOS:   Ghostty, iTerm2, Terminal
Linux:   Ghostty/Kitty, common xterm-compatible terminals, tmux
Windows: Windows Terminal / PowerShell, WSL where applicable
```

Required correctness includes:

- Unicode/wide glyph layout;
- resize;
- copy/select behavior;
- mouse optionality;
- bracketed paste;
- keyboard escape sequences;
- truecolor fallback;
- terminal disconnect/reconnect.

---

# 22. Killer demonstrations

## TUI-001 — crash-safe attach

Start a long Agent task, kill the TUI abruptly, let tools/subagents continue, relaunch and reattach to the exact current state without replaying full history.

## TUI-002 — large-history responsiveness

Open a synthetic 100,000-block session with multi-GB referenced artifacts. Attach, resize, search and scroll while memory/render cost remains bounded by the active viewport/cache.

## TUI-003 — parallel agent cockpit

Run at least three child Agents concurrently, switch focus, inspect their current model/tool/budget state, steer one Agent and queue a follow-up to another without interrupting unrelated work.

## TUI-004 — plan → transaction → review

Create a multi-file change: review/comment/rewrite the plan, approve execution, inspect streaming tools, review the WorkspaceWorld diff, run verification and commit through the G7 transaction UI.

## TUI-005 — uncertain outcome honesty

Force a transaction into `NEEDS_RECONCILIATION`. The UI must prominently preserve uncertainty and must never offer or display a false successful rollback/commit state.

## TUI-006 — fork comparison

After G8, run two candidate solutions in forks, compare tests/benchmarks/diffs side by side and promote only the winner without branch-state leakage.

## TUI-007 — no-mouse full workflow

Complete create/attach/plan/review/approve/commit/detach operations using only the keyboard.

---

# 23. Completion criteria

G13 is complete only when:

```text
production fullscreen TUI exists
keyboard-first workflow is complete
mouse interactions are optional enhancements
conversation/history is virtualized
plan review is first-class
diff review is first-class
Agent tree supports real parallel work
G7 transactions are fully operable/inspectable
G8 forks are visually comparable when available
MMU/context and authority are inspectable
model/scheduler state is understandable
themes/keymaps/commands are configurable
TUI crash does not affect runtime work
100k-block benchmark stays responsive/bounded
macOS/Linux/Windows terminal matrix passes
headless/runtime API parity is preserved
```

---

# Core product invariant

> **The best coding-agent TUI should make autonomy legible, steerable and pleasant without making the interface the owner of the autonomous process.**
