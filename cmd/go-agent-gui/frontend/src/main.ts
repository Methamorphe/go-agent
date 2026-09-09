import "./styles.css";
import { DesktopApp } from "./app";
import { WorkspaceController } from "./controller";
import { installPanelResizing } from "./panel-resize";
import { WailsRuntimeClient } from "./runtime";
import { WorkspaceStore } from "./store";

const root = document.querySelector<HTMLElement>("#app");
if (!root) throw new Error("#app root is missing");

const store = new WorkspaceStore();
const runtime = new WailsRuntimeClient();
const controller = new WorkspaceController(runtime, store);
const app = new DesktopApp(root, controller, store);

app.mount();
const removePanelResizing = installPanelResizing(root);
void runtime.frontendReady();
void controller.start();

window.addEventListener("beforeunload", () => {
  removePanelResizing();
  controller.stop();
  app.unmount();
});
