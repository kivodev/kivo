package com.mmcdole.kivo.core.settings

import co.touchlab.skie.configuration.annotations.DefaultArgumentInterop
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.api.models.QualityRung

/**
 * A quality setting: Auto, Original or a rung of the server's ladder. A
 * class rather than an interface, so Swift compares and hashes it as a value.
 */
sealed class Quality {
    /** Chosen when playback starts, from the limits and the bandwidth last measured. */
    data object Auto : Quality()

    /** The file as it is, with no cap. */
    data object Original : Quality()

    /** A rung of the ladder, by its bitrate. */
    data class Limit(val maxBitrateBps: Long) : Quality()

    /** The quality as stored and as a menu's choice: `auto`, `original` or a rung's bitrate. */
    val key: String
        get() = when (this) {
            Auto -> "auto"
            Original -> "original"
            is Limit -> maxBitrateBps.toString()
        }

    /** As a play request sends it; Auto with the last measured bandwidth, when there is one. */
    @DefaultArgumentInterop.Enabled
    fun request(bandwidthBps: Long? = null): QualityRequest = when (this) {
        Auto -> QualityRequest("auto", bandwidthBps = bandwidthBps)
        Original -> QualityRequest("original")
        is Limit -> QualityRequest("limit", maxBitrateBps = maxBitrateBps)
    }

    /** This quality as the server offers it: a rung its ladder ([rungs]) doesn't have is Auto. */
    fun onLadder(rungs: List<QualityRung>): Quality =
        if (this is Limit && rungs.none { it.bitrateBps == maxBitrateBps }) Auto else this

    companion object {
        /** The quality by its [key]; Auto for none or one this version doesn't know. */
        fun of(key: String?): Quality = when (key) {
            "original" -> Original
            else -> key?.toLongOrNull()?.takeIf { it > 0 }?.let(::Limit) ?: Auto
        }

        /** The quality a play request or a play says; Auto when it says none. */
        fun of(request: QualityRequest?): Quality = when (request?.mode) {
            "original" -> Original
            "limit" -> request.maxBitrateBps?.let(::Limit) ?: Auto
            else -> Auto
        }
    }
}
