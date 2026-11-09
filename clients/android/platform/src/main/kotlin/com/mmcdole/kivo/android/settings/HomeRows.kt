package com.mmcdole.kivo.android.settings

import com.mmcdole.kivo.api.models.HomeRowInfo

/**
 * [all] with the row [id] swapped with the next one up ([by] -1) or down (1)
 * among [listed], the rows the editor lists, the others staying where they
 * are; [all] itself when there's none.
 */
fun movedRow(all: List<HomeRowInfo>, listed: List<HomeRowInfo>, id: String, by: Int): List<HomeRowInfo> {
    val from = listed.indexOfFirst { it.id == id }
    if (from < 0) return all
    val other = listed.getOrNull(from + by) ?: return all
    val a = all.indexOf(listed[from])
    val b = all.indexOf(other)
    return all.toMutableList().apply {
        this[a] = other
        this[b] = listed[from]
    }
}
