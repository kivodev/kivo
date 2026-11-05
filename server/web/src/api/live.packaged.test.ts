import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { testServer } from "@/test/packagedSetup";
import { packagedServer, packagedToken, setPackagedToken } from "@/tv/packaged";
import { setUnauthorizedHandler } from "./client";
import { connectLiveEvents } from "./live";

/** A WebSocket that records how it was opened. */
class FakeSocket {
  static all: FakeSocket[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: unknown }) => void) | null = null;
  onclose: ((e: { code: number }) => void) | null = null;
  readonly url: string;
  readonly protocols: string | string[] | undefined;
  constructor(url: string, protocols?: string | string[]) {
    this.url = url;
    this.protocols = protocols;
    FakeSocket.all.push(this);
  }
  close() {}
}

beforeEach(() => {
  FakeSocket.all = [];
  vi.stubGlobal("WebSocket", FakeSocket);
});
afterEach(() => {
  vi.unstubAllGlobals();
});

const connect = () => connectLiveEvents({ onMessage: () => {}, onStatus: () => {} });

it("opens the events socket at the kept server, with the token as a subprotocol and never in the URL", () => {
  setPackagedToken("token-1");
  const connection = connect();
  const socket = FakeSocket.all[0];
  expect(socket?.url).toBe("ws://kivo.test/api/v1/events");
  expect(socket?.protocols).toEqual(["kivo", "kivo-bearer.token-1"]);
  expect(socket?.url).not.toContain("token-1");
  connection.close();
});

it("forgets the token and keeps the address when the socket closes for the session", () => {
  const signedOut = vi.fn();
  setUnauthorizedHandler(signedOut);
  setPackagedToken("token-1");
  const connection = connect();
  FakeSocket.all[0]?.onclose?.({ code: 4401 });
  expect(signedOut).toHaveBeenCalledOnce();
  expect(packagedToken()).toBeUndefined();
  expect(packagedServer()).toEqual(testServer);
  connection.close();
});

it("keeps a new token when a socket opened with the one before closes for its session", () => {
  const signedOut = vi.fn();
  setUnauthorizedHandler(signedOut);
  setPackagedToken("token-1");
  const connection = connect();
  setPackagedToken("token-2");
  FakeSocket.all[0]?.onclose?.({ code: 4401 });
  expect(signedOut).not.toHaveBeenCalled();
  expect(packagedToken()).toBe("token-2");
  connection.close();
});
