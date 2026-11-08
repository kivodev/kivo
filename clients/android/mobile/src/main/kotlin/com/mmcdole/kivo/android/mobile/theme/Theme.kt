package com.mmcdole.kivo.android.mobile.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

// The phone interface's theme: Material 3 in the dark colour set of the generated tokens (KivoTokens,
// shared/tokens.json), in the system's sans-serif as the TV has it, and the tokens' radii.

private val colors = KivoTokens.dark

private val scheme = darkColorScheme(
    primary = colors.accent,
    onPrimary = colors.onAccent,
    primaryContainer = colors.accent.copy(alpha = 0.2f),
    onPrimaryContainer = colors.text,
    secondary = colors.accent,
    onSecondary = colors.onAccent,
    secondaryContainer = colors.accent.copy(alpha = 0.2f),
    onSecondaryContainer = colors.text,
    background = colors.bg,
    onBackground = colors.text,
    surface = colors.bg,
    onSurface = colors.text,
    surfaceVariant = colors.surfaceRaised,
    onSurfaceVariant = colors.textMuted,
    surfaceContainerLowest = colors.bg,
    surfaceContainerLow = colors.surface,
    surfaceContainer = colors.surface,
    surfaceContainerHigh = colors.surfaceRaised,
    surfaceContainerHighest = colors.surfaceHover,
    surfaceBright = colors.surfaceHover,
    inverseSurface = colors.text,
    inverseOnSurface = colors.bg,
    outline = colors.border,
    outlineVariant = colors.border,
    error = colors.danger,
    scrim = colors.scrim,
)

private fun style(step: TextStep, weight: FontWeight = KivoTokens.FontWeights.normal) =
    TextStyle(fontFamily = FontFamily.SansSerif, fontSize = step.size, lineHeight = step.lineHeight, fontWeight = weight)

private val type = with(KivoTokens.Text) {
    Typography(
        displaySmall = style(xl3, KivoTokens.FontWeights.bold),
        headlineMedium = style(xl2, KivoTokens.FontWeights.bold),
        headlineSmall = style(xl, KivoTokens.FontWeights.bold),
        titleLarge = style(xl, KivoTokens.FontWeights.semibold),
        titleMedium = style(lg, KivoTokens.FontWeights.semibold),
        titleSmall = style(base, KivoTokens.FontWeights.semibold),
        bodyLarge = style(base),
        bodyMedium = style(sm),
        bodySmall = style(xs),
        labelLarge = style(sm, KivoTokens.FontWeights.semibold),
        labelMedium = style(xs, KivoTokens.FontWeights.medium),
        labelSmall = style(xs, KivoTokens.FontWeights.medium),
    )
}

private val shapes = Shapes(
    extraSmall = RoundedCornerShape(KivoTokens.Radius.sm),
    small = RoundedCornerShape(KivoTokens.Radius.md),
    medium = RoundedCornerShape(KivoTokens.Radius.lg),
    large = RoundedCornerShape(KivoTokens.Radius.lg),
    extraLarge = RoundedCornerShape(28.dp),
)

/** The tokens' colours that Material's scheme has no place for. */
@Immutable
data class KivoExtras(val textSubtle: Color, val placeholder: Color, val progressTrack: Color, val warning: Color, val badge: Color)

val LocalKivo = staticCompositionLocalOf {
    KivoExtras(colors.textSubtle, colors.placeholder, colors.progressTrack, colors.warning, colors.badge)
}

/** The tokens' spacing: [n] steps of 4 dp. */
fun space(n: Int): Dp = KivoTokens.spacing * n

@Composable
fun KivoMobileTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = scheme, typography = type, shapes = shapes, content = content)
}
