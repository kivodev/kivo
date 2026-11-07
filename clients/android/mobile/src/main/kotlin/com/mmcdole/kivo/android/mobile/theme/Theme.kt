package com.mmcdole.kivo.android.mobile.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable

// The phone interface's theme: Material 3 in the dark colour set of the generated tokens (KivoTokens,
// shared/tokens.json).

@Composable
fun KivoMobileTheme(content: @Composable () -> Unit) {
    val colors = KivoTokens.dark
    val scheme = darkColorScheme(
        primary = colors.accent,
        onPrimary = colors.onAccent,
        background = colors.bg,
        onBackground = colors.text,
        surface = colors.surface,
        onSurface = colors.text,
        surfaceVariant = colors.surfaceRaised,
        onSurfaceVariant = colors.textMuted,
        outline = colors.border,
        error = colors.danger,
        scrim = colors.scrim,
    )
    MaterialTheme(colorScheme = scheme, content = content)
}
