package com.mmcdole.kivo.android.signin

import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.discovery.FoundServer
import kotlin.test.Test
import kotlin.test.assertEquals

class SignInStepTest {
    private val den = FoundServer("http://den.test:8484", ServerInfo("s1", "Den", "0.5.0", minAppVersion = "0.6.0"))

    @Test
    fun aServerTheAppWorksWithGoesToItsSignInCode() {
        assertEquals(Step.Code(den), stepFor(den, Compatibility.Compatible))
    }

    @Test
    fun aServerTooOldForTheAppSaysSoInPlaceOfItsCode() {
        val problem = Compatibility.ServerTooOld(serverVersion = "0.5.0", minServerVersion = "0.6.0", appVersion = "0.8.0")
        assertEquals(Step.Incompatible(den, problem), stepFor(den, problem))
    }

    @Test
    fun anAppTooOldForTheServerSaysSoInPlaceOfItsCode() {
        val problem = Compatibility.AppTooOld(appVersion = "0.5.0", minAppVersion = "0.6.0")
        assertEquals(Step.Incompatible(den, problem), stepFor(den, problem))
    }
}
