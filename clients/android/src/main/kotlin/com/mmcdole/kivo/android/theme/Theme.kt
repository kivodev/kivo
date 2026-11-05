package com.mmcdole.kivo.android.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.tv.material3.LocalContentColor
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.darkColorScheme

// The TV interface's theme, from the generated tokens (KivoTokens,
// shared/tokens.json): the dark colour set, the TV type scale and the focus
// ring. Screens read colours from Kivo.colors, which has every colour token;
// tv-material's components get the same colours through its scheme.

private val LocalKivoColors = staticCompositionLocalOf { KivoTokens.dark }

object Kivo {
    val colors: KivoColors
        @Composable get() = LocalKivoColors.current

    /** A multiple of the spacing unit. */
    fun space(steps: Int): Dp = KivoTokens.spacing * steps
}

/** The TV type scale (body 28 px on a 1080p canvas, titles 48, nothing below 24), in the platform's sans-serif. */
object TvType {
    private fun style(step: TextStep, weight: FontWeight = KivoTokens.FontWeights.normal) =
        TextStyle(fontFamily = FontFamily.SansSerif, fontSize = step.size, lineHeight = step.lineHeight, fontWeight = weight)

    val small = style(KivoTokens.Tv.Text.sm)
    val smallStrong = style(KivoTokens.Tv.Text.sm, KivoTokens.FontWeights.semibold)
    val body = style(KivoTokens.Tv.Text.base)
    val bodyStrong = style(KivoTokens.Tv.Text.base, KivoTokens.FontWeights.semibold)

    /** A list item's headline, such as a setting's name. */
    val item = style(KivoTokens.Tv.Text.lg, KivoTokens.FontWeights.medium)
    val large = style(KivoTokens.Tv.Text.lg, KivoTokens.FontWeights.semibold)
    val heading = style(KivoTokens.Tv.Text.xl, KivoTokens.FontWeights.semibold)
    val title = style(KivoTokens.Tv.Text.title, KivoTokens.FontWeights.bold)

    /** Something to read from across the room, such as a sign-in code. */
    val display = style(KivoTokens.Tv.Text.display, KivoTokens.FontWeights.bold)
}

@Composable
fun KivoTheme(content: @Composable () -> Unit) {
    val colors = KivoTokens.dark
    val scheme = darkColorScheme(
        primary = colors.accent,
        onPrimary = colors.onAccent,
        primaryContainer = colors.surfaceRaised,
        onPrimaryContainer = colors.text,
        secondary = colors.accent,
        onSecondary = colors.onAccent,
        background = colors.bg,
        onBackground = colors.text,
        surface = colors.surface,
        onSurface = colors.text,
        surfaceVariant = colors.surfaceRaised,
        onSurfaceVariant = colors.textMuted,
        inverseSurface = colors.text,
        inverseOnSurface = colors.bg,
        border = colors.focus,
        borderVariant = colors.border,
        error = colors.danger,
        scrim = colors.scrim,
    )
    MaterialTheme(colorScheme = scheme) {
        CompositionLocalProvider(LocalKivoColors provides colors, LocalContentColor provides colors.text) {
            content()
        }
    }
}
