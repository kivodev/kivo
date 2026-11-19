package com.mmcdole.kivo.tokens

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

// shared/tokens.json as BrighterScript constants, for the Roku app. The app
// draws the TV interface in the dark theme at 1920x1080, where the TV
// tokens' pixels are the screen's: the dark colours as SceneGraph colours
// (0xRRGGBBAA), the TV type scale and the TV layout's lengths and counts,
// focus scales and ring, and the aspects. Constants are inlined where
// they're used. Light colours, CSS-only values (calc, env, grid templates),
// fonts, durations and easing are left out: the app uses Roku's system
// font and SceneGraph's own animations.

/** The BrighterScript source of the Roku tokens. */
fun rokuTokens(tokensJson: String): String {
    val tokens = Json.parseToJsonElement(tokensJson).jsonObject
    val tv = tokens.obj("tv")
    val web = tv.obj("web")
    return buildString {
        appendLine("' Generated from shared/tokens.json by :tokens (./gradlew :tokens:rokuTokens). Don't edit.")
        appendLine()
        namespace("KivoTokens.Color", "The dark theme's colours.") {
            tokens.obj("themes").obj("dark").obj("color").forEach { (key, value) ->
                constant(name(key), "\"${rokuColor(value.jsonPrimitive.content)}\"")
            }
        }
        namespace("KivoTokens.TextSize", "The TV type scale's sizes, in pixels.") {
            tv.obj("text").forEach { (key, value) -> constant(name(key), pixels(value.jsonObject.string("size"))) }
        }
        namespace("KivoTokens.LineHeight", "The TV type scale's line heights, in pixels.") {
            tv.obj("text").forEach { (key, value) -> constant(name(key), pixels(value.jsonObject.string("lineHeight"))) }
        }
        namespace("KivoTokens.Layout", "The TV layout's lengths in pixels, and its cards across a row.") {
            web.obj("layout").forEach { (key, value) ->
                val v = value.jsonPrimitive.content
                when {
                    pixelLength.matches(v) -> constant(name(key), pixels(v))
                    count.matches(v) -> constant(name(key), v)
                }
            }
        }
        namespace("KivoTokens.Focus", "How a focused item shows.") {
            constant("ringWidth", pixels(tv.string("focusRingWidth")))
            constant("scale", tv.string("focusScale"))
            constant("cardScale", tv.string("cardFocusScale"))
        }
        namespace("KivoTokens.Aspect", "Width over height.") {
            tokens.obj("aspect").forEach { (key, value) ->
                val (w, h) = value.jsonPrimitive.content.split("/").map { it.trim().toDouble() }
                constant(name(key), String.format(java.util.Locale.ROOT, "%.4f", w / h))
            }
        }
        // The last namespace's trailing blank line isn't wanted.
        setLength(length - 1)
    }
}

private val pixelLength = Regex("""^[0-9.]+px$""")
private val count = Regex("""^[0-9]+$""")

private class Block(val out: StringBuilder) {
    fun constant(name: String, value: String) {
        out.appendLine("    const $name = $value")
    }
}

private fun StringBuilder.namespace(name: String, doc: String, body: Block.() -> Unit) {
    appendLine("' $doc")
    appendLine("namespace $name")
    Block(this).body()
    appendLine("end namespace")
    appendLine()
}

private fun pixels(length: String): String = length.removeSuffix("px").let { v ->
    val d = v.toDouble()
    if (d == Math.floor(d)) d.toLong().toString() else v
}

/** `#rrggbb` or `#rrggbbaa` as a SceneGraph colour, `0xRRGGBBAA`. */
internal fun rokuColor(hex: String): String {
    val digits = hex.removePrefix("#").uppercase()
    require(digits.length == 6 || digits.length == 8) { "not a colour: $hex" }
    return "0x" + if (digits.length == 6) digits + "FF" else digits
}

private fun JsonObject.obj(key: String): JsonObject = getValue(key).jsonObject

private fun JsonObject.string(key: String): String = (getValue(key) as JsonPrimitive).content
