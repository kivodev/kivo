package com.mmcdole.kivo.android.mobile.linkdevice

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.refuse
import com.mmcdole.kivo.android.mobile.reply
import com.mmcdole.kivo.api.models.PendingDevice
import com.mmcdole.kivo.core.auth.SavedServer
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.flow.flowOf
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.time.Instant

class LinkDeviceViewModelTest : CoreTest() {
    private val answered = CopyOnWriteArrayList<String>()
    private val tv = PendingDevice("Living Room", nearby = true, startedAt = Instant.fromEpochMilliseconds(0), platform = "androidTv")

    init {
        server.on("GET", "/api/v1/device-codes/K7Q2M9") { reply(PendingDevice.serializer(), tv) }
        server.on("POST", "/api/v1/device-codes/K7Q2M9/approve") {
            answered += "approve"
            respond("", HttpStatusCode.NoContent)
        }
        server.on("POST", "/api/v1/device-codes/K7Q2M9/deny") {
            answered += "deny"
            respond("", HttpStatusCode.NoContent)
        }
        server.on("GET", "/api/v1/device-codes/AAAAAA") { refuse(HttpStatusCode.NotFound, "notFound") }
    }

    private fun model(initial: String? = null, servers: List<SavedServer> = emptyList()) = LinkDeviceViewModel(signIn(), flowOf(servers), initial)

    @Test
    fun aTypedCodeAsksAboutTheDeviceAndAllowSignsItIn() {
        val model = model()
        model.lookUp("k7q-2m9")
        val confirm = awaitValue(model.state) { it.step is LinkStep.Confirm }.step as LinkStep.Confirm
        assertEquals("K7Q2M9", confirm.code)
        assertEquals("Living Room", confirm.device.name)
        assertEquals("anna", model.state.value.username)
        model.answer(true)
        assertEquals(LinkStep.Done("Living Room", allowed = true), awaitValue(model.state) { it.step is LinkStep.Done }.step)
        assertEquals(listOf("approve"), answered)
    }

    @Test
    fun theTvsLinkPageUrlFromItsQrCodeOpensTheSameDeviceAndDontAllowDeniesIt() {
        val model = model("https://kivo.test/link?code=K7Q2M9&server=s1")
        awaitValue(model.state) { it.step is LinkStep.Confirm }
        model.answer(false)
        assertEquals(LinkStep.Done("Living Room", allowed = false), awaitValue(model.state) { it.step is LinkStep.Done }.step)
        assertEquals(listOf("deny"), answered)
    }

    @Test
    fun aCodeForAnotherServerSaysSo() {
        val model = model("kivo://link?code=K7Q2M9&server=s2", servers = listOf(SavedServer("s2", "Cabin", listOf("https://cabin.test"))))
        assertEquals(LinkStep.OtherServer("Cabin"), awaitValue(model.state) { it.step is LinkStep.OtherServer }.step)
    }

    @Test
    fun whatIsntACodeAndACodeNoDeviceWaitsOnSaySo() {
        val model = model()
        model.lookUp("hello")
        assertEquals(LinkMessage.NotACode, model.state.value.message)
        model.lookUp("AAAAAA")
        assertIs<LinkMessage.CodeFailed>(awaitValue(model.state) { it.message == LinkMessage.CodeFailed }.message)
        assertEquals(LinkStep.Enter, model.state.value.step)
    }
}
