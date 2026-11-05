package com.mmcdole.kivo.android.ui

import android.text.format.DateFormat
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import com.mmcdole.kivo.android.R
import kotlinx.coroutines.delay
import java.util.Date

/**
 * "Ends at 9:42 PM": now plus [remainingMs], in the device's language and
 * its 12- or 24-hour clock, following the clock while it shows.
 */
@Composable
fun endsAt(remainingMs: Long): String {
    val context = LocalContext.current
    val format = remember(context) { DateFormat.getTimeFormat(context) }
    val now by produceState(System.currentTimeMillis()) {
        while (true) {
            delay(1_000)
            value = System.currentTimeMillis()
        }
    }
    return stringResource(R.string.ends_at, format.format(Date(now + remainingMs.coerceAtLeast(0))))
}
