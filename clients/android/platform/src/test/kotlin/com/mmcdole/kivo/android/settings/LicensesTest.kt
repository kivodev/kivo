package com.mmcdole.kivo.android.settings

import com.mikepenz.aboutlibraries.entity.Library
import com.mikepenz.aboutlibraries.entity.License
import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals

class LicensesTest {
    private val apache = License("Apache License 2.0", "https://spdx.org/licenses/Apache-2.0.html", null, "Apache-2.0", "Apache License\nVersion 2.0", "Apache-2.0")

    private fun library(id: String, version: String, name: String) = Library(
        uniqueId = id,
        artifactVersion = version,
        name = name,
        description = null,
        website = null,
        developers = emptyList(),
        organization = null,
        scm = null,
        licenses = setOf(apache),
    )

    @Test
    fun librariesComeByNameWithTheirLicensesThenTheTermsThatNameAndroid() {
        val shared = File("../../../shared/credits")
        val libraries = listOf(library("io.ktor:ktor-client-core", "3.5.1", "ktor-client-core"), library("androidx.activity:activity", "1.13.0", "Activity"))
        val credits = credits(libraries, shared.resolve("terms.json").readText()) { shared.resolve(it).readText() }

        // The Cast SDK's terms are the web's and the iPhone's: Android has no Cast SDK.
        assertEquals(listOf("Activity", "ktor-client-core", "Unicode CLDR"), credits.map { it.name })
        assertEquals(Credit("io.ktor:ktor-client-core", "ktor-client-core", "3.5.1", "Apache-2.0", "Apache License\nVersion 2.0"), credits[1])
        val cldr = credits.last()
        assertEquals("Unicode License v3", cldr.license)
        assertEquals(true, cldr.text?.startsWith("UNICODE LICENSE V3"))
    }
}
