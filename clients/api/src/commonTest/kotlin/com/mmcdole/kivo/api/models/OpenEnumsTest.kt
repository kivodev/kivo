package com.mmcdole.kivo.api.models

import kotlinx.serialization.json.Json
import kotlin.test.Test
import kotlin.test.assertEquals

class OpenEnumsTest {
    private val json = Json { ignoreUnknownKeys = true }

    private val summary = """{"id":"w1","kind":"%s","title":"Dune","matchState":"%s","available":true,"images":{}}"""

    @Test
    fun aValueDecodesToItsCaseAndEncodesAsItCame() {
        val work = json.decodeFromString(WorkSummary.serializer(), summary.format("series", "identifying"))
        assertEquals(WorkKind.Series, work.kind)
        assertEquals(MatchState.Identifying, work.matchState)
        val sent = json.encodeToString(WorkSummary.serializer(), work)
        assertEquals(work, json.decodeFromString(WorkSummary.serializer(), sent))
        assertEquals(""""4k"""", json.encodeToString(Resolution.serializer(), Resolution.FourK))
        assertEquals("""{"sort":"dateAdded"}""", json.encodeToString(ListUpdate.serializer(), ListUpdate(sort = ListSort.DateAdded)))
    }

    @Test
    fun aValueALaterServerAddsDecodesAsUnknown() {
        val work = json.decodeFromString(WorkSummary.serializer(), summary.format("podcast", "guessing"))
        assertEquals(WorkKind.Unknown, work.kind)
        assertEquals(MatchState.Unknown, work.matchState)
        assertEquals("Dune", work.title)
        val query = json.decodeFromString(
            WorkQuery.serializer(),
            """{"kinds":["movie","podcast"],"where":{"match":"most","rules":[]},"sort":[{"field":"title","direction":"sideways"}]}""",
        )
        assertEquals(listOf(WorkKind.Movie, WorkKind.Unknown), query.kinds)
        assertEquals(RuleGroupMatch.Unknown, query.where?.match)
        assertEquals(SortDirection.Unknown, query.sort?.single()?.direction)
        val list = json.decodeFromString(WorkList.serializer(), """{"id":"l1","name":"Later","kind":"shared","builtIn":false,"workCount":0,"sort":"shuffled"}""")
        assertEquals(ListKind.Unknown, list.kind)
        assertEquals(ListSort.Unknown, list.sort)
    }

    @Test
    fun aCaseIsNamedByItsWireValue() {
        assertEquals("continueWatching", HomeRowKind.ContinueWatching.toString(), "a query parameter is written with toString")
        assertEquals("1080p", "${Resolution.P1080}")
        for (kind in WorkKind.entries) assertEquals(kind, WorkKind.of(kind.value))
        assertEquals(WorkKind.Unknown, WorkKind.of("podcast"))
        assertEquals(WorkKind.Unknown, WorkKind.of(null))
        assertEquals(WorkKind.Unknown, WorkKind.of(""))
    }
}

private fun String.format(kind: String, matchState: String) = replaceFirst("%s", kind).replaceFirst("%s", matchState)
