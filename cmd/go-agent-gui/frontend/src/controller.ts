import { DEFAULT_HISTORY_PAGE, MAX_RETAINED_BLOCKS, WorkspaceStore } from "./store";
import { errorMessage } from "./runtime";
import type { Mode, QueueKind, RuntimeClient } from "./types";

export type NoticeKind = "info" | "success" | "warning" | "error";
export interface Notice { kind: NoticeKind; message: string }

export class WorkspaceController {
  private refreshTimer = 0;
  private liveTimer = 0;
  private refreshBusy = false;
  private liveBusy = false;
  private refreshCount = 0;
  private stopped = false;
  private consecutiveErrors = 0;
  private noticeListeners = new Set<(notice: Notice) => void>();

  constructor(readonly runtime: RuntimeClient, readonly store: WorkspaceStore) {}

  onNotice(listener: (notice: Notice) => void): () => void {
    this.noticeListeners.add(listener);
    return () => this.noticeListeners.delete(listener);
  }

  private notice(kind: NoticeKind, message: string): void {
    for (const listener of this.noticeListeners) listener({ kind, message });
  }

  async start(): Promise<void> {
    this.stopped = false;
    this.store.setConnection("booting");
    try {
      const bootstrap = await this.runtime.bootstrap();
      this.store.setBootstrap(bootstrap);
      await this.attach(bootstrap.launch.agent_id, bootstrap.launch.mode ?? "ACT", false);
      const refreshMS = Math.max(80, bootstrap.recommended_refresh_ms || 160);
      const liveMS = Math.max(50, bootstrap.recommended_live_refresh_ms || 80);
      this.refreshTimer = window.setInterval(() => void this.refresh(), refreshMS);
      this.liveTimer = window.setInterval(() => void this.refreshLive(), liveMS);
    } catch (error) {
      this.store.setConnection("offline", errorMessage(error));
    }
  }

  stop(): void {
    this.stopped = true;
    if (this.refreshTimer) window.clearInterval(this.refreshTimer);
    if (this.liveTimer) window.clearInterval(this.liveTimer);
    this.refreshTimer = 0;
    this.liveTimer = 0;
  }

  async attach(agentID?: string, mode: Mode = this.store.value.mode, announce = true): Promise<void> {
    this.store.setBusy("attach", true);
    try {
      const snapshot = await this.runtime.attach({ agent_id: agentID || undefined, mode, history_limit: DEFAULT_HISTORY_PAGE, tree_limit: 512 });
      this.store.applySnapshot(snapshot);
      this.consecutiveErrors = 0;
      if (announce) this.notice("success", `Focused ${shortID(snapshot.focused_agent_id)}`);
      await this.refreshLive();
    } catch (error) {
      const message = errorMessage(error);
      this.store.setConnection("offline", message);
      if (announce) this.notice("error", message);
      throw error;
    } finally {
      this.store.setBusy("attach", false);
    }
  }

  async setMode(mode: Mode): Promise<void> {
    if (this.store.value.mode === mode) return;
    this.store.setMode(mode);
    if (this.store.value.focusedAgentID) await this.attach(this.store.value.focusedAgentID, mode, false);
  }

  async focusAgent(agentID: string): Promise<void> {
    if (!agentID || agentID === this.store.value.focusedAgentID) return;
    await this.attach(agentID, this.store.value.mode);
  }

  async refresh(forceInspector = false): Promise<void> {
    if (this.stopped || this.refreshBusy) return;
    const state = this.store.value;
    if (!state.rootAgentID || !state.focusedAgentID) return;
    this.refreshBusy = true;
    try {
      this.refreshCount += 1;
      const refresh = await this.runtime.refresh({
        root_agent_id: state.rootAgentID,
        focused_agent_id: state.focusedAgentID,
        after_sequence: state.cursor,
        limit: 320,
        inspector: forceInspector || this.refreshCount % 4 === 0,
      });
      if (refresh.behind) {
        await this.attach(state.focusedAgentID, state.mode, false);
        this.notice("warning", "Workspace projection resynchronised after falling behind.");
      } else {
        this.store.applyRefresh(refresh);
      }
      this.consecutiveErrors = 0;
    } catch (error) {
      this.consecutiveErrors += 1;
      const message = errorMessage(error);
      this.store.setConnection(this.consecutiveErrors >= 3 ? "offline" : "degraded", message);
    } finally {
      this.refreshBusy = false;
    }
  }

  async refreshLive(): Promise<void> {
    if (this.stopped || this.liveBusy) return;
    const agentID = this.store.value.focusedAgentID;
    if (!agentID) return;
    this.liveBusy = true;
    try {
      this.store.setLive(await this.runtime.live(agentID));
    } catch {
      this.store.setLive(null);
    } finally {
      this.liveBusy = false;
    }
  }

  async loadOlder(): Promise<boolean> {
    const state = this.store.value;
    if (!state.focusedAgentID || !state.hasPrevious) return false;
    if (state.blocks.length >= MAX_RETAINED_BLOCKS - DEFAULT_HISTORY_PAGE) {
      this.notice("info", "History window is bounded. Use search to jump further back without growing memory.");
      return false;
    }
    this.store.setBusy("history", true);
    try {
      const viewport = await this.runtime.history({ agent_id: state.focusedAgentID, before: state.oldest, limit: DEFAULT_HISTORY_PAGE });
      const accepted = this.store.prependHistory(viewport.blocks ?? [], viewport.has_previous, viewport.oldest ?? 0);
      if (!accepted) this.notice("info", "History cache reached its bounded limit. Use search for older events.");
      return accepted;
    } catch (error) {
      this.notice("error", errorMessage(error));
      return false;
    } finally {
      this.store.setBusy("history", false);
    }
  }

  async search(query: string) {
    const root = this.store.value.rootAgentID;
    const text = query.trim();
    if (!root || !text) return [];
    try {
      const result = await this.runtime.search({ root_agent_id: root, query: text, limit: 80 });
      return result.blocks ?? [];
    } catch (error) {
      this.notice("error", errorMessage(error));
      return [];
    }
  }

  async sendMessage(text: string, queue: QueueKind = "steer"): Promise<boolean> {
    const value = text.trim();
    const agentID = this.store.value.focusedAgentID;
    if (!agentID || !value) return false;
    this.store.setBusy("send", true);
    try {
      const result = await this.runtime.sendMessage({ agent_id: agentID, text: value, queue });
      this.notice("success", queue === "steer" ? "Steering message queued." : `Follow-up queued (${result.pending}).`);
      void this.refresh(true);
      return true;
    } catch (error) {
      this.notice("error", errorMessage(error));
      return false;
    } finally {
      this.store.setBusy("send", false);
    }
  }

  async suspend(): Promise<void> {
    const process = this.store.focusedProcess();
    if (!process) return;
    this.store.setBusy("transition", true);
    try {
      await this.runtime.suspend({ agent_id: process.agent_id, expected_version: process.version, reason: "desktop user suspended agent" });
      this.notice("success", "Agent suspended.");
      await this.refresh(true);
    } catch (error) {
      this.notice("error", errorMessage(error));
    } finally {
      this.store.setBusy("transition", false);
    }
  }

  async resume(): Promise<void> {
    const process = this.store.focusedProcess();
    if (!process) return;
    this.store.setBusy("transition", true);
    try {
      await this.runtime.resume({ agent_id: process.agent_id, expected_version: process.version, reason: "desktop user resumed agent" });
      this.notice("success", "Agent resumed.");
      await this.refresh(true);
    } catch (error) {
      this.notice("error", errorMessage(error));
    } finally {
      this.store.setBusy("transition", false);
    }
  }

  async transaction(transactionID: string, operation: "prepare" | "commit" | "rollback" | "reconcile"): Promise<boolean> {
    if (!transactionID) return false;
    const key = `tx:${transactionID}`;
    this.store.setBusy(key, true);
    try {
      await this.runtime.operateTransaction({ transaction_id: transactionID, operation });
      this.notice("success", `Transaction ${operation} requested.`);
      await this.refresh(true);
      return true;
    } catch (error) {
      this.notice("error", errorMessage(error));
      return false;
    } finally {
      this.store.setBusy(key, false);
    }
  }

  async verifyTransaction(transactionID: string, command: string[], timeoutMS: number): Promise<boolean> {
    if (!transactionID || command.length === 0 || !command[0]?.trim()) return false;
    const key = `tx:${transactionID}`;
    this.store.setBusy(key, true);
    try {
      await this.runtime.operateTransaction({ transaction_id: transactionID, operation: "verify", command, timeout_ms: timeoutMS });
      this.notice("success", "Transaction verification completed.");
      await this.refresh(true);
      return true;
    } catch (error) {
      this.notice("error", errorMessage(error));
      return false;
    } finally {
      this.store.setBusy(key, false);
    }
  }

  async resolveEffect(transactionID: string, effectID: string, certainty: "known_applied" | "known_absent" | "unknown", evidence: string): Promise<boolean> {
    if (!transactionID || !effectID.trim()) return false;
    const key = `tx:${transactionID}`;
    this.store.setBusy(key, true);
    try {
      await this.runtime.operateTransaction({ transaction_id: transactionID, operation: "resolve_effect", effect_id: effectID.trim(), certainty, evidence: evidence.trim() });
      this.notice("success", "Effect resolution recorded. Reconciliation still determines the final transaction state.");
      await this.refresh(true);
      return true;
    } catch (error) {
      this.notice("error", errorMessage(error));
      return false;
    } finally {
      this.store.setBusy(key, false);
    }
  }
}

export function shortID(value: string, length = 10): string {
  if (!value) return "—";
  if (value.length <= length + 4) return value;
  return `${value.slice(0, length)}…${value.slice(-3)}`;
}
