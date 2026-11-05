package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.SearchResults
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection

/** Search across the libraries the user sees: the works, people and collections that match, each best first. */
internal class SearchRepository(private val connection: ServerConnection) {
    suspend fun search(query: String, limit: Int? = null): SearchResults =
        connection.works.searchWorks(query, limit).result()
}
