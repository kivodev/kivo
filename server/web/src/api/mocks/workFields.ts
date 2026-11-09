// The mock server's field catalog and field values.
import type { WorkField, WorkFieldValue, WorkKind } from "../schemas";

const all: WorkKind[] = ["movie", "series", "season", "episode", "video", "book"];
const video: WorkKind[] = ["movie", "series", "season", "episode", "video"];
const versioned: WorkKind[] = ["movie", "episode", "video"];
const books: WorkKind[] = ["book"];

const manyOps = [
  { op: "anyOf", arity: -1 },
  { op: "allOf", arity: -1 },
  { op: "noneOf", arity: -1 },
  { op: "isSet", arity: 0 },
  { op: "isNotSet", arity: 0 },
];
const numberOps = [
  { op: "eq", arity: 1 },
  { op: "gte", arity: 1 },
  { op: "lte", arity: 1 },
  { op: "between", arity: 2 },
  { op: "isSet", arity: 0 },
  { op: "isNotSet", arity: 0 },
];
const dateOps = [
  { op: "gte", arity: 1 },
  { op: "lte", arity: 1 },
  { op: "between", arity: 2 },
  { op: "within", arity: 1 },
  { op: "notWithin", arity: 1 },
  { op: "isSet", arity: 0 },
  { op: "isNotSet", arity: 0 },
];

function choice(field: string, name: string, kinds: WorkKind[]): WorkField {
  return {
    field,
    name,
    type: "choice",
    ops: [
      { op: "anyOf", arity: -1 },
      { op: "noneOf", arity: -1 },
    ],
    kinds,
    sortable: false,
    valueSource: "inline",
  };
}

function set(field: string, name: string, kinds: WorkKind[], valueSource: "inline" | "search"): WorkField {
  return { field, name, type: "set", ops: manyOps, kinds, sortable: false, valueSource };
}

function person(field: string, name: string, kinds: WorkKind[]): WorkField {
  return { field, name, type: "person", ops: manyOps, kinds, sortable: false, valueSource: "search" };
}

function number(field: string, name: string, defaultDirection: "asc" | "desc", unit?: string): WorkField {
  return {
    field,
    name,
    type: "number",
    ops: numberOps,
    kinds: all,
    sortable: true,
    defaultDirection,
    ...(unit ? { unit } : {}),
  };
}

function date(field: string, name: string): WorkField {
  return { field, name, type: "date", ops: dateOps, kinds: all, sortable: true, defaultDirection: "desc" };
}

export const mockWorkFields: WorkField[] = [
  choice("library", "Library", all),
  choice("contentRating", "Content rating", all),
  choice("playState", "Play state", all),
  set("genre", "Genre", all, "inline"),
  set("resolution", "Resolution", versioned, "inline"),
  set("dynamicRange", "Dynamic range", versioned, "inline"),
  set("collection", "Collection", ["movie", "series", "video"], "search"),
  set("inList", "In list", all, "inline"),
  person("actor", "Actor", video),
  person("director", "Director", video),
  person("author", "Author", books),
  person("narrator", "Narrator", books),
  number("rating", "Rating", "desc"),
  number("runtimeMs", "Runtime", "asc", "ms"),
  number("year", "Year", "desc"),
  date("addedAt", "Date added"),
  date("releaseDate", "Release date"),
  {
    field: "title",
    name: "Title",
    type: "text",
    ops: [
      { op: "contains", arity: 1 },
      { op: "notContains", arity: 1 },
      { op: "startsWith", arity: 1 },
    ],
    kinds: all,
    sortable: true,
    defaultDirection: "asc",
  },
];

const value = (name: string): WorkFieldValue => ({ value: name, name });

/** The values of the fields that choose from a list, by field. */
export function mockFieldValues(ids: {
  movies: string;
  shows: string;
  videos: string;
  villeneuve: string;
  dateNight: string;
}): Record<string, WorkFieldValue[]> {
  return {
    library: [
      { value: ids.movies, name: "Movies" },
      { value: ids.shows, name: "Shows" },
      { value: ids.videos, name: "Home Videos" },
    ],
    contentRating: ["G", "PG", "PG-13", "R"].map(value),
    playState: [
      { value: "unplayed", name: "Unplayed" },
      { value: "inProgress", name: "In progress" },
      { value: "played", name: "Played" },
    ],
    genre: ["Adventure", "Animation", "Comedy", "Drama", "Science Fiction", "Thriller"].map(value),
    resolution: [
      { value: "4k", name: "4K" },
      { value: "1080p", name: "1080p" },
      { value: "720p", name: "720p" },
      { value: "sd", name: "SD" },
    ],
    dynamicRange: [
      { value: "hdr", name: "HDR" },
      { value: "dolbyVision", name: "Dolby Vision" },
    ],
    inList: [{ value: ids.dateNight, name: "Date Night" }],
    director: [{ value: ids.villeneuve, name: "Denis Villeneuve" }],
    actor: [
      { value: "actor-chalamet", name: "Timothée Chalamet" },
      { value: "actor-zendaya", name: "Zendaya" },
    ],
  };
}
