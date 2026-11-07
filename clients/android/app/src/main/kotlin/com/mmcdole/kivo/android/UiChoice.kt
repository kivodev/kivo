package com.mmcdole.kivo.android

import android.app.UiModeManager
import android.content.Context
import android.content.pm.PackageManager
import android.content.res.Configuration

/** The interface the app shows: the TV's, or the phone's. */
enum class Ui { Tv, Mobile }

/** The TV interface on a television, as the system's UI mode says or the leanback feature shows; else the phone's. */
fun chooseUi(television: Boolean, leanback: Boolean): Ui = if (television || leanback) Ui.Tv else Ui.Mobile

/** The interface for this device. */
fun chooseUi(context: Context): Ui = chooseUi(
    television = context.getSystemService(UiModeManager::class.java)?.currentModeType == Configuration.UI_MODE_TYPE_TELEVISION,
    leanback = context.packageManager.hasSystemFeature(PackageManager.FEATURE_LEANBACK),
)
