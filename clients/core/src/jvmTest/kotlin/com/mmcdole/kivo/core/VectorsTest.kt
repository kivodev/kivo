package com.mmcdole.kivo.core

import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.Marker
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.TrackTranscode
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.core.data.SubtitleSearchProblem
import com.mmcdole.kivo.core.data.problem
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.QualityOption
import com.mmcdole.kivo.core.display.TrackName
import com.mmcdole.kivo.core.display.bitrate
import com.mmcdole.kivo.core.display.clock
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.display.delivered
import com.mmcdole.kivo.core.display.problem
import com.mmcdole.kivo.core.display.quality
import com.mmcdole.kivo.core.display.rung
import com.mmcdole.kivo.core.display.runtime
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.display.timeLeft
import com.mmcdole.kivo.core.display.trackDetail
import com.mmcdole.kivo.core.display.trackNames
import com.mmcdole.kivo.core.display.versionName
import com.mmcdole.kivo.core.play.PlayerFailure
import com.mmcdole.kivo.core.play.failure
import com.mmcdole.kivo.core.player.SkipShow
import com.mmcdole.kivo.core.player.markersOf
import com.mmcdole.kivo.core.player.upNextCredits
import com.mmcdole.kivo.core.player.upNextAtMs
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.serialization.KSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.long
import kotlinx.serialization.json.longOrNull
import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The shared conformance vectors (`shared/display/vectors`), which the web
 * client's tests read too, so both say the same words and keep the same
 * rules: each case is an input and what comes out of it, in English.
 */
class VectorsTest {
    private val json = Json { ignoreUnknownKeys = true }
    private val en = Display.forLocale("en")

    private fun vectors(name: String): JsonObject =
        json.parseToJsonElement(File("../../shared/display/vectors/$name.json").readText()).jsonObject

    /** Each `[in, out]` case of [key], with what [actual] gives for its input. */
    private fun JsonObject.cases(key: String, actual: (JsonElement) -> Any?) {
        val cases = getValue(key).jsonArray
        assertTrue(cases.isNotEmpty(), "$key has no cases")
        for (case in cases) {
            val (input, out) = case.jsonArray
            val expected = out.jsonPrimitive.let { it.longOrNull ?: if (it.isString) it.content else null }
            assertEquals(expected, actual(input), "$key of $input")
        }
    }

    private fun <T> JsonElement.decode(serializer: KSerializer<T>): T = json.decodeFromJsonElement(serializer, this)

    private val JsonElement.number: Long get() = jsonPrimitive.long

    private fun JsonObject.string(key: String): String? = get(key)?.jsonPrimitive?.takeIf { it.isString }?.content

    @Test
    fun formats() {
        val formats = vectors("formats")
        formats.cases("bitrate") { en.bitrate(it.number) }
        formats.cases("size") { en.size(it.number) }
        formats.cases("runtime") { en.runtime(it.number) }
        formats.cases("timeLeft") { en.timeLeft(it.number) }
        formats.cases("clock") { en.clock(it.number) }
        formats.cases("episodeCode") {
            val e = it.jsonObject
            en.episodeCode(EpisodeInfo(e.getValue("seasonNumber").jsonPrimitive.int, e.getValue("number").jsonPrimitive.int, "", "", e["endNumber"]?.jsonPrimitive?.int))
        }
        formats.cases("rung") { en.rung(it.decode(QualityRung.serializer())) }
        formats.cases("quality") {
            val q = it.jsonObject
            val chosen = q.getValue("quality").jsonPrimitive
            val quality = when {
                chosen.content == "auto" -> Quality.Auto
                chosen.content == "original" -> Quality.Original
                else -> Quality.Limit(chosen.long)
            }
            en.quality(QualityOption(quality, q["rung"]?.decode(QualityRung.serializer())), q["sourceBps"]?.number)
        }
    }

    @Test
    fun problems() {
        val problems = vectors("problems")
        problems.cases("problem") { en.problem(it.jsonPrimitive.content) }
        problems.cases("subtitleSearch") {
            val p = it.jsonObject
            val time = p.string("time")
            val problem = when (val code = p.string("code")!!) {
                "notConfigured" -> SubtitleSearchProblem.NotConfigured
                "quotaUsed" -> SubtitleSearchProblem.QuotaUsed(time?.let { 0L })
                "notFound" -> SubtitleSearchProblem.NotFound
                "providerUnavailable" -> SubtitleSearchProblem.ProviderUnavailable
                else -> SubtitleSearchProblem.Other(code)
            }
            en.problem(problem) { time!! }
        }
        problems.cases("reason") { en.reason(it.jsonPrimitive.content) }
        problems.cases("playerFailure") {
            val f = it.jsonObject
            val failure = if (f["ended"]?.jsonPrimitive?.booleanOrNull == true) {
                PlayerFailure.Ended(f.string("message"))
            } else {
                PlayerFailure.Problem(f.string("code")!!, f.string("reason"))
            }
            en.failure(failure)
        }
    }

    @Test
    fun tracks() {
        val tracks = vectors("tracks")
        for (picker in tracks.getValue("pickers").jsonArray.map { it.jsonObject }) {
            val list = picker.getValue("tracks").jsonArray.map { it.decode(Track.serializer()) }
            val expected = picker.getValue("names").jsonArray.map { name ->
                val (n, note) = name.jsonArray
                TrackName(n.jsonPrimitive.content, note.jsonPrimitive.takeIf { it.isString }?.content)
            }
            val names = en.trackNames(list)
            assertEquals(expected, list.map { names.getValue(it.id) }, "names of ${list.map { it.id }}")
        }
        tracks.cases("languages") { en.language(it.jsonPrimitive.content) }
        tracks.cases("details") { en.trackDetail(it.decode(Track.serializer())) }
        tracks.cases("versions") { en.versionName(it.decode(Version.serializer())) }
        // A track as delivered: a vector's width or channels is its transcode's.
        tracks.cases("outputs") {
            val o = it.jsonObject
            val width = o["width"]?.jsonPrimitive?.int
            val channels = o["channels"]?.jsonPrimitive?.int
            val transcode = if (width != null || channels != null) TrackTranscode(width, o["height"]?.jsonPrimitive?.int, channels = channels) else null
            en.delivered(PlayTrack("t", "video", codec = o.getValue("codec").jsonPrimitive.content, transcode = transcode))
        }
    }

    /** A vector's markers. */
    private fun JsonObject.markers() = markersOf(
        get("markers")?.jsonArray?.map { it.decode(Marker.serializer()) },
    )

    @Test
    fun player() {
        val player = vectors("player")
        for (sequence in player.getValue("skipMarker").jsonArray.map { it.jsonObject }) {
            val markers = sequence.markers()
            val skip = SkipShow()
            for ((i, step) in sequence.getValue("steps").jsonArray.map { it.jsonObject }.withIndex()) {
                when {
                    step.containsKey("expire") -> skip.expire()
                    step.containsKey("skip") -> skip.skip()
                    else -> {
                        val at = skip.at(
                            markers,
                            step.getValue("positionMs").number,
                            step.getValue("controls").jsonPrimitive.booleanOrNull!!,
                            step.getValue("upNext").jsonPrimitive.booleanOrNull!!,
                        )
                        assertEquals(step.getValue("shows").jsonPrimitive.longOrNull, at?.startMs, "${sequence.string("about")}, step $i")
                    }
                }
            }
        }
        player.cases("upNextAt") { upNextAtMs(it.number) }
        for (case in player.getValue("upNextAtCredits").jsonArray.map { it.jsonObject }) {
            val at = upNextAtMs(case.getValue("durationMs").number, upNextCredits(case.markers())?.startMs)
            assertEquals(case.getValue("at").jsonPrimitive.longOrNull, at, case.string("about"))
        }
    }
}
