import type { Block } from "./types";

export interface PlanStep {
  id: string;
  title: string;
  state: string;
  detail: string;
}

export interface PlanReview {
  blockID: string;
  objectRef: string;
  title: string;
  steps: PlanStep[];
}

export interface DiffHunk { header: string; body: string }
export interface DiffFile { path: string; status: string; additions: number; deletions: number; hunks: DiffHunk[] }
export interface DiffReview {
  blockID: string;
  objectRef: string;
  baseRef: string;
  sourceRef: string;
  targetRef: string;
  files: DiffFile[];
}

function objectPayload(payload: unknown): Record<string, unknown> | null {
  if (payload && typeof payload === "object" && !Array.isArray(payload)) return payload as Record<string, unknown>;
  if (typeof payload !== "string") return null;
  try {
    const decoded = JSON.parse(payload) as unknown;
    return decoded && typeof decoded === "object" && !Array.isArray(decoded) ? decoded as Record<string, unknown> : null;
  } catch {
    return null;
  }
}

function unwrap(payload: unknown, key: string): Record<string, unknown> | null {
  const root = objectPayload(payload);
  if (!root) return null;
  const nested = root[key];
  if (nested && typeof nested === "object" && !Array.isArray(nested)) return nested as Record<string, unknown>;
  return root;
}

function text(source: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = source[key];
    if (typeof value === "string" && value.trim()) return value.trim();
    if (typeof value === "number") return String(value);
  }
  return "";
}

function number(source: Record<string, unknown>, ...keys: string[]): number {
  for (const key of keys) {
    const value = source[key];
    if (typeof value === "number" && Number.isFinite(value)) return value;
  }
  return 0;
}

export function decodePlan(block: Block): PlanReview {
  const root = unwrap(block.payload, "plan");
  const title = root ? text(root, "title", "goal", "objective") : "";
  const rawSteps = root ? (root.steps ?? root.items ?? root.tasks) : null;
  const steps: PlanStep[] = [];
  if (Array.isArray(rawSteps)) {
    rawSteps.forEach((entry, index) => {
      if (typeof entry === "string") {
        steps.push({ id: String(index + 1), title: entry, state: "proposed", detail: "" });
        return;
      }
      if (!entry || typeof entry !== "object" || Array.isArray(entry)) return;
      const item = entry as Record<string, unknown>;
      steps.push({
        id: text(item, "id", "step_id", "key") || String(index + 1),
        title: text(item, "title", "summary", "task", "name") || `Step ${index + 1}`,
        state: text(item, "state", "status") || "proposed",
        detail: text(item, "detail", "description", "reason"),
      });
    });
  }
  if (steps.length === 0) {
    steps.push({ id: "1", title: block.preview || block.object_ref || "Plan available", state: "proposed", detail: "" });
  }
  return { blockID: block.id, objectRef: block.object_ref ?? "", title: title || block.title || "Agent plan", steps };
}

export function decodeDiff(block: Block): DiffReview {
  const root = unwrap(block.payload, "diff");
  const files: DiffFile[] = [];
  const rawFiles = root ? (root.files ?? root.changes) : null;
  if (Array.isArray(rawFiles)) {
    for (const entry of rawFiles) {
      if (!entry || typeof entry !== "object" || Array.isArray(entry)) continue;
      const item = entry as Record<string, unknown>;
      const hunks: DiffHunk[] = [];
      const rawHunks = item.hunks ?? item.patches;
      if (Array.isArray(rawHunks)) {
        for (const raw of rawHunks) {
          if (typeof raw === "string") hunks.push({ header: "hunk", body: raw });
          else if (raw && typeof raw === "object" && !Array.isArray(raw)) {
            const hunk = raw as Record<string, unknown>;
            hunks.push({ header: text(hunk, "header", "range", "title") || "hunk", body: text(hunk, "body", "patch", "diff", "text") });
          }
        }
      }
      const path = text(item, "path", "file", "name");
      if (path) files.push({ path, status: text(item, "status", "state") || "changed", additions: number(item, "additions", "added"), deletions: number(item, "deletions", "removed"), hunks });
    }
  }
  if (files.length === 0) {
    files.push({ path: block.object_ref || block.title || "workspace diff", status: "changed", additions: 0, deletions: 0, hunks: [{ header: "projected diff", body: block.preview ?? "" }] });
  }
  return {
    blockID: block.id,
    objectRef: block.object_ref ?? "",
    baseRef: root ? text(root, "base_ref", "base", "base_identity") : "",
    sourceRef: root ? text(root, "source_ref", "source", "source_identity") : "",
    targetRef: root ? text(root, "target_ref", "target", "target_identity") : "",
    files,
  };
}

export function reviewReference(kind: "plan" | "diff", block: Block): string {
  return `${kind}:${block.object_ref || block.id || "latest"}`;
}
