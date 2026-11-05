package com.mmcdole.kivo.core.display

import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackType
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** The words for tracks, from a locale's display table. */
@Serializable
data class TrackWords(
    val unknownLanguage: String,
    val default: String,
    /** The short words a track's name adds: "Forced", "SDH", "Commentary", "Original". */
    val flags: TrackFlags,
    /** With `{channels}` for the count, for layouts without a name. */
    val channels: String,
    /** Where a subtitle lives: `embedded`, `external`, `downloaded`. */
    val sources: Map<String, String>,
    /** Where a `downloaded` subtitle came from, in its name: "OpenSubtitles". */
    val downloadedFrom: String,
    /** A version without video, in the version picker. */
    val audioOnly: String,
)

/** The words for a track's flags, as its short name says them. */
@Serializable
data class TrackFlags(val forced: String, val hearingImpaired: String, val commentary: String, val original: String)

@Serializable
internal data class LocaleTable(
    val methods: Map<String, String>,
    val subtitleKinds: Map<String, String>,
    val reasons: Map<String, String>,
    val unknownReason: String,
    val dialogueBoostCosts: Map<String, String>,
    val tracks: TrackWords,
    val numbers: NumberWords,
    /** With `{n}` for the megabits per second. */
    val bitrate: String,
    val sizes: SizeWords,
    val durations: DurationWords,
    val episodeCode: EpisodeCodeWords,
    val quality: QualityWords,
    /** A sentence per problem code. */
    val problems: Map<String, String>,
    val unknownProblem: String,
    /** A play stopped elsewhere. */
    val playStopped: String,
    /** A play an admin stopped with a message, with `{message}` for it. */
    val adminStopped: String,
    val subtitleSearch: SubtitleSearchWords,
    val downloads: DownloadWords,
)

/** The locale's decimal and grouping separators. */
@Serializable
internal data class NumberWords(val decimal: String, val group: String)

/** Sizes, each with `{n}`. */
@Serializable
internal data class SizeWords(
    val gigabytes: String,
    val megabytes: String,
    val kilobytes: String,
)

/** Runtimes and time left, with `{hours}` and `{minutes}`. */
@Serializable
internal data class DurationWords(
    val hours: String,
    val minutes: String,
    val hoursMinutes: String,
    val minutesLeft: String,
    val hoursLeft: String,
    val hoursMinutesLeft: String,
)

/** An episode's place, with `{season}`, `{episode}` and, for a file of several, `{end}`. */
@Serializable
internal data class EpisodeCodeWords(val one: String, val range: String)

/** The quality menu's choices: Original with `{bitrate}`, a rung with `{height}` and `{bitrate}`. */
@Serializable
internal data class QualityWords(val auto: String, val original: String, val originalAt: String, val rung: String)

/** A failed subtitle search or download; `quotaUsedUntil` with `{time}`. */
@Serializable
internal data class SubtitleSearchWords(
    val notConfigured: String,
    val quotaUsed: String,
    val quotaUsedUntil: String,
    val notFound: String,
    val providerUnavailable: String,
)

/** A download's quality and size, the device's own failures, and why one couldn't start. */
@Serializable
internal data class DownloadWords(
    /** The qualities' names, by the server's name for each. */
    val qualities: Map<String, String>,
    /** The device's own reasons a download failed, by code. */
    val failures: Map<String, String>,
    /** With `{size}` for what to free. */
    val notEnoughSpace: String,
    val refused: String,
)

@Serializable
internal data class MediaTable(
    val codecs: Map<String, String>,
    val containers: Map<String, String>,
    val channels: Map<String, String>,
    val layouts: Map<String, String>,
    val resolutions: Map<String, String>,
    val hdr: Map<String, String>,
    /** A lossless or premium audio format by its codec, or its codec and profile ("truehd atmos"). */
    val premiumAudio: Map<String, String>,
)

private val json = Json { ignoreUnknownKeys = true }

internal val media: MediaTable by lazy { json.decodeFromString(MediaTable.serializer(), DisplayJson.MEDIA) }

/**
 * How playback is put in words, from the display tables the server and the
 * web client share (`shared/display`), generated into the core: methods,
 * subtitle kinds, reason texts, dialogue boost costs, track words and language
 * names per locale, and codec, container, channel, resolution and HDR names
 * for every locale. The server names tracks and versions; bitrates, sizes,
 * runtimes, episode codes, quality choices and problems are put in words
 * here ([bitrate], [problem]), so every app says them alike. Each screen's
 * own words stay in its platform's resources.
 */
class Display private constructor(internal val table: LocaleTable, internal val languages: LanguageNames) {
    val tracks: TrackWords get() = table.tracks

    /** "Direct Play", "Direct Stream" or "Transcode"; an unknown method as sent. */
    fun method(method: String): String = table.methods[method] ?: method

    /** How [play] is delivered, by its selected video, or without video its audio: "Direct Play", "Direct Stream" or "Transcode". */
    fun method(play: PlayStarted): String {
        val main = play.tracks.firstOrNull { it.type == TrackType.Video && it.selected == true }
            ?: play.tracks.firstOrNull { it.type == TrackType.Audio && it.selected == true }
        return main?.let(::method) ?: method(if (play.delivery == PlayDelivery.File) "directPlay" else "directStream")
    }

    /** How [track] of a play is delivered: Direct Play from the file, Transcode when it's encoded, else Direct Stream. */
    fun method(track: PlayTrack): String = method(
        when {
            track.delivery == TrackDelivery.File -> "directPlay"
            track.transcode != null -> "transcode"
            else -> "directStream"
        },
    )

    /** A sentence for a playback reason code; one this version doesn't know says so. */
    fun reason(code: String): String = table.reasons[code] ?: table.unknownReason

    /** What a dialogue boost costs a play, for the line under its switch: "Plays the audio as AAC while on"; null for a cost this version doesn't know. */
    fun dialogueBoostCost(cost: String): String? = table.dialogueBoostCosts[cost]

    /**
     * "Burned in", "Styled", "Image" or "Plain", for a subtitle track of a
     * play as it's delivered: burned in, ASS, a picture format, or any other
     * text.
     */
    fun subtitleKind(track: PlayTrack): String {
        val kinds = table.subtitleKinds
        val kind = when {
            track.delivery == TrackDelivery.Burn -> "burn"
            track.codec in STYLED_CODECS -> "ass"
            track.codec in IMAGE_CODECS -> "pgs"
            else -> "vtt"
        }
        return kinds.getValue(kind)
    }

    /** Where a subtitle track lives: "Embedded", "External" or "Downloaded". */
    fun subtitleSource(source: String): String? = table.tracks.sources[source]

    /** A codec as people know it: "TrueHD", "SRT". */
    fun codec(codec: String): String = media.codecs[codec] ?: codec.uppercase()

    /** A container as people know it: "matroska,webm" is MKV. */
    fun container(container: String): String {
        val first = container.substringBefore(',')
        return media.containers[first] ?: first.uppercase()
    }

    /** "Stereo", "5.1", "7.1", or the count in words, for a track that names no layout. */
    fun channels(channels: Int): String =
        media.channels[channels.toString()] ?: table.tracks.channels.replace("{channels}", channels.toString())

    /** A channel layout as the server sends it ("2.0", "5.1"; "1.0" is "Mono"); else the [count] in words. */
    fun channels(layout: String?, count: Int): String = layout?.let { media.layouts[it] ?: it } ?: channels(count)

    /** A resolution class as the server sends it, in words: `4k` is "4K", `sd` "SD". */
    fun resolution(resolution: String): String = media.resolutions[resolution] ?: resolution

    /** An HDR format: "HDR10", "HLG", "Dolby Vision". */
    fun hdr(hdr: String): String = media.hdr[hdr] ?: hdr

    /** A language tag in words, "English", "Chinese (Traditional)"; "Unknown language" for none. */
    fun language(tag: String?): String = languages.name(tag, table.tracks.unknownLanguage)

    companion object {
        private val STYLED_CODECS = setOf("ass", "ssa")
        private val IMAGE_CODECS = setOf("hdmv_pgs_subtitle", "pgs", "dvd_subtitle")

        private val tables: Map<String, Lazy<Display>> = DisplayJson.locales.mapValues { (locale, text) ->
            lazy { Display(json.decodeFromString(LocaleTable.serializer(), text), LanguageNames.forLocale(locale)) }
        }

        /** The locales with a display table. */
        val locales: Set<String> get() = tables.keys

        /**
         * The table for a BCP 47 language tag, by its language (`fr-CA` is
         * `fr`), else English.
         */
        fun forLocale(tag: String): Display {
            val language = tag.substringBefore('-').substringBefore('_').lowercase()
            return (tables[language] ?: tables.getValue("en")).value
        }
    }
}
