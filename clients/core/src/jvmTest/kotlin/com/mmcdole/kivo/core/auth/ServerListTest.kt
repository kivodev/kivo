package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.core.tempDataStore
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals

class ServerListTest {
    @Test
    fun keepsServersByTheirId() = runTest {
        val servers = ServerList(backgroundScope.tempDataStore())
        val basement = SavedServer(ServerInfo("server-1", "Basement", "1.0"), "http://192.168.1.20:8484")
        servers.save(basement)
        servers.save(SavedServer(ServerInfo("server-2", "Attic", "1.0"), "http://192.168.1.30:8484"))
        servers.save(basement.copy(name = "Downstairs"))

        assertEquals(listOf("Attic", "Downstairs"), servers.servers.first().map { it.name })

        servers.remove("server-2")
        assertEquals(listOf("server-1"), servers.servers.first().map { it.id })
    }
}
