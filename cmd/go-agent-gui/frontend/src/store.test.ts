import { describe, expect, it } from "vitest";
import { MAX_RETAINED_BLOCKS, WorkspaceStore } from "./store";
import type { Block, Refresh, Snapshot } from "./types";

function block(index: number): Block {
  return {
    id: `b-${index}`,
    agent_id: "agt-root",
    sequence: index,
    process_version: index,
    kind: "SystemNotice",
    preview: `event ${index}`,
  };
}

function snapshot(blocks: Block[]): Snapshot {
  return {
    protocol_version: 1,
    root_agent_id: "agt-root",
    focused_agent_id: "agt-root",
    mode: "ACT",
    cursor: blocks.at(-1)?.sequence ?? 0,
    tree: [{ agent_id: "agt-root", root_agent_id: "agt-root", depth: 0, status: "RUNNING", version: 1, children: 0 }],
    viewport: { agent_id: "agt-root", blocks, has_previous: true },
    inspector: {
      transactions: [], forks: [], context: { page_count: 0, estimated_tokens: 0, active_lease_count: 0, unresolved_faults: 0 }, context_faults: [],
      scheduler: { limit_money_micros: 0, limit_tokens: 0, spent_money_micros: 0, spent_tokens: 0, reserved_money_micros: 0, reserved_tokens: 0 },
      authority: { capability_projection: "runtime" }, teams: [], improvements: [],
    },
  };
}

describe("WorkspaceStore", () => {
  it("keeps synthetic 100k history bounded", () => {
    const store = new WorkspaceStore();
    store.applySnapshot(snapshot(Array.from({ length: 100_000 }, (_, index) => block(index + 1))));
    expect(store.value.blocks).toHaveLength(MAX_RETAINED_BLOCKS);
    expect(store.value.blocks[0]?.id).toBe(`b-${100_000 - MAX_RETAINED_BLOCKS + 1}`);
  });

  it("deduplicates incremental refreshes and advances the cursor", () => {
    const store = new WorkspaceStore();
    store.applySnapshot(snapshot([block(1), block(2)]));
    const refresh: Refresh = { cursor: 3, tree_patch: [], blocks: [block(2), block(3)], behind: false };
    store.applyRefresh(refresh);
    expect(store.value.blocks.map((entry) => entry.id)).toEqual(["b-1", "b-2", "b-3"]);
    expect(store.value.cursor).toBe(3);
  });

  it("does not silently discard the live tail while paging old history", () => {
    const store = new WorkspaceStore();
    const current = Array.from({ length: MAX_RETAINED_BLOCKS }, (_, index) => block(index + 10_000));
    store.applySnapshot(snapshot(current));
    const accepted = store.prependHistory([block(1)], true, 1);
    expect(accepted).toBe(false);
    expect(store.value.blocks.at(-1)?.id).toBe(`b-${10_000 + MAX_RETAINED_BLOCKS - 1}`);
  });
});
