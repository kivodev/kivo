package com.mmcdole.kivo.tokens

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

// shared/tokens.json as Compose values.
// CSS lengths become dp at one CSS pixel each (a rem is 16), as Android
// sizes a phone's layout; the TV tokens are pixels on a 1080p canvas, which
// Android TV lays out at 960 by 540 dp, so two to a dp. Colours keep their
// alpha. Tokens only CSS can use (fonts, shadows, breakpoints, containers,
// grid templates) are left out: the apps use the platform's sans-serif, as
// the web's font stack does on Android.

/** The Kotlin source of the Compose tokens, in [packageName]. */
fun composeTokens(tokensJson: String, packageName: String): String {
    val tokens = Json.parseToJsonElement(tokensJson).jsonObject
    val themes = tokens.obj("themes")
    val dark = themes.obj("dark").obj("color")
    val light = themes.obj("light").obj("color")
    val missing = dark.keys - light.keys
    require(missing.isEmpty()) { "light theme lacks colour tokens: ${missing.joinToString()}" }

    return buildString {
        appendLine("// Generated from shared/tokens.json by :tokens. Don't edit.")
        appendLine("package $packageName")
        appendLine()
        appendLine("import androidx.compose.animation.core.CubicBezierEasing")
        appendLine("import androidx.compose.ui.graphics.Color")
        appendLine("import androidx.compose.ui.text.font.FontWeight")
        appendLine("import androidx.compose.ui.unit.TextUnit")
        appendLine("import androidx.compose.ui.unit.dp")
        appendLine("import androidx.compose.ui.unit.sp")
        appendLine()
        appendLine("/** A step of a type scale. */")
        appendLine("data class TextStep(val size: TextUnit, val lineHeight: TextUnit)")
        appendLine()
        appendLine("/** A theme's colours, one per colour token. */")
        appendLine("data class KivoColors(")
        for (key in dark.keys) appendLine("    val ${name(key)}: Color,")
        appendLine(")")
        appendLine()
        appendLine("object KivoTokens {")
        appendLine("    val dark = ${colors(dark)}")
        appendLine()
        appendLine("    val light = ${colors(light)}")
        appendLine()
        appendLine("    /** The spacing unit: every gap and padding is a multiple of it. */")
        appendLine("    val spacing = ${dp(tokens.string("spacing"))}")
        appendLine()
        textObject("Text", tokens.obj("text"), ::sp)
        appendLine()
        appendLine("    object FontWeights {")
        for ((key, value) in tokens.obj("fontWeight")) appendLine("        val ${name(key)} = FontWeight(${value.jsonPrimitive.content})")
        appendLine("    }")
        appendLine()
        lengthObject("Radius", tokens.obj("radius"))
        appendLine()
        appendLine("    object Aspect {")
        for ((key, value) in tokens.obj("aspect")) {
            val (w, h) = value.jsonPrimitive.content.split("/").map { it.trim().toFloat() }
            appendLine("        const val ${name(key)} = ${w / h}f")
        }
        appendLine("    }")
        appendLine()
        lengthObject("Layout", tokens.obj("layout"))
        appendLine()
        appendLine("    object Duration {")
        for ((key, value) in tokens.obj("duration")) {
            appendLine("        const val ${name(key)}Ms = ${value.jsonPrimitive.content.removeSuffix("ms").toInt()}")
        }
        appendLine("    }")
        appendLine()
        appendLine("    object Ease {")
        for ((key, value) in tokens.obj("ease")) {
            val points = value.jsonPrimitive.content.removePrefix("cubic-bezier(").removeSuffix(")").split(",").map { "${it.trim().toFloat()}f" }
            appendLine("        val ${name(key)} = CubicBezierEasing(${points.joinToString()})")
        }
        appendLine("    }")
        appendLine()
        val tv = tokens.obj("tv")
        appendLine("    /** The TV interface's type scale and focus. */")
        appendLine("    object Tv {")
        appendLine("        val focusRingWidth = ${canvasDp(tv.string("focusRingWidth"))}")
        appendLine("        const val focusScale = ${tv.string("focusScale").toFloat()}f")
        appendLine("        const val cardFocusScale = ${tv.string("cardFocusScale").toFloat()}f")
        appendLine()
        textObject("Text", tv.obj("text"), ::canvasSp, indent = "        ")
        appendLine("    }")
        appendLine("}")
    }
}

private fun StringBuilder.textObject(objectName: String, steps: JsonObject, unit: (String) -> String, indent: String = "    ") {
    appendLine("${indent}object $objectName {")
    for ((key, value) in steps) {
        val step = value.jsonObject
        appendLine("$indent    val ${name(key)} = TextStep(${unit(step.string("size"))}, ${unit(step.string("lineHeight"))})")
    }
    appendLine("$indent}")
}

/** Lengths only: a value that isn't one (a grid template) has no Compose meaning. */
private fun StringBuilder.lengthObject(objectName: String, values: JsonObject) {
    appendLine("    object $objectName {")
    for ((key, value) in values) {
        val length = value.jsonPrimitive.content
        if (lengthPattern.matches(length)) appendLine("        val ${name(key)} = ${dp(length)}")
    }
    appendLine("    }")
}

private fun colors(set: JsonObject): String = buildString {
    appendLine("KivoColors(")
    for ((key, value) in set) appendLine("        ${name(key)} = ${color(value.jsonPrimitive.content)},")
    append("    )")
}

private val lengthPattern = Regex("""^-?[0-9.]+(rem|px)$""")

/** CSS pixels: a rem is 16, a pixel one. */
private fun cssPixels(length: String): Double = when {
    length.endsWith("rem") -> length.removeSuffix("rem").toDouble() * 16
    length.endsWith("px") -> length.removeSuffix("px").toDouble()
    else -> error("not a length: $length")
}

private fun dp(length: String) = "${number(cssPixels(length))}.dp"

private fun sp(length: String) = "${number(cssPixels(length))}.sp"

/** A 1080p canvas is 540 dp high on Android TV: two pixels to a dp. */
private fun canvasDp(length: String) = "${number(cssPixels(length) / 2)}.dp"

private fun canvasSp(length: String) = "${number(cssPixels(length) / 2)}.sp"

private fun number(value: Double): String =
    if (value == Math.floor(value)) value.toLong().toString() else "${value.toFloat()}f"

/** `#rrggbb` or `#rrggbbaa` as a Compose colour, ARGB. */
internal fun color(hex: String): String {
    val digits = hex.removePrefix("#").lowercase()
    require(digits.length == 6 || digits.length == 8) { "not a colour: $hex" }
    val alpha = if (digits.length == 8) digits.substring(6) else "ff"
    return "Color(0x${(alpha + digits.substring(0, 6)).uppercase()})"
}

/** A token's key as a Kotlin name: `surface-raised` is `surfaceRaised`, `2xl` is `xl2`. */
internal fun name(key: String): String {
    val camel = key.split("-").mapIndexed { i, part -> if (i == 0) part else part.replaceFirstChar { it.uppercase() } }.joinToString("")
    val digits = camel.takeWhile { it.isDigit() }
    return camel.removePrefix(digits) + digits
}

private fun JsonObject.obj(key: String): JsonObject = getValue(key).jsonObject

private fun JsonObject.string(key: String): String = getValue(key).jsonPrimitive.content
