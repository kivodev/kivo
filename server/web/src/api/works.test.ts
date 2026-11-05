import { QueryClient } from "@tanstack/react-query";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/server";
import { apiPaths } from "./paths";
import type { WorkSummary } from "./schemas";
import { applyUserState, gridPageSize, gridPages, workQueries } from "./works";

const lib = "0190a000-0000-7000-8000-000000000001";
const other = "0190a000-0000-7000-8000-000000000002";
const work = "0190a000-0000-7000-8000-000000000003";

describe("a library grid's pages", () => {
  it("are those of the items from start to end, none past the last item", () => {
    expect(gridPages(0, 36, 2000)).toEqual([]);
    expect(gridPages(150, 420, 2000)).toEqual([1, 2, 3, 4]);
    // At the end, a screen past the last row reaches past the total.
    expect(gridPages(1980, 2040, 2000)).toEqual([19]);
    expect(gridPages(1992, 4000, 2000)).toEqual([19]);
    expect(gridPages(2000, 2100, 2000)).toEqual([]);
    expect(gridPages(40, 2000, 44)).toEqual([]);
  });

  it("are each read at their offset", async () => {
    const items = Array.from({ length: gridPageSize }, () => ({}) as WorkSummary);
    const asked: string[] = [];
    server.use(
      http.get("*/api/v1/libraries/:id/works", ({ request }) => {
        const query = new URL(request.url).searchParams;
        asked.push(`offset=${query.get("offset")}`);
        return HttpResponse.json({ items, total: 1000 });
      }),
    );
    const queryClient = new QueryClient();
    const page = (i: number) => workQueries.browsePage(lib, {}, i);

    await queryClient.fetchQuery(page(3));
    await queryClient.fetchQuery(page(0));
    expect(asked).toEqual([`offset=${3 * gridPageSize}`, "offset=null"]);
  });
});

describe("applyUserState", () => {
  const card = (id: string, kind: WorkSummary["kind"] = "movie"): WorkSummary => ({
    id,
    kind,
    title: id,
    matchState: "matched",
    available: true,
    images: {},
    userState: { played: false, favorite: false, playCount: 0 },
  });
  const played = { played: true, favorite: false, playCount: 1 };

  it("updates the work's detail and its cards in every list in place, and refetches only Home and Favorites", () => {
    const qc = new QueryClient();
    const detailKey = ["get", apiPaths.work, { params: { path: { id: work } } }];
    const searchKey = ["get", apiPaths.search, { params: { query: { q: "x" } } }];
    const pageKey = ["get", apiPaths.libraryWorks, { params: { path: { id: lib }, query: {} } }, "page", 1];
    qc.setQueryData(detailKey, { ...card(work), genres: [] });
    qc.setQueryData(searchKey, { items: [card(work), card(other)] });
    qc.setQueryData(pageKey, { items: [card(other), card(work)] });
    const invalidate = vi.spyOn(qc, "invalidateQueries");

    applyUserState(qc, work, played);

    expect(qc.getQueryData<WorkSummary>(detailKey)?.userState).toEqual(played);
    const search = qc.getQueryData<{ items: WorkSummary[] }>(searchKey);
    expect(search?.items.map((w) => w.userState?.played)).toEqual([true, false]);
    const page = qc.getQueryData<{ items: WorkSummary[] }>(pageKey);
    expect(page?.items.map((w) => w.userState?.played)).toEqual([false, true]);
    // Home's rows follow what was watched. A work marked or unmarked joins
    // or leaves Favorites.
    expect(invalidate.mock.calls).toEqual([
      [{ queryKey: ["get", apiPaths.home] }],
      [{ queryKey: ["get", apiPaths.favorites] }],
    ]);
  });

  it("refetches the views of a series' episodes, which marking it cascades to", () => {
    const qc = new QueryClient();
    qc.setQueryData(["get", apiPaths.work, { params: { path: { id: work } } }], card(work, "series"));
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    applyUserState(qc, work, played);
    expect(invalidate).toHaveBeenCalled();
  });
});
