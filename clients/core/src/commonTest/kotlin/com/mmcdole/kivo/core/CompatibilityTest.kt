package com.mmcdole.kivo.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class CompatibilityTest {
    @Test
    fun versionsCompareByMajorMinorAndPatch() {
        assertTrue(compareVersions("0.5.0", "0.4.0")!! > 0)
        assertTrue(compareVersions("0.4.1", "0.4.0")!! > 0)
        assertTrue(compareVersions("1.0.0", "0.10.9")!! > 0)
        assertTrue(compareVersions("0.10.0", "0.9.0")!! > 0)
        assertTrue(compareVersions("0.4.0", "0.5.0")!! < 0)
        assertEquals(0, compareVersions("0.4.0", "0.4.0"))
        assertEquals(0, compareVersions("v0.4.0", "0.4.0"))
    }

    @Test
    fun aBuildPastAReleaseIsThatRelease() {
        assertEquals(0, compareVersions("0.3.0-dev+abc1234", "0.3.0"))
        assertEquals(0, compareVersions("0.3.0+abc1234", "0.3.0"))
        assertEquals(0, compareVersions("v0.3.0-3-gabc1234-dirty", "0.3.0"))
        assertEquals(0, compareVersions("0.3.0+78", "0.3.0"))
        assertTrue(compareVersions("0.3.1-dev+56c0018d", "0.3.0")!! > 0)
        assertTrue(compareVersions("0.3.0-dev+abc1234", "0.4.0")!! < 0)
        assertTrue(compareVersions("0.4.0-dev+abc1234", "0.3.0")!! > 0)
    }

    @Test
    fun aVersionThatIsntAReleaseComparesWithNothing() {
        for (odd in listOf("dev", "d76b7e0", "", " ", "latest", "0.5", "1.0", "0.3.x", "0.3.0.1", "0..3", "-1.0.0", "0.3.0 (12)")) {
            assertNull(compareVersions(odd, "0.3.0"), odd)
            assertNull(compareVersions("0.3.0", odd), odd)
        }
    }

    @Test
    fun aServerOlderThanTheAppWorksWithIsTooOld() {
        assertEquals(
            Compatibility.ServerTooOld(serverVersion = "0.1.0", minServerVersion = MIN_SERVER_VERSION, appVersion = "0.8.0"),
            Compatibility.of(serverVersion = "0.1.0", minAppVersion = "0.1.0", appVersion = "0.8.0"),
        )
    }

    @Test
    fun anAppOlderThanTheServerSupportsIsTooOld() {
        assertEquals(
            Compatibility.AppTooOld(appVersion = "0.5.0", minAppVersion = "0.6.0"),
            Compatibility.of(serverVersion = MIN_SERVER_VERSION, minAppVersion = "0.6.0", appVersion = "0.5.0"),
        )
    }

    @Test
    fun anAppAndAServerAtEachOthersOldestAreCompatible() {
        assertEquals(Compatibility.Compatible, Compatibility.of(MIN_SERVER_VERSION, "0.5.0", "0.5.0"))
        assertEquals(Compatibility.Compatible, Compatibility.of("99.0.0", "0.5.0", "0.7.2"))
    }

    @Test
    fun aBuildPastAReleaseIsNotOlderThanItAndShowsAsIt() {
        assertEquals(Compatibility.Compatible, Compatibility.of("$MIN_SERVER_VERSION-dev+abc1234", "0.5.0", "0.5.0-dev+abc1234"))
        assertEquals(
            Compatibility.ServerTooOld("0.1.0", MIN_SERVER_VERSION, "0.8.0"),
            Compatibility.of("0.1.0-dev+abc1234", "0.1.0", "0.8.0+abc1234"),
        )
    }

    @Test
    fun missingOrOddVersionsNeverBlock() {
        // An older server names no oldest app.
        assertEquals(Compatibility.Compatible, Compatibility.of(MIN_SERVER_VERSION, null, "0.1.0"))
        // A hand build reports a commit, `dev` or nothing.
        for (odd in listOf("dev", "d76b7e0", "")) {
            assertEquals(Compatibility.Compatible, Compatibility.of(odd, "9.0.0", odd), odd)
            assertEquals(Compatibility.Compatible, Compatibility.of(MIN_SERVER_VERSION, odd, "0.1.0"), odd)
        }
        // Only the side that can be told is.
        assertEquals(Compatibility.ServerTooOld("0.1.0", MIN_SERVER_VERSION, "dev"), Compatibility.of("0.1.0", "9.0.0", "dev"))
        assertEquals(Compatibility.AppTooOld("0.1.0", "0.6.0"), Compatibility.of("dev", "0.6.0", "0.1.0"))
    }
}
