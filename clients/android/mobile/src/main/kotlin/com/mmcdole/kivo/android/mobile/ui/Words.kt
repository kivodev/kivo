package com.mmcdole.kivo.android.mobile.ui

import android.text.format.DateFormat
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.api.models.MovieInfo
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.CreditRole
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.display.problem
import java.util.Date

/** The core's words for runtimes, episode codes, problems and tracks. */
val display: Display = Display()

/** A problem in a sentence, as the core words it. */
fun sentence(problem: RequestProblem): String = display.problem(problem)

/** A card's shape: its artwork's aspect. */
enum class CardShape(val aspect: Float) { Poster(2f / 3f), Landscape(16f / 9f), Square(1f) }

/** A work's own card shape: an episode, video or extra by its frame, a book square, the rest posters. */
fun shapeOf(work: WorkSummary): CardShape = when (work.kind) {
    WorkKind.Episode, WorkKind.Video, WorkKind.Extra -> CardShape.Landscape
    WorkKind.Book -> CardShape.Square
    else -> CardShape.Poster
}

/** A card's shape in a list's grid or row, where an episode is a poster. */
fun listShapeOf(work: WorkSummary): CardShape = if (work.kind == WorkKind.Episode) CardShape.Poster else shapeOf(work)

/** A movie's title with its edition, "Blade Runner · Final Cut", when its cut is named. */
fun labelledTitle(title: String, movie: MovieInfo?): String = movie?.edition?.let { "$title · $it" } ?: title

/** A card's first line: an episode's series, else the title with its edition. */
fun cardTitle(work: WorkSummary): String = when (work.kind) {
    WorkKind.Episode -> work.episode?.seriesTitle ?: work.title
    else -> labelledTitle(work.title, work.movie)
}

/** A card's second line: an episode's code and title, a book's author, else the year. */
fun cardCaption(work: WorkSummary): String? = when {
    work.episode != null -> "${display.episodeCode(work.episode!!)} · ${work.title}"
    work.book != null -> work.book!!.authors.firstOrNull()?.name
    work.season != null -> work.title
    else -> work.year?.toString()
}

/** Whether the work plays: a video or book that's available. */
fun playable(work: WorkSummary): Boolean = work.available && work.kind in setOf(WorkKind.Movie, WorkKind.Episode, WorkKind.Video, WorkKind.Extra, WorkKind.Book)

/** Played progress, 0 to 1, while in progress; null otherwise. */
fun progressOf(work: WorkSummary): Float? {
    val position = work.userState?.positionMs ?: return null
    val duration = work.durationMs ?: return null
    if (duration <= 0 || work.userState?.played == true) return null
    return (position.toFloat() / duration).coerceIn(0f, 1f)
}

/** "Mark as watched", or "Mark as finished" for a book. */
@Composable
fun markPlayedLabel(played: Boolean, kind: WorkKind): String = stringResource(
    when {
        kind == WorkKind.Book && played -> R.string.mobile_mark_unfinished
        kind == WorkKind.Book -> R.string.mobile_mark_finished
        played -> R.string.mobile_mark_unwatched
        else -> R.string.mobile_mark_watched
    },
)

/** A credit's role in a word: Actor, Director, Writer, Author, Narrator, or its own name capitalised. */
@Composable
fun roleName(role: String): String = when (CreditRole.of(role)) {
    CreditRole.Actor -> stringResource(R.string.mobile_role_actor)
    CreditRole.Director -> stringResource(R.string.mobile_role_director)
    CreditRole.Writer -> stringResource(R.string.mobile_role_writer)
    CreditRole.Author -> stringResource(R.string.mobile_role_author)
    CreditRole.Narrator -> stringResource(R.string.mobile_role_narrator)
    else -> role.replaceFirstChar { it.uppercase() }
}

/** "Ends at 9:42 PM": now plus [remainingMs], on the device's 12- or 24-hour clock. */
@Composable
fun endsAt(remainingMs: Long): String {
    val context = LocalContext.current
    val format = remember(context) { DateFormat.getTimeFormat(context) }
    return stringResource(R.string.mobile_ends_at, format.format(Date(System.currentTimeMillis() + remainingMs.coerceAtLeast(0))))
}
