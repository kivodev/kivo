package com.mmcdole.kivo.core

import io.ktor.http.URLBuilder
import io.ktor.http.takeFrom

/**
 * [relative] resolved against [base]: an absolute URL as it is, a path from
 * the root on its host, or another beside it, with its own query and none of
 * [base]'s.
 */
internal fun resolveUrl(base: String, relative: String): String =
    URLBuilder(base).apply {
        parameters.clear()
        fragment = ""
    }.takeFrom(relative).buildString()
