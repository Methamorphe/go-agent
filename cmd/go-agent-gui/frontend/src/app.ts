import { shortID, WorkspaceController, type Notice } from "./controller";
import { escapeHTML, formatCompact, formatMoneyMicros, formatRelative, formatTime, percent, statusTone } from "./format";
import { icon, type IconName } from "./icons";
import { applyPreferences, loadPreferences, savePreferences, type Preferences } from "./preferences";
import { decodeDiff, decodePlan, reviewReference } from "./review";
import { WorkspaceStore } from "./store";
import type { Block, Mode, ProcessSummary, QueueKind, TransactionSummary, WorkspaceState } from "./types";
import { VirtualFeed } from "./virtual-feed";

type InspectorTab = "overview" | "review" | "operations" | "runtime";
interface CommandItem { id: string; label: string; detail: string; keys?: string; run: () => void | Promise<void> }

export class DesktopApp {
  private preferences: Preferences = loadPreferences();
  private feed: VirtualFeed | null = null;
  private unsubscribeStore: (() => void) | null = null;
  private unsubscribeNotices: (() => void) | null = null;
  private inspectorTab: InspectorTab = "overview";
  private queue: QueueKind = "steer";
  private searchTimer = 0;
  private paletteItems: CommandItem[] = [];
  private paletteIndex = 0;
  private confirmation: ((accepted: boolean) => void) | null = null;
  private toastCounter = 0;

  constructor(
    private readonly root: HTMLElement,
    private readonly controller: WorkspaceController,
    private readonly store: WorkspaceStore,
  ) {}

  mount(): void {
    applyPreferences(this.preferences);
    this.root.innerHTML = this.shell();
    this.root.classList.add("app-root");

    this.feed = new VirtualFeed(
      this.element("feed-scroll"),
      this.element("feed-top-spacer"),
      this.element("feed-content"),
      this.element("feed-bottom-spacer"),
      (block) => this.renderBlock(block),
    );

    this.root.addEventListener("click", this.handleClick);
    this.root.addEventListener("input", this.handleInput);
    this.root.addEventListener("change", this.handleChange);
    this.root.addEventListener("submit", this.handleSubmit);
    this.root.addEventListener("keydown", this.handleLocalKeydown);
    document.addEventListener("keydown", this.handleGlobalKeydown);

    this.unsubscribeStore = this.store.subscribe((state) => this.renderState(state));
    this.unsubscribeNotices = this.controller.onNotice((notice) => this.toast(notice));
    this.syncLayout();
  }

  unmount(): void {
    this.unsubscribeStore?.();
    this.unsubscribeNotices?.();
    this.root.removeEventListener("click", this.handleClick);
    this.root.removeEventListener("input", this.handleInput);
    this.root.removeEventListener("change", this.handleChange);
    this.root.removeEventListener("submit", this.handleSubmit);
    this.root.removeEventListener("keydown", this.handleLocalKeydown);
    document.removeEventListener("keydown", this.handleGlobalKeydown);
    if (this.searchTimer) window.clearTimeout(this.searchTimer);
  }

  private shell(): string {
    return `
      <div class="shell" id="shell">
        <aside class="sidebar" id="sidebar" aria-label="Agent navigation">
          <div class="brand-row">
            <div class="brand-mark" aria-hidden="true">${icon("spark", 17)}</div>
            <div class="brand-copy"><strong>GO Agent</strong><span>durable workspace</span></div>
            <button class="icon-button sidebar-close" type="button" data-action="toggle-sidebar" aria-label="Close navigation">${icon("x")}</button>
          </div>
          <div class="sidebar-section-heading"><span>Agents</span><button class="icon-button" type="button" data-action="refresh" aria-label="Refresh agents">${icon("refresh", 14)}</button></div>
          <div id="agent-tree" class="agent-tree" role="tree"></div>
          <div class="sidebar-spacer"></div>
          <div class="sidebar-footer">
            <button class="sidebar-utility" type="button" data-action="open-search">${icon("search", 15)}<span>Search history</span><kbd>⌘P</kbd></button>
            <button class="sidebar-utility" type="button" data-action="open-settings">${icon("settings", 15)}<span>Preferences</span></button>
            <div class="connection-mini" id="connection-mini"><span class="status-dot neutral"></span><span>Connecting</span></div>
          </div>
        </aside>

        <main class="workspace" id="workspace">
          <header class="topbar">
            <div class="topbar-leading">
              <button class="icon-button sidebar-open" type="button" data-action="toggle-sidebar" aria-label="Toggle navigation">${icon("menu")}</button>
              <div class="workspace-identity" id="workspace-identity"><strong>Agent workspace</strong><span>Connecting to local runtime…</span></div>
            </div>
            <div class="mode-switcher" id="mode-switcher" aria-label="Interaction mode">
              ${(["ASK", "PLAN", "ACT", "REVIEW", "OBSERVE"] as Mode[]).map((mode) => `<button type="button" data-mode="${mode}" class="mode-button">${mode}</button>`).join("")}
            </div>
            <div class="topbar-actions">
              <button class="icon-button" type="button" data-action="open-search" aria-label="Search history" title="Search history (⌘P)">${icon("search")}</button>
              <button class="icon-button" type="button" data-action="open-palette" aria-label="Command palette" title="Command palette (⌘K)">${icon("command")}</button>
              <button class="icon-button inspector-toggle" type="button" data-action="toggle-inspector" aria-label="Toggle inspector" title="Toggle inspector">${icon("context")}</button>
            </div>
          </header>

          <div id="connection-banner" class="connection-banner" hidden></div>

          <section class="feed-frame" aria-label="Agent activity">
            <div class="feed-scroll" id="feed-scroll" tabindex="0">
              <div class="feed-header">
                <button id="load-older" class="text-button" type="button" data-action="load-older" hidden>${icon("history", 14)} Load older activity</button>
              </div>
              <div id="feed-empty" class="empty-state">
                <div class="empty-mark">${icon("spark", 20)}</div>
                <h1>Agent workspace</h1>
                <p>Durable activity, reviews and runtime state appear here when an Agent Process is available.</p>
                <button type="button" class="button secondary" data-action="refresh">Retry connection</button>
              </div>
              <div id="feed-top-spacer" aria-hidden="true"></div>
              <div id="feed-content" class="feed-content"></div>
              <div id="feed-bottom-spacer" aria-hidden="true"></div>
              <div id="live-slot"></div>
            </div>
            <button id="latest-activity" class="latest-activity" type="button" data-action="show-latest" hidden>${icon("arrow-up", 14)} Latest activity</button>
          </section>

          <section class="composer-wrap" aria-label="Message Agent">
            <form id="composer-form" class="composer">
              <textarea id="composer-input" rows="1" maxlength="65536" placeholder="Steer the agent…" aria-label="Message the focused agent"></textarea>
              <div class="composer-footer">
                <div class="queue-switch" role="group" aria-label="Message queue">
                  <button type="button" class="queue-button selected" data-queue="steer">Steer</button>
                  <button type="button" class="queue-button" data-queue="follow_up">Follow-up</button>
                </div>
                <div class="composer-hints"><span id="composer-status">Enter to send · Shift+Enter for newline</span></div>
                <button id="send-button" class="send-button" type="submit" aria-label="Send message">${icon("arrow-up", 17)}</button>
              </div>
            </form>
          </section>
        </main>

        <aside class="inspector" id="inspector" aria-label="Runtime inspector">
          <div class="inspector-header">
            <div><strong>Inspector</strong><span id="inspector-subtitle">current Agent</span></div>
            <button class="icon-button inspector-close" type="button" data-action="toggle-inspector" aria-label="Close inspector">${icon("x")}</button>
          </div>
          <div class="inspector-tabs" role="tablist">
            <button type="button" role="tab" data-inspector-tab="overview">Overview</button>
            <button type="button" role="tab" data-inspector-tab="review">Review</button>
            <button type="button" role="tab" data-inspector-tab="operations">Ops</button>
            <button type="button" role="tab" data-inspector-tab="runtime">Runtime</button>
          </div>
          <div id="inspector-body" class="inspector-body"></div>
        </aside>
        <button class="mobile-scrim" id="mobile-scrim" type="button" data-action="close-overlays" aria-label="Close panels"></button>
      </div>

      <dialog id="palette-dialog" class="command-dialog">
        <div class="dialog-search">${icon("command", 16)}<input id="palette-input" autocomplete="off" placeholder="Type a command…" aria-label="Command search"><kbd>esc</kbd></div>
        <div id="palette-results" class="command-results" role="listbox"></div>
      </dialog>

      <dialog id="search-dialog" class="search-dialog">
        <div class="dialog-title"><div><strong>Search durable history</strong><span>Search stays server-side and returns a bounded result set.</span></div><button class="icon-button" type="button" data-action="close-search">${icon("x")}</button></div>
        <label class="search-field">${icon("search", 16)}<input id="history-search-input" autocomplete="off" placeholder="Search events, tools, artifacts…"></label>
        <div id="history-search-results" class="search-results"><div class="dialog-empty">Start typing to search this root Agent.</div></div>
      </dialog>

      <dialog id="settings-dialog" class="settings-dialog">
        <div class="dialog-title"><div><strong>Preferences</strong><span>Presentation only — never runtime policy.</span></div><button class="icon-button" type="button" data-action="close-settings">${icon("x")}</button></div>
        <div class="settings-list">
          <label><span><strong>Appearance</strong><small>Follow the OS or choose a theme.</small></span><select id="theme-setting"><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select></label>
          <label><span><strong>Density</strong><small>Adjust information density without hiding state.</small></span><select id="density-setting"><option value="comfortable">Comfortable</option><option value="compact">Compact</option></select></label>
          <label><span><strong>Motion</strong><small>Reduced mode removes non-essential transitions.</small></span><select id="motion-setting"><option value="system">System preference</option><option value="reduced">Reduced</option></select></label>
        </div>
      </dialog>

      <dialog id="verify-dialog" class="form-dialog">
        <form id="verify-form">
          <div class="dialog-title"><div><strong>Verify transaction</strong><span>Runs inside the isolated transaction World before promotion.</span></div><button class="icon-button" type="button" data-action="close-verify">${icon("x")}</button></div>
          <input type="hidden" id="verify-transaction-id">
          <label class="field"><span>Executable</span><input id="verify-executable" required placeholder="go"></label>
          <label class="field"><span>Arguments <small>one argument per line</small></span><textarea id="verify-arguments" rows="5" placeholder="test\n./..."></textarea></label>
          <label class="field"><span>Timeout</span><select id="verify-timeout"><option value="120000">2 minutes</option><option value="300000">5 minutes</option><option value="600000">10 minutes</option></select></label>
          <div class="dialog-actions"><button class="button secondary" type="button" data-action="close-verify">Cancel</button><button class="button primary" type="submit">Run verification</button></div>
        </form>
      </dialog>

      <dialog id="resolve-dialog" class="form-dialog">
        <form id="resolve-form">
          <div class="dialog-title"><div><strong>Resolve uncertain effect</strong><span>This records evidence; reconciliation remains explicit.</span></div><button class="icon-button" type="button" data-action="close-resolve">${icon("x")}</button></div>
          <input type="hidden" id="resolve-transaction-id">
          <label class="field"><span>Effect record ID</span><input id="resolve-effect-id" required placeholder="effect_…"></label>
          <label class="field"><span>Observed outcome</span><select id="resolve-certainty"><option value="known_applied">Known applied</option><option value="known_absent">Known absent</option><option value="unknown">Still unknown</option></select></label>
          <label class="field"><span>Evidence</span><textarea id="resolve-evidence" rows="4" placeholder="What was inspected to establish this outcome?"></textarea></label>
          <div class="dialog-actions"><button class="button secondary" type="button" data-action="close-resolve">Cancel</button><button class="button primary" type="submit">Record outcome</button></div>
        </form>
      </dialog>

      <dialog id="confirm-dialog" class="confirm-dialog">
        <div class="confirm-mark">${icon("warning", 20)}</div>
        <div><strong id="confirm-title">Confirm action</strong><p id="confirm-body"></p></div>
        <div class="dialog-actions"><button class="button secondary" type="button" data-action="confirm-cancel">Cancel</button><button class="button primary" id="confirm-ok" type="button" data-action="confirm-ok">Continue</button></div>
      </dialog>

      <div id="toast-region" class="toast-region" aria-live="polite" aria-atomic="false"></div>
    `;
  }

  private renderState(state: Readonly<WorkspaceState>): void {
    this.renderConnection(state);
    this.renderIdentity(state);
    this.renderAgents(state);
    this.renderModes(state);
    this.renderComposer(state);
    this.renderLive(state);
    this.renderInspector(state);
    this.feed?.setBlocks(state.blocks);

    const empty = this.element("feed-empty");
    empty.hidden = state.blocks.length > 0 || Boolean(state.live?.data);
    const older = this.element<HTMLButtonElement>("load-older");
    older.hidden = !state.hasPrevious;
    older.disabled = state.busy.has("history");
    older.innerHTML = state.busy.has("history") ? `${icon("refresh", 14)} Loading…` : `${icon("history", 14)} Load older activity`;
  }

  private renderConnection(state: Readonly<WorkspaceState>): void {
    const mini = this.element("connection-mini");
    const tone = state.connection === "online" ? "ok" : state.connection === "degraded" ? "warn" : state.connection === "offline" ? "danger" : "neutral";
    mini.innerHTML = `<span class="status-dot ${tone}"></span><span>${state.connection === "online" ? "Runtime connected" : state.connection === "booting" ? "Connecting" : escapeHTML(state.connection)}</span>`;
    const banner = this.element("connection-banner");
    if ((state.connection === "degraded" || state.connection === "offline") && state.error) {
      banner.hidden = false;
      banner.innerHTML = `${icon(state.connection === "offline" ? "error" : "warning", 15)}<span>${escapeHTML(state.error)}</span><button type="button" class="text-button" data-action="refresh">Retry</button>`;
    } else {
      banner.hidden = true;
      banner.innerHTML = "";
    }
  }

  private renderIdentity(state: Readonly<WorkspaceState>): void {
    const process = this.store.focusedProcess();
    const identity = this.element("workspace-identity");
    const goal = process?.goal || state.inspector?.authority.goal || "Agent workspace";
    identity.innerHTML = `<strong title="${escapeHTML(goal)}">${escapeHTML(goal)}</strong><span>${process ? `${escapeHTML(shortID(process.agent_id))} · ${escapeHTML(process.status.toLowerCase())}` : "No Agent Process selected"}</span>`;
    this.element("inspector-subtitle").textContent = process ? shortID(process.agent_id) : "current Agent";
  }

  private renderAgents(state: Readonly<WorkspaceState>): void {
    const tree = this.element("agent-tree");
    if (state.tree.length === 0) {
      tree.innerHTML = `<div class="sidebar-empty">No durable Agent Process is available yet.</div>`;
      return;
    }
    tree.innerHTML = state.tree.map((process) => this.agentRow(process, state.focusedAgentID)).join("");
  }

  private agentRow(process: ProcessSummary, focused: string): string {
    const active = process.agent_id === focused;
    const tone = statusTone(process.status);
    const label = process.depth === 0 ? (process.goal || "Root Agent") : (process.goal || `Child Agent ${shortID(process.agent_id, 6)}`);
    const waiting = process.waiting_reason ? `<span class="agent-wait">${escapeHTML(process.waiting_reason)}</span>` : "";
    return `<button type="button" role="treeitem" aria-selected="${active}" class="agent-row ${active ? "selected" : ""}" data-agent-id="${escapeHTML(process.agent_id)}" style="--agent-depth:${Math.min(process.depth, 7)}">
      <span class="tree-line" aria-hidden="true"></span><span class="status-dot ${tone}"></span>
      <span class="agent-copy"><strong>${escapeHTML(label)}</strong><small>${escapeHTML(shortID(process.agent_id, 7))}${waiting}</small></span>
      ${process.children ? `<span class="count">${process.children}</span>` : ""}
    </button>`;
  }

  private renderModes(state: Readonly<WorkspaceState>): void {
    for (const button of this.root.querySelectorAll<HTMLButtonElement>("[data-mode]")) {
      const selected = button.dataset.mode === state.mode;
      button.classList.toggle("selected", selected);
      button.setAttribute("aria-pressed", String(selected));
    }
  }

  private renderComposer(state: Readonly<WorkspaceState>): void {
    const process = this.store.focusedProcess();
    const input = this.element<HTMLTextAreaElement>("composer-input");
    const button = this.element<HTMLButtonElement>("send-button");
    const canSend = Boolean(state.focusedAgentID) && state.connection !== "offline";
    input.disabled = !canSend;
    button.disabled = !canSend || state.busy.has("send");
    input.placeholder = state.mode === "OBSERVE" ? "Steer the agent when needed…" : state.mode === "REVIEW" ? "Give review feedback…" : "Steer the agent…";
    const status = this.element("composer-status");
    if (process?.waiting_reason) status.textContent = `Waiting: ${process.waiting_reason}`;
    else if (state.busy.has("send")) status.textContent = "Queuing message…";
    else status.textContent = this.queue === "steer" ? "Enter to send · Shift+Enter for newline" : "Queued after the current run";

    for (const queueButton of this.root.querySelectorAll<HTMLButtonElement>("[data-queue]")) {
      const selected = queueButton.dataset.queue === this.queue;
      queueButton.classList.toggle("selected", selected);
      queueButton.setAttribute("aria-pressed", String(selected));
    }
  }

  private renderLive(state: Readonly<WorkspaceState>): void {
    const slot = this.element("live-slot");
    const live = state.live;
    if (!live || (!live.data && live.status !== "failed")) {
      slot.innerHTML = "";
      return;
    }
    const durable = live.response_ref && state.blocks.some((block) => block.kind === "AssistantMessage" && block.object_ref === live.response_ref);
    if (live.status === "completed" && durable) {
      slot.innerHTML = "";
      return;
    }
    const failed = live.status === "failed";
    slot.innerHTML = `<article class="activity-block assistant live-block ${failed ? "critical" : ""}">
      <header><span class="block-symbol">${icon(failed ? "error" : "spark", 15)}</span><strong>${failed ? "Generation failed" : "Agent"}</strong><span class="live-state">${failed ? "failed" : live.status === "running" ? "streaming" : "finishing"}${live.status === "running" ? '<i class="stream-dot"></i>' : ""}</span></header>
      <div class="message-body">${escapeHTML(failed ? (live.error || live.data) : live.data)}</div>
    </article>`;
  }

  private renderBlock(block: Block): string {
    const tone = block.critical ? " critical" : "";
    const role = block.kind === "UserMessage" ? "user" : block.kind === "AssistantMessage" ? "assistant" : "system";
    const symbol = blockIcon(block.kind);
    const label = blockLabel(block.kind);
    const time = formatTime(block.occurred_at);
    const objectRef = block.object_ref ? `<button class="object-ref" type="button" data-copy="${escapeHTML(block.object_ref)}" title="Copy reference">${escapeHTML(shortID(block.object_ref, 14))}</button>` : "";
    const review = block.kind === "Plan" || block.kind === "Diff" ? `<button class="block-action" type="button" data-review-block="${escapeHTML(block.id)}">Review ${icon("chevron-right", 13)}</button>` : "";
    const preview = block.preview || block.title || block.event_type || "Activity";
    return `<article class="activity-block ${role}${tone}" data-block-id="${escapeHTML(block.id)}" data-kind="${escapeHTML(block.kind)}">
      <header><span class="block-symbol">${icon(symbol, 15)}</span><strong>${escapeHTML(label)}</strong>${block.critical ? '<span class="critical-label">attention</span>' : ""}<time>${escapeHTML(time)}</time></header>
      <div class="${role === "system" ? "event-body" : "message-body"}">${escapeHTML(preview)}</div>
      ${(objectRef || review) ? `<footer>${objectRef}<span class="footer-spacer"></span>${review}</footer>` : ""}
    </article>`;
  }

  private renderInspector(state: Readonly<WorkspaceState>): void {
    for (const button of this.root.querySelectorAll<HTMLButtonElement>("[data-inspector-tab]")) {
      const selected = button.dataset.inspectorTab === this.inspectorTab;
      button.classList.toggle("selected", selected);
      button.setAttribute("aria-selected", String(selected));
    }
    const body = this.element("inspector-body");
    if (!state.inspector) {
      body.innerHTML = `<div class="inspector-empty">Inspector data is unavailable while the runtime is disconnected.</div>`;
      return;
    }
    switch (this.inspectorTab) {
      case "review": body.innerHTML = this.reviewInspector(state); break;
      case "operations": body.innerHTML = this.operationsInspector(state); break;
      case "runtime": body.innerHTML = this.runtimeInspector(state); break;
      default: body.innerHTML = this.overviewInspector(state); break;
    }
  }

  private overviewInspector(state: Readonly<WorkspaceState>): string {
    const inspector = state.inspector!;
    const process = this.store.focusedProcess();
    const scheduler = inspector.scheduler;
    const tokenPercent = percent(scheduler.spent_tokens + scheduler.reserved_tokens, scheduler.limit_tokens);
    const moneyPercent = percent(scheduler.spent_money_micros + scheduler.reserved_money_micros, scheduler.limit_money_micros);
    return `
      <section class="inspector-section">
        <div class="section-label">Agent</div>
        <dl class="key-values">
          <div><dt>Status</dt><dd><span class="status-dot ${statusTone(process?.status ?? "")}"></span>${escapeHTML(process?.status ?? "—")}</dd></div>
          <div><dt>Version</dt><dd>${process?.version ?? "—"}</dd></div>
          <div><dt>Children</dt><dd>${process?.children ?? 0}</dd></div>
          <div><dt>Mode</dt><dd>${escapeHTML(state.mode)}</dd></div>
        </dl>
      </section>
      <section class="inspector-section">
        <div class="section-heading"><span class="section-label">Model routing</span>${icon("activity", 14)}</div>
        <div class="model-line"><strong>${escapeHTML(scheduler.last_model || "No routed model yet")}</strong><span>${escapeHTML(scheduler.last_provider || "")}</span></div>
        ${scheduler.last_decision_id ? `<div class="muted-line">decision ${escapeHTML(shortID(scheduler.last_decision_id, 12))} · ${escapeHTML(formatRelative(scheduler.last_decision_at))}</div>` : ""}
      </section>
      <section class="inspector-section">
        <div class="section-label">Budget</div>
        ${budgetLine("Tokens", formatCompact(scheduler.spent_tokens), formatCompact(scheduler.limit_tokens), tokenPercent)}
        ${budgetLine("Spend", formatMoneyMicros(scheduler.spent_money_micros), formatMoneyMicros(scheduler.limit_money_micros), moneyPercent)}
        ${(scheduler.reserved_tokens || scheduler.reserved_money_micros) ? `<div class="muted-line">Reserved ${formatCompact(scheduler.reserved_tokens)} tokens · ${formatMoneyMicros(scheduler.reserved_money_micros)}</div>` : ""}
      </section>
      <section class="inspector-section">
        <div class="section-heading"><span class="section-label">Context</span>${icon("context", 14)}</div>
        <dl class="key-values">
          <div><dt>Pages</dt><dd>${inspector.context.page_count}</dd></div>
          <div><dt>Estimated tokens</dt><dd>${formatCompact(inspector.context.estimated_tokens)}</dd></div>
          <div><dt>Active leases</dt><dd>${inspector.context.active_lease_count}</dd></div>
          <div><dt>Open faults</dt><dd class="${inspector.context.unresolved_faults ? "warning-text" : ""}">${inspector.context.unresolved_faults}</dd></div>
        </dl>
      </section>
      <section class="inspector-section">
        <div class="section-heading"><span class="section-label">Authority</span>${icon("shield", 14)}</div>
        <p class="authority-goal">${escapeHTML(inspector.authority.goal || "No root Intent projected")}</p>
        <div class="muted-line">${escapeHTML(inspector.authority.capability_projection || "Runtime-controlled capability projection")}</div>
        ${inspector.authority.last_security_event ? `<div class="security-event">${icon("shield", 13)} ${escapeHTML(inspector.authority.last_security_event)}</div>` : ""}
      </section>`;
  }

  private reviewInspector(state: Readonly<WorkspaceState>): string {
    const planBlock = [...state.blocks].reverse().find((block) => block.kind === "Plan");
    const diffBlock = [...state.blocks].reverse().find((block) => block.kind === "Diff");
    if (!planBlock && !diffBlock) return `<div class="inspector-empty">No plan or diff is projected for this Agent yet.</div>`;
    let html = "";
    if (planBlock) {
      const plan = decodePlan(planBlock);
      html += `<section class="inspector-section review-section"><div class="section-heading"><span class="section-label">Latest plan</span><span class="review-ref">${escapeHTML(shortID(plan.objectRef || plan.blockID, 10))}</span></div><h3>${escapeHTML(plan.title)}</h3><ol class="plan-list">${plan.steps.map((step) => `<li><span class="step-state ${statusTone(step.state)}"></span><div><strong>${escapeHTML(step.title)}</strong>${step.detail ? `<p>${escapeHTML(step.detail)}</p>` : ""}<small>${escapeHTML(step.state)}</small></div></li>`).join("")}</ol>${reviewComposer(reviewReference("plan", planBlock))}</section>`;
    }
    if (diffBlock) {
      const diff = decodeDiff(diffBlock);
      html += `<section class="inspector-section review-section"><div class="section-heading"><span class="section-label">Latest diff</span><span class="review-ref">${escapeHTML(shortID(diff.objectRef || diff.blockID, 10))}</span></div><div class="diff-files">${diff.files.map((file) => `<details><summary>${icon("file", 13)}<span>${escapeHTML(file.path)}</span><em>+${file.additions} −${file.deletions}</em></summary>${file.hunks.length ? file.hunks.map((hunk) => `<div class="diff-hunk"><div class="hunk-header">${escapeHTML(hunk.header)}</div><pre>${renderDiff(hunk.body)}</pre></div>`).join("") : '<div class="muted-line padded">No inline hunk body in the bounded projection.</div>'}</details>`).join("")}</div>${reviewComposer(reviewReference("diff", diffBlock))}</section>`;
    }
    return html;
  }

  private operationsInspector(state: Readonly<WorkspaceState>): string {
    const inspector = state.inspector!;
    const transactions = inspector.transactions.length ? inspector.transactions.map((tx) => this.transactionRow(tx, state)).join("") : `<div class="inspector-empty compact">No active transaction.</div>`;
    const forks = inspector.forks.length ? inspector.forks.map((fork) => `<div class="fork-group"><div class="operation-title"><span>${icon("fork", 14)}<strong>${escapeHTML(shortID(fork.group_id, 11))}</strong></span><span class="status-text ${statusTone(fork.state)}">${escapeHTML(fork.state)}</span></div>${fork.branches.map((branch) => `<div class="branch-row ${branch.fork_id === fork.winner_fork_id ? "winner" : ""}"><span>${branch.fork_id === fork.winner_fork_id ? icon("check", 13) : icon("branch", 13)} ${escapeHTML(shortID(branch.fork_id, 9))}</span><span>${formatCompact(branch.spent_tokens)} tok</span><span>${formatMoneyMicros(branch.spent_money_micros)}</span></div>`).join("")}${fork.selection_reason ? `<p class="muted-line">${escapeHTML(fork.selection_reason)}</p>` : ""}</div>`).join("") : `<div class="inspector-empty compact">No cognitive forks.</div>`;
    return `<section class="inspector-section"><div class="section-heading"><span class="section-label">Transactions</span>${icon("diff", 14)}</div>${transactions}</section><section class="inspector-section"><div class="section-heading"><span class="section-label">Forks</span>${icon("fork", 14)}</div>${forks}</section>`;
  }

  private transactionRow(tx: TransactionSummary, state: Readonly<WorkspaceState>): string {
    const busy = state.busy.has(`tx:${tx.id}`);
    const reconcile = tx.state === "NEEDS_RECONCILIATION" || tx.uncertain_effects > 0;
    return `<div class="transaction-row ${reconcile ? "needs-reconcile" : ""}">
      <div class="operation-title"><span>${icon(reconcile ? "warning" : "diff", 14)}<strong>${escapeHTML(shortID(tx.id, 11))}</strong></span><span class="status-text ${statusTone(tx.state)}">${escapeHTML(tx.state)}</span></div>
      <div class="operation-meta"><span>${tx.effect_count} effects</span>${tx.verification_status ? `<span>${escapeHTML(tx.verification_status)}</span>` : ""}${tx.uncertain_effects ? `<span class="warning-text">${tx.uncertain_effects} uncertain</span>` : ""}</div>
      ${tx.reconcile_reason ? `<p class="reconcile-reason">${escapeHTML(tx.reconcile_reason)}</p>` : ""}
      <div class="operation-actions">
        <button class="mini-button" type="button" data-tx-verify="${escapeHTML(tx.id)}" ${busy ? "disabled" : ""}>Verify</button>
        <button class="mini-button" type="button" data-tx-operation="prepare" data-tx-id="${escapeHTML(tx.id)}" ${busy ? "disabled" : ""}>Prepare</button>
        <button class="mini-button primary" type="button" data-tx-operation="commit" data-tx-id="${escapeHTML(tx.id)}" ${busy || reconcile ? "disabled" : ""}>Commit</button>
        <button class="mini-button" type="button" data-tx-operation="rollback" data-tx-id="${escapeHTML(tx.id)}" ${busy ? "disabled" : ""}>Rollback</button>
        ${reconcile ? `<button class="mini-button warning" type="button" data-tx-operation="reconcile" data-tx-id="${escapeHTML(tx.id)}" ${busy ? "disabled" : ""}>Reconcile</button>` : ""}
        ${tx.uncertain_effects ? `<button class="mini-button warning" type="button" data-tx-resolve="${escapeHTML(tx.id)}" ${busy ? "disabled" : ""}>Resolve effect</button>` : ""}
      </div>
    </div>`;
  }

  private runtimeInspector(state: Readonly<WorkspaceState>): string {
    const inspector = state.inspector!;
    const faults = inspector.context_faults.length ? inspector.context_faults.map((fault) => `<div class="runtime-row"><span>${icon("warning", 13)}<strong>${escapeHTML(fault.kind)}</strong><small>${escapeHTML(shortID(fault.reference, 14))}</small></span><em class="status-text ${statusTone(fault.state)}">${escapeHTML(fault.state)}</em></div>`).join("") : `<div class="inspector-empty compact">No context faults.</div>`;
    const teams = inspector.teams.length ? inspector.teams.map((team) => `<div class="runtime-row"><span>${icon("users", 13)}<strong>${escapeHTML(team.objective || shortID(team.team_id, 10))}</strong><small>${team.member_count} members · round ${team.round}/${team.max_rounds}</small></span><em class="status-text ${statusTone(team.state)}">${escapeHTML(team.state)}</em></div>`).join("") : `<div class="inspector-empty compact">No adaptive team.</div>`;
    const improvements = inspector.improvements.length ? inspector.improvements.map((item) => `<div class="runtime-row"><span>${icon("spark", 13)}<strong>${escapeHTML(item.kind)} v${item.version}</strong><small>${escapeHTML(shortID(item.artifact_id, 13))} · ${escapeHTML(item.scope_level)}</small></span><em class="status-text ${statusTone(item.killed ? "failed" : item.status)}">${item.killed ? "KILLED" : escapeHTML(item.status)}</em></div>`).join("") : `<div class="inspector-empty compact">No verified improvement artifacts.</div>`;
    return `<section class="inspector-section"><div class="section-heading"><span class="section-label">Context faults</span><span>${inspector.context_faults.length}</span></div>${faults}</section><section class="inspector-section"><div class="section-heading"><span class="section-label">Teams</span><span>${inspector.teams.length}</span></div>${teams}</section><section class="inspector-section"><div class="section-heading"><span class="section-label">Verified improvements</span><span>${inspector.improvements.length}</span></div>${improvements}</section>`;
  }

  private handleClick = (event: MouseEvent): void => {
    const target = event.target instanceof Element ? event.target : null;
    if (!target) return;

    const agent = target.closest<HTMLElement>("[data-agent-id]");
    if (agent?.dataset.agentId) { void this.controller.focusAgent(agent.dataset.agentId); this.closeMobilePanels(); return; }

    const mode = target.closest<HTMLButtonElement>("[data-mode]");
    if (mode?.dataset.mode) { void this.controller.setMode(mode.dataset.mode as Mode); return; }

    const queue = target.closest<HTMLButtonElement>("[data-queue]");
    if (queue?.dataset.queue) { this.queue = queue.dataset.queue as QueueKind; this.renderComposer(this.store.value); return; }

    const tab = target.closest<HTMLButtonElement>("[data-inspector-tab]");
    if (tab?.dataset.inspectorTab) { this.inspectorTab = tab.dataset.inspectorTab as InspectorTab; this.renderInspector(this.store.value); return; }

    const copy = target.closest<HTMLButtonElement>("[data-copy]");
    if (copy?.dataset.copy) { void navigator.clipboard?.writeText(copy.dataset.copy); this.toast({ kind: "info", message: "Reference copied." }); return; }

    const reviewBlock = target.closest<HTMLButtonElement>("[data-review-block]");
    if (reviewBlock) { this.inspectorTab = "review"; this.preferences.inspector = true; this.persistPreferences(); this.renderInspector(this.store.value); return; }

    const txVerify = target.closest<HTMLButtonElement>("[data-tx-verify]");
    if (txVerify?.dataset.txVerify) { this.openVerify(txVerify.dataset.txVerify); return; }
    const txResolve = target.closest<HTMLButtonElement>("[data-tx-resolve]");
    if (txResolve?.dataset.txResolve) { this.openResolve(txResolve.dataset.txResolve); return; }
    const tx = target.closest<HTMLButtonElement>("[data-tx-operation]");
    if (tx?.dataset.txId && tx.dataset.txOperation) { void this.runTransaction(tx.dataset.txId, tx.dataset.txOperation as "prepare" | "commit" | "rollback" | "reconcile"); return; }

    const command = target.closest<HTMLButtonElement>("[data-command-id]");
    if (command?.dataset.commandId) { const item = this.paletteItems.find((candidate) => candidate.id === command.dataset.commandId); if (item) void this.executeCommand(item); return; }

    const searchResult = target.closest<HTMLButtonElement>("[data-search-block]");
    if (searchResult?.dataset.searchBlock) {
      if (this.feed?.scrollToBlock(searchResult.dataset.searchBlock)) this.closeDialog("search-dialog");
      else this.toast({ kind: "info", message: "That result is outside the bounded local history window. Refine the search or load older activity." });
      return;
    }

    const action = target.closest<HTMLElement>("[data-action]")?.dataset.action;
    if (!action) return;
    switch (action) {
      case "toggle-sidebar": this.preferences.sidebar = !this.preferences.sidebar; this.persistPreferences(); break;
      case "toggle-inspector": this.preferences.inspector = !this.preferences.inspector; this.persistPreferences(); break;
      case "close-overlays": this.closeMobilePanels(); break;
      case "refresh": void this.controller.refresh(true); break;
      case "load-older": void this.controller.loadOlder(); break;
      case "show-latest": this.feed?.showLatest(); break;
      case "open-palette": this.openPalette(); break;
      case "open-search": this.openSearch(); break;
      case "open-settings": this.openSettings(); break;
      case "close-search": this.closeDialog("search-dialog"); break;
      case "close-settings": this.closeDialog("settings-dialog"); break;
      case "close-verify": this.closeDialog("verify-dialog"); break;
      case "close-resolve": this.closeDialog("resolve-dialog"); break;
      case "confirm-ok": this.resolveConfirmation(true); break;
      case "confirm-cancel": this.resolveConfirmation(false); break;
      case "send-review": void this.sendReview(target.closest<HTMLElement>("[data-review-ref]")?.dataset.reviewRef ?? ""); break;
    }
  };

  private handleInput = (event: Event): void => {
    const target = event.target;
    if (!(target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement)) return;
    if (target.id === "composer-input" && target instanceof HTMLTextAreaElement) {
      target.style.height = "0px";
      target.style.height = `${Math.min(180, Math.max(42, target.scrollHeight))}px`;
    } else if (target.id === "palette-input") {
      this.renderPalette(target.value);
    } else if (target.id === "history-search-input") {
      if (this.searchTimer) window.clearTimeout(this.searchTimer);
      this.searchTimer = window.setTimeout(() => void this.performSearch(target.value), 180);
    }
  };

  private handleChange = (event: Event): void => {
    const target = event.target;
    if (!(target instanceof HTMLSelectElement)) return;
    if (target.id === "theme-setting") this.preferences.theme = target.value as Preferences["theme"];
    else if (target.id === "density-setting") this.preferences.density = target.value as Preferences["density"];
    else if (target.id === "motion-setting") this.preferences.motion = target.value as Preferences["motion"];
    else return;
    this.persistPreferences();
  };

  private handleSubmit = (event: SubmitEvent): void => {
    const form = event.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (form.id === "composer-form") { event.preventDefault(); void this.submitComposer(); }
    else if (form.id === "verify-form") { event.preventDefault(); void this.submitVerify(); }
    else if (form.id === "resolve-form") { event.preventDefault(); void this.submitResolve(); }
  };

  private handleLocalKeydown = (event: KeyboardEvent): void => {
    const target = event.target;
    if (target instanceof HTMLTextAreaElement && target.id === "composer-input" && event.key === "Enter" && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      void this.submitComposer();
    }
    if (target instanceof HTMLInputElement && target.id === "palette-input") {
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        const delta = event.key === "ArrowDown" ? 1 : -1;
        this.paletteIndex = Math.max(0, Math.min(this.paletteItems.length - 1, this.paletteIndex + delta));
        this.paintPaletteSelection();
      } else if (event.key === "Enter") {
        event.preventDefault();
        const item = this.paletteItems[this.paletteIndex];
        if (item) void this.executeCommand(item);
      }
    }
  };

  private handleGlobalKeydown = (event: KeyboardEvent): void => {
    const modifier = event.metaKey || event.ctrlKey;
    if (modifier && event.key.toLowerCase() === "k") { event.preventDefault(); this.openPalette(); }
    else if (modifier && event.key.toLowerCase() === "p") { event.preventDefault(); this.openSearch(); }
    else if (modifier && event.key.toLowerCase() === "b") { event.preventDefault(); this.preferences.sidebar = !this.preferences.sidebar; this.persistPreferences(); }
    else if (modifier && event.key.toLowerCase() === "i") { event.preventDefault(); this.preferences.inspector = !this.preferences.inspector; this.persistPreferences(); }
  };

  private async submitComposer(): Promise<void> {
    const input = this.element<HTMLTextAreaElement>("composer-input");
    const text = input.value.trim();
    if (!text) return;
    const sent = await this.controller.sendMessage(text, this.queue);
    if (sent) { input.value = ""; input.style.height = "42px"; input.focus(); this.feed?.showLatest(); }
  }

  private async sendReview(reference: string): Promise<void> {
    const textarea = this.root.querySelector<HTMLTextAreaElement>(`textarea[data-review-input="${CSS.escape(reference)}"]`);
    const text = textarea?.value.trim() ?? "";
    if (!text) return;
    if (await this.controller.sendMessage(`[review ${reference}] ${text}`, "steer")) textarea!.value = "";
  }

  private async runTransaction(id: string, operation: "prepare" | "commit" | "rollback" | "reconcile"): Promise<void> {
    if (operation === "commit") {
      const accepted = await this.confirm("Commit transaction?", "Promotion changes the target workspace. The daemon will revalidate current Intent and policy before committing.", "Commit");
      if (!accepted) return;
    } else if (operation === "rollback") {
      const accepted = await this.confirm("Rollback transaction?", "The isolated transaction branch will be rolled back. Unknown external outcomes remain explicit.", "Rollback");
      if (!accepted) return;
    }
    await this.controller.transaction(id, operation);
  }

  private openVerify(transactionID: string): void {
    this.element<HTMLInputElement>("verify-transaction-id").value = transactionID;
    this.element<HTMLInputElement>("verify-executable").value = "go";
    this.element<HTMLTextAreaElement>("verify-arguments").value = "test\n./...";
    this.openDialog("verify-dialog", "verify-executable");
  }

  private async submitVerify(): Promise<void> {
    const tx = this.element<HTMLInputElement>("verify-transaction-id").value;
    const executable = this.element<HTMLInputElement>("verify-executable").value.trim();
    const args = this.element<HTMLTextAreaElement>("verify-arguments").value.split(/\r?\n/).map((value) => value.trim()).filter(Boolean);
    const timeout = Number(this.element<HTMLSelectElement>("verify-timeout").value) || 120000;
    if (await this.controller.verifyTransaction(tx, [executable, ...args], timeout)) this.closeDialog("verify-dialog");
  }

  private openResolve(transactionID: string): void {
    this.element<HTMLInputElement>("resolve-transaction-id").value = transactionID;
    this.element<HTMLInputElement>("resolve-effect-id").value = "";
    this.element<HTMLTextAreaElement>("resolve-evidence").value = "";
    this.openDialog("resolve-dialog", "resolve-effect-id");
  }

  private async submitResolve(): Promise<void> {
    const tx = this.element<HTMLInputElement>("resolve-transaction-id").value;
    const effect = this.element<HTMLInputElement>("resolve-effect-id").value;
    const certainty = this.element<HTMLSelectElement>("resolve-certainty").value as "known_applied" | "known_absent" | "unknown";
    const evidence = this.element<HTMLTextAreaElement>("resolve-evidence").value;
    if (await this.controller.resolveEffect(tx, effect, certainty, evidence)) this.closeDialog("resolve-dialog");
  }

  private openPalette(): void {
    const input = this.element<HTMLInputElement>("palette-input");
    input.value = "";
    this.renderPalette("");
    this.openDialog("palette-dialog", "palette-input");
  }

  private renderPalette(query: string): void {
    const needle = query.trim().toLowerCase();
    const all = this.commands();
    this.paletteItems = needle ? all.filter((item) => `${item.label} ${item.detail}`.toLowerCase().includes(needle)) : all;
    this.paletteIndex = Math.min(this.paletteIndex, Math.max(0, this.paletteItems.length - 1));
    this.element("palette-results").innerHTML = this.paletteItems.length ? this.paletteItems.map((item, index) => `<button type="button" role="option" class="command-row ${index === this.paletteIndex ? "selected" : ""}" data-command-id="${escapeHTML(item.id)}"><span><strong>${escapeHTML(item.label)}</strong><small>${escapeHTML(item.detail)}</small></span>${item.keys ? `<kbd>${escapeHTML(item.keys)}</kbd>` : ""}</button>`).join("") : `<div class="dialog-empty">No matching command.</div>`;
  }

  private paintPaletteSelection(): void {
    this.root.querySelectorAll<HTMLElement>("[data-command-id]").forEach((row, index) => row.classList.toggle("selected", index === this.paletteIndex));
    this.root.querySelector<HTMLElement>("[data-command-id].selected")?.scrollIntoView({ block: "nearest" });
  }

  private commands(): CommandItem[] {
    const process = this.store.focusedProcess();
    const suspended = process?.status.toLowerCase().includes("suspend") ?? false;
    const modes: Mode[] = ["ASK", "PLAN", "ACT", "REVIEW", "OBSERVE"];
    return [
      { id: "search", label: "Search durable history", detail: "Search the current root Agent without growing the local cache", keys: "⌘P", run: () => this.openSearch() },
      { id: "refresh", label: "Refresh workspace", detail: "Request a fresh bounded runtime projection", run: () => this.controller.refresh(true) },
      { id: "sidebar", label: "Toggle navigation", detail: "Show or hide the Agent tree", keys: "⌘B", run: () => { this.preferences.sidebar = !this.preferences.sidebar; this.persistPreferences(); } },
      { id: "inspector", label: "Toggle inspector", detail: "Show or hide runtime context", keys: "⌘I", run: () => { this.preferences.inspector = !this.preferences.inspector; this.persistPreferences(); } },
      { id: suspended ? "resume" : "suspend", label: suspended ? "Resume focused Agent" : "Suspend focused Agent", detail: "Runtime transition with optimistic version checking", run: () => suspended ? this.controller.resume() : this.controller.suspend() },
      { id: "review", label: "Open review inspector", detail: "Inspect the latest plan and diff", run: () => { this.inspectorTab = "review"; this.preferences.inspector = true; this.persistPreferences(); this.renderInspector(this.store.value); } },
      { id: "older", label: "Load older activity", detail: "Page older history into the bounded client window", run: () => this.controller.loadOlder() },
      { id: "settings", label: "Open preferences", detail: "Theme, density and reduced motion", run: () => this.openSettings() },
      ...modes.map((mode) => ({ id: `mode-${mode}`, label: `Switch to ${mode}`, detail: "Presentation mode; canonical state remains in the daemon", run: () => this.controller.setMode(mode) })),
    ];
  }

  private async executeCommand(item: CommandItem): Promise<void> {
    this.closeDialog("palette-dialog");
    await item.run();
  }

  private openSearch(): void {
    this.element<HTMLInputElement>("history-search-input").value = "";
    this.element("history-search-results").innerHTML = `<div class="dialog-empty">Start typing to search this root Agent.</div>`;
    this.openDialog("search-dialog", "history-search-input");
  }

  private async performSearch(query: string): Promise<void> {
    const results = this.element("history-search-results");
    if (!query.trim()) { results.innerHTML = `<div class="dialog-empty">Start typing to search this root Agent.</div>`; return; }
    results.innerHTML = `<div class="dialog-empty">Searching…</div>`;
    const blocks = await this.controller.search(query);
    results.innerHTML = blocks.length ? blocks.map((block) => `<button type="button" class="search-result" data-search-block="${escapeHTML(block.id)}"><span class="search-result-icon">${icon(blockIcon(block.kind), 14)}</span><span><strong>${escapeHTML(blockLabel(block.kind))}</strong><small>${escapeHTML(block.preview || block.title || block.event_type || "")}</small></span><time>${escapeHTML(formatTime(block.occurred_at))}</time></button>`).join("") : `<div class="dialog-empty">No matching durable events.</div>`;
  }

  private openSettings(): void {
    this.element<HTMLSelectElement>("theme-setting").value = this.preferences.theme;
    this.element<HTMLSelectElement>("density-setting").value = this.preferences.density;
    this.element<HTMLSelectElement>("motion-setting").value = this.preferences.motion;
    this.openDialog("settings-dialog", "theme-setting");
  }

  private persistPreferences(): void {
    savePreferences(this.preferences);
    this.syncLayout();
  }

  private syncLayout(): void {
    const shell = this.root.querySelector<HTMLElement>("#shell");
    if (!shell) return;
    shell.classList.toggle("sidebar-hidden", !this.preferences.sidebar);
    shell.classList.toggle("inspector-hidden", !this.preferences.inspector);
  }

  private closeMobilePanels(): void {
    if (window.matchMedia("(max-width: 940px)").matches) {
      this.preferences.sidebar = false;
      this.preferences.inspector = false;
      this.persistPreferences();
    }
  }

  private openDialog(id: string, focusID?: string): void {
    const dialog = this.element<HTMLDialogElement>(id);
    if (!dialog.open) dialog.showModal();
    if (focusID) requestAnimationFrame(() => this.element<HTMLElement>(focusID).focus());
  }

  private closeDialog(id: string): void {
    const dialog = this.element<HTMLDialogElement>(id);
    if (dialog.open) dialog.close();
  }

  private confirm(title: string, body: string, action: string): Promise<boolean> {
    this.element("confirm-title").textContent = title;
    this.element("confirm-body").textContent = body;
    this.element("confirm-ok").textContent = action;
    this.openDialog("confirm-dialog", "confirm-ok");
    return new Promise<boolean>((resolve) => { this.confirmation = resolve; });
  }

  private resolveConfirmation(accepted: boolean): void {
    this.closeDialog("confirm-dialog");
    const resolve = this.confirmation;
    this.confirmation = null;
    resolve?.(accepted);
  }

  private toast(notice: Notice): void {
    const region = this.element("toast-region");
    const id = `toast-${++this.toastCounter}`;
    const symbol = notice.kind === "error" ? "error" : notice.kind === "warning" ? "warning" : notice.kind === "success" ? "check" : "activity";
    region.insertAdjacentHTML("beforeend", `<div id="${id}" class="toast ${notice.kind}" role="status">${icon(symbol, 14)}<span>${escapeHTML(notice.message)}</span><button type="button" aria-label="Dismiss">${icon("x", 12)}</button></div>`);
    const toast = this.element(id);
    toast.querySelector("button")?.addEventListener("click", () => toast.remove(), { once: true });
    window.setTimeout(() => { toast.classList.add("leaving"); window.setTimeout(() => toast.remove(), 180); }, notice.kind === "error" ? 6500 : 3600);
  }

  private element<T extends HTMLElement = HTMLElement>(id: string): T {
    const element = this.root.querySelector<T>(`#${CSS.escape(id)}`);
    if (!element) throw new Error(`Missing UI element #${id}`);
    return element;
  }
}

function blockIcon(kind: Block["kind"]): IconName {
  switch (kind) {
    case "AssistantMessage": return "spark";
    case "UserMessage": return "agent";
    case "ToolCall": case "ToolResult": return "terminal";
    case "Plan": return "activity";
    case "Diff": return "diff";
    case "TestResult": return "check";
    case "Artifact": return "file";
    case "ChildAgent": return "users";
    case "Transaction": return "diff";
    case "ForkComparison": return "fork";
    case "ContextFault": return "warning";
    case "SecurityDecision": return "shield";
    case "Error": return "error";
    default: return "activity";
  }
}

function blockLabel(kind: Block["kind"]): string {
  switch (kind) {
    case "AssistantMessage": return "Agent";
    case "UserMessage": return "You";
    case "ToolCall": return "Tool call";
    case "ToolResult": return "Tool result";
    case "ProgressUpdate": return "Progress";
    case "Plan": return "Plan";
    case "Diff": return "Workspace diff";
    case "TestResult": return "Verification";
    case "Artifact": return "Artifact";
    case "Approval": return "Approval";
    case "ChildAgent": return "Child Agent";
    case "Transaction": return "Transaction";
    case "ForkComparison": return "Fork comparison";
    case "ContextFault": return "Context fault";
    case "SecurityDecision": return "Security decision";
    case "Checkpoint": return "Checkpoint";
    case "Error": return "Error";
    default: return "Runtime";
  }
}

function budgetLine(label: string, used: string, limit: string, value: number): string {
  return `<div class="budget-line"><div><span>${escapeHTML(label)}</span><span>${escapeHTML(used)} / ${escapeHTML(limit)}</span></div><div class="budget-track"><i style="width:${value.toFixed(1)}%"></i></div></div>`;
}

function reviewComposer(reference: string): string {
  return `<div class="review-composer"><textarea rows="2" data-review-input="${escapeHTML(reference)}" placeholder="Targeted feedback for this review…"></textarea><button class="mini-button primary" type="button" data-action="send-review" data-review-ref="${escapeHTML(reference)}">Send feedback</button></div>`;
}

function renderDiff(body: string): string {
  return body.split("\n").map((line) => {
    const className = line.startsWith("+") && !line.startsWith("+++") ? "add" : line.startsWith("-") && !line.startsWith("---") ? "remove" : line.startsWith("@@") ? "meta" : "";
    return `<span class="${className}">${escapeHTML(line)}</span>`;
  }).join("\n");
}
