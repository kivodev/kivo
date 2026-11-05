import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { startClientErrorReports } from "@/api/clientErrors";
import { restoreTheme } from "@/lib/deviceSettings";
import { readStored, writeStored } from "@/lib/storage";
import { isTv } from "@/tv/flag";
import { App } from "./app/App";
import "./theme/index.css";

// A deploy replaces the app's chunks, so a page open across one reloads into the new build when a chunk it asks for is
// gone, as Vite recommends. At most once a minute, so a server that keeps failing doesn't reload it forever.
window.addEventListener("vite:preloadError", () => {
  const now = Date.now();
  if (now - Number(readStored("kivo.chunkReload") ?? 0) < 60_000) return;
  writeStored("kivo.chunkReload", String(now));
  location.reload();
});

async function start() {
  // First, so an error while starting is reported too, once signed in.
  if (isTv) startClientErrorReports();
  // `npm run dev:mock` serves the API from MSW in the browser.
  if (import.meta.env.DEV && import.meta.env.VITE_KIVO_MOCK === "1") {
    const { startMockWorker } = await import("./api/mocks/browser");
    await startMockWorker();
  }
  restoreTheme();
  const root = document.getElementById("root");
  if (!root) throw new Error("missing #root");
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
}

void start();
