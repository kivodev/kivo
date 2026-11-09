package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.addPathNodes
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Icon
import androidx.tv.material3.LocalContentColor

/**
 * The web client's stroke icons (server/web/src/components/Icon.tsx), on
 * the same 24-unit grid, drawn in the content colour. Only the ones the TV
 * interface uses.
 */
enum class KivoIcon(val path: String, val stroke: Float = 2f) {
    Home("M3 10.5 12 3l9 7.5V21h-6v-6H9v6H3z"),
    Search("M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zm10 17-5-5"),
    Film("M4 4h16v16H4zM8 4v16M16 4v16M4 8h4M4 12h4M4 16h4M16 8h4M16 12h4M16 16h4"),
    Tv("M3 6h18v12H3zM8 21h8M12 18v3"),
    Video("M3 6h13v12H3zM16 10l5-3v10l-5-3"),
    Folder("M3 6h6l2 2h10v11H3z"),
    Book("M5 4.5A1.5 1.5 0 0 1 6.5 3H20v15H6.5A1.5 1.5 0 0 0 5 19.5zM5 19.5A1.5 1.5 0 0 0 6.5 21H20v-3M9 7h7"),
    Play("M7 4.5v15l12-7.5z", stroke = 2.5f),
    Pause("M8 5v14M16 5v14"),
    Replay("M3 12a9 9 0 1 0 3-6.7M3 4v5h5"),
    Forward("M21 12a9 9 0 1 1-3-6.7M21 4v5h-5"),
    Previous("M6 5v14M18 5 9 12l9 7z"),
    Next("M18 5v14M6 5l9 7-9 7z"),
    Info("M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 11v5M12 8h.01"),
    Volume("M4 9h4l5-4v14l-5-4H4zM16.5 9a4 4 0 0 1 0 6M19 6.5a8 8 0 0 1 0 11"),
    Gauge("M4 18a8 8 0 1 1 16 0M12 18l4-6"),
    SpeechBubble("M4 5h16v11H10l-5 4v-4H4zM8 9h8M8 12h5"),
    Check("M5 12.5 10 17.5 19 7"),
    Plus("M12 5v14M5 12h14"),
    List("M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01"),
    More("M5 12h.01M12 12h.01M19 12h.01", stroke = 3f),
    ChevronLeft("m15 6-6 6 6 6"),
    ChevronRight("m9 6 6 6-6 6"),
    ChevronDown("m6 9 6 6 6-6"),
    ChevronUp("m6 15 6-6 6 6"),
    Alert("M12 3 2 21h20zM12 10v5M12 18h.01"),
    Settings("M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 0 0 1.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 0 0-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 0 0-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 0 0-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 0 0-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 0 0 1.066-2.573c-.94-1.543.826-3.31 2.37-2.37c1 .608 2.296.07 2.572-1.065zM9 12a3 3 0 1 0 6 0a3 3 0 0 0-6 0"),
    SignOut("M15 4h4v16h-4M10 8l-4 4 4 4M6 12h11"),
    Delete("M6 6l12 12M18 6 6 18"),
    Mic("M12 3a3 3 0 0 0-3 3v6a3 3 0 0 0 6 0V6a3 3 0 0 0-3-3zM5 11a7 7 0 0 0 14 0M12 18v3M8 21h8"),
    Person("M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM4 21a8 8 0 0 1 16 0"),
    ;

    val vector: ImageVector by lazy {
        ImageVector.Builder(name.lowercase(), 24.dp, 24.dp, 24f, 24f)
            .addPath(
                pathData = addPathNodes(path),
                fill = null,
                stroke = SolidColor(Color.White),
                strokeLineWidth = stroke,
                strokeLineCap = StrokeCap.Round,
                strokeLineJoin = StrokeJoin.Round,
            )
            .build()
    }
}

/** An icon, read out as [contentDescription] when it's the only label of what holds it; else a decoration. */
@Composable
fun KivoIconView(
    icon: KivoIcon,
    modifier: Modifier = Modifier,
    size: Dp = 24.dp,
    tint: Color = LocalContentColor.current,
    contentDescription: String? = null,
) {
    Icon(icon.vector, contentDescription = contentDescription, modifier = modifier.size(size), tint = tint)
}
