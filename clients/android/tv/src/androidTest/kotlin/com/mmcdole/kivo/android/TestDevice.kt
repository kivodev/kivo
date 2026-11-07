package com.mmcdole.kivo.android

import android.content.Context
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.mmcdole.kivo.android.player.AndroidCapabilityProbe
import com.mmcdole.kivo.core.auth.SecureStore
import com.mmcdole.kivo.core.session.Device
import java.io.File

/** This TV for a test: its decoders, with preferences kept in [preferences] and sessions in memory. */
class TestDevice(context: Context, preferences: File) : Device {
    override val name = "Test TV"
    override val identity = AndroidIdentity(context, "test")
    override val preferences = PreferenceDataStoreFactory.create { preferences }
    override val secureStore: SecureStore = MemoryStore()
    override val capabilities = AndroidCapabilityProbe(context)
}

/** A [SecureStore] in memory. */
private class MemoryStore : SecureStore {
    private val values = mutableMapOf<String, String>()

    override suspend fun read(key: String) = values[key]

    override suspend fun write(key: String, value: String) {
        values[key] = value
    }

    override suspend fun delete(key: String) {
        values.remove(key)
    }
}
