package com.mmcdole.kivo.android.settings

import android.content.Context
import com.mikepenz.aboutlibraries.Libs
import com.mikepenz.aboutlibraries.entity.Library
import com.mmcdole.kivo.android.HasAppGraph
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** A library in the app, or terms no build reports, with its license and that license's text. */
data class Credit(val id: String, val name: String, val version: String?, val license: String, val text: String?)

/**
 * The open source licenses: each library the build reports, the app's and
 * :core's, as AboutLibraries reads them from res/raw/aboutlibraries.json,
 * by name, then the entries of shared/credits/terms.json ([terms]) that
 * name Android, whose texts [file] reads.
 */
fun credits(libraries: List<Library>, terms: String, file: (String) -> String): List<Credit> {
    val libs = libraries.map { lib ->
        Credit(
            id = lib.uniqueId,
            name = lib.name,
            version = lib.artifactVersion,
            license = lib.licenses.joinToString(" AND ") { it.spdxId ?: it.name },
            text = lib.licenses.mapNotNull { it.licenseContent }.joinToString("\n\n").ifEmpty { null },
        )
    }.sortedBy { it.name.lowercase() }
    val others = (Json.parseToJsonElement(terms).jsonObject["terms"] as JsonArray).map { it.jsonObject }
        .filter { term -> term["apps"]?.jsonArray?.any { it.jsonPrimitive.content == "android" } == true }
        .map { term ->
            val name = term.string("name")!!
            Credit(name, name, null, term.string("license")!!, term.string("text")?.let(file))
        }
    return libs + others
}

private fun JsonObject.string(key: String): String? = this[key]?.jsonPrimitive?.content

/** The app's credits, for the TV's screens and the phone's, read once, off the main thread. */
object AppCredits {
    @Volatile private var loaded: List<Credit>? = null

    suspend fun get(context: Context): List<Credit> = loaded ?: withContext(Dispatchers.IO) {
        val res = context.resources
        val assets = context.assets
        val libraries = res.openRawResource((context as HasAppGraph).graph.build.libraries).bufferedReader().use { it.readText() }
        val read = { name: String -> assets.open("credits/$name").bufferedReader().use { it.readText() } }
        credits(Libs.Builder().withJson(libraries).build().libraries, read("terms.json"), read).also { loaded = it }
    }
}
