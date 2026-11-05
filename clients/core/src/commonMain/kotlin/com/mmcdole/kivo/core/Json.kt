package com.mmcdole.kivo.core

import kotlinx.serialization.json.Json

/** How the core reads and writes the API's JSON: compact, and keys this version doesn't know are ignored. */
internal val kivoJson = Json { ignoreUnknownKeys = true }
