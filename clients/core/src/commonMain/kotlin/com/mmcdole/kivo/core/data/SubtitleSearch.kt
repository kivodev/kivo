package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.FileSubtitleAdd
import com.mmcdole.kivo.api.models.SubtitleResult
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.TrackName
import com.mmcdole.kivo.core.display.fill
import com.mmcdole.kivo.core.display.problem

/**
 * Subtitle search, which every signed-in user has: OpenSubtitles' subtitles
 * for a version's file, through the server. Downloading one adds it to the
 * file's tracks for everyone, as a subtitle whose source is `downloaded`.
 */
internal class SubtitleSearcher(private val connection: ServerConnection) {
    /**
     * The results for [version]'s file in the user's subtitle language, in
     * the server's order, best first; a failure is a [SubtitleSearchProblem]
     * ([SubtitleSearch.problem]).
     */
    suspend fun results(version: Version): List<SubtitleResult> =
        connection.files.listFileSubtitleResults(fileOf(version)).result().items

    /** Adds [resultId] to [version]'s file on the server; the track it adds. */
    suspend fun add(version: Version, resultId: String): Track =
        connection.files.addFileSubtitle(fileOf(version), FileSubtitleAdd(resultId)).result()

    private fun fileOf(version: Version): String =
        requireNotNull(version.subtitleSearchFile) { "version ${version.id} has no file to search subtitles for" }
}

object SubtitleSearch {
    /**
     * Whether Subtitles offers "Search subtitles…" for [version]: always for
     * the server's play or title page of a video, even with no subtitle track,
     * never for a download playing from its files ([offline]).
     */
    fun offered(version: Version?, offline: Boolean = false): Boolean =
        !offline && version?.subtitleSearchFile != null

    /** The [SubtitleSearchProblem] a failed search or download's [error] means. */
    fun problem(error: Throwable): SubtitleSearchProblem {
        val problem = RequestProblem.of(error)
        if (problem !is RequestProblem.Refused) return SubtitleSearchProblem.Other(problem.code)
        return when (problem.code) {
            "notConfigured" -> SubtitleSearchProblem.NotConfigured
            "quotaUsed" -> SubtitleSearchProblem.QuotaUsed(problem.problem?.resetsAt?.toEpochMilliseconds())
            "notFound" -> SubtitleSearchProblem.NotFound
            "providerUnavailable" -> SubtitleSearchProblem.ProviderUnavailable
            else -> SubtitleSearchProblem.Other(problem.code)
        }
    }
}

/**
 * The file whose subtitles a search finds, the version's first part's;
 * null for a version without video or without a file, which has none.
 */
internal val Version.subtitleSearchFile: String?
    get() = parts.firstOrNull()?.fileId?.takeIf { tracks.any { it.type == "video" } }

/** [track] in [versionId]'s tracks, in place of one with its ID, so a track just downloaded shows before the work is read again. */
fun WorkDetail.withTrack(versionId: String, track: Track): WorkDetail = copy(
    versions = versions.map { v ->
        if (v.id != versionId) v else v.copy(tracks = v.tracks.filter { it.id != track.id } + track)
    },
)

/**
 * A result's row: the language and its flags as [TrackName.name]
 * ("English · Exact match · SDH": [exactMatch], the platform's words, for
 * one made for this exact file, then Forced and SDH), and the release it
 * was made for as the [TrackName.note].
 */
fun Display.subtitleResultName(result: SubtitleResult, exactMatch: String): TrackName {
    val parts = buildList {
        add(language(result.language))
        if (result.hashMatch) add(exactMatch)
        if (result.forced) add(tracks.flags.forced)
        if (result.hearingImpaired) add(tracks.flags.hearingImpaired)
    }
    return TrackName(parts.joinToString(" · "), result.release?.trim()?.takeIf { it.isNotEmpty() })
}

/** Why a subtitle search or download failed, which [Display.problem] puts in words. */
sealed interface SubtitleSearchProblem {
    /** The server has no OpenSubtitles key. */
    data object NotConfigured : SubtitleSearchProblem

    /** The server's downloads for the day are used up; until [resetsAtEpochMs] when OpenSubtitles said. */
    data class QuotaUsed(val resetsAtEpochMs: Long?) : SubtitleSearchProblem

    /** The result, or the file, is gone. */
    data object NotFound : SubtitleSearchProblem

    /** OpenSubtitles isn't answering the server. */
    data object ProviderUnavailable : SubtitleSearchProblem

    /** Anything else, by its [RequestProblem.code]. */
    data class Other(val code: String) : SubtitleSearchProblem
}

/**
 * A failed subtitle search or download in a sentence; a quota used up says
 * when there's more, the epoch milliseconds as [timeOfDay] puts them, when
 * the server knows.
 */
fun Display.problem(problem: SubtitleSearchProblem, timeOfDay: (Long) -> String): String {
    val words = table.subtitleSearch
    return when (problem) {
        SubtitleSearchProblem.NotConfigured -> words.notConfigured
        is SubtitleSearchProblem.QuotaUsed ->
            problem.resetsAtEpochMs?.let { words.quotaUsedUntil.fill("time" to timeOfDay(it)) } ?: words.quotaUsed
        SubtitleSearchProblem.NotFound -> words.notFound
        SubtitleSearchProblem.ProviderUnavailable -> words.providerUnavailable
        is SubtitleSearchProblem.Other -> problem(problem.code)
    }
}
