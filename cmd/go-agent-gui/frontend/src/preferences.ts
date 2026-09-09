export type ThemePreference = "system" | "light" | "dark";
export type DensityPreference = "comfortable" | "compact";
export type MotionPreference = "system" | "reduced";

export interface Preferences {
  theme: ThemePreference;
  density: DensityPreference;
  motion: MotionPreference;
  sidebar: boolean;
  inspector: boolean;
}

const key = "go-agent:g14-preferences";
const defaults: Preferences = { theme: "system", density: "comfortable", motion: "system", sidebar: true, inspector: true };

export function loadPreferences(): Preferences {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return { ...defaults };
    const parsed = JSON.parse(raw) as Partial<Preferences>;
    return {
      theme: parsed.theme === "light" || parsed.theme === "dark" ? parsed.theme : "system",
      density: parsed.density === "compact" ? "compact" : "comfortable",
      motion: parsed.motion === "reduced" ? "reduced" : "system",
      sidebar: parsed.sidebar !== false,
      inspector: parsed.inspector !== false,
    };
  } catch {
    return { ...defaults };
  }
}

export function savePreferences(value: Preferences): void {
  try { localStorage.setItem(key, JSON.stringify(value)); } catch { /* presentation preferences are optional */ }
  applyPreferences(value);
}

export function applyPreferences(value: Preferences): void {
  const root = document.documentElement;
  root.dataset.theme = value.theme;
  root.dataset.density = value.density;
  root.dataset.motion = value.motion;
}
