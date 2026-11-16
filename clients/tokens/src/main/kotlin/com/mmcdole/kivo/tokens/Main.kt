package com.mmcdole.kivo.tokens

import java.io.File

/**
 * `tokens <tokens.json> <output.kt> <package>` writes the Compose tokens;
 * `tokens swift <tokens.json> <output.swift>` the SwiftUI ones;
 * `tokens roku <tokens.json> <output.bs>` the Roku app's.
 */
fun main(args: Array<String>) {
    when (args.firstOrNull()) {
        "swift" -> write(args, "swift", ::swiftTokens)
        "roku" -> write(args, "roku", ::rokuTokens)
        else -> {
            require(args.size == 3) { "usage: tokens <tokens.json> <output.kt> <package>" }
            val (input, output, packageName) = args
            File(output).apply { parentFile.mkdirs() }.writeText(composeTokens(File(input).readText(), packageName))
        }
    }
}

private fun write(args: Array<String>, target: String, tokens: (String) -> String) {
    require(args.size == 3) { "usage: tokens $target <tokens.json> <output>" }
    File(args[2]).apply { parentFile.mkdirs() }.writeText(tokens(File(args[1]).readText()))
}
