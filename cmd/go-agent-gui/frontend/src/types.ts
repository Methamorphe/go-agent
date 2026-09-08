export type Mode = "ASK" | "PLAN" | "ACT" | "REVIEW" | "OBSERVE";
export type ConnectionState = "booting" | "online" | "degraded" | "offline";
export type QueueKind = "steer" | "follow_up";

export type BlockKind =
  | "UserMessage"
  | "AssistantMessage"
  | "ProgressUpdate"
  | "Plan"
  | "ToolCall"
  | "ToolResult"
  | "Diff"
  | "TestResult"
  | "Artifact"
  | "Approval"
  | "ChildAgent"
  | "Transaction"
  | "ForkComparison"
  | "ContextFault"
  | "SecurityDecision"
  | "Checkpoint"
  | "Error"
  | "SystemNotice";

export interface Block {
  id: string;
  agent_id: string;
  sequence: number;
  process_version: number;
  kind: BlockKind;
  title?: string;
  preview?: string;
  object_ref?: string;
  event_type?: string;
  payload?: unknown;
  occurred_at?: unknown;
  critical?: boolean;
}

export interface ProcessSummary {
  agent_id: string;
  root_agent_id: string;
  parent_agent_id?: string | null;
  depth: number;
  status: string;
  version: number;
  goal?: string;
  waiting_reason?: string;
  waiting_ref?: string;
  failure?: string;
  updated_at?: unknown;
  created_at?: unknown;
  children: number;
}

export interface ConversationViewport {
  agent_id: string;
  blocks: Block[];
  before?: number;
  oldest?: number;
  newest?: number;
  has_previous: boolean;
}

export interface TransactionSummary {
  id: string;
  agent_id: string;
  world_id: string;
  state: string;
  version: number;
  verification_status?: string;
  effect_count: number;
  uncertain_effects: number;
  reconcile_reason?: string;
  updated_at?: unknown;
}

export interface ForkBranchSummary {
  fork_id: string;
  agent_id: string;
  world_id: string;
  state: string;
  spent_money_micros: number;
  spent_tokens: number;
  evaluation?: string;
}

export interface ForkSummary {
  group_id: string;
  source_agent_id: string;
  state: string;
  winner_fork_id?: string;
  selection_reason?: string;
  branches: ForkBranchSummary[];
  updated_at?: unknown;
}

export interface ContextFaultSummary {
  fault_id: string;
  agent_id: string;
  kind: string;
  reference: string;
  state: string;
  resolution?: string;
  created_at?: unknown;
  updated_at?: unknown;
}

export interface Inspector {
  transactions: TransactionSummary[];
  forks: ForkSummary[];
  context: {
    agent_id?: string;
    page_count: number;
    estimated_tokens: number;
    active_lease_count: number;
    latest_manifest_ref?: string;
    unresolved_faults: number;
  };
  context_faults: ContextFaultSummary[];
  scheduler: {
    root_agent_id?: string;
    limit_money_micros: number;
    limit_tokens: number;
    spent_money_micros: number;
    spent_tokens: number;
    reserved_money_micros: number;
    reserved_tokens: number;
    last_decision_id?: string;
    last_provider?: string;
    last_model?: string;
    last_profile_version?: number;
    last_estimated_money_micros?: number;
    last_estimated_tokens?: number;
    last_decision_at?: unknown;
  };
  authority: {
    agent_id?: string;
    intent_id?: string;
    intent_version?: number;
    goal?: string;
    capability_projection: string;
    last_security_event?: string;
    last_security_at?: unknown;
  };
  teams: Array<{
    team_id: string;
    root_agent_id: string;
    lead_agent_id: string;
    state: string;
    objective: string;
    member_count: number;
    round: number;
    max_rounds: number;
    escalated: boolean;
    updated_at?: unknown;
  }>;
  improvements: Array<{
    artifact_id: string;
    kind: string;
    version: number;
    status: string;
    scope_level: string;
    scope_key?: string;
    active: boolean;
    killed: boolean;
    created_at?: unknown;
  }>;
}

export interface Snapshot {
  protocol_version: number;
  root_agent_id: string;
  focused_agent_id: string;
  mode: Mode;
  cursor: number;
  tree: ProcessSummary[];
  viewport: ConversationViewport;
  inspector: Inspector;
  generated_at?: unknown;
}

export interface Refresh {
  cursor: number;
  tree_patch: ProcessSummary[];
  blocks: Block[];
  inspector?: Inspector | null;
  behind: boolean;
  generated_at?: unknown;
}

export interface LiveSnapshot {
  agent_id: string;
  status: "running" | "completed" | "failed";
  start_offset: number;
  end_offset: number;
  data: string;
  response_ref?: string;
  error?: string;
}

export interface Bootstrap {
  bridge_version: number;
  workspace_protocol_version: number;
  launch: { agent_id?: string; mode?: Mode };
  modes: Mode[];
  max_history_page: number;
  max_tree: number;
  recommended_refresh_ms: number;
  recommended_live_refresh_ms: number;
}

export interface SearchResult {
  blocks: Block[];
}

export interface RuntimeClient {
  bootstrap(): Promise<Bootstrap>;
  attach(request: { agent_id?: string; mode?: Mode; history_limit?: number; tree_limit?: number }): Promise<Snapshot>;
  refresh(request: { root_agent_id: string; focused_agent_id: string; after_sequence: number; limit?: number; inspector?: boolean }): Promise<Refresh>;
  history(request: { agent_id: string; before?: number; limit?: number }): Promise<ConversationViewport>;
  search(request: { root_agent_id: string; query: string; limit?: number }): Promise<SearchResult>;
  live(agentID: string): Promise<LiveSnapshot>;
  sendMessage(request: { agent_id: string; text: string; queue: QueueKind }): Promise<{ message_id: string; agent_id: string; queue: QueueKind; pending: number }>;
  suspend(request: { agent_id: string; expected_version: number; reason?: string }): Promise<unknown>;
  resume(request: { agent_id: string; expected_version: number; reason?: string }): Promise<unknown>;
  operateTransaction(request: {
    transaction_id: string;
    operation: "verify" | "prepare" | "commit" | "rollback" | "reconcile" | "resolve_effect";
    command?: string[];
    timeout_ms?: number;
    effect_id?: string;
    certainty?: string;
    evidence?: string;
  }): Promise<unknown>;
}

export interface WorkspaceState {
  bootstrap: Bootstrap | null;
  connection: ConnectionState;
  error: string | null;
  rootAgentID: string;
  focusedAgentID: string;
  mode: Mode;
  cursor: number;
  tree: ProcessSummary[];
  blocks: Block[];
  hasPrevious: boolean;
  oldest: number;
  inspector: Inspector | null;
  live: LiveSnapshot | null;
  revision: number;
  busy: ReadonlySet<string>;
}
