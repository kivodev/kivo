package com.mmcdole.kivo.tokens

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.util.Locale

// shared/tokens.json as SwiftUI values, for KivoUI. CSS lengths become points
// at one CSS pixel each (a rem is 16), as iOS sizes a phone's layout; the TV
// tokens are pixels on a 1080p canvas, which tvOS lays out at 1920 by 1080
// points, so one to a point. Colours are sRGB components with their alpha;
// CSS shadows are SwiftUI shadow layers, whose radius is half the CSS blur.
// Fonts, breakpoints, containers' grid templates and easing are left out: the
// apps use the system font.

/** The Swift source of the SwiftUI tokens. */
fun swiftTokens(tokensJson: String): String {
    val tokens = Json.parseToJsonElement(tokensJson).jsonObject
    val themes = tokens.obj("themes")
    val colorKeys = themes.obj("dark").obj("color").keys
    val shadowKeys = themes.obj("dark").obj("shadow").keys
    for ((theme, values) in themes) {
        val missing = colorKeys - values.jsonObject.obj("color").keys
        require(missing.isEmpty()) { "$theme theme lacks colour tokens: ${missing.joinToString()}" }
    }

    return buildString {
        appendLine("// Generated from shared/tokens.json by :tokens. Don't edit.")
        appendLine()
        appendLine("import SwiftUI")
        appendLine()
        appendLine("/// One layer of a shadow.")
        appendLine("public struct KivoShadow: Sendable {")
        appendLine("    public let color: Color")
        appendLine("    public let radius: CGFloat")
        appendLine("    public let x: CGFloat")
        appendLine("    public let y: CGFloat")
        appendLine("}")
        appendLine()
        appendLine("/// A step of a type scale, in points.")
        appendLine("public struct KivoTextStyle: Sendable {")
        appendLine("    public let size: CGFloat")
        appendLine("    public let lineHeight: CGFloat")
        appendLine("}")
        appendLine()
        appendLine("/// A theme's colours and shadows, one per token.")
        appendLine("public struct KivoPalette: Sendable {")
        appendLine("    public let scheme: ColorScheme")
        for (key in colorKeys) appendLine("    public let ${name(key)}: Color")
        for (key in shadowKeys) appendLine("    public let ${name(key)}Shadow: [KivoShadow]")
        appendLine("}")
        appendLine()
        appendLine("public extension KivoPalette {")
        for ((theme, value) in themes) {
            val set = value.jsonObject
            val colors = set.obj("color")
            val shadows = set.obj("shadow")
            appendLine("    static let ${name(theme)} = KivoPalette(")
            val args = listOf("scheme: .${set.string("scheme")}") +
                colorKeys.map { "${name(it)}: ${swiftColor(colors.string(it))}" } +
                shadowKeys.map { "${name(it)}Shadow: ${swiftShadow(shadows.string(it))}" }
            appendLine(args.joinToString(",\n") { "        $it" })
            appendLine("    )")
        }
        appendLine("}")
        appendLine()
        appendLine("public enum KivoTokens {")
        appendLine("    /// The spacing unit: every gap and padding is a multiple of it.")
        appendLine("    public static let spacing: CGFloat = ${points(tokens.string("spacing"))}")
        appendLine()
        textEnum("TextSize", tokens.obj("text"))
        appendLine()
        appendLine("    public enum Weight {")
        for ((key, value) in tokens.obj("fontWeight")) {
            appendLine("        public static let ${name(key)}: Font.Weight = .${fontWeight(value.jsonPrimitive.content)}")
        }
        appendLine("    }")
        appendLine()
        lengthEnum("Radius", tokens.obj("radius"))
        appendLine()
        appendLine("    /// Width over height.")
        appendLine("    public enum Aspect {")
        for ((key, value) in tokens.obj("aspect")) {
            val (w, h) = value.jsonPrimitive.content.split("/").map { it.trim() }
            appendLine("        public static let ${name(key)}: CGFloat = $w.0 / $h.0")
        }
        appendLine("    }")
        appendLine()
        lengthEnum("Layout", tokens.obj("layout"))
        appendLine()
        lengthEnum("Container", tokens.obj("container"))
        appendLine()
        appendLine("    /// In seconds.")
        appendLine("    public enum Duration {")
        for ((key, value) in tokens.obj("duration")) {
            appendLine("        public static let ${name(key)}: Double = ${value.jsonPrimitive.content.removeSuffix("ms").toInt() / 1000.0}")
        }
        appendLine("    }")
        appendLine()
        val tv = tokens.obj("tv")
        appendLine("    /// The TV interface's type scale and focus, in points of a 1080p screen.")
        appendLine("    public enum Tv {")
        appendLine("        public static let focusRingWidth: CGFloat = ${points(tv.string("focusRingWidth"))}")
        appendLine("        public static let focusScale: CGFloat = ${tv.string("focusScale")}")
        appendLine("        public static let cardFocusScale: CGFloat = ${tv.string("cardFocusScale")}")
        appendLine()
        textEnum("TextSize", tv.obj("text"), indent = "        ")
        appendLine("    }")
        appendLine("}")
    }
}

private fun StringBuilder.textEnum(enumName: String, steps: JsonObject, indent: String = "    ") {
    appendLine("${indent}public enum $enumName {")
    for ((key, value) in steps) {
        val step = value.jsonObject
        appendLine("$indent    public static let ${name(key)} = KivoTextStyle(size: ${points(step.string("size"))}, lineHeight: ${points(step.string("lineHeight"))})")
    }
    appendLine("$indent}")
}

/** Lengths only: a value that isn't one (a grid template) has no SwiftUI meaning. */
private fun StringBuilder.lengthEnum(enumName: String, values: JsonObject) {
    appendLine("    public enum $enumName {")
    for ((key, value) in values) {
        val length = value.jsonPrimitive.content
        if (swiftLength.matches(length)) appendLine("        public static let ${name(key)}: CGFloat = ${points(length)}")
    }
    appendLine("    }")
}

private val swiftLength = Regex("""^-?[0-9.]+(rem|px)$""")

/** CSS pixels as points: a rem is 16, a pixel one. */
private fun points(length: String): String {
    val value = when {
        length.endsWith("rem") -> length.removeSuffix("rem").toDouble() * 16
        length.endsWith("px") -> length.removeSuffix("px").toDouble()
        else -> length.toDouble()
    }
    return if (value == Math.floor(value)) value.toLong().toString() else value.toString()
}

private fun fontWeight(weight: String) = when (weight) {
    "400" -> "regular"
    "500" -> "medium"
    "600" -> "semibold"
    "700" -> "bold"
    else -> error("no Font.Weight for $weight")
}

/** `#rrggbb` or `#rrggbbaa` as an sRGB SwiftUI colour. */
internal fun swiftColor(hex: String): String {
    val digits = hex.removePrefix("#").lowercase()
    require(digits.length == 6 || digits.length == 8) { "not a colour: $hex" }
    fun component(i: Int) = String.format(Locale.ROOT, "%.4f", digits.substring(i * 2, i * 2 + 2).toInt(16) / 255.0)
    val opacity = if (digits.length == 8) component(3) else "1"
    return "Color(.sRGB, red: ${component(0)}, green: ${component(1)}, blue: ${component(2)}, opacity: $opacity)"
}

/** A CSS box-shadow list, `x y blur colour` per layer. */
internal fun swiftShadow(css: String): String = css.split(",").joinToString(", ", "[", "]") { layer ->
    val parts = layer.trim().split(Regex("\\s+"))
    val (x, y, blur) = parts.take(3).map { points(it).toDouble() }
    val radius = blur / 2
    fun n(v: Double) = if (v == Math.floor(v)) v.toLong().toString() else v.toString()
    "KivoShadow(color: ${swiftColor(parts[3])}, radius: ${n(radius)}, x: ${n(x)}, y: ${n(y)})"
}

private fun JsonObject.obj(key: String): JsonObject = getValue(key).jsonObject

private fun JsonObject.string(key: String): String = getValue(key).jsonPrimitive.content
