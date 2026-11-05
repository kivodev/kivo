package com.mmcdole.kivo.android.ui

import com.mmcdole.kivo.core.display.Display
import java.text.NumberFormat
import java.util.Locale

/**
 * The core's words for bitrates, sizes, runtimes, episode codes, quality
 * choices, problems and tracks.
 */
val display: Display = Display()

/** A rating out of ten, such as "8.2". */
fun formatRating(rating: Double, locale: Locale = Locale.getDefault()): String =
    NumberFormat.getNumberInstance(locale).apply {
        minimumFractionDigits = 1
        maximumFractionDigits = 1
    }.format(rating)
