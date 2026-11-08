package com.mmcdole.kivo.android.player

import androidx.media3.common.Player
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import java.lang.reflect.Proxy
import kotlin.test.Test
import kotlin.test.assertNull
import kotlin.test.assertSame

@OptIn(ExperimentalCoroutinesApi::class)
class SessionPlayersTest {
    /** A player that's only itself: the holder never plays it. */
    private fun player(): Player = Proxy.newProxyInstance(javaClass.classLoader, arrayOf(Player::class.java)) { proxy, method, args ->
        when (method.name) {
            "equals" -> proxy === args?.get(0)
            "hashCode" -> System.identityHashCode(proxy)
            "toString" -> "player"
            else -> null
        }
    } as Player

    @Test
    fun aWithdrawnPlayerStaysAWhileForTheNext() {
        val scope = TestScope()
        val players = SessionPlayers(scope)
        val first = player()
        val withdraw = players.offer(first)
        withdraw()
        scope.advanceTimeBy(SessionPlayers.GRACE_MS - 1)
        scope.runCurrent()
        assertSame(first, players.current.value)
        val second = player()
        players.offer(second)
        scope.advanceTimeBy(SessionPlayers.GRACE_MS * 2)
        scope.runCurrent()
        assertSame(second, players.current.value)
    }

    @Test
    fun aPlayerWithdrawnForGoodLeavesNone() {
        val scope = TestScope()
        val players = SessionPlayers(scope)
        val withdraw = players.offer(player())
        withdraw()
        scope.advanceTimeBy(SessionPlayers.GRACE_MS + 1)
        scope.runCurrent()
        assertNull(players.current.value)
    }

    @Test
    fun withdrawingAPlayerAnotherReplacedDoesNothing() {
        val scope = TestScope()
        val players = SessionPlayers(scope)
        val withdrawFirst = players.offer(player())
        val second = player()
        players.offer(second)
        withdrawFirst()
        scope.advanceTimeBy(SessionPlayers.GRACE_MS * 2)
        scope.runCurrent()
        assertSame(second, players.current.value)
    }
}
