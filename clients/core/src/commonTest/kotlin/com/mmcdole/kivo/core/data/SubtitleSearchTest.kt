package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.SubtitleResult
import com.mmcdole.kivo.api.models.SubtitleResultList
import com.mmcdole.kivo.api.models.SubtitleTrackInfo
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.VersionPart
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.TrackName
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.problem
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SubtitleSearchTest {
    private val video = Track("t-video", TrackType.Video, "hevc", true, "HEVC", "HEVC")

    private fun version(tracks: List<Track> = listOf(video), parts: List<VersionPart> = listOf(VersionPart("f1", "Dune.mkv", 1, 1))) =
        Version("v1", true, 1, 1, parts, tracks, listOf("w1"), "")

    private fun result(id: String, hashMatch: Boolean = false, forced: Boolean = false, sdh: Boolean = false, release: String? = null) =
        SubtitleResult(id, "en", 10, hashMatch, forced, sdh, release)

    @Test
    fun searchesAndDownloadsForTheVersionsFirstFileKeepingTheServersOrder() = runTest {
        val server = FakeServer()
        server.on("GET", "/api/v1/files/f1/subtitle-results") {
            json(SubtitleResultList.serializer(), SubtitleResultList(listOf(result("b", hashMatch = true), result("a"))))
        }
        val added = Track("t-new", TrackType.Subtitle, "subrip", false, "English", "English", language = "en", subtitle = SubtitleTrackInfo("downloaded"))
        server.on("POST", "/api/v1/files/f1/subtitles") { json(Track.serializer(), added) }
        val search = SubtitleSearcher(server.connection())

        assertEquals(listOf("b", "a"), search.results(version()).map { it.id })
        assertEquals("", server.to("GET", "/api/v1/files/f1/subtitle-results").single().query, "the server picks the language")
        assertEquals(added, search.add(version(), "b"))
        assertEquals("""{"resultId":"b"}""", server.to("POST", "/api/v1/files/f1/subtitles").single().body)
    }

    @Test
    fun aSearchIsOfferedForAServerVideoEvenWithoutSubtitleTracks() {
        assertTrue(SubtitleSearch.offered(version()))
        assertFalse(SubtitleSearch.offered(version(), offline = true), "a download playing from its files")
        assertFalse(SubtitleSearch.offered(version(tracks = listOf(Track("a", TrackType.Audio, "aac", true, "", "")))), "audio alone")
        assertFalse(SubtitleSearch.offered(version(parts = emptyList())))
        assertFalse(SubtitleSearch.offered(null))
    }

    @Test
    fun aDownloadedTrackJoinsItsVersion() {
        val other = version().copy(id = "v2")
        val work = WorkDetail(
            id = "w1", kind = "movie", title = "Dune", matchState = "matched", available = true, images = WorkImages(),
            genres = emptyList(), studios = emptyList(), credits = emptyList(), externalIds = emptyList(),
            versions = listOf(version(), other), extras = emptyList(), collections = emptyList(),
        )
        val added = Track("t-new", TrackType.Subtitle, "subrip", false, "", "", subtitle = SubtitleTrackInfo("downloaded"))
        val shown = work.withTrack("v1", added).withTrack("v1", added)
        assertEquals(listOf(video, added), shown.versions[0].tracks)
        assertEquals(listOf(video), shown.versions[1].tracks)
    }

    @Test
    fun namesAResultByLanguageAndFlagsWithItsReleaseUnder() {
        val en = Display()
        assertEquals(
            TrackName("English · Exact match · SDH", "Dune.2021.2160p.UHD.BluRay"),
            en.subtitleResultName(result("a", hashMatch = true, sdh = true, release = "Dune.2021.2160p.UHD.BluRay"), "Exact match"),
        )
        assertEquals(TrackName("English · Forced"), en.subtitleResultName(result("b", forced = true, release = " "), "Exact match"))
    }

    @Test
    fun aFailureIsTheProblemItMeans() = runTest {
        val server = FakeServer()
        val search = SubtitleSearcher(server.connection())
        suspend fun answer(status: HttpStatusCode, code: String, extra: String = ""): SubtitleSearchProblem {
            server.on("GET", "/api/v1/files/f1/subtitle-results") { problem(status, code, extra) }
            return SubtitleSearch.problem(assertFailsWith<ProblemException> { search.results(version()) })
        }
        assertEquals(SubtitleSearchProblem.NotConfigured, answer(HttpStatusCode.ServiceUnavailable, "notConfigured"))
        assertEquals(
            SubtitleSearchProblem.QuotaUsed(1_760_000_000_000),
            answer(HttpStatusCode.ServiceUnavailable, "quotaUsed", ""","resetsAt":"2025-10-09T08:53:20Z""""),
        )
        assertEquals(SubtitleSearchProblem.QuotaUsed(null), answer(HttpStatusCode.ServiceUnavailable, "quotaUsed"))
        assertEquals(SubtitleSearchProblem.NotFound, answer(HttpStatusCode.NotFound, "notFound"))
        assertEquals(SubtitleSearchProblem.ProviderUnavailable, answer(HttpStatusCode.ServiceUnavailable, "providerUnavailable"))
        assertEquals(SubtitleSearchProblem.Other("internal"), answer(HttpStatusCode.InternalServerError, "internal"))
        assertEquals(SubtitleSearchProblem.Other("network"), SubtitleSearch.problem(ProblemException(503, null)))
        assertEquals(SubtitleSearchProblem.Other("network"), SubtitleSearch.problem(IOException("offline")))
        assertEquals(SubtitleSearchProblem.Other("unknown"), SubtitleSearch.problem(IllegalStateException("unreadable")))
        assertNull(version(parts = emptyList()).subtitleSearchFile)
    }
}
