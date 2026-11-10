package com.mmcdole.kivo.android

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull

class AppBuildTest {
    @Test
    fun theVersionRequestsNameIsTheReleaseWithTheBuild() {
        val build = appBuild()
        val version = assertNotNull(Regex("""(\d+\.\d+\.\d+)\+(\d+)""").matchEntire(build.version), build.version)
        val (release, number) = version.destructured
        assertEquals("$release ($number)", build.versionName)
    }
}
