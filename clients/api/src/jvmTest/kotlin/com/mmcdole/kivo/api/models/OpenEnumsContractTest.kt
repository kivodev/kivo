package com.mmcdole.kivo.api.models

import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals

class OpenEnumsContractTest {
    private val enums: Map<String, List<String>> = mapOf(
        "TrackType" to TrackType.entries.map { it.value },
        "TrackDelivery" to TrackDelivery.entries.map { it.value },
        "PlayDelivery" to PlayDelivery.entries.map { it.value },
        "DownloadDelivery" to DownloadDelivery.entries.map { it.value },
        "SubtitleMode" to SubtitleMode.entries.map { it.value },
        "WorkKind" to WorkKind.entries.map { it.value },
        "LibraryKind" to LibraryKind.entries.map { it.value },
        "ListKind" to ListKind.entries.map { it.value },
        "ListSort" to ListSort.entries.map { it.value },
        "HomeRowKind" to HomeRowKind.entries.map { it.value },
        "MatchState" to MatchState.entries.map { it.value },
        "CollectionSource" to CollectionSource.entries.map { it.value },
        "Resolution" to Resolution.entries.map { it.value },
        "ClientPlatform" to ClientPlatform.entries.map { it.value },
        "SessionKind" to SessionKind.entries.map { it.value },
        "TrackReason" to TrackReason.entries.map { it.value },
        "RuleGroupMatch" to RuleGroupMatch.entries.map { it.value },
        "SortDirection" to SortDirection.entries.map { it.value },
        "WorkFieldType" to WorkFieldType.entries.map { it.value },
        "WorkFieldValueSource" to WorkFieldValueSource.entries.map { it.value },
    )

    /** Each hand-written enum has the contract's values, in its order, then Unknown; and the generator maps each one. */
    @Test
    fun theEnumsAreTheContracts() {
        val spec = File("../../server/internal/api/openapi.yaml").readText()
        for ((name, values) in enums) {
            val list = Regex("""\n    $name:\n(?:      .*\n|\n)*?      enum: \[([^\]]*)]""").find(spec)?.groupValues?.get(1)
                ?: error("$name isn't an enum of the contract")
            assertEquals(list.split(',').map { it.trim() } + "", values, name)
        }
        val mapped = Regex("""KOTLIN_ENUMS := ((?:.*\\\n)*.*)""").find(File("../../server/Makefile").readText())!!.groupValues[1]
        assertEquals(enums.keys, mapped.split(Regex("""[\s\\]+""")).filter { it.isNotEmpty() }.toSet())
    }
}
