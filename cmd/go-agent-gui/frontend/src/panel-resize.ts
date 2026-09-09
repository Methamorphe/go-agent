const STORAGE_KEY = "go-agent:g14-panel-layout";
const SIDEBAR_MIN = 208;
const SIDEBAR_MAX = 360;
const INSPECTOR_MIN = 300;
const INSPECTOR_MAX = 520;
const DEFAULT_SIDEBAR = 248;
const DEFAULT_INSPECTOR = 350;

interface PanelLayout {
  sidebar: number;
  inspector: number;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function load(): PanelLayout {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) || "{}") as Partial<PanelLayout>;
    return {
      sidebar: clamp(Number(parsed.sidebar) || DEFAULT_SIDEBAR, SIDEBAR_MIN, SIDEBAR_MAX),
      inspector: clamp(Number(parsed.inspector) || DEFAULT_INSPECTOR, INSPECTOR_MIN, INSPECTOR_MAX),
    };
  } catch {
    return { sidebar: DEFAULT_SIDEBAR, inspector: DEFAULT_INSPECTOR };
  }
}

function persist(layout: PanelLayout): void {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(layout)); } catch { /* optional presentation state */ }
}

function apply(shell: HTMLElement, layout: PanelLayout): void {
  shell.style.setProperty("--sidebar-width", `${layout.sidebar}px`);
  shell.style.setProperty("--inspector-width", `${layout.inspector}px`);
}

function createHandle(side: "sidebar" | "inspector"): HTMLDivElement {
  const handle = document.createElement("div");
  handle.className = `panel-resizer panel-resizer-${side}`;
  handle.tabIndex = 0;
  handle.setAttribute("role", "separator");
  handle.setAttribute("aria-orientation", "vertical");
  handle.setAttribute("aria-label", side === "sidebar" ? "Resize Agent navigation" : "Resize runtime inspector");
  handle.dataset.panelResize = side;
  return handle;
}

function installStyle(): void {
  if (document.getElementById("g14-panel-resize-style")) return;
  const style = document.createElement("style");
  style.id = "g14-panel-resize-style";
  style.textContent = `
    .shell {
      --sidebar-width: 248px;
      --inspector-width: 350px;
      grid-template-columns: var(--sidebar-width) minmax(420px, 1fr) var(--inspector-width) !important;
      position: relative;
    }
    .shell.sidebar-hidden { grid-template-columns: 0 minmax(420px, 1fr) var(--inspector-width) !important; }
    .shell.inspector-hidden { grid-template-columns: var(--sidebar-width) minmax(420px, 1fr) 0 !important; }
    .shell.sidebar-hidden.inspector-hidden { grid-template-columns: 0 minmax(420px, 1fr) 0 !important; }
    .panel-resizer {
      position: absolute;
      z-index: 20;
      top: 0;
      bottom: 0;
      width: 7px;
      cursor: col-resize;
      touch-action: none;
      outline: none;
    }
    .panel-resizer::after {
      content: "";
      position: absolute;
      top: 0;
      bottom: 0;
      left: 3px;
      width: 1px;
      background: transparent;
      transition: background-color var(--motion-micro) ease;
    }
    .panel-resizer:hover::after,
    .panel-resizer:focus-visible::after,
    .panel-resizer.dragging::after { background: var(--accent); }
    .panel-resizer-sidebar { left: calc(var(--sidebar-width) - 4px); }
    .panel-resizer-inspector { right: calc(var(--inspector-width) - 4px); }
    .shell.sidebar-hidden .panel-resizer-sidebar,
    .shell.inspector-hidden .panel-resizer-inspector { display: none; }
    @media (max-width: 1180px) {
      .shell, .shell.inspector-hidden { grid-template-columns: min(var(--sidebar-width), 228px) minmax(0, 1fr) !important; }
      .shell.sidebar-hidden, .shell.sidebar-hidden.inspector-hidden { grid-template-columns: 0 minmax(0, 1fr) !important; }
      .panel-resizer { display: none !important; }
    }
    @media (max-width: 940px) {
      .shell, .shell.sidebar-hidden, .shell.inspector-hidden, .shell.sidebar-hidden.inspector-hidden { display: block !important; }
    }
    @media (prefers-reduced-motion: reduce) {
      .panel-resizer::after { transition: none; }
    }
  `;
  document.head.append(style);
}

export function installPanelResizing(root: HTMLElement): () => void {
  installStyle();
  const shell = root.querySelector<HTMLElement>("#shell");
  if (!shell) return () => undefined;

  const layout = load();
  apply(shell, layout);
  const sidebar = createHandle("sidebar");
  const inspector = createHandle("inspector");
  shell.append(sidebar, inspector);

  let active: "sidebar" | "inspector" | null = null;
  let startX = 0;
  let startSize = 0;

  const updateARIA = (): void => {
    sidebar.setAttribute("aria-valuemin", String(SIDEBAR_MIN));
    sidebar.setAttribute("aria-valuemax", String(SIDEBAR_MAX));
    sidebar.setAttribute("aria-valuenow", String(layout.sidebar));
    inspector.setAttribute("aria-valuemin", String(INSPECTOR_MIN));
    inspector.setAttribute("aria-valuemax", String(INSPECTOR_MAX));
    inspector.setAttribute("aria-valuenow", String(layout.inspector));
  };
  updateARIA();

  const pointerDown = (event: PointerEvent): void => {
    const target = event.currentTarget as HTMLElement;
    active = target.dataset.panelResize as "sidebar" | "inspector";
    startX = event.clientX;
    startSize = active === "sidebar" ? layout.sidebar : layout.inspector;
    target.classList.add("dragging");
    target.setPointerCapture(event.pointerId);
    event.preventDefault();
  };

  const pointerMove = (event: PointerEvent): void => {
    if (!active) return;
    const delta = event.clientX - startX;
    if (active === "sidebar") layout.sidebar = clamp(startSize + delta, SIDEBAR_MIN, SIDEBAR_MAX);
    else layout.inspector = clamp(startSize - delta, INSPECTOR_MIN, INSPECTOR_MAX);
    apply(shell, layout);
    updateARIA();
  };

  const pointerUp = (event: PointerEvent): void => {
    if (!active) return;
    const target = event.currentTarget as HTMLElement;
    target.classList.remove("dragging");
    if (target.hasPointerCapture(event.pointerId)) target.releasePointerCapture(event.pointerId);
    active = null;
    persist(layout);
  };

  const keyDown = (event: KeyboardEvent): void => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight" && event.key !== "Home" && event.key !== "End") return;
    const target = event.currentTarget as HTMLElement;
    const side = target.dataset.panelResize as "sidebar" | "inspector";
    const minimum = side === "sidebar" ? SIDEBAR_MIN : INSPECTOR_MIN;
    const maximum = side === "sidebar" ? SIDEBAR_MAX : INSPECTOR_MAX;
    let value = side === "sidebar" ? layout.sidebar : layout.inspector;
    if (event.key === "Home") value = minimum;
    else if (event.key === "End") value = maximum;
    else {
      const visualDelta = event.key === "ArrowRight" ? 12 : -12;
      value += side === "sidebar" ? visualDelta : -visualDelta;
    }
    value = clamp(value, minimum, maximum);
    if (side === "sidebar") layout.sidebar = value;
    else layout.inspector = value;
    apply(shell, layout);
    updateARIA();
    persist(layout);
    event.preventDefault();
  };

  for (const handle of [sidebar, inspector]) {
    handle.addEventListener("pointerdown", pointerDown);
    handle.addEventListener("pointermove", pointerMove);
    handle.addEventListener("pointerup", pointerUp);
    handle.addEventListener("pointercancel", pointerUp);
    handle.addEventListener("keydown", keyDown);
  }

  return () => {
    sidebar.remove();
    inspector.remove();
  };
}
