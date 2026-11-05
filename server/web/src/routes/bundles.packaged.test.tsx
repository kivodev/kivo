import { createMemoryHistory } from "@tanstack/react-router";
import { expect, it } from "vitest";
import { createQueryClient } from "@/app/queryClient";
import { createAppRouter } from "@/app/router";

it("keeps the packaged address route and Settings without the admin tree", () => {
  const router = createAppRouter(createQueryClient(), createMemoryHistory());
  expect(router.routesByPath["/server"]).toBeDefined();
  expect(router.routesByPath["/settings"]).toBeDefined();
  expect(Object.keys(router.routesByPath).filter((path) => path.startsWith("/admin"))).toEqual([]);
});

it("defers the address screen until the packaged app needs it", () => {
  const router = createAppRouter(createQueryClient(), createMemoryHistory());
  expect(router.routesByPath["/server"].options.component).toHaveProperty("preload", expect.any(Function));
});
