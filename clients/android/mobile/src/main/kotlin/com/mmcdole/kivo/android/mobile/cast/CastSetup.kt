package com.mmcdole.kivo.android.mobile.cast

import android.content.Context
import com.google.android.gms.cast.framework.CastOptions
import com.google.android.gms.cast.framework.OptionsProvider
import com.google.android.gms.cast.framework.SessionProvider
import com.google.android.gms.cast.framework.media.CastMediaOptions
import com.google.android.gms.cast.framework.media.NotificationOptions
import com.mmcdole.kivo.core.cast.Cast

/**
 * Google Cast's setup, which the framework reads from the manifest as the
 * phone's screens first ask for it: Kivo's Web Receiver, a session joined
 * again after the app was away, and Google's cast notification, opening the
 * app. A TV never asks, so it never sets Cast up.
 */
class CastSetup : OptionsProvider {
    override fun getCastOptions(context: Context): CastOptions {
        val app = context.packageManager.getLaunchIntentForPackage(context.packageName)?.component?.className
        val notification = NotificationOptions.Builder().apply { app?.let(::setTargetActivityClassName) }.build()
        return CastOptions.Builder()
            .setReceiverApplicationId(Cast.APP_ID)
            .setResumeSavedSession(true)
            .setCastMediaOptions(CastMediaOptions.Builder().setNotificationOptions(notification).build())
            .build()
    }

    override fun getAdditionalSessionProviders(context: Context): List<SessionProvider>? = null
}
