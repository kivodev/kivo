package com.mmcdole.kivo.core.data

import platform.Foundation.NSNumber
import platform.Foundation.NSUserDefaults

// Migration (grid-choice): remove after every client has updated past this release.
/**
 * The user defaults, where an earlier app kept each library's grid as a
 * dictionary under `grid.<library>` and whether it had collections under
 * `grid.<library>.collections`.
 */
internal actual val platformGridDefaults: EarlierGridDefaults = EarlierGridDefaults(
    read = {
        val defaults = NSUserDefaults.standardUserDefaults
        gridKeys(defaults).map { it.removePrefix(PREFIX).removeSuffix(COLLECTIONS) }.distinct().associateWith { libraryId ->
            val values = defaults.dictionaryForKey(PREFIX + libraryId).orEmpty().mapNotNull { (name, value) ->
                if (name is String && value is String) name to value else null
            }
            EarlierGridDefault(values.toMap(), (defaults.objectForKey(PREFIX + libraryId + COLLECTIONS) as? NSNumber)?.boolValue)
        }
    },
    forget = {
        val defaults = NSUserDefaults.standardUserDefaults
        gridKeys(defaults).forEach(defaults::removeObjectForKey)
    },
)

private const val PREFIX = "grid."
private const val COLLECTIONS = ".collections"

private fun gridKeys(defaults: NSUserDefaults): List<String> =
    defaults.dictionaryRepresentation().keys.filterIsInstance<String>().filter { it.startsWith(PREFIX) }
