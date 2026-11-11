package com.mmcdole.kivo.android.mobile.library

import com.mmcdole.kivo.android.library.decadesOf
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.PersonRef
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.core.data.DynamicRange
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.LibraryFilters
import com.mmcdole.kivo.core.data.WorkPlayState

/**
 * What the filter sheet offers, of the values the library's works have, in
 * the order to show them; a group with none doesn't show. A books library
 * offers genres, decades and narrators; the others genres, decades, content
 * ratings and quality.
 */
data class FilterOffer(
    val genres: List<String> = emptyList(),
    /** By their first year, newest first. */
    val decades: List<Int> = emptyList(),
    val contentRatings: List<String> = emptyList(),
    val resolutions: List<Resolution> = emptyList(),
    val dynamicRanges: List<DynamicRange> = emptyList(),
    val narrators: List<PersonRef> = emptyList(),
)

/** What a library of [kind] offers to filter by, of its [filters] once read. */
fun filterOffer(kind: LibraryKind?, filters: LibraryFilters?): FilterOffer {
    if (filters == null) return FilterOffer()
    val books = kind == LibraryKind.Audiobooks
    return FilterOffer(
        genres = filters.genres,
        decades = decadesOf(filters.years),
        contentRatings = if (books) emptyList() else filters.contentRatings,
        resolutions = if (books) emptyList() else filters.resolutions,
        dynamicRanges = if (books) emptyList() else filters.dynamicRanges,
        narrators = if (books) filters.narrators else emptyList(),
    )
}

/** A filter that's on, shown as a chip that turns it off: [without] is the query with it off. */
sealed interface ActiveFilter {
    val without: GridQuery

    data class Played(val state: WorkPlayState, override val without: GridQuery) : ActiveFilter

    data class Genre(val name: String, override val without: GridQuery) : ActiveFilter

    data class ContentRating(val name: String, override val without: GridQuery) : ActiveFilter

    /** [decade] by its first year. */
    data class Decade(val decade: Int, override val without: GridQuery) : ActiveFilter

    data class Quality(val resolution: Resolution, override val without: GridQuery) : ActiveFilter

    data class Range(val range: DynamicRange, override val without: GridQuery) : ActiveFilter

    /** [name] is null until the library's narrators are read. */
    data class Narrator(val name: String?, override val without: GridQuery) : ActiveFilter
}

/**
 * The filters on in [query]: the watch status, genres, content ratings,
 * decade, resolutions, dynamic ranges and narrator, the values of each in
 * [offer]'s order, then any it doesn't have.
 */
fun activeFilters(query: GridQuery, offer: FilterOffer): List<ActiveFilter> = buildList {
    query.played?.let { add(ActiveFilter.Played(it, query.copy(played = null))) }
    ordered(query.genres, offer.genres) { it }.forEach { add(ActiveFilter.Genre(it, query.toggleGenre(it))) }
    ordered(query.contentRatings, offer.contentRatings) { it }.forEach { add(ActiveFilter.ContentRating(it, query.toggleContentRating(it))) }
    query.yearFrom?.let { add(ActiveFilter.Decade(it, query.copy(yearFrom = null, yearTo = null))) }
    ordered(query.resolutions, offer.resolutions) { it.value }.forEach { add(ActiveFilter.Quality(it, query.toggleResolution(it))) }
    ordered(query.dynamicRanges, offer.dynamicRanges) { it.value }.forEach { add(ActiveFilter.Range(it, query.toggleDynamicRange(it))) }
    query.narratorId?.let { id -> add(ActiveFilter.Narrator(offer.narrators.firstOrNull { it.id == id }?.name, query.copy(narratorId = null))) }
}

/** [values] in [offered]'s order, then any it doesn't have by [name]. */
private fun <T> ordered(values: List<T>?, offered: List<T>, name: (T) -> String): List<T> {
    val on = values.orEmpty()
    return offered.filter { it in on } + (on - offered.toSet()).sortedBy(name)
}

fun GridQuery.toggleGenre(name: String) = copy(genres = toggled(genres, name))

fun GridQuery.toggleContentRating(name: String) = copy(contentRatings = toggled(contentRatings, name))

fun GridQuery.toggleResolution(resolution: Resolution) = copy(resolutions = toggled(resolutions, resolution))

fun GridQuery.toggleDynamicRange(range: DynamicRange) = copy(dynamicRanges = toggled(dynamicRanges, range))

/** The works of [decade], its ten years from its first, or any decade when it's the one on. */
fun GridQuery.toggleDecade(decade: Int) =
    if (yearFrom == decade) copy(yearFrom = null, yearTo = null) else copy(yearFrom = decade, yearTo = decade + 9)

/** The books [narratorId] narrates, or any narrator's when it's the one on. */
fun GridQuery.toggleNarrator(narratorId: String) = copy(narratorId = if (this.narratorId == narratorId) null else narratorId)

private fun <T> toggled(values: List<T>?, value: T): List<T>? {
    val now = values.orEmpty()
    return (if (value in now) now - value else now + value).ifEmpty { null }
}

/** A group longer than this shows its first [SHORT_LENGTH] values, and any chosen, until it's opened whole. */
private const val LONG_GROUP = 12

private const val SHORT_LENGTH = 9

/** Whether a group of [count] values is long enough to show shortened. */
fun isLongGroup(count: Int) = count > LONG_GROUP

/** [values], or while not [whole] and long, the first few and any [chosen]. */
fun <T> shortened(values: List<T>, whole: Boolean, chosen: (T) -> Boolean): List<T> =
    if (whole || !isLongGroup(values.size)) values else values.filterIndexed { i, value -> i < SHORT_LENGTH || chosen(value) }
