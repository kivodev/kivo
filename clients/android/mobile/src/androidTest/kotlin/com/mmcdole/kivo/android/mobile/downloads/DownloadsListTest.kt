package com.mmcdole.kivo.android.mobile.downloads

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeLeft
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.mobile.theme.KivoMobileTheme
import com.mmcdole.kivo.core.downloads.DownloadItem
import com.mmcdole.kivo.core.downloads.DownloadStatus
import com.mmcdole.kivo.core.downloads.DownloadStorage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.assertEquals

// The Downloads tab, drawn alone from a list of downloads: each row's words,
// playing a ready one, a long press's actions, and swiping a show away.

@RunWith(AndroidJUnit4::class)
class DownloadsListTest {
    @get:Rule
    val rule = createComposeRule()

    private fun item(id: String, title: String, status: DownloadStatus, kind: String = "movie", series: String? = null, episode: Int? = null, prepared: Boolean = false) = DownloadItem(
        id = id, workId = "w-$id", versionId = "v", title = title, kind = kind, seriesId = series, seriesTitle = series?.let { "Show" },
        season = episode?.let { 1 }, episode = episode, authors = null, quality = "standard", durationMs = 2_820_000,
        sizeBytes = 327_155_712, sizeExact = true, prepared = prepared, status = status, artworkPath = null, posterPath = null, createdAtMs = 0,
    )

    private val done = CopyOnWriteArrayList<String>()
    private val actions = RowActions(
        play = { done += "play ${it.id}" },
        openShow = { done += "open ${it.seriesId}" },
        delete = { done += "delete ${it.joinToString()}" },
        pause = { done += "pause $it" },
        resume = { done += "resume $it" },
        retry = { done += "retry $it" },
        problemSeen = {},
    )

    private fun show(state: DownloadsUiState) = rule.setContent { KivoMobileTheme { DownloadsList(state, actions) } }

    @Test
    fun rowsSayWhereEachDownloadIsAndAReadyOnePlays() {
        val ready = item("m1", "Charade", DownloadStatus.Ready)
        val coming = item("m2", "Heat", DownloadStatus.Converting(0.68, null), prepared = true)
        show(DownloadsUiState(read = true, movies = listOf(coming, ready), storage = DownloadStorage(3_400_000_000, 41_000_000_000)))
        rule.onNode(hasTestTag("line") and hasAnyAncestor(hasTestTag("download:Heat")), useUnmergedTree = true).assertTextEquals("Preparing · 34%")
        rule.onNode(hasTestTag("line") and hasAnyAncestor(hasTestTag("download:Charade")), useUnmergedTree = true).assertTextEquals("47m · 327 MB")
        rule.onNodeWithTag("storage").assertTextEquals("3.4 GB used · 41 GB free")
        rule.onNodeWithTag("download:Charade").performClick()
        rule.onNodeWithTag("download:Heat").performClick()
        rule.runOnIdle { assertEquals(listOf("play m1"), done.toList(), "only a ready download plays") }
    }

    @Test
    fun aDownloadComingPausesFromItsRingAndALongPressOffersItsActions() {
        show(DownloadsUiState(read = true, movies = listOf(item("m2", "Heat", DownloadStatus.Downloading(50, 100, 10)))))
        rule.onNodeWithTag("pause").performClick()
        rule.onNodeWithTag("download:Heat").performTouchInput { longClick() }
        rule.onNodeWithTag("downloadActions").assertIsDisplayed()
        rule.onNodeWithTag("action:Delete").performClick()
        rule.runOnIdle { assertEquals(listOf("pause m2", "delete m2"), done.toList()) }
    }

    @Test
    fun aShowSaysHowFarItsEpisodesAreAndSwipingAwayAsksBeforeDeletingThemAll() {
        val episodes = listOf(
            item("e1", "Pilot", DownloadStatus.Ready, "episode", "show", 1),
            item("e2", "Second", DownloadStatus.Downloading(30, 100, 10), "episode", "show", 2),
        )
        show(DownloadsUiState(read = true, shows = listOf(ShowDownloads("show", "Show", episodes))))
        rule.onNode(hasTestTag("line") and hasAnyAncestor(hasTestTag("show:Show")), useUnmergedTree = true).assertTextEquals("Downloading · 65%")
        rule.onNodeWithTag("show:Show").performClick()
        rule.onNodeWithTag("show:Show").performTouchInput { swipeLeft() }
        rule.onNodeWithText("Delete 2 episodes (654 MB)?").assertIsDisplayed()
        rule.onNodeWithTag("confirmDelete").performClick()
        rule.runOnIdle { assertEquals(listOf("open show", "delete e1, e2"), done.toList()) }
    }

    @Test
    fun withNoDownloadsItSaysWhereTheyWillBe() {
        show(DownloadsUiState(read = true))
        rule.onNodeWithText("Downloaded books and videos appear here for offline playback.").assertIsDisplayed()
    }
}
