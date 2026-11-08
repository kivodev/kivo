package com.mmcdole.kivo.android.player

import androidx.media3.common.Player
import kotlin.test.Test
import kotlin.test.assertEquals

class SessionPlayerTest {
    /** The session hears every event the player has: each of the listener's methods is passed on, none left to its empty default. */
    @Test
    fun aSessionsListenerHearsEveryEvent() {
        val offered = Class.forName("com.mmcdole.kivo.android.player.SessionPlayer\$Offered")
        fun signature(m: java.lang.reflect.Method) = m.name + m.parameterTypes.joinToString(",", "(", ")") { it.name }
        val events = Player.Listener::class.java.methods.map(::signature).toSet()
        val passed = offered.declaredMethods.map(::signature).toSet()
        assertEquals(emptySet(), events - passed)
    }
}
