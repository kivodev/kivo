package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.models.ServerInfo
import io.ktor.http.URLBuilder
import io.ktor.http.URLProtocol
import io.ktor.http.Url
import io.ktor.http.encodedPath
import io.ktor.http.isSecure
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.cancelChildren
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.joinAll
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.coroutines.CoroutineContext
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TimeMark
import kotlin.time.TimeSource
import kotlinx.io.IOException

/** What makes the app pick a server's address again. */
internal enum class AddressChange {
    /** The device's network changed: another Wi-Fi, cellular, a VPN on or off. */
    Network,

    /** A request got no answer at the address in use. */
    NoAnswer,

    /** The app came to the foreground. */
    Foreground,

    /** A request would carry the session's token, and the address in use may not be trusted with it now. */
    Untrusted,
}

/**
 * The device's network attachments, counted: each change the platform
 * reports starts a new one ([changed]). A plain-HTTP address proven on an
 * earlier one is trusted no longer. Any thread.
 */
internal class NetworkEpoch {
    private val mutable = MutableStateFlow(0)

    /** The attachment now, counting from 0. */
    val epoch: StateFlow<Int> = mutable.asStateFlow()

    val now: Int get() = mutable.value

    fun changed() = mutable.update { it + 1 }
}

/** No address of the server may be trusted with the session's token now, so a request that would carry it isn't sent. */
internal class UntrustedAddress : IOException("no address of the server may be trusted with the session now")

/**
 * Where a server is reached now ([current]), among the addresses it gives,
 * and picking again as the situation changes ([repick]): a local or private
 * address when one answers, else a public one ([pickAddress]). The address
 * in use stays while it answers as well as any other, so a pick moves only
 * off an address that no longer answers, or from a public address to a
 * local one; with no answer anywhere it stays where it is.
 *
 * The session's token goes only to an address that may be trusted with it
 * now ([carries]): an HTTPS one, which WebPKI vouches for, or a plain-HTTP
 * one picked since the device's [network] last changed. [probe] answers
 * only for an address that may be trusted, a plain-HTTP one by the server's
 * proof to the probe's challenge, so a pick proves the address it picks. A
 * network change ends the trust in a plain-HTTP address at once: the
 * address moves to an HTTPS one when the server has one ([untrust]), else a
 * request waits for a pick to prove one again ([trusted]).
 *
 * A play's media, whose URLs carry the play rather than the token, stays
 * at the address the play started on for as long as the play lasts: a move
 * never loads it again. A play started after the move is at the new
 * address, as is a play's media that got no answer, loaded again once the
 * server answers at an address picked again. While a stream plays from the server ([streaming]), the
 * address in use stays while it answers too: [preferLocal] waits, and a
 * repick moves only off an address that no longer answers.
 *
 * A repick asked for while one runs takes that one's answer, and one asked
 * for within [rest] of the last, other than for a network change or for an
 * address not yet proven on this network, is skipped, so a burst of
 * failing requests asks once.
 *
 * A URL made for any address the session has known is sent to the current
 * one ([rebase]), so a request made just before a move still arrives.
 *
 * Picks wait on [context]'s clock, off the main thread by default, as the
 * HTTP client's requests do; a test gives its own.
 */
internal class ServerAddress(
    private val serverId: String,
    initial: String,
    addresses: List<String>,
    private val probe: suspend (String) -> ServerInfo?,
    private val network: NetworkEpoch = NetworkEpoch(),
    private val grace: Duration = PICK_GRACE,
    private val wait: Duration = PICK_WAIT,
    private val rest: Duration = PICK_REST,
    private val time: TimeSource = TimeSource.Monotonic,
    private val context: CoroutineContext = Dispatchers.Default,
) {
    private val mutableCurrent = MutableStateFlow(initial)

    /** The base URL requests go to now. */
    val current: StateFlow<String> = mutableCurrent.asStateFlow()

    /** The server's addresses, to pick among. */
    private val addresses = MutableStateFlow((addresses + initial).filterNot(::isLinkLocal).distinct())

    /** Every address the session has known, parsed, for [rebase]. */
    private val known = MutableStateFlow(this.addresses.value.map(::Url))

    /** A plain-HTTP address a pick proved, on which network, for which token ([reset]). */
    private data class Proven(val address: String, val epoch: Int, val token: Int)

    private val proven = MutableStateFlow<Proven?>(null)

    /** Tokens the session has had, counted, so a pick for the last can't prove an address for the next. */
    private val tokens = MutableStateFlow(0)

    /** The network the last pick ran on, for the token held now; -1 for none. */
    private val pickedOn = MutableStateFlow(-1)

    private val lock = Mutex()

    /** Picks done, so one asked for while another ran takes its answer. */
    private val done = MutableStateFlow(0)
    /** When the address was last picked: as the session opened, at the address just picked. */
    private var lastPick: TimeMark = time.markNow()

    /** The streams playing from the server now. */
    private val playing = MutableStateFlow(emptySet<Any>())

    /** Whether a stream plays from the server now, so a move to a local address waits. */
    val streaming: Flow<Boolean> = playing.map { it.isNotEmpty() }.distinctUntilChanged()

    /** Whether the address in use may be trusted with the token now, as it changes. */
    val trusted: Flow<Boolean> =
        combine(mutableCurrent, proven, network.epoch, tokens) { _, _, _, _ -> trustedNow() }.distinctUntilChanged()

    /** Whether [stream] plays from the server now. */
    fun streaming(stream: Any, playing: Boolean) =
        this.playing.update { if (playing) it + stream else it - stream }

    /** The addresses the server gives now, kept to pick among. */
    fun remember(addresses: List<String>) {
        val kept = addresses.filterNot(::isLinkLocal)
        this.addresses.value = (kept + mutableCurrent.value).filterNot(::isLinkLocal).distinct()
        known.update { all -> all + kept.map(::Url).filter { it !in all } }
    }

    /** Whether the address in use may be trusted with the token now. */
    fun trustedNow(): Boolean = trusts(mutableCurrent.value)

    private fun trusts(address: String): Boolean =
        isSecure(address) || proven.value == Proven(address, network.now, tokens.value)

    /**
     * Off an address that may no longer be trusted with the token, moves
     * to the server's HTTPS address when it has one, at once; the address
     * in use now.
     */
    fun untrust(): String {
        val at = mutableCurrent.value
        if (trusts(at)) return at
        val secure = addresses.value.firstOrNull(::isSecure) ?: return at
        mutableCurrent.compareAndSet(at, secure)
        return mutableCurrent.value
    }

    /**
     * Returns once the address in use may be trusted with the token: at
     * once while it may, else after [untrust] or a pick proves one; throws
     * [UntrustedAddress] when none can be.
     */
    suspend fun trusted() {
        untrust()
        if (trustedNow()) return
        repick(AddressChange.Untrusted)
        if (!trustedNow()) throw UntrustedAddress()
    }

    /** The token changed: no plain-HTTP address is trusted with the new one until a pick proves it. */
    fun reset() {
        tokens.update { it + 1 }
        proven.value = null
        pickedOn.value = -1
    }

    /** The server's HTTPS address: the one in use when it is one, else its first; null when it has none. */
    fun secure(): String? = mutableCurrent.value.takeIf(::isSecure) ?: addresses.value.firstOrNull(::isSecure)

    /** Picks again for [change]; whether the address moved. */
    suspend fun repick(change: AddressChange): Boolean {
        val before = mutableCurrent.value
        val asked = done.value
        return lock.withLock {
            // One done since this was asked has answered it.
            if (done.value != asked) return@withLock mutableCurrent.value != before
            val unproven = change == AddressChange.Untrusted && pickedOn.value != network.now
            if (change != AddressChange.Network && !unproven && lastPick.elapsedNow() < rest) return@withLock false
            val current = mutableCurrent.value
            val on = Proven(current, network.now, tokens.value)
            val picked = withContext(context) {
                val keep = change != AddressChange.NoAnswer && playing.value.isNotEmpty() && answers(current)
                if (keep) current else pickAddress(serverId, addresses.value, current, grace, wait, probe)?.first
            }
            picked(on)
            move(picked, on)
        }
    }

    /**
     * Picks among every address as the session opens: the address picked
     * and what it said of itself; null when none answers, and the address
     * stays.
     */
    suspend fun open(): Pair<String, ServerInfo>? = lock.withLock {
        val on = Proven(mutableCurrent.value, network.now, tokens.value)
        val picked = withContext(context) { pickAddress(serverId, addresses.value, null, grace, wait, probe) }
        picked(on)
        move(picked?.first, on)
        picked
    }

    /**
     * On a public address, moves to a local one that answers, as the device
     * may have come home without its network changing; whether it moved.
     * It asks the local addresses alone, so it's cheap to do now and then.
     */
    suspend fun preferLocal(): Boolean {
        if (isLocal(mutableCurrent.value) || playing.value.isNotEmpty()) return false
        val locals = addresses.value.filter(::isLocal).ifEmpty { return false }
        return lock.withLock {
            if (isLocal(mutableCurrent.value)) return@withLock false
            val on = Proven(mutableCurrent.value, network.now, tokens.value)
            move(withContext(context) { pickAddress(serverId, locals, null, grace, wait, probe) }?.first, on)
        }
    }

    private suspend fun answers(address: String): Boolean =
        withTimeoutOrNull(wait) { quietly { probe(address) } }?.id == serverId

    /** A pick begun [on] a network has run. */
    private fun picked(on: Proven) {
        lastPick = time.markNow()
        done.update { it + 1 }
        if (tokens.value == on.token) pickedOn.value = on.epoch
    }

    /** Moves to [to], which answered a pick begun [on] a network, as one trusted there; whether it moved. */
    private fun move(to: String?, on: Proven): Boolean {
        if (to == null) return false
        if (!isSecure(to) && tokens.value == on.token) proven.value = on.copy(address = to)
        if (to == mutableCurrent.value) return false
        mutableCurrent.value = to
        return true
    }

    /** Whether [url] is at an address the session has known. */
    fun knows(url: URLBuilder): Boolean = known.value.any { it.covers(url) }

    /**
     * Whether [url] may carry the token now: it's at an HTTPS address of
     * the server's, or at the address in use while that may be trusted.
     */
    fun carries(url: URLBuilder): Boolean {
        val at = known.value.filter { it.covers(url) }.maxByOrNull { it.encodedPath.trimEnd('/').length } ?: return false
        if (at.protocol.isSecure()) return true
        val current = mutableCurrent.value
        return at.isSameBase(Url(current)) && trusts(current)
    }

    /** Sends [url], when it's at an address the session has known, to the current one instead, path and query kept. */
    fun rebase(url: URLBuilder) {
        val to = Url(mutableCurrent.value)
        val from = known.value.filter { it.covers(url) }.maxByOrNull { it.encodedPath.trimEnd('/').length } ?: return
        if (from.isSameBase(to)) return
        val socket = url.protocol.name.startsWith("ws")
        url.protocol = when {
            socket && to.protocol.isSecure() -> URLProtocol.WSS
            socket -> URLProtocol.WS
            else -> to.protocol
        }
        url.host = to.host
        url.port = to.specifiedPort
        url.encodedPath = to.encodedPath.trimEnd('/') + url.encodedPath.removePrefix(from.encodedPath.trimEnd('/'))
    }
}

/** How long a pick waits, after its first answer, for a local address to answer too. */
internal val PICK_GRACE: Duration = 400.milliseconds

/** How long a pick waits for an address to answer at all. */
internal val PICK_WAIT: Duration = 3.seconds

/** How soon after a pick another is worth asking for, but for a network change. */
internal val PICK_REST: Duration = 10.seconds

/**
 * Of [addresses], the one to reach server [serverId] at, with what it said
 * of itself; null when none answers as it. Every address is asked at once
 * ([probe], each for at most [wait]); one that answers as another server
 * doesn't count. A local address wins as soon as it answers; the first
 * public one to answer waits [grace] for a local one. Among answers alike,
 * [prefer], the address in use, wins, so a pick doesn't move for nothing;
 * otherwise the first to answer does.
 */
internal suspend fun pickAddress(
    serverId: String,
    addresses: List<String>,
    prefer: String?,
    grace: Duration,
    wait: Duration,
    probe: suspend (String) -> ServerInfo?,
): Pair<String, ServerInfo>? = coroutineScope {
    val answers = Channel<Pair<String, ServerInfo>>(Channel.UNLIMITED)
    val probes = addresses.distinct().map { address ->
        launch {
            val info = withTimeoutOrNull(wait) { quietly { probe(address) } }
            if (info?.id == serverId) answers.send(address to info)
        }
    }
    launch {
        probes.joinAll()
        answers.close()
    }
    val answered = mutableListOf<Pair<String, ServerInfo>>()
    // Whether a later answer could change the pick: a local one to come, or the address in use.
    fun settled(): Boolean {
        val preferred = prefer == null || prefer !in addresses || answered.any { it.first == prefer }
        return if (answered.any { isLocal(it.first) }) preferred || !isLocal(prefer) else preferred && addresses.none(::isLocal)
    }
    answers.receiveCatching().getOrNull()?.let { answered += it }
    if (answered.isNotEmpty() && !settled()) {
        withTimeoutOrNull(grace) {
            for (answer in answers) {
                answered += answer
                if (settled()) break
            }
        }
    }
    this.coroutineContext.cancelChildren()
    val best = answered.filter { isLocal(it.first) }.ifEmpty { answered }
    best.firstOrNull { it.first == prefer } ?: best.firstOrNull()
}

private suspend fun <T> quietly(block: suspend () -> T): T? = try {
    block()
} catch (e: CancellationException) {
    throw e
} catch (_: Exception) {
    null
}

/**
 * Whether [address] is on a local or private network rather than the
 * internet: a private, shared (as a VPN's), link-local or loopback IP, or a
 * name only a local network or VPN resolves (one label, `.local`, `.lan`,
 * `.home.arpa`, `.ts.net`).
 */
internal fun isLocal(address: String): Boolean {
    val host = try {
        Url(address).host
    } catch (_: Exception) {
        return false
    }.lowercase().removePrefix("[").removeSuffix("]")
    val ip = host.split('.').map { it.toIntOrNull() ?: -1 }
    if (ip.size == 4 && ip.all { it in 0..255 }) {
        val (a, b) = ip
        return a == 10 || a == 127 || (a == 172 && b in 16..31) || (a == 192 && b == 168) || (a == 169 && b == 254) ||
            (a == 100 && b in 64..127)
    }
    if (':' in host) return host == "::1" || host.startsWith("fc") || host.startsWith("fd") || host.startsWith("fe80")
    return '.' !in host || LOCAL_SUFFIXES.any { host.endsWith(it) }
}

/** Whether [address] is HTTPS. */
internal fun isSecure(address: String): Boolean = try {
    Url(address).protocol.isSecure()
} catch (_: Exception) {
    false
}

/** Whether [address] is at an IPv6 link-local address, which names an interface rather than a server, so isn't kept. */
internal fun isLinkLocal(address: String): Boolean {
    val host = try {
        Url(address).host
    } catch (_: Exception) {
        return false
    }.lowercase().removePrefix("[")
    return ':' in host && host.length >= 4 && host.startsWith("fe") && host[2] in "89ab"
}

private val LOCAL_SUFFIXES = listOf(".local", ".lan", ".home.arpa", ".ts.net")

/** Whether this base URL's server is where [url] goes: its scheme (a socket's alike), host, port and path below. */
private fun Url.covers(url: URLBuilder): Boolean {
    if (!host.equals(url.host, ignoreCase = true) || protocol.isSecure() != url.protocol.isSecure()) return false
    val port = url.port.takeIf { it != 0 } ?: url.protocol.defaultPort
    if (this.port != port) return false
    val base = encodedPath.trimEnd('/')
    return url.encodedPath == base || url.encodedPath.startsWith("$base/")
}

private fun Url.isSameBase(other: Url): Boolean =
    host.equals(other.host, ignoreCase = true) && port == other.port && protocol.isSecure() == other.protocol.isSecure() &&
        encodedPath.trimEnd('/') == other.encodedPath.trimEnd('/')
