package com.mmcdole.kivo.android.settings

import android.content.Context
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.tv.material3.Text
import com.mikepenz.aboutlibraries.Libs
import com.mikepenz.aboutlibraries.entity.Library
import com.mmcdole.kivo.android.HasAppGraph
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.rememberFocusMemory
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

/** The app's credits, read once, off the main thread. */
private object AppCredits {
    @Volatile private var loaded: List<Credit>? = null

    suspend fun get(context: Context): List<Credit> = loaded ?: withContext(Dispatchers.IO) {
        val res = context.resources
        val assets = context.assets
        val libraries = res.openRawResource((context as HasAppGraph).graph.build.libraries).bufferedReader().use { it.readText() }
        val read = { name: String -> assets.open("credits/$name").bufferedReader().use { it.readText() } }
        credits(Libs.Builder().withJson(libraries).build().libraries, read("terms.json"), read).also { loaded = it }
    }
}

@Composable
private fun rememberCredits(): List<Credit>? {
    val context = LocalContext.current.applicationContext
    val credits by produceState<List<Credit>?>(null) { value = AppCredits.get(context) }
    return credits
}

/** Open source licenses, from Credits: a row for each library, with its version and license, opening the license's text. */
@Composable
internal fun LicensesPage(navigator: Navigator) {
    val credits = rememberCredits()
    val memory = rememberFocusMemory()
    SettingsList(stringResource(R.string.open_source_licenses), memory, initial = credits?.firstOrNull()?.id) {
        items(credits.orEmpty(), key = { it.id }) { credit ->
            ActionRow(
                credit.name,
                { if (credit.text != null) navigator.push(Screen.LicenseText(credit.id)) },
                Modifier.focusKey(memory, credit.id).testTag("license:${credit.id}"),
                supporting = listOfNotNull(credit.version, credit.license).joinToString(" · "),
                opensPage = credit.text != null,
            )
        }
    }
}

/**
 * One license's text, from Open source licenses. Each paragraph takes focus
 * in turn, so Up and Down scroll through it.
 */
@Composable
internal fun LicenseTextPage(id: String) {
    val credit = rememberCredits()?.firstOrNull { it.id == id }
    val memory = rememberFocusMemory()
    val paragraphs = credit?.text?.split(Regex("\n\\s*\n"))?.map { it.trim() }?.filter { it.isNotEmpty() }.orEmpty()
    SettingsList(credit?.name.orEmpty(), memory, initial = "p0".takeIf { paragraphs.isNotEmpty() }) {
        itemsIndexed(paragraphs) { i, paragraph ->
            Text(
                paragraph,
                style = TvType.body,
                color = Kivo.colors.textMuted,
                modifier = Modifier
                    .padding(horizontal = Kivo.space(4), vertical = Kivo.space(1))
                    .focusKey(memory, "p$i")
                    .focusable(),
            )
        }
    }
}
