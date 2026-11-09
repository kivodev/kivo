package com.mmcdole.kivo.android.main

import android.view.KeyEvent
import androidx.activity.compose.BackHandler
import androidx.compose.animation.core.Animatable
import androidx.compose.foundation.background
import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.graphics.drawscope.translate
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.util.lerp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import androidx.core.view.ViewCompat
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.detail.CollectionScreen
import com.mmcdole.kivo.android.detail.DetailScreen
import com.mmcdole.kivo.android.detail.PersonScreen
import com.mmcdole.kivo.android.lists.ListWorksScreen
import com.mmcdole.kivo.android.lists.ListsScreen
import com.mmcdole.kivo.android.home.HomeScreen
import com.mmcdole.kivo.android.library.LibraryScreen
import com.mmcdole.kivo.android.player.PlayerScreen
import com.mmcdole.kivo.android.search.SearchScreen
import com.mmcdole.kivo.android.settings.CreditsPage
import com.mmcdole.kivo.android.settings.LicenseTextPage
import com.mmcdole.kivo.android.settings.LicensesPage
import com.mmcdole.kivo.android.settings.SettingsScreen
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Back
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.ChoicePanel
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.android.ui.LocalFocusRail
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.RemoteKeys
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.WorkMenuHost
import com.mmcdole.kivo.android.ui.fadeSpec
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.android.ui.panelProperties
import com.mmcdole.kivo.android.ui.rememberPanelKeys
import com.mmcdole.kivo.api.models.Library
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.withTimeoutOrNull

/** The rail's width with only its icons showing, which keeps them clear of a TV's cropped edge. */
private val RailClosedWidth = KivoTokens.Layout.sidebarCollapsedWidth

/** Where the rail leaves the content, whose screens start a further 24 dp in. */
val RailWidth = RailClosedWidth + 8.dp

/** The rail's width with its labels showing. */
private val RailOpenWidth = 272.dp

/** How far the open rail floats in from the screen's top, bottom and left edges. */
private val RailInset = 16.dp

/** The open rail's corners. */
private val RailCorner = 16.dp

/** A rail item's height, at which seven libraries show with Search, Home, Lists and Settings. */
private val RailItemHeight = 40.dp

/** The room above and below the rail's items, clear of a TV's cropped edges. */
private val RailPaddingY = 24.dp

/** A rail item's width without its label: its icon's place. */
private val RailIconWidth = RailClosedWidth - 16.dp

/** The libraries the TV shows: every kind but audiobooks. */
fun tvLibraries(libraries: List<Library>) = libraries.filter { it.kind != "audiobooks" }

fun libraryIcon(kind: String) = when (kind) {
    "shows" -> KivoIcon.Tv
    "videos" -> KivoIcon.Video
    "audiobooks" -> KivoIcon.Book
    else -> KivoIcon.Film
}

/**
 * The signed-in app: a rail at the left, icons only until focus
 * moves onto it, with Search, Home, Lists, a line, the pinned libraries
 * in the user's order, More libraries and Settings; the screen beside it;
 * the player over everything. Back goes back a page, opens the rail from a
 * destination, and at Home asks "Exit Kivo?"; in the rail it goes Home
 * from a destination.
 */
@Composable
fun MainScreen(session: ServerSession, onExit: () -> Unit) {
    val navigator = remember(session) { Navigator() }
    DisposableEffect(navigator) { onDispose { navigator.clear() } }
    val all by session.libraryList.collectAsStateWithLifecycle()
    val libraries = remember(all) { tvLibraries(all.orEmpty()) }

    val holder = rememberSaveableStateHolder()

    // A Watch Next program opens its work's page over Home.
    LaunchedEffect(navigator) {
        session.graph.opening.filterNotNull().collect { id ->
            session.graph.opening.value = null
            navigator.open(Screen.Home)
            navigator.push(Screen.Work(id))
        }
    }

    var exiting by remember { mutableStateOf(false) }
    val railEntry = remember { RailEntry() }
    BackHandler {
        when (navigator.back) {
            Back.Pop -> navigator.pop()
            Back.Rail -> railEntry.focus()
            Back.Exit -> exiting = true
        }
    }

    val screen = navigator.current.screen
    if (screen is Screen.Player) {
        navigator.Host(holder) { PlayerScreen(session, it as Screen.Player, navigator) }
        return
    }

    var choosingLibrary by remember { mutableStateOf(false) }
    val content = remember { FocusRequester() }
    // Focus is lost when its item leaves the screen, as a live update can
    // take it; a remote key then goes nowhere, so it puts focus back on the
    // screen.
    var focusedHere by remember { mutableStateOf(false) }
    val view = LocalView.current
    DisposableEffect(view) {
        val lost = ViewCompat.OnUnhandledKeyEventListenerCompat { _, event ->
            event.action == KeyEvent.ACTION_DOWN && !focusedHere && event.keyCode in RemoteKeys && content.requestFocus()
        }
        ViewCompat.addOnUnhandledKeyEventListener(view, lost)
        onDispose { ViewCompat.removeOnUnhandledKeyEventListener(view, lost) }
    }
    val others = libraries.filter { !it.pinned }

    Box(
        Modifier
            .fillMaxSize()
            .onFocusChanged { focusedHere = it.hasFocus },
    ) {
        // The remote's Search key opens Search, from anywhere but the player.
        Box(
            Modifier
                .fillMaxSize()
                .padding(start = RailWidth)
                .onPreviewKeyEvent { event ->
                    if (event.key.nativeKeyCode != KeyEvent.KEYCODE_SEARCH) return@onPreviewKeyEvent false
                    if (event.type == KeyEventType.KeyUp && screen != Screen.Search) navigator.open(Screen.Search)
                    true
                }
                .focusRequester(content)
                .focusGroup(),
        ) {
            CompositionLocalProvider(LocalFocusRail provides railEntry::focus) {
                WorkMenuHost(session, navigator) {
                    navigator.Host(holder) { target ->
                        when (target) {
                            Screen.Home -> HomeScreen(session, navigator)
                            Screen.Search -> SearchScreen(session, navigator)
                            Screen.Settings -> SettingsScreen(session, navigator)
                            Screen.Credits -> CreditsPage(navigator)
                            Screen.Licenses -> LicensesPage(navigator)
                            is Screen.LicenseText -> LicenseTextPage(target.id)
                            is Screen.Library -> LibraryScreen(session, target.id, navigator)
                            Screen.Lists -> ListsScreen(session, navigator)
                            is Screen.ListWorks -> ListWorksScreen(session, target.id, target.name, navigator)
                            is Screen.Work -> DetailScreen(session, target.id, navigator, target.episode)
                            is Screen.Collection -> CollectionScreen(session, target.id, navigator)
                            is Screen.Person -> PersonScreen(session, target.id, navigator)
                            is Screen.Player -> Unit
                        }
                    }
                }
            }
        }
        Rail(
            content,
            railEntry,
            onOpen = session.session.library::rereadLibraries,
            onBack = {
                // Over a destination Back goes Home. Elsewhere it closes the rail, or, with nothing on the screen to focus, does the screen's Back.
                when {
                    navigator.back == Back.Rail -> closeAfter { navigator.open(Screen.Home) }
                    close() -> Unit
                    navigator.back == Back.Exit -> exiting = true
                    else -> closeAfter { navigator.pop() }
                }
            },
        ) {
            val destination = navigator.destination
            // An item acts as OK is released, so the release doesn't reach the screen it opens. The destination opens
            // first, and the rail closes over it.
            val go = { target: Screen -> if (target == screen) close() else closeAfter { navigator.open(target) } }
            RailItem(KivoIcon.Search, stringResource(R.string.search), destination == Screen.Search) { go(Screen.Search) }
            RailItem(KivoIcon.Home, stringResource(R.string.home), destination == Screen.Home) { go(Screen.Home) }
            RailItem(KivoIcon.List, stringResource(R.string.lists), destination == Screen.Lists) { go(Screen.Lists) }
            RailSeparator()
            RailLibraries { first, last ->
                val pinned = libraries.filter { it.pinned }
                val count = pinned.size + if (others.isEmpty()) 0 else 1
                fun end(index: Int) = Modifier
                    .then(if (index == 0) Modifier.focusRequester(first) else Modifier)
                    .then(if (index == count - 1) Modifier.focusRequester(last) else Modifier)
                pinned.forEachIndexed { index, library ->
                    val target = Screen.Library(library.id)
                    RailItem(libraryIcon(library.kind), library.name, destination == target, end(index)) { go(target) }
                }
                if (others.isNotEmpty()) {
                    val current = others.any { destination == Screen.Library(it.id) }
                    RailItem(KivoIcon.More, stringResource(R.string.more_libraries), current, end(count - 1)) { choosingLibrary = true }
                }
            }
            RailItem(KivoIcon.Settings, stringResource(R.string.settings), destination == Screen.Settings) { go(Screen.Settings) }
        }
    }

    if (exiting) {
        ExitDialog(
            onExit = {
                exiting = false
                onExit()
            },
            onDismiss = { exiting = false },
        )
    }

    if (choosingLibrary) {
        ChoicePanel(
            title = stringResource(R.string.more_libraries),
            choices = others.map { Choice(it.id, it.name) },
            selected = (navigator.destination as? Screen.Library)?.id,
            onSelect = {
                choosingLibrary = false
                navigator.open(Screen.Library(it))
            },
            onDismiss = { choosingLibrary = false },
        )
    }
}

/** "Exit Kivo?" in the middle of the screen, with No focused first; No or Back stays in the app. */
@Composable
internal fun ExitDialog(onExit: () -> Unit, onDismiss: () -> Unit) {
    val no = remember { FocusRequester() }
    // A press that began before the dialog opened, such as the Back that opened it, doesn't reach its buttons.
    val keys = rememberPanelKeys()
    Dialog(onDismissRequest = onDismiss, properties = panelProperties()) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Column(
                Modifier
                    .onPreviewKeyEvent { keys.dispatch(it.nativeKeyEvent) }
                    .width(392.dp)
                    .background(Kivo.colors.surfaceRaised, RoundedCornerShape(KivoTokens.Radius.lg))
                    .padding(35.dp),
            ) {
                Text(stringResource(R.string.exit_kivo), style = TvType.heading, color = Kivo.colors.text)
                Row(
                    Modifier.align(Alignment.End).padding(top = 35.dp),
                    horizontalArrangement = Arrangement.spacedBy(14.dp),
                ) {
                    KivoButton(stringResource(R.string.no), onDismiss, Modifier.focusRequester(no).height(38.dp).testTag("exit:no"))
                    KivoButton(stringResource(R.string.exit), onExit, Modifier.height(38.dp).testTag("exit:yes"))
                }
            }
        }
        LaunchedEffect(Unit) { focusWhenPlaced(no) }
    }
}

/**
 * The current destination's rail item, which a screen left with nothing to
 * focus hands focus to ([focus]), as Left from the screen would.
 */
private class RailEntry {
    val selected = FocusRequester()
    var entering = false

    fun focus() {
        entering = true
        if (!selected.requestFocus()) entering = false
    }
}

/**
 * How the rail closes, for its items and its Back: [close] returns focus to
 * the screen, false when the screen has nothing to focus, and [closeAfter]
 * runs its action first and returns focus to the screen once the screen has
 * drawn a frame since.
 */
private open class RailClosing(val close: () -> Boolean, val closeAfter: (action: () -> Unit) -> Unit)

/** What a rail item needs from the rail: whether its label shows, the requester of the current destination's item, and how the rail closes. */
private class RailScope(
    column: ColumnScope,
    val selected: FocusRequester,
    val wide: Boolean,
    closing: RailClosing,
) : RailClosing(closing.close, closing.closeAfter), ColumnScope by column

/**
 * The rail at the screen's left edge, opening while it has focus into a
 * panel with rounded corners floating over the dimmed screen, clear of the
 * screen's top, bottom and left edges. It opens by revealing, drawn wider
 * and further in each frame: nothing is measured again as it moves. Only
 * Left from the screen, or a screen left with nothing to focus
 * ([LocalFocusRail]), moves focus onto it, so focus left behind by a
 * screen that goes away never opens it; it returns to the item focused
 * last, else the current destination's. An item's action runs at once, and
 * the rail closes over the screen it leaves, focus entering [content] at its
 * first item, once that screen has drawn its first frame and has an item to
 * focus. [onOpen] runs as it opens, and [onBack] on Back while it's open.
 */
@Composable
private fun Rail(
    content: FocusRequester,
    entry: RailEntry,
    onOpen: () -> Unit,
    onBack: RailClosing.() -> Unit,
    items: @Composable RailScope.() -> Unit,
) {
    val colors = Kivo.colors
    val focusManager = LocalFocusManager.current
    val group = remember { FocusRequester() }
    val selected = entry.selected
    var open by remember { mutableStateOf(false) }
    val reveal = remember { Animatable(0f) }
    LaunchedEffect(open) { reveal.animateTo(if (open) 1f else 0f, fadeSpec()) }
    // Each pick waits to return focus to the screen until the screen has an
    // item to take it, for up to two seconds; a key pressed on the rail, or
    // focus leaving it, drops the wait.
    var pick by remember { mutableIntStateOf(0) }
    var waiting by remember { mutableStateOf(false) }
    LaunchedEffect(pick) {
        // Launched in the frame that composes the new screen, so the first
        // frame it waits for comes after that screen has drawn.
        withTimeoutOrNull(2_000) {
            while (waiting) {
                withFrameNanos {}
                if (waiting && content.requestFocus()) waiting = false
            }
        }
        waiting = false
    }
    val closing = remember {
        RailClosing(
            close = { focusManager.moveFocus(FocusDirection.Right) },
            closeAfter = { action ->
                action()
                waiting = true
                pick++
            },
        )
    }
    LaunchedEffect(open) { if (open) onOpen() }
    val wide by remember { derivedStateOf { open || reveal.value > 0f } }

    Box(
        Modifier
            .fillMaxSize()
            .graphicsLayer {
                alpha = reveal.value
                compositingStrategy = CompositingStrategy.ModulateAlpha
            }
            .background(colors.scrim),
    )
    Column(
        Modifier
            .width(if (wide) RailOpenWidth else RailClosedWidth)
            .fillMaxHeight()
            .graphicsLayer()
            .drawWithContent {
                val inset = RailInset.toPx() * reveal.value
                val shown = lerp(RailClosedWidth.toPx(), size.width, reveal.value)
                val corner = RailCorner.toPx() * reveal.value
                // Closed, the rail is its icons alone, over the screen's backdrop; its panel shows as it opens.
                if (reveal.value > 0f) {
                    drawRoundRect(
                        colors.surface,
                        topLeft = Offset(inset, inset),
                        size = Size(shown, size.height - inset * 2),
                        cornerRadius = CornerRadius(corner),
                        alpha = reveal.value,
                    )
                }
                translate(left = inset) {
                    clipRect(right = shown) { this@drawWithContent.drawContent() }
                }
            }
            .padding(horizontal = 8.dp, vertical = RailPaddingY)
            .onFocusChanged {
                open = it.hasFocus
                if (!open) waiting = false
            }
            .onPreviewKeyEvent {
                if (it.type == KeyEventType.KeyDown) waiting = false
                false
            }
            .focusRequester(group)
            .focusProperties {
                onEnter = {
                    when {
                        entry.entering -> entry.entering = false
                        requestedFocusDirection != FocusDirection.Left -> cancelFocusChange()
                        !group.restoreFocusedChild() -> selected.requestFocus()
                    }
                }
                onExit = { group.saveFocusedChild() }
            }
            .focusGroup(),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        RailScope(this, selected, wide, closing).items()
    }
    if (open) BackHandler { closing.onBack() }
}

/**
 * The rail's libraries, filling the space between the items above and the
 * last item, and scrolling when they don't fit, with the focused item kept
 * in view. Down into them lands on the [first] item and Up into them on the
 * [last]. Focus moves by where items are laid out, in view or not, so with
 * the list left scrolled, Down from Lists would pass over the items
 * scrolled up level with it, and Up from Settings over those scrolled down
 * level with it.
 */
@Composable
private fun RailScope.RailLibraries(items: @Composable RailScope.(first: FocusRequester, last: FocusRequester) -> Unit) {
    val rail = this
    val first = remember { FocusRequester() }
    val last = remember { FocusRequester() }
    Column(
        Modifier
            .weight(1f)
            .focusProperties {
                onEnter = {
                    when (requestedFocusDirection) {
                        FocusDirection.Down -> first.requestFocus()
                        FocusDirection.Up -> last.requestFocus()
                    }
                }
            }
            .focusGroup()
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        RailScope(this, rail.selected, rail.wide, rail).items(first, last)
    }
}

/** A line between the rail's groups, as wide as the items. */
@Composable
private fun RailScope.RailSeparator() {
    Box(
        Modifier
            .padding(vertical = 2.dp)
            .then(if (wide) Modifier.fillMaxWidth() else Modifier.width(RailIconWidth))
            .height(1.dp)
            .background(Kivo.colors.border),
    )
}

@Composable
private fun RailScope.RailItem(icon: KivoIcon, label: String, current: Boolean, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val colors = Kivo.colors
    val wide = wide
    Surface(
        onClick = onClick,
        modifier = modifier
            .then(if (current) Modifier.focusRequester(selected) else Modifier)
            .then(if (wide) Modifier.fillMaxWidth() else Modifier.width(RailIconWidth))
            .height(RailItemHeight)
            .testTag("rail:$label"),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md)),
        // As the web TV's open rail: the focused entry fills with the text colour, its icon and name dark on it.
        colors = ClickableSurfaceDefaults.colors(
            containerColor = if (current && wide) colors.surfaceRaised else Color.Transparent,
            contentColor = if (current) colors.accent else colors.textMuted,
            focusedContainerColor = colors.text,
            focusedContentColor = colors.bg,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = 1f),
    ) {
        Row(Modifier.fillMaxHeight(), verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.width(RailIconWidth), contentAlignment = Alignment.Center) {
                KivoIconView(icon, size = 22.dp)
            }
            if (wide) Text(label, style = TvType.bodyStrong, maxLines = 1)
        }
    }
}
