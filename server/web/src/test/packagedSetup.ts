// The packaged app's requests go to the server it keeps, so each of its
// tests starts with one kept, as after the address screen, and no token.
// The app starts on it, as on one kept from an earlier run: the API
// client's address is the kept server's as the app starts. The mocks, the
// events socket's too, answer any origin. jsdom loads no pages, so starting
// the app again, on keeping another server, is a mock to check.
import { beforeEach, vi } from "vitest";
import { mockId } from "@/api/mocks/ids";
import { type PackagedServer, setPackagedServer } from "@/tv/packaged";

vi.mock("@/tv/restart", () => ({ restartApp: vi.fn() }));

// The mock server's own ID, as the address screen keeps it.
export const testServer: PackagedServer = { address: "http://kivo.test", serverId: mockId(7200) };

localStorage.setItem("kivo.server", JSON.stringify(testServer));
// setup.ts clears the browser's storage after each test, and the app holds
// what it keeps in memory too, so each test forgets the token as well.
beforeEach(() => {
  setPackagedServer(undefined);
  setPackagedServer(testServer);
});
