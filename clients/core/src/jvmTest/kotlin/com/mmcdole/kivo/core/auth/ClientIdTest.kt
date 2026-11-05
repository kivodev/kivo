package com.mmcdole.kivo.core.auth

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.mmcdole.kivo.core.tempDataStore
import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotEquals
import kotlin.test.assertTrue
import kotlin.io.path.createTempDirectory
import okio.Path.Companion.toOkioPath

class ClientIdTest {
    @Test
    fun isMadeOnceAndKept() = runTest {
        val store = backgroundScope.tempDataStore()
        val id = ClientId(store).get()

        assertEquals(id, ClientId(store).get())
        assertNotEquals(id, ClientId(backgroundScope.tempDataStore()).get())
    }

    @Test
    fun aFixedIdReplacesTheInstallsOwnAsTheStoreOpens() = runTest {
        val file = createTempDirectory("kivo").resolve("device.preferences_pb").toOkioPath()
        val fixed = PreferenceDataStoreFactory.createWithPath(migrations = listOf(fixedClientId("kivo-ui-tests")), scope = backgroundScope) { file }
        assertEquals("kivo-ui-tests", ClientId(fixed).get())

        val key = stringPreferencesKey("kivo.clientId")
        val own = preferencesOf(key to "own")
        val migration = fixedClientId("kivo-ui-tests")
        assertTrue(migration.shouldMigrate(own))
        assertEquals("kivo-ui-tests", migration.migrate(own)[key])
    }
}
