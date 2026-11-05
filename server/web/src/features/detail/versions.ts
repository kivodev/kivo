import type { Version, WorkDetail } from "@/api/schemas";

type Versions = Pick<WorkDetail, "versions" | "userState">;

/**
 * The version a work's page selects before any choice: the one the user
 * last played while the work still has it, else the first. The server
 * lists versions in order and plays this same version when a play names
 * none.
 */
export function defaultVersion(work: Versions): Version | undefined {
  return work.versions.find((v) => v.id === work.userState?.versionId) ?? work.versions[0];
}

/** The version chosen in the page's URL, else the default one. */
export function selectedVersion(work: Versions, chosen: string | undefined): Version | undefined {
  return work.versions.find((v) => v.id === chosen) ?? defaultVersion(work);
}
