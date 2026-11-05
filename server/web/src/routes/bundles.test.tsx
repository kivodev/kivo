import { createMemoryHistory } from "@tanstack/react-router";
import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { createQueryClient } from "@/app/queryClient";
import { createAppRouter } from "@/app/router";
import { LibrarySkeleton, PageSkeleton } from "@/components/Skeleton";
import { useCast } from "@/features/player/CastButton";
import { castLink } from "@/features/player/castSession";
import { favoritesGridRoute, favoritesRoute } from "./favorites";
import { signInRoute } from "./signIn";
import { watchRoute } from "./watch";

describe("web route bundles", () => {
  it("keeps every admin page available on web", () => {
    const router = createAppRouter(createQueryClient(), createMemoryHistory());
    expect(Object.keys(router.routesByPath).filter((path) => path.startsWith("/admin"))).toHaveLength(17);
    expect(router.routesByPath["/admin/settings/api-keys"]).toBeDefined();
  });

  it("keeps the web Cast sender active", () => {
    const start = vi.spyOn(castLink, "start").mockImplementation(() => {});
    const subscribe = vi.spyOn(castLink, "subscribe");
    renderHook(useCast);
    expect(start).toHaveBeenCalledOnce();
    expect(subscribe).toHaveBeenCalledOnce();
  });

  it("keeps the router delay and gives deferred Favorites screens layout placeholders", () => {
    const router = createAppRouter(createQueryClient(), createMemoryHistory());
    expect(router.options.defaultPendingMs).toBe(100);
    expect(favoritesRoute.options.pendingComponent).toBe(PageSkeleton);
    expect(favoritesGridRoute.options.pendingComponent).toBe(LibrarySkeleton);
  });

  it.each([signInRoute, favoritesRoute, favoritesGridRoute, watchRoute])("defers the screen for $path", (route) => {
    expect(route.options.component).toHaveProperty("preload", expect.any(Function));
  });
});
