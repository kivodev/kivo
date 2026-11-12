package com.mmcdole.kivo.core.display

import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackType
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** The words for tracks, from the display table. */
@Serializable
internal data class TrackWords(
    val unknownLanguage: String,
    val default: String,
    /** The short words a track's name adds: "Forced", "SDH", "Commentary", "Original". */
    val flags: TrackFlags,
    /** Where a subtitle lives: `embedded`, `external`, `downloaded`. */
    val sources: Map<String, String>,
)

/** The words for a track's flags, as its short name says them. */
@Serializable
internal data class TrackFlags(val forced: String, val hearingImpaired: String, val commentary: String, val original: String)

@Serializable
internal data class WordsTable(
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

/** The decimal and grouping separators. */
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
    val resolutions: Map<String, String>,
)

private val json = Json { ignoreUnknownKeys = true }

internal val media: MediaTable by lazy { json.decodeFromString(MediaTable.serializer(), DisplayJson.MEDIA) }

private val words: WordsTable by lazy { json.decodeFromString(WordsTable.serializer(), DisplayJson.WORDS) }

private val STYLED_CODECS = setOf("ass", "ssa")
private val IMAGE_CODECS = setOf("hdmv_pgs_subtitle", "pgs", "dvd_subtitle")

/**
 * How playback is put in words, from the display tables the server and the
 * web client share (`shared/display`), generated into the core: methods,
 * subtitle kinds, reason texts, dialogue boost costs, track words, language
 * names, and codec, container and resolution names. The server names tracks and versions; bitrates, sizes,
 * runtimes, episode codes, quality choices and problems are put in words
 * here ([bitrate], [problem]), so every app says them alike. Each screen's
 * own words stay in its platform's resources.
 */
class Display {
    internal val table: WordsTable = words
    internal val languages: LanguageNames = LanguageNames.table

    internal val tracks: TrackWords get() = table.tracks

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

    /** A resolution class in words: "4K", "1080p", "SD". */
    fun resolution(resolution: Resolution): String = media.resolutions[resolution.value] ?: resolution.value

    /** A language tag in words, "English", "Chinese (Traditional)"; "Unknown language" for none. */
    fun language(tag: String?): String = languages.name(tag, table.tracks.unknownLanguage)
}
