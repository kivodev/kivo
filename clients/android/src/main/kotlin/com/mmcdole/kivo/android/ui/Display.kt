package com.mmcdole.kivo.android.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.res.stringResource
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.core.display.Display
import java.text.NumberFormat
import java.util.Locale

/**
 * The core's words for bitrates, sizes, runtimes, episode codes, quality
 * choices, problems and tracks, in the language the app's strings resolved
 * to, as the core's session has them.
 */
val display: Display
    @Composable @ReadOnlyComposable
    get() = Display.forLocale(stringResource(R.string.language_tag))

/** A rating out of ten, such as "8.2". */
fun formatRating(rating: Double, locale: Locale = Locale.getDefault()): String =
    NumberFormat.getNumberInstance(locale).apply {
        minimumFractionDigits = 1
        maximumFractionDigits = 1
    }.format(rating)
