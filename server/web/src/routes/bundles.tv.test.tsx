import { createMemoryHistory } from "@tanstack/react-router";
import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { createQueryClient } from "@/app/queryClient";
import { createAppRouter } from "@/app/router";
import { useCast } from "@/features/player/CastButton";
import { castLink } from "@/features/player/castSession";

describe("TV route bundles", () => {
  it("omits admin pages and keeps every other hosted TV route", () => {
    const router = createAppRouter(createQueryClient(), createMemoryHistory());
    expect(Object.keys(router.routesByPath).sort()).toEqual(
      [
        "/",
        "/book-series/$seriesId",
        "/collections/$collectionId",
        "/lists",
        "/lists/$listId",
        "/history",
        "/invite/$token",
        "/libraries",
        "/libraries/$libraryId",
        "/link",
        "/people/$personId",
        "/search",
        "/settings",
        "/setup",
        "/sign-in",
        "/watch/$workId",
        "/works/$workId",
      ].sort(),
    );
  });

  it("does not start or subscribe to a Cast sender on TV", () => {
    const start = vi.spyOn(castLink, "start");
    const subscribe = vi.spyOn(castLink, "subscribe");
    const { result } = renderHook(useCast);
    expect(result.current.connection).toBe("unavailable");
    expect(start).not.toHaveBeenCalled();
    expect(subscribe).not.toHaveBeenCalled();
  });
});
