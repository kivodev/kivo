package com.mmcdole.kivo.core.display

import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.Json

/**
 * CLDR's names for languages, scripts and regions in one locale, as
 * Intl.DisplayNames gives them, with the pattern and separator that join
 * a language to its script and region.
 */
@Serializable
internal data class LanguageNames(
    val pattern: String,
    val separator: String,
    val languages: Map<String, String>,
    val scripts: Map<String, String>,
    val regions: Map<String, String>,
) {
    /**
     * A language tag in words: a dialect by its own name ("American
     * English"), else the language with its script and region ("Chinese
     * (Traditional, Taiwan)"). An ISO 639-2 code is read as its language
     * ("eng" is English). A tag that isn't one is shown as it is; no tag, und
     * or "null" is [unknown].
     */
    fun name(tag: String?, unknown: String): String {
        if (tag.isNullOrBlank() || tag.trim().equals("null", ignoreCase = true)) return unknown
        var (language, script, region) = split(tag) ?: return tag
        aliases[language]?.let { alias ->
            val (l, s, r) = split(alias) ?: return@let
            language = l
            script = script.ifEmpty { s }
            region = region.ifEmpty { r }
        }
        if (language == "und") return unknown
        val name = languages[language] ?: language
        if (script.isEmpty() || region.isEmpty()) languages[language + "-" + script + region]?.let { return it }
        val pieces = listOfNotNull(
            script.takeIf { it.isNotEmpty() }?.let { scripts[it] ?: it },
            region.takeIf { it.isNotEmpty() }?.let { regions[it] ?: it },
        ).map { it.replace('(', '[').replace(')', ']') }
        if (pieces.isEmpty()) return name
        val piece = pieces.drop(1).fold(pieces.first()) { joined, p -> separator.replace("{0}", joined).replace("{1}", p) }
        return pattern.replace("{0}", name).replace("{1}", piece)
    }

    internal companion object {
        private val json = Json { ignoreUnknownKeys = true }

        private val aliases: Map<String, String> by lazy {
            json.decodeFromString(MapSerializer(String.serializer(), String.serializer()), DisplayJson.LANGUAGE_ALIASES)
        }

        private val tables: Map<String, Lazy<LanguageNames>> = DisplayJson.languages.mapValues { (_, text) ->
            lazy { json.decodeFromString(serializer(), text) }
        }

        fun forLocale(locale: String): LanguageNames = (tables[locale] ?: tables.getValue("en")).value

        /** A tag's language, script and region in their canonical case; null for anything else, such as a variant. */
        private fun split(tag: String): Triple<String, String, String>? {
            val parts = tag.split('-', '_')
            val language = parts[0].lowercase()
            if (language.length !in 2..3 || !language.all { it in 'a'..'z' }) return null
            var script = ""
            var region = ""
            for (p in parts.drop(1)) {
                when {
                    script.isEmpty() && region.isEmpty() && p.length == 4 && p.all { it.isLetter() } ->
                        script = p.lowercase().replaceFirstChar { it.uppercaseChar() }
                    region.isEmpty() && p.length == 2 && p.all { it.isLetter() } -> region = p.uppercase()
                    region.isEmpty() && p.length == 3 && p.all { it.isDigit() } -> region = p
                    else -> return null
                }
            }
            return Triple(language, script, region)
        }
    }
}
