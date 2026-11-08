package com.mmcdole.kivo.android.settings

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
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.rememberFocusMemory

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
