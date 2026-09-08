import { describe, expect, it } from "vitest";
import { WorkspaceController } from "./controller";
import { WorkspaceStore } from "./store";
import type { Bootstrap, ConversationViewport, LiveSnapshot, Refresh, RuntimeClient, SearchResult, Snapshot } from "./types";

function snapshot(cursor = 4): Snapshot {
  return {
    protocol_version: 1,
    root_agent_id: "agt-root",
    focused_agent_id: "agt-root",
    mode: "ACT",
    cursor,
    tree: [{ agent_id: "agt-root", root_agent_id: "agt-root", depth: 0, status: "RUNNING", version: cursor, children: 0 }],
    viewport: { agent_id: "agt-root", blocks: [], has_previous: false },
    inspector: {
      transactions: [], forks: [], context: { page_count: 0, estimated_tokens: 0, active_lease_count: 0, unresolved_faults: 0 }, context_faults: [],
      scheduler: { limit_money_micros: 0, limit_tokens: 0, spent_money_micros: 0, spent_tokens: 0, reserved_money_micros: 0, reserved_tokens: 0 },
      authority: { capability_projection: "runtime" }, teams: [], improvements: [],
    },
  };
}

class RuntimeStub implements RuntimeClient {
  refreshCalls = 0;
  attachCalls = 0;
  nextRefresh: () => Promise<Refresh> = async () => ({ cursor: 4, tree_patch: [], blocks: [], behind: false });
  nextAttach: () => Promise<Snapshot> = async () => snapshot();

  async bootstrap(): Promise<Bootstrap> { throw new Error("unused"); }
  async attach(): Promise<Snapshot> { this.attachCalls += 1; return this.nextAttach(); }
  async refresh(): Promise<Refresh> { this.refreshCalls += 1; return this.nextRefresh(); }
  async history(): Promise<ConversationViewport> { throw new Error("unused"); }
  async search(): Promise<SearchResult> { return { blocks: [] }; }
  async live(): Promise<LiveSnapshot> { throw new Error("unused"); }
  async sendMessage(): Promise<{ message_id: string; agent_id: string; queue: "steer" | "follow_up"; pending: number }> { throw new Error("unused"); }
  async suspend(): Promise<unknown> { throw new Error("unused"); }
  async resume(): Promise<unknown> { throw new Error("unused"); }
  async operateTransaction(): Promise<unknown> { throw new Error("unused"); }
}

describe("WorkspaceController", () => {
  it("reattaches to daemon truth when the bounded projection reports behind", async () => {
    const store = new WorkspaceStore();
    store.applySnapshot(snapshot(4));
    const runtime = new RuntimeStub();
    runtime.nextRefresh = async () => ({ cursor: 9, tree_patch: [], blocks: [], behind: true });
    runtime.nextAttach = async () => snapshot(17);
    const controller = new WorkspaceController(runtime, store);

    await controller.refresh();

    expect(runtime.refreshCalls).toBe(1);
    expect(runtime.attachCalls).toBe(1);
    expect(store.value.cursor).toBe(17);
    expect(store.value.focusedAgentID).toBe("agt-root");
  });

  it("coalesces concurrent refresh attempts for a slow client", async () => {
    const store = new WorkspaceStore();
    store.applySnapshot(snapshot(4));
    const runtime = new RuntimeStub();
    let release!: (refresh: Refresh) => void;
    runtime.nextRefresh = () => new Promise<Refresh>((resolve) => { release = resolve; });
    const controller = new WorkspaceController(runtime, store);

    const first = controller.refresh();
    const second = controller.refresh();
    expect(runtime.refreshCalls).toBe(1);

    release({ cursor: 5, tree_patch: [], blocks: [], behind: false });
    await Promise.all([first, second]);

    expect(runtime.refreshCalls).toBe(1);
    expect(store.value.cursor).toBe(5);
  });
});
