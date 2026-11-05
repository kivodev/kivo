package com.mmcdole.kivo.tokens

import java.io.File

/**
 * `tokens <tokens.json> <output.kt> <package>` writes the Compose tokens;
 * `tokens swift <tokens.json> <output.swift>` the SwiftUI ones.
 */
fun main(args: Array<String>) {
    if (args.firstOrNull() == "swift") {
        require(args.size == 3) { "usage: tokens swift <tokens.json> <output.swift>" }
        File(args[2]).apply { parentFile.mkdirs() }.writeText(swiftTokens(File(args[1]).readText()))
        return
    }
    require(args.size == 3) { "usage: tokens <tokens.json> <output.kt> <package>" }
    val (input, output, packageName) = args
    File(output).apply { parentFile.mkdirs() }.writeText(composeTokens(File(input).readText(), packageName))
}
