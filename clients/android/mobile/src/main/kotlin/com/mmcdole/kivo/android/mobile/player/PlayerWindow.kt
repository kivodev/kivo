package com.mmcdole.kivo.android.mobile.player

import android.app.PendingIntent
import android.app.PictureInPictureParams
import android.app.RemoteAction
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.ActivityInfo
import android.content.pm.PackageManager
import android.graphics.drawable.Icon
import android.os.Build
import android.util.Rational
import androidx.activity.ComponentActivity
import androidx.activity.compose.LocalActivity
import androidx.annotation.RequiresApi
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.core.content.ContextCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import com.mmcdole.kivo.android.mobile.R

/**
 * The player's hold on the window while it shows: landscape whichever way
 * the phone is turned, the system bars hidden until a swipe from an edge
 * brings them back for a moment, and the screen kept on while [awake];
 * all as they were once it goes.
 */
@Composable
fun PlayerWindow(awake: Boolean) {
    val activity = LocalActivity.current ?: return
    val view = LocalView.current
    DisposableEffect(activity) {
        val bars = WindowCompat.getInsetsController(activity.window, activity.window.decorView)
        // The first player to come keeps what to put back, and the last to go puts it back.
        if (Held.players++ == 0) Held.orientation = activity.requestedOrientation
        activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
        bars.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        bars.hide(WindowInsetsCompat.Type.systemBars())
        onDispose {
            if (--Held.players == 0) {
                bars.show(WindowInsetsCompat.Type.systemBars())
                activity.requestedOrientation = Held.orientation
            }
        }
    }
    DisposableEffect(awake) {
        view.keepScreenOn = awake
        onDispose { view.keepScreenOn = false }
    }
}

/**
 * How many players hold the window and the orientation it had before the
 * first, and how many offer picture in picture, as the next episode's
 * player comes before the last one goes. On the main thread.
 */
private object Held {
    var players = 0
    var orientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
    var pictureInPicture = 0
}

/** Whether this device offers picture in picture. */
@Composable
fun pictureInPictureOffered(): Boolean =
    LocalActivity.current?.packageManager?.hasSystemFeature(PackageManager.FEATURE_PICTURE_IN_PICTURE) == true

/**
 * Picture in picture for the player: while the video [playing], leaving
 * the app shrinks it into a window of the video's [aspect] (by itself from
 * Android 12, and on leaving before), with Play or Pause in it, which
 * [onToggle]s; [onChange] hears it begin and end. Leaving the player
 * turns it off, so the app's other screens never shrink.
 */
@Composable
fun PictureInPicture(playing: Boolean, aspect: Rational?, onToggle: () -> Unit, onChange: (Boolean) -> Unit) {
    val activity = LocalActivity.current as? ComponentActivity ?: return
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
    val toggle by rememberUpdatedState(onToggle)
    val change by rememberUpdatedState(onChange)
    val label = stringResource(if (playing) R.string.mobile_pause else R.string.mobile_play)

    DisposableEffect(activity) {
        val modes = androidx.core.util.Consumer<androidx.core.app.PictureInPictureModeChangedInfo> { change(it.isInPictureInPictureMode) }
        activity.addOnPictureInPictureModeChangedListener(modes)
        val receiver = object : BroadcastReceiver() {
            override fun onReceive(context: Context, intent: Intent) = toggle()
        }
        ContextCompat.registerReceiver(activity, receiver, IntentFilter(ACTION_TOGGLE), ContextCompat.RECEIVER_NOT_EXPORTED)
        Held.pictureInPicture++
        onDispose {
            activity.removeOnPictureInPictureModeChangedListener(modes)
            activity.unregisterReceiver(receiver)
            if (--Held.pictureInPicture == 0 && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                activity.setPictureInPictureParams(PictureInPictureParams.Builder().setAutoEnterEnabled(false).build())
            }
        }
    }

    DisposableEffect(playing, aspect, label) {
        val params = pictureInPictureParams(activity, playing, aspect, label)
        activity.setPictureInPictureParams(params)
        // Before Android 12 the player asks as the user leaves the app.
        val leaving = Runnable { if (playing) activity.enterPictureInPictureMode(params) }
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S) activity.addOnUserLeaveHintListener(leaving)
        onDispose { if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S) activity.removeOnUserLeaveHintListener(leaving) }
    }
}

/** Shrinks the player into picture in picture now, as its button does. */
fun enterPictureInPicture(activity: ComponentActivity?, aspect: Rational?) {
    if (activity == null || Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
    activity.enterPictureInPictureMode(PictureInPictureParams.Builder().setAspectRatio(aspect ?: Rational(16, 9)).build())
}

@RequiresApi(Build.VERSION_CODES.O)
private fun pictureInPictureParams(context: Context, playing: Boolean, aspect: Rational?, label: String): PictureInPictureParams {
    val intent = PendingIntent.getBroadcast(context, 0, Intent(ACTION_TOGGLE).setPackage(context.packageName), PendingIntent.FLAG_IMMUTABLE)
    val icon = Icon.createWithResource(context, if (playing) R.drawable.mobile_pause else R.drawable.mobile_play)
    val builder = PictureInPictureParams.Builder()
        .setAspectRatio(aspect ?: Rational(16, 9))
        .setActions(listOf(RemoteAction(icon, label, label, intent)))
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) builder.setAutoEnterEnabled(playing).setSeamlessResizeEnabled(true)
    return builder.build()
}

/** The aspect of a video [width] by [height], inside what picture in picture takes: 2.39:1 at the widest and 1:2.39 at the tallest. */
fun pictureInPictureAspect(width: Int?, height: Int?): Rational? {
    if (width == null || height == null || width <= 0 || height <= 0) return null
    val ratio = width.toFloat() / height
    return when {
        ratio > MAX_ASPECT -> Rational(239, 100)
        ratio < 1 / MAX_ASPECT -> Rational(100, 239)
        else -> Rational(width, height)
    }
}

private const val MAX_ASPECT = 2.39f

/** Picture in picture's Play or Pause, sent to the app itself. */
private const val ACTION_TOGGLE = "com.mmcdole.kivo.action.TOGGLE_PLAY"
