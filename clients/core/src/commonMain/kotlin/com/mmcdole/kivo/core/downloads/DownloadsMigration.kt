package com.mmcdole.kivo.core.downloads

import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.kivoJson
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
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

// Migration (track-names): remove after every client has updated past this release.
/**
 * A download's work as stored before the server named versions and tracks,
 * with plain names in their place: a track goes by its language, a video
 * by its codec, and a version by its edition or resolution. A work whose
 * versions have names, or that isn't an object, is returned as it is.
 */
internal fun withTrackNames(work: String): String {
    val json = kivoJson.parseToJsonElement(work) as? JsonObject ?: return work
    val versions = json["versions"] as? JsonArray ?: return work
    if (versions.all { it !is JsonObject || "name" in it }) return work
    val display = Display()
    fun JsonObject.text(key: String) = (this[key] as? JsonPrimitive)?.contentOrNull
    fun track(t: JsonObject): JsonObject {
        if ("name" in t) return t
        val name = if (t.text("type") == "video") display.codec(t.text("codec").orEmpty()) else display.language(t.text("language"))
        return JsonObject(t + mapOf("name" to JsonPrimitive(name), "fullName" to JsonPrimitive(name)))
    }
    fun version(v: JsonObject): JsonObject {
        val name = v.text("edition") ?: v.text("resolution")?.let { Resolution.of(it).takeIf { r -> r != Resolution.Unknown }?.let(display::resolution) ?: it }.orEmpty()
        val tracks = (v["tracks"] as? JsonArray)?.map { (it as? JsonObject)?.let(::track) ?: it }
        val added = buildMap<String, JsonElement> {
            if ("name" !in v) put("name", JsonPrimitive(name))
            tracks?.let { put("tracks", JsonArray(it)) }
        }
        return JsonObject(v + added)
    }
    return JsonObject(json + ("versions" to JsonArray(versions.map { (it as? JsonObject)?.let(::version) ?: it }))).toString()
}
