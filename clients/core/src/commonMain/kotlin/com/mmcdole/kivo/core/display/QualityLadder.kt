package com.mmcdole.kivo.core.display

import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.core.settings.Quality

/** One choice of the quality menu; [rung] is null for Auto and Original. [Display.quality] words it. */
data class QualityOption(val quality: Quality, val rung: QualityRung?)

/**
 * The quality menu: Auto, Original, and the server's fixed ladder below
 * them ([rungs], highest first), each rung with the height a transcode at
 * it is sent at. The ladder comes from `GET /api/v1/server`, so every client
 * offers the same one.
 */
fun qualityMenu(rungs: List<QualityRung>): List<QualityOption> =
    listOf(QualityOption(Quality.Auto, null), QualityOption(Quality.Original, null)) +
        rungs.map { QualityOption(Quality.Limit(it.bitrateBps), it) }
