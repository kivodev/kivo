package com.mmcdole.kivo.android.discovery

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import androidx.annotation.RequiresApi
import com.mmcdole.kivo.core.discovery.SERVICE_TYPE
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.withTimeoutOrNull
import java.net.InetAddress
import kotlin.time.Duration.Companion.seconds

/**
 * Browsing for servers with Android's [NsdManager]. A resolve uses the
 * service-info callback from Android 14, and the older resolveService
 * before, which allows one resolve at a time; [find] asks for one at a time
 * either way.
 */
internal class NsdServiceBrowser(private val context: Context) : ServiceBrowser<NsdServiceInfo> {
    private val nsd = context.getSystemService(NsdManager::class.java)
    private var listener: NsdManager.DiscoveryListener? = null

    override fun start(onFound: (NsdServiceInfo) -> Unit) {
        val l = object : NsdManager.DiscoveryListener {
            override fun onServiceFound(service: NsdServiceInfo) = onFound(service)
            override fun onServiceLost(service: NsdServiceInfo) = Unit
            override fun onDiscoveryStarted(serviceType: String) = Unit
            override fun onDiscoveryStopped(serviceType: String) = Unit
            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) = Unit
            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) = Unit
        }
        listener = l
        nsd.discoverServices(SERVICE_TYPE, NsdManager.PROTOCOL_DNS_SD, l)
    }

    override fun stop() {
        val l = listener ?: return
        listener = null
        // Stopping a discovery that failed to start throws; there's nothing left to stop then.
        runCatching { nsd.stopServiceDiscovery(l) }
    }

    override suspend fun resolve(service: NsdServiceInfo): ResolvedService? = withTimeoutOrNull(RESOLVE_TIMEOUT) {
        val info = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) resolveByCallback(service) else resolveOnce(service)
        info?.let { ResolvedService(hostsOf(it), it.port, txtOf(it)) }
    }

    @RequiresApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
    private suspend fun resolveByCallback(service: NsdServiceInfo): NsdServiceInfo? {
        val resolved = CompletableDeferred<NsdServiceInfo?>()
        val callback = object : NsdManager.ServiceInfoCallback {
            override fun onServiceInfoCallbackRegistrationFailed(errorCode: Int) {
                resolved.complete(null)
            }

            override fun onServiceUpdated(serviceInfo: NsdServiceInfo) {
                if (serviceInfo.hostAddresses.isNotEmpty()) resolved.complete(serviceInfo)
            }

            override fun onServiceLost() = Unit

            override fun onServiceInfoCallbackUnregistered() = Unit
        }
        nsd.registerServiceInfoCallback(service, context.mainExecutor, callback)
        try {
            return resolved.await()
        } finally {
            runCatching { nsd.unregisterServiceInfoCallback(callback) }
        }
    }

    @Suppress("DEPRECATION") // The service-info callback replaces it from Android 14.
    private suspend fun resolveOnce(service: NsdServiceInfo): NsdServiceInfo? {
        val resolved = CompletableDeferred<NsdServiceInfo?>()
        nsd.resolveService(
            service,
            object : NsdManager.ResolveListener {
                override fun onResolveFailed(serviceInfo: NsdServiceInfo, errorCode: Int) {
                    resolved.complete(null)
                }

                override fun onServiceResolved(serviceInfo: NsdServiceInfo) {
                    resolved.complete(serviceInfo)
                }
            },
        )
        return resolved.await()
    }

    @Suppress("DEPRECATION")
    private fun hostsOf(info: NsdServiceInfo): List<String> {
        val addresses: List<InetAddress> =
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) info.hostAddresses else listOfNotNull(info.host)
        return addresses.mapNotNull { it.hostAddress }
    }

    private fun txtOf(info: NsdServiceInfo): Map<String, String> =
        info.attributes.mapValues { (_, value) -> value?.decodeToString().orEmpty() }
}

/** How long one resolve may take before the next service's turn. */
private val RESOLVE_TIMEOUT = 2.seconds
