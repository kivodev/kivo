package com.mmcdole.kivo.core.downloads

import com.mmcdole.kivo.core.kivoJson
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive

// Migration (downloads): remove after every client has updated past this release.
/**
 * A download's plan as stored before the server said whether its size is
 * exact, with `sizeExact` as the server gives it now: exact once ready, or
 * for an original that goes as it is. A plan that has it, or that isn't an
 * object, is returned as it is.
 */
internal fun withSizeExact(plan: String): String {
    val json = kivoJson.parseToJsonElement(plan) as? JsonObject ?: return plan
    if ("sizeExact" in json) return plan
    val exact = json["state"]?.jsonPrimitive?.content == "ready" || json["delivery"]?.jsonPrimitive?.content == "file"
    return JsonObject(json + ("sizeExact" to JsonPrimitive(exact))).toString()
}
