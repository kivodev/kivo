package com.mmcdole.kivo.android

import com.mmcdole.kivo.core.data.GridSort

// Migration (api-naming): remove after every client has updated past this release.
/** What an earlier version of this app saved under names the server has since changed, read as the names it uses now. */
object ApiNamingMigration {
    // Migration (api-naming): remove after every client has updated past this release.
    /** A library's saved grid, its filter of dynamic ranges once saved as `hdr` and its last-episode sort as `lastEpisodeAdded`. */
    fun savedGrid(values: Map<String, String>): Map<String, String> {
        val out = values.toMutableMap()
        out.remove("hdr")?.let { out.putIfAbsent("dynamicRange", it) }
        if (out["sort"] == "lastEpisodeAdded") out["sort"] = GridSort.LastEpisodeAddedAt.value
        return out
    }
}
