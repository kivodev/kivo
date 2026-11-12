package com.mmcdole.kivo.android.mobile.cast

import androidx.lifecycle.ViewModel
import com.mmcdole.kivo.core.cast.CastLink
import com.mmcdole.kivo.core.session.CastRemote
import com.mmcdole.kivo.core.session.KivoSession

/** Casting while signed in: the core's remote over [link], Google Cast, where Google Play services is. */
class CastViewModel(session: KivoSession, link: CastLink?) : ViewModel() {
    val remote: CastRemote? = link?.let(session.cast::remote)

    override fun onCleared() {
        remote?.close()
    }
}
