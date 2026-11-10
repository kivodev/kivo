import { HttpResponse, ws } from "msw";
import { createOpenApiHttp } from "openapi-msw";
import { apiPaths } from "../paths";
import type { paths } from "../schema";
import type {
  EventMessage,
  HomeRowInfo,
  LetterStart,
  PlayCreate,
  PlayStarted,
  PlayTrack,
  Problem,
  Rule,
  WorkDetail,
  WorkList,
  WorkQuery,
} from "../schemas";
import { type MockApi, type MockList, summaryOf } from "./api";
import { mockId } from "./ids";

/** MSW's handlers, typed by the spec, for tests that answer a request themselves. */
export const http = createOpenApiHttp<paths>({ baseUrl: "*" });

/** The rows of Home's layout, the mock's stored ones or every row shown. */
function homeLayout(api: MockApi): HomeRowInfo[] {
  return (
    api.homeLayout ??
    api.home.rows.map(({ id, kind, library, list }) => ({
      id,
      kind,
      ...(library ? { library } : {}),
      ...(list ? { list } : {}),
      shown: true,
    }))
  );
}

export function problem(status: number, code: Problem["code"], extra: Partial<Problem> = {}): HttpResponse<Problem> {
  return HttpResponse.json(
    { type: "about:blank", title: code, status, code, requestId: "mock", ...extra },
    { status, headers: { "Content-Type": "application/problem+json" } },
  );
}

const notFound = () => problem(404, "notFound");
const unauthenticated = () => problem(401, "unauthenticated");

/**
 * A list as the API answers, its count from the works it holds, and with
 * `counts` its first three posters. A smart list is counted only with `counts`.
 */
function workListOf(api: MockApi, { workIds, ...list }: MockList, counts = false): WorkList {
  const covers = workIds.slice(0, 3).flatMap((id) => api.works[id]?.images.poster ?? []);
  const workCount = list.kind === "smart" && !counts ? 0 : workIds.length;
  return { ...list, workCount, ...(counts ? { covers } : {}) };
}

/** A page's `total` where only the first page (offset 0) has it. */
const firstPageTotal = (start: number, total: number) => (start === 0 ? total : 0);

/** Whether a work passes a rule. Fields without work data here pass every work. */
function matchesRule(work: WorkDetail, { field, op, values = [] }: Rule): boolean {
  switch (field) {
    case "genre":
      return op === "noneOf"
        ? !values.some((v) => work.genres.includes(v))
        : values.some((v) => work.genres.includes(v));
    case "year": {
      const year = work.year ?? 0;
      const [first = 0, second = 0] = values.map(Number);
      if (op === "between") return year >= first && year <= second;
      if (op === "gte") return year >= first;
      return op === "lte" ? year <= first : year === first;
    }
    case "title":
      return work.title.toLowerCase().includes((values[0] ?? "").toLowerCase());
    default:
      return true;
  }
}

/** The mock's works a query selects, by the root's rules and its groups' as AND. */
function worksMatching(api: MockApi, query: WorkQuery): WorkDetail[] {
  const groups = [query.where, ...(query.where?.groups ?? [])];
  return Object.values(api.works).filter(
    (work) =>
      query.kinds.includes(work.kind) &&
      groups.every((group) => (group?.rules ?? []).every((rule) => matchesRule(work, rule))),
  );
}

/** The token an app session signed in with a password gets. */
const mockAppToken = "mock-app-token";

const playlist = (lines: string[]) =>
  new HttpResponse(["#EXTM3U", "#EXT-X-VERSION:7", ...lines, ""].join("\n"), {
    headers: { "Content-Type": "application/vnd.apple.mpegurl" },
  });
const cue = "WEBVTT\n\n00:00:00.000 --> 99:00:00.000\nA mock subtitle\n";

/**
 * Where each letter starts in a grid, by the first character of each name.
 * Letters are A to Z, with # for anything else.
 */
function lettersOf(names: string[]): LetterStart[] {
  const letters: LetterStart[] = [];
  names.forEach((name, offset) => {
    const first = name.charAt(0).toUpperCase();
    const letter = first >= "A" && first <= "Z" ? first : "#";
    if (!letters.some((l) => l.letter === letter)) letters.push({ letter, offset });
  });
  return letters;
}

/** A grid page's items, from offset up to limit, in a grid of total items. */
function pageOf(offset: string | null, limit: string | null, total: number): { start: number; end: number } {
  const start = Number(offset ?? 0);
  return { start, end: start + Number(limit ?? total) };
}

/**
 * A play as the server would start it for `body`: at the position asked
 * for, with the audio and subtitle asked for. The audio asked for plays
 * from the file and the others take a new play, as for a browser that
 * can't switch inside a file. A subtitle the play has no delivery for
 * comes burned in, as the server burns in an image subtitle, and then
 * every other subtitle choice takes a new play.
 */
function asAsked(play: PlayStarted, body: PlayCreate): PlayStarted {
  const selected = (type: PlayTrack["type"]) => play.tracks.find((t) => t.type === type && t.selected);
  const audio = body.audioTrackId ?? selected("audio")?.trackId;
  const subtitle = body.subtitleTrackId ?? selected("subtitle")?.trackId ?? "off";
  const burned = play.tracks.some((t) => t.type === "subtitle" && t.trackId === subtitle && !t.delivery);
  const fileAudio = play.tracks.find((t) => t.type === "audio" && t.delivery === "file");
  const tracks = play.tracks.map((t): PlayTrack => {
    const { selected: _, ...unselected } = t;
    if (t.type === "audio") {
      if (t.trackId !== audio) return { trackId: t.trackId, type: t.type };
      return { ...t, selected: true, delivery: "file", codec: t.codec ?? fileAudio?.codec, index: fileAudio?.index };
    }
    if (t.type !== "subtitle") return t;
    if (t.trackId === subtitle) return { ...t, selected: true, delivery: burned ? "burn" : t.delivery };
    return burned ? { trackId: t.trackId, type: t.type } : unselected;
  });
  return { ...play, startMs: body.positionMs ?? play.startMs, tracks };
}

/**
 * Handlers that answer every route from api as it is at the request. They
 * change nothing. A request that would change the server answers what api
 * holds, and a test that needs the change made sets it on api (see
 * test/server.ts).
 */
export function createHandlers(api: MockApi) {
  return [
    http.get("/api/v1/setup", ({ response }) => response(200).json(api.setup)),
    http.post("/api/v1/setup", ({ response }) => response(201).json(api.session)),
    // An app session, as the packaged TV app asks for, answers with a token.
    http.post("/api/v1/sessions", async ({ request, response }) => {
      const { kind } = await request.json();
      if (kind !== "app") return response(201).json(api.session);
      return response(201).json({ ...api.session, kind, token: api.session.token ?? mockAppToken });
    }),
    http.get("/api/v1/sessions/current", ({ response }) =>
      api.signedIn ? response(200).json(api.session) : unauthenticated(),
    ),
    http.delete("/api/v1/sessions/current", ({ response }) => response(204).empty()),
    http.get("/api/v1/sessions", ({ response }) => response(200).json({ items: api.sessions })),
    http.delete("/api/v1/sessions", ({ response }) => response(204).empty()),
    http.get("/api/v1/sessions/{id}", ({ params, response }) => {
      const session = api.sessions.find((s) => s.id === params.id);
      return session ? response(200).json(session) : notFound();
    }),
    http.patch("/api/v1/sessions/{id}", async ({ params, request, response }) => {
      const session = api.sessions.find((s) => s.id === params.id);
      if (!session) return notFound();
      session.name = (await request.json()).name.trim();
      return response(200).json(session);
    }),
    http.delete("/api/v1/sessions/{id}", ({ params, response }) =>
      api.sessions.some((s) => s.id === params.id) ? response(204).empty() : notFound(),
    ),
    http.get("/api/v1/api-keys", ({ response }) => response(200).json({ items: api.apiKeys })),
    http.post("/api/v1/api-keys", ({ response }) => response(201).json(api.createdApiKey)),
    http.get("/api/v1/api-keys/{id}", ({ params, response }) => {
      const key = api.apiKeys.find((k) => k.id === params.id);
      return key ? response(200).json(key) : notFound();
    }),
    http.delete("/api/v1/api-keys/{id}", ({ params, response }) =>
      api.apiKeys.some((k) => k.id === params.id) ? response(204).empty() : notFound(),
    ),
    http.get("/api/v1/device-codes/{code}", ({ response }) =>
      api.waitingDevice ? response(200).json(api.waitingDevice) : notFound(),
    ),
    http.post("/api/v1/device-codes/{code}/approve", ({ response }) =>
      api.waitingDevice ? response(204).empty() : notFound(),
    ),
    http.post("/api/v1/device-codes/{code}/deny", ({ response }) =>
      api.waitingDevice ? response(204).empty() : notFound(),
    ),
    http.post("/api/v1/device-codes", ({ response }) =>
      response(201).json({
        code: "K7Q2M9",
        secret: "secret",
        expiresAt: new Date(Date.now() + 5 * 60_000).toISOString(),
        interval: 2,
        linkUrl: "http://192.168.1.10:8484/link?code=K7Q2M9",
      }),
    ),
    http.post("/api/v1/device-codes/poll", ({ response }) => {
      switch (api.deviceCodePoll) {
        case "approved":
          api.signedIn = true;
          return response(201).json(api.session);
        case "denied":
          api.deviceCodePoll = "waiting";
          return problem(403, "denied");
        default:
          return response(202).empty();
      }
    }),
    http.get("/api/v1/users/me", ({ response }) =>
      api.signedIn ? response(200).json(api.currentUser) : unauthenticated(),
    ),
    http.patch("/api/v1/users/me", async ({ request, response }) => {
      const { playback } = await request.json();
      if (playback) Object.assign(api.currentUser.playback, playback);
      return response(200).json(api.currentUser);
    }),
    http.post("/api/v1/users/me/change-password", ({ response }) => response(204).empty()),
    http.get("/api/v1/users", ({ response }) => response(200).json({ items: api.users })),
    http.post("/api/v1/users", ({ response }) => {
      const user = api.users.at(-1);
      return user ? response(201).json(user) : notFound();
    }),
    http.get("/api/v1/users/{id}", ({ params, response }) => {
      const user = api.users.find((u) => u.id === params.id);
      return user ? response(200).json(user) : notFound();
    }),
    http.patch("/api/v1/users/{id}", async ({ params, request, response }) => {
      const user = api.users.find((u) => u.id === params.id);
      if (!user) return notFound();
      Object.assign(user, await request.json());
      return response(200).json(user);
    }),
    http.delete("/api/v1/users/{id}", ({ response }) => response(204).empty()),
    http.post("/api/v1/users/{id}/set-password", ({ response }) => response(204).empty()),
    http.get("/api/v1/invites", ({ response }) => response(200).json({ items: api.invites })),
    http.post("/api/v1/invites", ({ response }) => response(201).json(api.createdInvite)),
    http.delete("/api/v1/invites/{id}", ({ response }) => response(204).empty()),
    http.get("/api/v1/invite-links/{token}", ({ response }) =>
      api.inviteLink ? response(200).json(api.inviteLink) : notFound(),
    ),
    // An invite link makes one user and signs them in.
    http.post("/api/v1/invite-links/{token}/accept", ({ response }) => {
      if (!api.inviteLink) return notFound();
      delete api.inviteLink;
      api.signedIn = true;
      return response(201).json(api.session);
    }),
    http.get("/api/v1/content-ratings", ({ response }) => response(200).json({ items: api.contentRatings })),
    http.get("/api/v1/server", ({ response }) => response(200).json(api.server)),
    http.get("/api/v1/settings", ({ response }) => response(200).json(api.settings)),
    http.patch("/api/v1/settings", ({ response }) => response(200).json(api.settings)),
    http.get("/api/v1/directories", ({ query, response }) => {
      const items = api.directories[query.get("path") ?? ""];
      return items ? response(200).json({ items }) : notFound();
    }),

    http.get("/api/v1/libraries", ({ response }) => response(200).json({ items: api.libraries })),
    http.get("/api/v1/libraries/{id}", ({ params, response }) => {
      const library = api.libraries.find((l) => l.id === params.id);
      return library ? response(200).json(library) : notFound();
    }),
    http.get("/api/v1/libraries/{id}/facets", ({ params, response }) => {
      const facets = api.facets[params.id];
      return facets ? response(200).json(facets) : notFound();
    }),
    http.post("/api/v1/libraries", ({ response }) => {
      const library = api.libraries.at(-1);
      return library ? response(201).json(library) : notFound();
    }),
    http.patch("/api/v1/libraries/{id}", ({ params, response }) => {
      const library = api.libraries.find((l) => l.id === params.id);
      return library ? response(200).json(library) : notFound();
    }),
    http.delete("/api/v1/libraries/{id}", ({ response }) => response(202).json({})),
    http.post("/api/v1/libraries/{id}/scan", ({ response }) => response(202).json({})),
    http.post("/api/v1/libraries/{id}/stop-scan", ({ params, response }) =>
      api.libraries.some((l) => l.id === params.id) ? response(204).empty() : notFound(),
    ),
    http.post("/api/v1/libraries/{id}/refresh-metadata", ({ response }) => response(202).json({})),

    // A list row holds what the list holds, most recently added first.
    http.get("/api/v1/home", ({ response }) =>
      response(200).json({
        ...api.home,
        rows: api.home.rows.map((row) => {
          const list = row.list && api.lists.find((l) => l.id === row.list?.id);
          return list
            ? { ...row, items: list.workIds.flatMap((id) => (api.works[id] ? [summaryOf(api.works[id])] : [])) }
            : row;
        }),
      }),
    ),
    http.get("/api/v1/home/rows", ({ response }) => response(200).json({ items: homeLayout(api) })),
    http.patch("/api/v1/home/rows", async ({ request, response }) => {
      const byId = new Map(homeLayout(api).map((r) => [r.id, r]));
      api.homeLayout = (await request.json()).items.flatMap(({ id, shown }) => {
        const row = byId.get(id);
        return row ? [{ ...row, shown }] : [];
      });
      return response(200).json({ items: homeLayout(api) });
    }),
    http.post("/api/v1/home/rows/reset", ({ response }) => {
      api.homeLayout = undefined;
      return response(200).json({ items: homeLayout(api) });
    }),
    // My List first, then by name.
    http.get("/api/v1/lists", ({ query, response }) => {
      const sorted = [...api.lists].sort(
        (a, b) => Number(b.builtIn) - Number(a.builtIn) || a.name.localeCompare(b.name),
      );
      return response(200).json({ items: sorted.map((list) => workListOf(api, list, query.get("counts") === "true")) });
    }),
    http.post("/api/v1/lists", async ({ request, response }) => {
      const { name, smart } = await request.json();
      if (api.lists.some((l) => l.name.toLowerCase() === name.toLowerCase())) return problem(409, "nameTaken");
      if (api.lists.length >= 100) return problem(409, "conflict");
      const list: MockList = smart
        ? {
            id: mockId(9600 + api.lists.length),
            name,
            kind: "smart",
            builtIn: false,
            smart: { query: { ...smart.query, version: 1 } },
            workIds: worksMatching(api, smart.query).map((w) => w.id),
          }
        : { id: mockId(9600 + api.lists.length), name, kind: "manual", builtIn: false, workIds: [] };
      api.lists.push(list);
      return response(201).json(workListOf(api, list));
    }),
    http.get("/api/v1/lists/{id}", ({ params, response }) => {
      const list = api.lists.find((l) => l.id === params.id);
      return list ? response(200).json(workListOf(api, list)) : notFound();
    }),
    http.patch("/api/v1/lists/{id}", async ({ params, request, response }) => {
      const list = api.lists.find((l) => l.id === params.id);
      if (!list) return notFound();
      if (list.builtIn) return problem(409, "conflict");
      const { name, smart } = await request.json();
      if (name !== undefined) {
        if (api.lists.some((l) => l !== list && l.name.toLowerCase() === name.toLowerCase())) {
          return problem(409, "nameTaken");
        }
        list.name = name;
      }
      if (smart) {
        list.smart = { query: { ...smart.query, version: 1 } };
        list.workIds = worksMatching(api, smart.query).map((w) => w.id);
      }
      return response(200).json(workListOf(api, list));
    }),
    http.delete("/api/v1/lists/{id}", ({ params, response }) => {
      const list = api.lists.find((l) => l.id === params.id);
      if (!list) return notFound();
      if (list.builtIn) return problem(409, "conflict");
      api.lists = api.lists.filter((l) => l !== list);
      return response(204).empty();
    }),
    http.get("/api/v1/work-fields", ({ response }) => response(200).json({ items: api.workFields })),
    http.get("/api/v1/work-fields/{field}/values", ({ params, query, response }) => {
      const values = api.workFieldValues[params.field] ?? [];
      const ids = query.getAll("ids");
      const q = (query.get("q") ?? "").toLowerCase();
      const items =
        ids.length > 0
          ? values.filter((v) => ids.includes(v.value))
          : values.filter((v) => v.name.toLowerCase().includes(q));
      return response(200).json({ items });
    }),
    http.post("/api/v1/works/query", async ({ request, response }) => {
      const { query, offset, limit } = await request.json();
      const matched = worksMatching(api, query).map(summaryOf);
      const { start, end } = pageOf(
        offset === undefined ? null : String(offset),
        limit === undefined ? null : String(limit),
        matched.length,
      );
      return response(200).json({ items: matched.slice(start, end), total: firstPageTotal(start, matched.length) });
    }),
    // A list's works, most recently added first, by offset.
    http.get("/api/v1/lists/{id}/works", ({ params, query, response }) => {
      const list = api.lists.find((l) => l.id === params.id);
      if (!list) return notFound();
      const items = list.workIds.flatMap((id) => (api.works[id] ? [summaryOf(api.works[id])] : []));
      const { start, end } = pageOf(query.get("offset"), query.get("limit"), items.length);
      return response(200).json({ items: items.slice(start, end), total: firstPageTotal(start, items.length) });
    }),
    http.post("/api/v1/lists/{id}/works", async ({ params, request, response }) => {
      const list = api.lists.find((l) => l.id === params.id);
      if (!list) return notFound();
      if (list.kind === "smart") return problem(409, "conflict");
      const { workId } = await request.json();
      if (!list.workIds.includes(workId)) list.workIds.unshift(workId);
      return response(204).empty();
    }),
    http.delete("/api/v1/lists/{id}/works/{workId}", ({ params, response }) => {
      const list = api.lists.find((l) => l.id === params.id);
      if (!list) return notFound();
      if (list.kind === "smart") return problem(409, "conflict");
      list.workIds = list.workIds.filter((id) => id !== params.workId);
      return response(204).empty();
    }),
    // A library's grid, by offset. The title sort has letters.
    http.get("/api/v1/libraries/{id}/works", ({ params, query, response }) => {
      const items = api.browse[params.id];
      if (!items) return notFound();
      const { start, end } = pageOf(query.get("offset"), query.get("limit"), items.length);
      const byTitle = (query.get("sort") ?? "title") === "title";
      return response(200).json({
        items: items.slice(start, end),
        total: items.length,
        letters: start === 0 && byTitle ? lettersOf(items.map((w) => w.title)) : undefined,
      });
    }),
    // A library's collections, by name, with letters.
    http.get("/api/v1/libraries/{id}/collections", ({ params, query, response }) => {
      const items = api.collections[params.id] ?? [];
      const { start, end } = pageOf(query.get("offset"), query.get("limit"), items.length);
      return response(200).json({
        items: items.slice(start, end),
        total: items.length,
        letters: start === 0 ? lettersOf(items.map((c) => c.name)) : undefined,
      });
    }),
    // An audiobooks library's authors and series, by name, with letters.
    http.get("/api/v1/libraries/{id}/authors", ({ query, response }) => {
      const { start, end } = pageOf(query.get("offset"), query.get("limit"), api.authors.length);
      return response(200).json({
        items: api.authors.slice(start, end),
        total: api.authors.length,
        letters: start === 0 ? lettersOf(api.authors.map((a) => a.name)) : undefined,
      });
    }),
    http.get("/api/v1/libraries/{id}/book-series", ({ query, response }) => {
      const { start, end } = pageOf(query.get("offset"), query.get("limit"), api.bookSeries.length);
      return response(200).json({
        items: api.bookSeries.slice(start, end),
        total: api.bookSeries.length,
        letters: start === 0 ? lettersOf(api.bookSeries.map((s) => s.name)) : undefined,
      });
    }),
    http.get("/api/v1/works/{id}/children", ({ params, response }) => {
      const items = api.children[params.id];
      return items ? response(200).json({ items }) : notFound();
    }),
    http.get("/api/v1/collections/{id}", ({ params, response }) => {
      const collection = api.collectionPages[params.id];
      return collection ? response(200).json(collection) : notFound();
    }),
    // The Unmatched list. The mock's cursor is an index.
    http.get("/api/v1/unmatched-works", ({ query, response }) => {
      const start = Number(query.get("after") ?? 0);
      const end = start + Number(query.get("limit") ?? api.unmatched.length);
      return response(200).json({
        items: api.unmatched.slice(start, end),
        next: end < api.unmatched.length ? String(end) : undefined,
      });
    }),
    http.get("/api/v1/works/{id}", ({ params, response }) => {
      const work = api.works[params.id];
      if (!work) return notFound();
      return response(200).json({
        ...work,
        listIds: api.lists.filter((l) => l.workIds.includes(work.id)).map((l) => l.id),
      });
    }),
    http.patch("/api/v1/works/{id}/user-state", async ({ params, request, response }) => {
      const work = api.works[params.id];
      if (!work?.userState) return notFound();
      const { played } = await request.json();
      work.userState = { ...work.userState, played: played ?? work.userState.played };
      // Lists hold their own copies of the work's card, and marking a
      // series or season marks what's under it.
      const cards = Object.values(api.children).flat();
      for (const card of cards) if (card.id === params.id) card.userState = work.userState;
      const mark = (id: string, played: boolean) => {
        for (const child of api.children[id] ?? []) {
          for (const w of [api.works[child.id], ...cards.filter((c) => c.id === child.id)]) {
            if (w?.userState) w.userState = { ...w.userState, played };
          }
          mark(child.id, played);
        }
      };
      if (played !== undefined && (work.kind === "series" || work.kind === "season")) mark(params.id, played);
      return response(200).json(work.userState);
    }),
    http.post("/api/v1/works/{id}/user-state/remove-from-continue", ({ params, response }) => {
      for (const row of api.home.rows) {
        if (row.kind === "continueWatching" || row.kind === "continueListening")
          row.items = row.items.filter((w) => w.id !== params.id);
      }
      return response(204).empty();
    }),
    ...(["/api/v1/works/{id}/match", "/api/v1/works/{id}/unmatch"] as const).map((path) =>
      http.post(path, ({ params, response }) => {
        const work = api.works[params.id];
        return work ? response(200).json(work) : notFound();
      }),
    ),
    http.patch("/api/v1/works/{id}", ({ params, response }) => {
      const work = api.works[params.id];
      return work ? response(200).json(work) : notFound();
    }),
    http.patch("/api/v1/works", async ({ request, response }) => {
      const edited = (await request.json()).workIds.flatMap((id) => {
        const work = api.works[id];
        return work ? [summaryOf(work)] : [];
      });
      return response(200).json({ items: edited });
    }),
    http.get("/api/v1/people/{id}", ({ params, response }) => {
      const person = api.people[params.id];
      return person ? response(200).json(person) : notFound();
    }),
    http.patch("/api/v1/people/{id}", ({ params, response }) => {
      const person = api.people[params.id];
      return person ? response(200).json(person) : notFound();
    }),
    http.get("/api/v1/book-series/{id}", ({ params, response }) => {
      const series = api.seriesPages[params.id];
      return series ? response(200).json(series) : notFound();
    }),
    http.patch("/api/v1/book-series/{id}", ({ params, response }) => {
      const series = api.seriesPages[params.id];
      return series ? response(200).json(series) : notFound();
    }),
    http.get("/api/v1/works/{id}/matches", ({ params, response }) => {
      const items = api.works[params.id]?.kind === "book" ? api.bookMatches : api.matches;
      return response(200).json({ items });
    }),
    http.post("/api/v1/works/{id}/refresh-metadata", ({ response }) => response(202).json({})),
    http.get("/api/v1/works/{id}/artwork", ({ query, response }) => {
      const items = api.artwork.filter((a) => a.type === (query.get("type") ?? "poster"));
      return response(200).json({ items });
    }),
    http.post("/api/v1/works/{id}/artwork/{artworkId}/select", ({ params, response }) => {
      const artwork = api.artwork.find((a) => a.id === params.artworkId);
      return artwork ? response(200).json(artwork) : notFound();
    }),
    http.post("/api/v1/works/{id}/artwork", ({ response }) => {
      const artwork = api.artwork[0];
      return artwork ? response(201).json(artwork) : notFound();
    }),
    http.get("/api/v1/search", ({ response }) =>
      response(200).json({ works: api.search, people: api.searchPeople, collections: api.searchCollections }),
    ),

    http.get("/api/v1/works/{id}/files", ({ params, response }) =>
      response(200).json({ items: api.files[params.id] ?? [] }),
    ),
    http.get("/api/v1/files", ({ query, response }) => {
      const libraryId = query.get("libraryId");
      const items = api.missingFiles.filter((f) => !libraryId || f.libraryId === libraryId);
      const limit = Number(query.get("limit") ?? items.length);
      return response(200).json({ items: items.slice(0, limit), total: items.length });
    }),
    http.delete("/api/v1/files/{id}", ({ response }) => response(204).empty()),
    http.delete("/api/v1/files", ({ response }) => response(204).empty()),
    http.get("/api/v1/files/{id}/subtitle-results", ({ response }) =>
      response(200).json({ items: api.subtitleResults }),
    ),
    http.post("/api/v1/files/{id}/subtitles", ({ response }) => response(200).json(api.addedSubtitle)),

    http.get("/api/v1/tasks", ({ response }) => response(200).json({ items: api.tasks })),
    http.post("/api/v1/tasks/{task}/run", ({ params, response }) => {
      const task = api.tasks.find((t) => t.id === params.task && t.section !== "onDemand");
      return task ? response(204).empty() : notFound();
    }),
    // Stop ends the task's run, and none of its work is left in flight.
    http.post("/api/v1/tasks/{task}/stop", ({ params, response }) => {
      const task = api.tasks.find((t) => t.id === params.task);
      if (!task) return notFound();
      Object.assign(task, { running: false, stoppable: false });
      delete task.progress;
      return response(204).empty();
    }),
    http.get("/api/v1/tasks/{task}/failures", ({ params, response }) => {
      const items = api.taskFailures[params.task];
      return items ? response(200).json({ items }) : notFound();
    }),
    http.post("/api/v1/tasks/{task}/retry", ({ params, response }) =>
      api.taskFailures[params.task] ? response(204).empty() : notFound(),
    ),
    http.get("/api/v1/problems", ({ response }) => response(200).json({ items: api.problems })),
    http.delete("/api/v1/problems/{id}", ({ response }) => response(204).empty()),
    // The cursor is the last event's ID.
    http.get("/api/v1/activity", ({ query, response }) => {
      const group = query.get("group");
      const kinds: Record<string, string[]> = {
        plays: ["playStarted", "playFinished"],
        added: ["added"],
        signIns: ["signedIn", "signInFailed", "userJoined", "passwordChanged"],
        server: ["taskEnded", "problemRaised"],
      };
      const all = api.activity.filter((e) => !group || kinds[group]?.includes(e.kind));
      const after = query.get("after");
      const start = after ? all.findIndex((e) => e.id === after) + 1 : 0;
      const items = all.slice(start, start + Number(query.get("limit") ?? 50));
      const last = items.at(-1);
      return response(200).json({ items, next: last && start + items.length < all.length ? last.id : undefined });
    }),
    http.get("/api/v1/logs", ({ response }) => response(200).json({ items: api.logs, total: api.logs.length })),
    http.get("/api/v1/logs/file", ({ response }) =>
      response(200).text(api.logs.map((l) => `level=${l.level} msg="${l.message}"\n`).join(""), {
        headers: { "Content-Disposition": 'attachment; filename="kivo-log.txt"' },
      }),
    ),
    http.get("/api/v1/accelerators", ({ response }) => response(200).json(api.accelerators)),

    http.post("/api/v1/plays", async ({ request, response }) => {
      const body = await request.json();
      const play = api.starts[body.workId];
      if (!play) return notFound();
      // A play boosts dialogue where it can when asked.
      const boost = play.dialogueBoost && { ...play.dialogueBoost, enabled: body.dialogueBoost === true };
      return response(201).json({ ...(api.playsAsAsked ? asAsked(play, body) : play), dialogueBoost: boost });
    }),
    http.post("/api/v1/plays/{id}/fallback", ({ response }) => response(201).json(api.fallback)),
    http.patch("/api/v1/plays/{id}", ({ response }) => response(204).empty()),
    http.post("/api/v1/plays/{id}/stop", ({ response }) => response(204).empty()),
    // The mock's client is on the local network, so there's nothing to
    // measure.
    http.get("/api/v1/bandwidth-test", ({ response }) => response(204).empty()),
    http.get("/api/v1/plays", ({ response }) => response(200).json({ items: api.plays })),
    http.get("/api/v1/plays/{id}", ({ params, response }) => {
      const play = api.plays.find((s) => s.id === params.id);
      return play ? response(200).json(play) : notFound();
    }),
    http.get("/api/v1/users/me/play-events", ({ response }) => response(200).json({ items: api.history })),
    http.get("/api/v1/play-events", ({ response }) => response(200).json({ items: api.history })),

    // Media, which hand-written handlers serve on the real server. This is
    // a playlist of one empty segment, a cue, and empty files. It's enough
    // for tests, whose media element is a stand-in. In `npm run dev:mock`
    // playback fails, unless ?mock=playable plays a real file instead.
    http.untyped.get("*/api/v1/plays/:id/hls/:name", ({ params }) => {
      const name = String(params.name);
      if (name === "index.m3u8") return playlist(["#EXT-X-STREAM-INF:BANDWIDTH=8000000", "main.m3u8"]);
      if (name.endsWith(".m3u8")) {
        return playlist([
          "#EXT-X-TARGETDURATION:6",
          '#EXT-X-MAP:URI="init.mp4"',
          "#EXTINF:6.000,",
          "0.m4s",
          "#EXT-X-ENDLIST",
        ]);
      }
      return new HttpResponse(new Uint8Array(0), { headers: { "Content-Type": "video/mp4" } });
    }),
    http.untyped.get("*/api/v1/plays/:id/subtitles/:name", () =>
      HttpResponse.text(cue, { headers: { "Content-Type": "text/vtt" } }),
    ),
    http.untyped.get(
      "*/api/v1/plays/:id/files/:part",
      () => new HttpResponse(new Uint8Array(0), { headers: { "Content-Type": "application/octet-stream" } }),
    ),
    // Each image is a muted gradient of its own hue, so artwork can be told
    // apart.
    http.untyped.get("*/api/v1/images/:id", ({ params }) => {
      let hue = 0;
      for (const c of String(params.id)) hue = (hue * 31 + c.charCodeAt(0)) % 360;
      return new HttpResponse(
        `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 3" preserveAspectRatio="none"><linearGradient id="g" x2="1" y2="1"><stop stop-color="hsl(${hue} 30% 38%)"/><stop offset="1" stop-color="hsl(${(hue + 50) % 360} 35% 16%)"/></linearGradient><rect width="2" height="3" fill="url(#g)"/></svg>`,
        { headers: { "Content-Type": "image/svg+xml" } },
      );
    }),
  ];
}

/**
 * The live-events socket. It sends the snapshot on connect, and tests send
 * the events. It is the events path at any server, as the API's handlers
 * are: the packaged app's socket goes to the address it keeps, which it has
 * none of yet when this module loads.
 */
const events = ws.link(`*${apiPaths.events}`);

export function sendEvent(event: EventMessage): void {
  events.broadcast(JSON.stringify(event));
}

/** Closes the open sockets with a close code, as the server would. Returns how many it closed. */
export function closeEvents(code: number): number {
  for (const client of events.clients) client.close(code);
  return events.clients.size;
}

export function createEventsHandler(api: MockApi) {
  return events.addEventListener("connection", ({ client }) => client.send(JSON.stringify(api.snapshot)));
}
