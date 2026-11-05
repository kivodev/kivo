package com.mmcdole.kivo.core.discovery

import com.mmcdole.kivo.api.apis.SetupApi
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.PICK_GRACE
import com.mmcdole.kivo.core.auth.PICK_WAIT
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.auth.pickAddress
import io.ktor.client.HttpClient
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlin.coroutines.CoroutineContext
import kotlin.time.Duration

/** The DNS-SD service type servers advertise themselves as. */
const val SERVICE_TYPE = "_kivo._tcp"

/** The port a server listens on when an address names none. */
internal const val DEFAULT_PORT = 8484

/** A server the platform's browse found: the ID its advertisement names, and the base URLs it may be at, best first. */
internal data class FoundAddress(val id: String, val urls: List<String>)

/** A server confirmed at an address: the base URL it answered at, and what it said of itself. */
data class FoundServer(val baseUrl: String, val info: ServerInfo)

/**
 * Finding servers: as the platform's browse for [SERVICE_TYPE] services
 * finds them on the local network, by a typed address, or, for a saved
 * server, by asking all its addresses at once ([pickAddress]).
 * Each server is named by `GET /api/v1/server`, whose stable ID keys its
 * sign-in, and a found server counts only when the ID it answers with is
 * the one it advertised. It asks through the app's [probes] client.
 */
internal class Discovery(
    private val probes: HttpClient,
    private val timeout: Duration = PICK_WAIT,
    private val context: CoroutineContext = Dispatchers.Default,
) {

    /** [found] at the first of its URLs that answers with the ID it advertised; null when none does. */
    suspend fun confirm(found: FoundAddress): FoundServer? {
        for (url in found.urls) {
            val info = infoOrNull(url) ?: continue
            if (info.id == found.id) return FoundServer(url, info)
        }
        return null
    }

    /** The server at a typed address, such as `192.168.1.20`, `nas:8484` or `https://kivo.example.com`. */
    suspend fun serverAt(address: String): FoundServer {
        val url = normalizeAddress(address)
        return FoundServer(url, info(url))
    }

    /**
     * [server] at the address to reach it at: a local one that answers as
     * the same server, else a public one; null when none does.
     */
    suspend fun reach(server: SavedServer): FoundServer? {
        val (address, info) = withContext(context) {
            pickAddress(server.id, server.addresses, null, PICK_GRACE, timeout) { SetupApi(it, probes).getServer().result() }
        } ?: return null
        return FoundServer(address, info)
    }

    private suspend fun infoOrNull(baseUrl: String): ServerInfo? = try {
        info(baseUrl)
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        null
    }

    private suspend fun info(baseUrl: String): ServerInfo = SetupApi(baseUrl, probes).getServer().result()
}

/**
 * The base URL of a resolved service: its host and port, `https` when its
 * TXT record says `tls=1`, and its TXT record's `path`.
 */
fun serviceUrl(host: String, port: Int, tls: Boolean, path: String): String {
    val bracketed = if (':' in host && !host.startsWith("[")) "[${host.substringBefore('%')}]" else host
    return "${if (tls) "https" else "http"}://$bracketed:$port${path.trimEnd('/')}"
}

/**
 * A typed address as a base URL: `http://` when it names no scheme, port
 * [DEFAULT_PORT] when plain HTTP names none, and no trailing slash, keeping
 * any base path (`https://example.com/kivo`).
 */
internal fun normalizeAddress(address: String): String {
    val trimmed = address.trim().trimEnd('/')
    val withScheme = if ("://" in trimmed) trimmed else "http://$trimmed"
    val scheme = withScheme.substringBefore("://")
    val rest = withScheme.substringAfter("://")
    val host = rest.substringBefore('/')
    val path = rest.removePrefix(host)
    val hasPort = if (host.startsWith("[")) "]:" in host else ':' in host
    val port = if (!hasPort && scheme == "http") ":$DEFAULT_PORT" else ""
    return "$scheme://$host$port$path"
}
