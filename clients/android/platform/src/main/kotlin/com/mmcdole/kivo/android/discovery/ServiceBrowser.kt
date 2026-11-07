package com.mmcdole.kivo.android.discovery

import com.mmcdole.kivo.core.discovery.FoundServer
import com.mmcdole.kivo.core.session.KivoClient
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.time.Duration

/**
 * A platform's DNS-SD browse for servers: it reports each service it finds
 * of type `_kivo._tcp` until stopped, and resolves one to its addresses,
 * port and TXT record.
 */
internal interface ServiceBrowser<S> {
    fun start(onFound: (S) -> Unit)

    fun stop()

    /** [service]'s addresses, port and TXT record; null when it can't be resolved. */
    suspend fun resolve(service: S): ResolvedService?
}

/** A resolved service: its host addresses, port and TXT record. */
internal data class ResolvedService(val hosts: List<String>, val port: Int, val txt: Map<String, String>)

/** A server as its service advertises it: the ID its TXT record names, its hosts (IPv4 first), port, and whether and where it's served. */
internal data class Advertised(val id: String, val hosts: List<String>, val port: Int, val tls: Boolean, val path: String)

/**
 * Each server this browse finds within [timeout] that [core] confirms
 * answers with the ID it advertised, once each, in the order found.
 */
internal fun <S> ServiceBrowser<S>.servers(core: KivoClient, timeout: Duration): Flow<FoundServer> = flow {
    val seen = mutableSetOf<String>()
    find(timeout).collect { advertised ->
        if (advertised.id in seen) return@collect
        core.confirmDiscovered(advertised.id, advertised.hosts, advertised.port, advertised.tls, advertised.path)?.let {
            seen += advertised.id
            emit(it)
        }
    }
}

/**
 * The servers this browse finds until [timeout] passes, each as it's
 * advertised. Services are resolved one at a time, in the order
 * found, since some systems refuse a resolve while another runs. The browse
 * stops when the flow ends or its collector stops.
 */
internal fun <S> ServiceBrowser<S>.find(timeout: Duration): Flow<Advertised> = channelFlow {
    val found = Channel<S>(Channel.UNLIMITED)
    start { found.trySend(it) }
    try {
        withTimeoutOrNull(timeout) {
            for (service in found) {
                resolve(service)?.advertised()?.let { send(it) }
            }
        }
    } finally {
        stop()
    }
}

/** The server a resolved service advertises, IPv4 hosts first; null without an ID. */
internal fun ResolvedService.advertised(): Advertised? {
    val id = txt["id"]?.takeIf { it.isNotEmpty() } ?: return null
    return Advertised(id, hosts.sortedBy { ':' in it }.distinct(), port, txt["tls"] == "1", txt["path"].orEmpty())
}
