package com.mmcdole.kivo.core.play

import co.touchlab.skie.configuration.annotations.DefaultArgumentInterop

/**
 * What to play, cast or download: a work, with the version and tracks
 * chosen for it, each null for the server's choice. Where it starts isn't
 * part of it: the same choice plays on at another position on a TV, and
 * a download has none.
 */
data class PlayChoice
@DefaultArgumentInterop.Enabled
constructor(
    val workId: String,
    val versionId: String? = null,
    val audioTrackId: String? = null,
    /** A subtitle track's ID, or `off`. */
    val subtitleTrackId: String? = null,
)
