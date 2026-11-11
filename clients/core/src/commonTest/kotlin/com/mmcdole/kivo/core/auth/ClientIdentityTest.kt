package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.models.ClientPlatform
import com.mmcdole.kivo.core.FakeServer
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

private data class Identity(
    override val app: String = "kivo-android",
    override val appVersion: String = "0.3.0+78",
    override val platform: ClientPlatform = ClientPlatform.AndroidTv,
    override val osVersion: String = "14",
    override val deviceModel: String = "SHIELD Android TV",
) : ClientIdentity

class ClientIdentityTest {
    @Test
    fun aHeaderPerField() {
        assertEquals(
            mapOf(
                "Kivo-App" to "kivo-android",
                "Kivo-App-Version" to "0.3.0+78",
                "Kivo-Platform" to "androidTv",
                "Kivo-OS-Version" to "14",
                "Kivo-Device-Model" to "SHIELD Android TV",
            ),
            identityHeaders(Identity()),
        )
    }

    @Test
    fun aValueKeepsPrintableAsciiOnlyUpToAHundredCharacters() {
        val model = identityHeaders(Identity(deviceModel = "Télé\tSalon\u0007 " + "x".repeat(200))).getValue("Kivo-Device-Model")
        assertEquals(100, model.length)
        assertTrue(model.startsWith("TlSalon xxx"), model)
    }

    @Test
    fun aFieldItCantFillIsLeftOut() {
        val headers = identityHeaders(Identity(appVersion = "", deviceModel = "中文"))
        assertEquals(setOf("Kivo-App", "Kivo-Platform", "Kivo-OS-Version"), headers.keys)
    }

    @Test
    fun everyRequestCarriesThem() = runTest {
        val server = FakeServer()
        val connection = server.connection(identity = Identity())
        runCatching { connection.setup.getServer() }
        runCatching { connection.users.getCurrentUser() }
        assertEquals(listOf(identityHeaders(Identity()), identityHeaders(Identity())), server.seen.map { it.identity })
    }

    @Test
    fun withoutAnIdentityThereAreNoHeaders() = runTest {
        val server = FakeServer()
        runCatching { server.connection().setup.getServer() }
        assertTrue(server.seen.single().identity.isEmpty())
    }
}
