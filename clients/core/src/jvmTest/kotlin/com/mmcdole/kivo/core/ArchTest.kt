package com.mmcdole.kivo.core

import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The core's package rules, read from its sources: which core packages each
 * package imports, which packages stay off the network, and that no UI
 * toolkit is imported at all. A package is the one under
 * com.mmcdole.kivo.core, whatever source set holds the file ("" is the root
 * package), and its subpackages count as itself.
 */
class ArchTest {
    /**
     * The core packages each package imports. The list is exact: a listed
     * package that no file imports fails too, so when an import goes, so
     * does its entry.
     */
    private val mayImport = mapOf(
        "" to setOf(),
        "player" to setOf(""),
        "caps" to setOf(),
        "auth" to setOf(""),
        "live" to setOf("", "auth"),
        "discovery" to setOf("auth"),
        "settings" to setOf("auth", "caps", "live"),
        "display" to setOf("", "settings"),
        "play" to setOf("", "auth", "display", "player"),
        "cast" to setOf("", "auth", "player"),
        "data" to setOf("", "auth", "display", "live"),
        "downloads" to setOf("", "auth", "display", "live", "play", "player", "settings"),
        "books" to setOf("auth", "downloads", "play", "player", "settings"),
        "session" to setOf(
            "", "auth", "books", "caps", "cast", "data", "discovery", "display", "downloads", "live", "play", "player",
            "settings",
        ),
        "apple" to setOf("auth", "caps", "downloads", "session", "settings"),
    )

    /**
     * Packages that hold rules and words over values the caller has, so they
     * reach no server: no HTTP client, generated API or server connection.
     */
    private val offline = setOf("player", "caps", "display")

    /** Imports that mean reaching the server. */
    private val network = listOf(
        "io.ktor.",
        "com.mmcdole.kivo.api.apis.",
        "com.mmcdole.kivo.api.infrastructure.",
        "com.mmcdole.kivo.core.auth.ServerConnection",
        "com.mmcdole.kivo.core.live.",
    )

    /**
     * Network imports an offline package still has, each as "file: import".
     * The list is exact, so it only shrinks.
     */
    private val offlineExceptions = setOf<String>()

    /**
     * The wiring a session puts together, which the apps reach only through
     * [com.mmcdole.kivo.core.session.KivoSession] and
     * [com.mmcdole.kivo.core.session.KivoClient], so no app can build its own
     * copy of the session: these, and every repository, are internal.
     */
    private val wiring = setOf(
        "ServerConnection", "SignIn", "LiveEvents", "Discovery", "ServerList", "ClientId", "DeviceSettingsStore",
        "Bandwidth", "CapabilityReport", "Reachability", "UserSettings", "LinkDevice", "SubtitleSearcher", "ServerPlays",
        "CastSender", "Downloads",
    )

    /** UI toolkits: the apps draw, the core doesn't. */
    private val ui = listOf(
        "androidx.compose.", "android.view.", "android.widget.", "androidx.activity.", "androidx.lifecycle.",
        "platform.UIKit.", "platform.AppKit.", "platform.SwiftUI.", "platform.AVKit.",
    )

    private class Source(
        val set: String,
        val pkg: String,
        val fullPackage: String,
        val path: String,
        val imports: List<String>,
        val text: String,
    )

    private val sources: List<Source> = File("src").listFiles()!!
        .filter { it.isDirectory && it.name.endsWith("Main") }
        .flatMap { set -> set.resolve("kotlin").walk().filter { it.extension == "kt" }.map { set.name to it }.toList() }
        .map { (set, file) ->
            val lines = file.readLines()
            val pkg = lines.first { it.startsWith("package ") }.removePrefix("package ").trim()
            Source(
                set,
                corePackage(pkg) ?: error("$file is outside com.mmcdole.kivo.core"),
                pkg,
                file.path.substringAfter("com/mmcdole/kivo/core/"),
                lines.filter { it.startsWith("import ") }.map { it.removePrefix("import ").substringBefore(" as ").trim() },
                lines.joinToString("\n"),
            )
        }

    /** A top-level declaration: its visibility modifier, if any, its kind, its name and, for a class, its supertypes. */
    private class Declaration(val visibility: String?, val kind: String, val name: String, val supertypes: List<String>)

    private val declaration = Regex(
        """^(?:(public|internal|private) )?(?:(?:data|sealed|abstract|open|enum|fun|value) )*(class|object|interface) (\w+)""",
        RegexOption.MULTILINE,
    )

    private fun declarations(source: Source): List<Declaration> = declaration.findAll(source.text).map { match ->
        val (visibility, kind, name) = match.destructured
        // The header up to the body, with the constructor's parameters taken out, names the supertypes after its colon.
        val header = source.text.substring(match.range.last + 1).substringBefore('{').substringBefore("\n\n")
        val outside = buildString {
            var depth = 0
            for (c in header) {
                if (c == '(') depth++
                if (depth == 0) append(c)
                if (c == ')') depth--
            }
        }
        val supertypes = outside.substringAfter(':', "").split(',').map { it.trim().substringBefore('<').trim() }.filter { it.isNotEmpty() }
        Declaration(visibility.ifEmpty { null }, kind, name, supertypes)
    }.toList()

    /** The core's packages in full, longest first. */
    private val fullPackages = sources.map { it.fullPackage }.distinct().sortedByDescending { it.length }

    /** The core package a package name is in, or null when it's outside the core. */
    private fun corePackage(name: String): String? = when {
        name == "com.mmcdole.kivo.core" -> ""
        name.startsWith("com.mmcdole.kivo.core.") -> name.removePrefix("com.mmcdole.kivo.core.").substringBefore('.')
        else -> null
    }

    /** The core package an import is from, or null for anything else. */
    private fun importedPackage(import: String): String? =
        fullPackages.firstOrNull { import.startsWith("$it.") }?.let(::corePackage)

    @Test
    fun sourcesAreRead() {
        assertTrue(sources.size > 50, "read only ${sources.size} sources from ${File("src").absolutePath}")
    }

    @Test
    fun packagesImportOnlyTheirListedPackages() {
        val violations = mutableListOf<String>()
        val used = mutableMapOf<String, MutableSet<String>>()
        for (s in sources) {
            val allowed = mayImport[s.pkg]
            if (allowed == null) {
                violations += "${s.path}: package ${s.pkg} isn't in mayImport"
                continue
            }
            for (import in s.imports) {
                val p = importedPackage(import) ?: continue
                if (p == s.pkg) continue
                used.getOrPut(s.pkg) { mutableSetOf() } += p
                if (p !in allowed) violations += "${s.path}: ${s.pkg} may not import $p ($import)"
            }
        }
        for ((pkg, allowed) in mayImport) {
            for (p in allowed - used[pkg].orEmpty()) violations += "mayImport: $pkg lists $p, which none of its files imports"
        }
        assertEquals(emptyList(), violations)
    }

    @Test
    fun offlinePackagesDontReachTheServer() {
        val found = sources.filter { it.pkg in offline }.flatMap { s ->
            s.imports.filter { i -> network.any { i.startsWith(it) } }.map { "${s.path}: $it" }
        }.toSet()
        assertEquals(offlineExceptions, found)
    }

    /**
     * The platforms' source sets hold adapters only: each public class there
     * implements an interface of commonMain, which is where the policy
     * lives, so it's the same on every platform.
     */
    @Test
    fun platformClassesAreAdapters() {
        val common = sources.filter { it.set == "commonMain" }.flatMap(::declarations).filter { it.kind == "interface" }.map { it.name }.toSet()
        val found = sources.filter { it.set != "commonMain" }.flatMap { s ->
            declarations(s).filter { it.visibility == null || it.visibility == "public" }
                .filter { d -> d.supertypes.none { it in common } }
                .map { "${s.set}/${s.path}: ${it.name}" }
        }
        assertEquals(emptyList(), found)
    }

    /** The session's wiring, and every repository, stays internal, so an app takes the session rather than building one. */
    @Test
    fun wiringIsInternal() {
        val declared = sources.flatMap(::declarations).filter { it.kind == "class" }
        val wired = declared.filter { it.name in wiring || it.name.endsWith("Repository") }
        assertEquals(wiring, wired.map { it.name }.filter { it in wiring }.toSet(), "a wiring class is missing")
        assertEquals(emptyList(), wired.filter { it.visibility != "internal" }.map { it.name })
    }

    @Test
    fun noUiToolkit() {
        val found = sources.flatMap { s -> s.imports.filter { i -> ui.any { i.startsWith(it) } }.map { "${s.path}: $it" } }
        assertEquals(emptyList(), found)
    }
}
