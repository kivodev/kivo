package com.mmcdole.kivo.android

import android.content.Intent
import android.graphics.Color
import android.os.Bundle
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.fragment.app.FragmentActivity
import com.mmcdole.kivo.android.main.TvRoot
import com.mmcdole.kivo.android.mobile.app.MobileRoot
import com.mmcdole.kivo.android.watchnext.WatchNext

/**
 * The one activity, showing the TV interface on a TV and the phone's
 * elsewhere. It tells the core while it's in the foreground, where live
 * events run: the socket closes in the background, and opening it again
 * on return refetches what's on screen; the downloads catch up as it comes
 * back. A fragment activity, as the phone's Cast button opens its device
 * list as a dialog fragment. A `kivo://link` link opens Link a device on a phone.
 */
class MainActivity : FragmentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val graph = (application as KivoApp).graph
        if (savedInstanceState == null) open(intent)

        val ui = chooseUi(this)
        // The phone's screens draw behind the system bars.
        if (ui == Ui.Mobile) enableEdgeToEdge(SystemBarStyle.dark(Color.TRANSPARENT), SystemBarStyle.dark(Color.TRANSPARENT))
        setContent {
            when (ui) {
                Ui.Tv -> TvRoot(graph, onExit = ::finish)
                Ui.Mobile -> MobileRoot(graph)
            }
        }
    }

    override fun onStart() {
        super.onStart()
        (application as KivoApp).graph.core.setForeground(true)
    }

    override fun onStop() {
        super.onStop()
        val graph = (application as KivoApp).graph
        graph.core.setForeground(false)
        graph.leftForeground()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        open(intent)
    }

    /** A Watch Next program's link: its work's page, once signed in; a `kivo://link` link: Link a device, with its code. */
    private fun open(intent: Intent?) {
        val graph = (application as KivoApp).graph
        val uri = intent?.data
        WatchNext.workOf(uri)?.let { graph.opening.value = it }
        if (uri?.scheme == "kivo" && uri.host == "link") graph.linking.value = uri.toString()
    }
}
