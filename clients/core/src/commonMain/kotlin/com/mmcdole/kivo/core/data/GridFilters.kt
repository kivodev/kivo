package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.Resolution as VideoResolution
import com.mmcdole.kivo.core.data.DynamicRange as VideoRange

/**
 * The groups a library's filters come in, in the order they show. Of a
 * [single] group one value is on at a time; of another, any number, a work
 * matching any of them. The groups that are on narrow the works together.
 */
enum class FilterGroupKind(val single: Boolean) {
    WatchStatus(true),
    Decade(true),
    Genre(false),
    ContentRating(false),

    /** The resolutions, then the dynamic ranges. */
    Quality(false),
    Narrator(true),
}

/** One value a library's works can be filtered by, in its [group]; [key] names it among them, as `genre:Drama`. */
sealed class GridFilter {
    abstract val group: FilterGroupKind
    abstract val key: String

    data class Played(val state: WorkPlayState) : GridFilter() {
        override val group get() = FilterGroupKind.WatchStatus
        override val key get() = "played:${state.value}"
    }

    /** The ten years from [year], 1990 for the 1990s. */
    data class Decade(val year: Int) : GridFilter() {
        override val group get() = FilterGroupKind.Decade
        override val key get() = "decade:$year"
    }

    data class Genre(val name: String) : GridFilter() {
        override val group get() = FilterGroupKind.Genre
        override val key get() = "genre:$name"
    }

    data class ContentRating(val name: String) : GridFilter() {
        override val group get() = FilterGroupKind.ContentRating
        override val key get() = "contentRating:$name"
    }

    data class Resolution(val resolution: VideoResolution) : GridFilter() {
        override val group get() = FilterGroupKind.Quality
        override val key get() = "resolution:${resolution.value}"
    }

    data class DynamicRange(val range: VideoRange) : GridFilter() {
        override val group get() = FilterGroupKind.Quality
        override val key get() = "dynamicRange:${range.value}"
    }

    /** A books library's narrator, by person ID; [name] is null until something has named them. */
    data class Narrator(val id: String, val name: String?) : GridFilter() {
        override val group get() = FilterGroupKind.Narrator
        override val key get() = "narrator:$id"
    }
}

/**
 * The filters on for a library's works, in the groups' order. A value is
 * known by its [GridFilter.key], so a narrator is the same one whatever
 * name it carries.
 */
@ConsistentCopyVisibility
data class GridFilters internal constructor(val chosen: List<GridFilter>) {
    /** Whether none narrows the works. */
    val isEmpty: Boolean get() = chosen.isEmpty()

    /** The watch status that's on; null for all. */
    val played: WorkPlayState? get() = chosen.firstNotNullOfOrNull { (it as? GridFilter.Played)?.state }

    fun has(filter: GridFilter): Boolean = chosen.any { it.key == filter.key }

    /** These with [filter] turned off when it's on, else turned on as [choosing] does. */
    fun toggling(filter: GridFilter): GridFilters = if (has(filter)) of(chosen.filter { it.key != filter.key }) else choosing(filter)

    /** These with [filter] on, in place of the one on in its group when the group takes one. */
    fun choosing(filter: GridFilter): GridFilters {
        val others = chosen.filter { it.key != filter.key && !(filter.group.single && it.group == filter.group) }
        return of(others + filter)
    }

    /** These with none of [group] on. */
    fun clearing(group: FilterGroupKind): GridFilters = of(chosen.filter { it.group != group })

    /** The works' query in [libraryId] by [sort] with these on: a decade is its ten years, and the values of a group are in one order however they were chosen. */
    internal fun query(libraryId: String, sort: GridSort): GridQuery {
        val decade = chosen.firstNotNullOfOrNull { (it as? GridFilter.Decade)?.year }
        return GridQuery(
            libraryId = libraryId,
            sort = sort,
            played = played,
            genres = chosen.mapNotNull { (it as? GridFilter.Genre)?.name }.ifEmpty { null },
            yearFrom = decade,
            yearTo = decade?.plus(9),
            contentRatings = chosen.mapNotNull { (it as? GridFilter.ContentRating)?.name }.ifEmpty { null },
            resolutions = chosen.mapNotNull { (it as? GridFilter.Resolution)?.resolution }.ifEmpty { null },
            dynamicRanges = chosen.mapNotNull { (it as? GridFilter.DynamicRange)?.range }.ifEmpty { null },
            narratorId = chosen.firstNotNullOfOrNull { (it as? GridFilter.Narrator)?.id },
        )
    }

    companion object {
        /** No filter on. */
        val None = GridFilters(emptyList())

        /** [filters] as a choice: by group, resolutions before dynamic ranges, then by key; one of a key, and the last of a group that takes one. */
        internal fun of(filters: List<GridFilter>): GridFilters {
            val kept = filters.asReversed()
                .distinctBy { if (it.group.single) it.group.name else it.key }
                .sortedWith(compareBy({ it.group }, { it is GridFilter.DynamicRange }, { it.key }))
            return GridFilters(kept)
        }
    }
}

/** A group of a library's filter offer: its [values] in the order to show them. */
data class FilterGroup(val kind: FilterGroupKind, val values: List<GridFilter>) {
    /** Whether a touch sheet shows it shortened until it's opened whole: the genres or narrators, when there are many. */
    val isLong: Boolean get() = (kind == FilterGroupKind.Genre || kind == FilterGroupKind.Narrator) && values.size > LONG_GROUP

    /** The values to show: all of them, or while a long group isn't opened [whole], its first few and any of [chosen]. */
    fun shown(whole: Boolean, chosen: GridFilters): List<GridFilter> =
        if (whole || !isLong) values else values.filterIndexed { i, value -> i < SHORT_LENGTH || chosen.has(value) }

    private companion object {
        const val LONG_GROUP = 12
        const val SHORT_LENGTH = 9
    }
}

/**
 * The groups a library of [kind] offers, of the values its works have
 * ([filters], null until read), in the groups' order, one with no values
 * left out: the watch status always; decades, newest first, and genres; a
 * books library's narrators; another's content ratings and quality.
 */
internal fun filterOffer(kind: LibraryKind?, filters: LibraryFilters?): List<FilterGroup> {
    val books = kind == LibraryKind.Audiobooks
    val groups = listOf(
        FilterGroup(FilterGroupKind.WatchStatus, WorkPlayState.entries.map(GridFilter::Played)),
        FilterGroup(FilterGroupKind.Decade, decadesOf(filters?.years.orEmpty()).map(GridFilter::Decade)),
        FilterGroup(FilterGroupKind.Genre, filters?.genres.orEmpty().map(GridFilter::Genre)),
        FilterGroup(FilterGroupKind.ContentRating, filters?.contentRatings.orEmpty().takeUnless { books }.orEmpty().map(GridFilter::ContentRating)),
        FilterGroup(
            FilterGroupKind.Quality,
            if (books) emptyList() else filters?.resolutions.orEmpty().map(GridFilter::Resolution) + filters?.dynamicRanges.orEmpty().map(GridFilter::DynamicRange),
        ),
        FilterGroup(FilterGroupKind.Narrator, filters?.narrators.orEmpty().takeIf { books }.orEmpty().map { GridFilter.Narrator(it.id, it.name) }),
    )
    return groups.filter { it.values.isNotEmpty() }
}

/**
 * The filters on in [chosen], in the order [offer] shows them: by group,
 * a group's values as offered, then any it doesn't offer, by key. A
 * narrator goes by the name the offer gives.
 */
internal fun activeFilters(chosen: GridFilters, offer: List<FilterGroup>): List<GridFilter> {
    val offered = offer.flatMap { it.values }
    val known = offered.filter(chosen::has)
    val others = chosen.chosen.filter { on -> known.none { it.key == on.key } }
    return (known + others).sortedBy { it.group }
}

/** The decades [years] fall in, by their first year, newest first. */
internal fun decadesOf(years: List<Int>): List<Int> = years.map { it - it % 10 }.distinct().sortedDescending()

/**
 * The sorts a library of [kind] offers, in the menu's order: an
 * audiobooks library's by title, author, date added and release date;
 * any other's by title, date added, the last episode added for shows,
 * release date, rating, runtime and last played.
 */
internal fun sortsOf(kind: LibraryKind?): List<GridSort> = when (kind) {
    LibraryKind.Audiobooks -> listOf(GridSort.Title, GridSort.Author, GridSort.AddedAt, GridSort.ReleaseDate)
    LibraryKind.Shows -> listOf(
        GridSort.Title,
        GridSort.AddedAt,
        GridSort.LastEpisodeAddedAt,
        GridSort.ReleaseDate,
        GridSort.Rating,
        GridSort.Runtime,
        GridSort.LastPlayedAt,
    )
    else -> listOf(GridSort.Title, GridSort.AddedAt, GridSort.ReleaseDate, GridSort.Rating, GridSort.Runtime, GridSort.LastPlayedAt)
}
