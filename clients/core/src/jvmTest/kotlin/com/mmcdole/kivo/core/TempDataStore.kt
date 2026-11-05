package com.mmcdole.kivo.core

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import kotlinx.coroutines.CoroutineScope
import okio.Path.Companion.toOkioPath
import kotlin.io.path.createTempDirectory

/** A preferences DataStore in a new temporary directory, running in this scope. */
fun CoroutineScope.tempDataStore(): DataStore<Preferences> {
    val file = createTempDirectory("kivo").resolve("device.preferences_pb").toOkioPath()
    return PreferenceDataStoreFactory.createWithPath(scope = this) { file }
}
