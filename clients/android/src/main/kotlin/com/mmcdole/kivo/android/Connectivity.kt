package com.mmcdole.kivo.android

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import com.mmcdole.kivo.core.session.KivoClient

/**
 * Tells the core each time the device's network changes, as to another
 * network or a VPN, so the session picks its server's address again:
 * whether there's one, and whether it's metered. Only a new default network
 * or losing it counts, not each change in its signal.
 */
fun watchNetwork(context: Context, core: KivoClient) {
    val manager = context.getSystemService(ConnectivityManager::class.java) ?: return
    manager.registerDefaultNetworkCallback(
        object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                val metered = manager.getNetworkCapabilities(network)?.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED) == false
                core.networkChanged(online = true, expensive = metered)
            }

            override fun onLost(network: Network) {
                core.networkChanged(online = false, expensive = false)
            }
        },
    )
}
