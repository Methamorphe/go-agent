import type {
  Block,
  Bootstrap,
  ConnectionState,
  Inspector,
  LiveSnapshot,
  Mode,
  ProcessSummary,
  Refresh,
  Snapshot,
  WorkspaceState,
} from "./types";

export const MAX_RETAINED_BLOCKS = 4000;
export const MAX_RENDERED_BLOCKS = 260;
export const DEFAULT_HISTORY_PAGE = 120;

export type StoreListener = (state: Readonly<WorkspaceState>) => void;

function initialState(): WorkspaceState {
  return {
    bootstrap: null,
    connection: "booting",
    error: null,
    rootAgentID: "",
    focusedAgentID: "",
    mode: "ACT",
    cursor: 0,
    tree: [],
    blocks: [],
    hasPrevious: false,
    oldest: 0,
    inspector: null,
    live: null,
    revision: 0,
    busy: new Set<string>(),
  };
}

function bySequence(a: Block, b: Block): number {
  const left = Number.isFinite(a.sequence) ? a.sequence : a.process_version;
  const right = Number.isFinite(b.sequence) ? b.sequence : b.process_version;
  return left - right;
}

function uniqueBlocks(blocks: Block[]): Block[] {
  const seen = new Set<string>();
  const result: Block[] = [];
  for (const block of blocks) {
    const key = block.id || `${block.agent_id}:${block.sequence}:${block.process_version}`;
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(block);
  }
  result.sort(bySequence);
  return result;
}

function mergeTree(current: ProcessSummary[], patch: ProcessSummary[]): ProcessSummary[] {
  if (patch.length === 0) return current;
  const map = new Map(current.map((entry) => [entry.agent_id, entry]));
  for (const entry of patch) map.set(entry.agent_id, entry);
  return [...map.values()].sort((a, b) => a.depth - b.depth || String(a.created_at ?? "").localeCompare(String(b.created_at ?? "")));
}

export class WorkspaceStore {
  private state: WorkspaceState = initialState();
  private listeners = new Set<StoreListener>();

  get value(): Readonly<WorkspaceState> {
    return this.state;
  }

  subscribe(listener: StoreListener): () => void {
    this.listeners.add(listener);
    listener(this.state);
    return () => this.listeners.delete(listener);
  }

  setBootstrap(bootstrap: Bootstrap): void {
    this.patch({ bootstrap, mode: bootstrap.launch.mode ?? this.state.mode });
  }

  setConnection(connection: ConnectionState, error: string | null = null): void {
    if (this.state.connection === connection && this.state.error === error) return;
    this.patch({ connection, error });
  }

  setMode(mode: Mode): void {
    if (this.state.mode === mode) return;
    this.patch({ mode });
  }

  setBusy(key: string, active: boolean): void {
    const busy = new Set(this.state.busy);
    if (active) busy.add(key);
    else busy.delete(key);
    this.patch({ busy });
  }

  applySnapshot(snapshot: Snapshot): void {
    const blocks = uniqueBlocks(snapshot.viewport.blocks ?? []).slice(-MAX_RETAINED_BLOCKS);
    this.state = {
      ...this.state,
      connection: "online",
      error: null,
      rootAgentID: snapshot.root_agent_id,
      focusedAgentID: snapshot.focused_agent_id,
      mode: snapshot.mode,
      cursor: snapshot.cursor,
      tree: snapshot.tree ?? [],
      blocks,
      hasPrevious: Boolean(snapshot.viewport.has_previous),
      oldest: snapshot.viewport.oldest ?? blocks[0]?.process_version ?? 0,
      inspector: snapshot.inspector,
      live: null,
      revision: this.state.revision + 1,
    };
    this.emit();
  }

  applyRefresh(refresh: Refresh): void {
    if (refresh.behind) {
      this.patch({ error: "Client projection fell behind; resynchronising." });
      return;
    }
    const incoming = refresh.blocks ?? [];
    const merged = incoming.length === 0
      ? this.state.blocks
      : uniqueBlocks([...this.state.blocks, ...incoming]).slice(-MAX_RETAINED_BLOCKS);
    const tree = mergeTree(this.state.tree, refresh.tree_patch ?? []);
    this.state = {
      ...this.state,
      connection: "online",
      error: null,
      cursor: Math.max(this.state.cursor, refresh.cursor),
      blocks: merged,
      tree,
      inspector: refresh.inspector ?? this.state.inspector,
      oldest: merged[0]?.process_version ?? this.state.oldest,
      revision: this.state.revision + 1,
    };
    this.emit();
  }

  prependHistory(blocks: Block[], hasPrevious: boolean, oldest: number): boolean {
    if (blocks.length === 0) {
      this.patch({ hasPrevious });
      return true;
    }
    const merged = uniqueBlocks([...blocks, ...this.state.blocks]);
    if (merged.length > MAX_RETAINED_BLOCKS) return false;
    this.state = {
      ...this.state,
      blocks: merged,
      hasPrevious,
      oldest: oldest || merged[0]?.process_version || 0,
      revision: this.state.revision + 1,
    };
    this.emit();
    return true;
  }

  setLive(live: LiveSnapshot | null): void {
    const previous = this.state.live;
    if (previous?.end_offset === live?.end_offset && previous?.status === live?.status && previous?.error === live?.error) return;
    this.patch({ live });
  }

  setInspector(inspector: Inspector): void {
    this.patch({ inspector });
  }

  focusedProcess(): ProcessSummary | undefined {
    return this.state.tree.find((entry) => entry.agent_id === this.state.focusedAgentID);
  }

  renderWindow(end = this.state.blocks.length, size = MAX_RENDERED_BLOCKS): { blocks: Block[]; start: number; end: number; total: number } {
    const safeEnd = Math.max(0, Math.min(end, this.state.blocks.length));
    const start = Math.max(0, safeEnd - Math.max(1, size));
    return { blocks: this.state.blocks.slice(start, safeEnd), start, end: safeEnd, total: this.state.blocks.length };
  }

  reset(): void {
    this.state = initialState();
    this.emit();
  }

  private patch(patch: Partial<WorkspaceState>): void {
    this.state = { ...this.state, ...patch, revision: this.state.revision + 1 };
    this.emit();
  }

  private emit(): void {
    for (const listener of this.listeners) listener(this.state);
  }
}
