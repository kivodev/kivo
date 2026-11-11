package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkSummary

private const val TRAILER = "trailer"

/**
 * The trailer files in the work's folder, which play here. A page's
 * Trailers row shows them before the movie's trailers on YouTube.
 */
fun WorkDetail.localTrailers(): List<WorkSummary> = extras.filter { it.extra?.type == TRAILER }

/** The work's extras apart from its trailer files, which its Trailers row shows. */
fun WorkDetail.otherExtras(): List<WorkSummary> = extras.filterNot { it.extra?.type == TRAILER }
