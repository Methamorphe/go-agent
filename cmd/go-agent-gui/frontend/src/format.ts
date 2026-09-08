export function escapeHTML(value: unknown): string {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

export function formatCompact(value: number): string {
  if (!Number.isFinite(value)) return "0";
  return new Intl.NumberFormat(undefined, { notation: "compact", maximumFractionDigits: 1 }).format(value);
}

export function formatMoneyMicros(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "$0";
  return new Intl.NumberFormat(undefined, { style: "currency", currency: "USD", maximumFractionDigits: 3 }).format(value / 1_000_000);
}

export function formatTime(value: unknown): string {
  if (!value) return "";
  const raw = value instanceof Date ? value : new Date(String(value));
  if (Number.isNaN(raw.getTime())) return "";
  return new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" }).format(raw);
}

export function formatRelative(value: unknown): string {
  if (!value) return "";
  const raw = value instanceof Date ? value : new Date(String(value));
  if (Number.isNaN(raw.getTime())) return "";
  const delta = raw.getTime() - Date.now();
  const abs = Math.abs(delta);
  if (abs < 60_000) return "now";
  if (abs < 3_600_000) return `${Math.round(abs / 60_000)}m`;
  if (abs < 86_400_000) return `${Math.round(abs / 3_600_000)}h`;
  return `${Math.round(abs / 86_400_000)}d`;
}

export function percent(used: number, limit: number): number {
  if (!Number.isFinite(limit) || limit <= 0) return 0;
  return Math.max(0, Math.min(100, (used / limit) * 100));
}

export function statusTone(status: string): "ok" | "warn" | "danger" | "neutral" | "active" {
  const value = status.toLowerCase();
  if (value.includes("fail") || value.includes("denied") || value.includes("cancel")) return "danger";
  if (value.includes("reconcil") || value.includes("uncertain") || value.includes("wait") || value.includes("suspend")) return "warn";
  if (value.includes("running") || value.includes("active") || value.includes("ready")) return "active";
  if (value.includes("complete") || value.includes("commit") || value.includes("pass") || value.includes("verified")) return "ok";
  return "neutral";
}
