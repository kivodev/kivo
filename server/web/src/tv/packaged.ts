// The packaged TV app's server and session. The app keeps one
// server, the address as typed with the server's ID, and its session's
// token beside it. The page is a file: page, so the session cookie would
// never be sent; the API client sends the token instead. What the app keeps
// is held here, read from the app's own storage under "kivo." keys once as
// it starts and written back on each change. Storage that can't keep a
// change, being full or unavailable, costs only the next start: this run
// carries on with what it holds. Keeping another server starts the app
// again, so the app always runs on the server it started with. Only the
// packaged build (isPackaged) uses this, and each function first checks the
// flag: the web build imports the module too, from code the flag drops,
// and the check lets it drop the bodies and the keys as well.
import { readStored, readStoredJson, writeStored, writeStoredJson } from "@/lib/storage";
import { isPackaged } from "./flag";
import { restartApp } from "./restart";

/** The server the app opens: its base URL, without a trailing slash, and its ID. */
export type PackagedServer = { address: string; serverId: string };

const serverKey = "kivo.server";
const tokenKey = "kivo.token";
const clientIdKey = "kivo.clientId";

type Kept = { server: PackagedServer | undefined; token: string | undefined; clientId: string | undefined };

// What the app holds, read from storage on first use, and the server it
// held then, which the API client's address is fixed to.
let kept: Kept | undefined;
let startedOn: PackagedServer | undefined;

function held(): Kept {
  if (kept === undefined) {
    kept = { server: storedServer(), token: readStored(tokenKey), clientId: readStored(clientIdKey) };
    startedOn = kept.server;
  }
  return kept;
}

function storedServer(): PackagedServer | undefined {
  const value = readStoredJson(serverKey);
  if (typeof value !== "object" || value === null) return undefined;
  const { address, serverId } = value as Record<string, unknown>;
  return typeof address === "string" && typeof serverId === "string" ? { address, serverId } : undefined;
}

function sameServer(a: PackagedServer | undefined, b: PackagedServer | undefined): boolean {
  return a?.address === b?.address && a?.serverId === b?.serverId;
}

/** The kept server, or undefined before one is chosen. */
export function packagedServer(): PackagedServer | undefined {
  if (!isPackaged) return undefined;
  return held().server;
}

/**
 * Keeps the server. Another server than the one kept forgets the token,
 * which is for its own server alone, and one other than the server the app
 * started with starts the app again on it, from its first screen. Returns
 * whether it does, so the caller leaves the rest to the new start.
 * Undefined forgets the server, and the token with it.
 */
export function setPackagedServer(server: PackagedServer | undefined): boolean {
  if (!isPackaged) return false;
  const app = held();
  const next = server && { address: server.address.replace(/\/+$/, ""), serverId: server.serverId };
  if (!sameServer(next, app.server)) {
    app.token = undefined;
    writeStored(tokenKey, undefined);
  }
  app.server = next;
  if (next === undefined) {
    writeStored(serverKey, undefined);
    return false;
  }
  writeStoredJson(serverKey, next);
  if (sameServer(next, startedOn)) return false;
  restartApp();
  return true;
}

/** The session's token, or undefined when signed out. */
export function packagedToken(): string | undefined {
  if (!isPackaged) return undefined;
  return held().token;
}

/**
 * Keeps the session's token, while the kept server is the one the app
 * started with: a sign-in answered once the app has left that server, or
 * as it starts again on another, is the old server's. Undefined forgets it.
 */
export function setPackagedToken(token: string | undefined): void {
  if (!isPackaged) return;
  const app = held();
  if (token !== undefined && !sameServer(app.server, startedOn)) return;
  app.token = token;
  writeStored(tokenKey, token);
}

/**
 * The app's client ID, made once and kept, so that signing in again
 * replaces the TV's session rather than adding one.
 */
export function packagedClientId(): string {
  if (!isPackaged) throw new Error("Only the packaged app keeps a client ID");
  const app = held();
  if (app.clientId === undefined) {
    app.clientId = crypto.randomUUID();
    writeStored(clientIdKey, app.clientId);
  }
  return app.clientId;
}
