package com.mmcdole.kivo.android.mobile.lists

import android.icu.text.ListFormatter
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.api.models.Rule
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkQuery
import java.text.NumberFormat

/** "42 movies", or "42 titles" over more than one kind. */
@Composable
internal fun smartCount(count: Int, kinds: List<String>): String = pluralStringResource(
    when (kinds.singleOrNull()) {
        "movie" -> R.plurals.mobile_count_movies
        "series" -> R.plurals.mobile_count_shows
        "season" -> R.plurals.mobile_count_seasons
        "episode" -> R.plurals.mobile_count_episodes
        "video" -> R.plurals.mobile_count_videos
        "book" -> R.plurals.mobile_count_books
        else -> R.plurals.mobile_count_titles
    },
    count,
    count,
)

@Composable
internal fun kindName(kind: String): String = when (kind) {
    "movie" -> stringResource(R.string.mobile_tab_movies)
    "series" -> stringResource(R.string.mobile_tab_shows)
    "season" -> stringResource(R.string.mobile_seasons)
    "episode" -> stringResource(R.string.mobile_episodes)
    "video" -> stringResource(R.string.mobile_tab_videos)
    "book" -> stringResource(R.string.mobile_tab_books)
    else -> kind
}

/** A list's kinds in a phrase: "Movies and shows". */
@Composable
internal fun kindsPhrase(kinds: List<String>): String {
    val phrase = ListFormatter.getInstance().format(kinds.map { kindName(it).lowercase() })
    return phrase.replaceFirstChar { it.uppercase() }
}

/** A field's label by its key, or the catalog's name for one this app doesn't know. */
@Composable
internal fun fieldName(field: WorkField): String = when (field.field) {
    LIBRARY_FIELD -> stringResource(R.string.mobile_smart_field_library)
    "contentRating" -> stringResource(R.string.mobile_smart_field_content_rating)
    "playState" -> stringResource(R.string.mobile_smart_field_play_state)
    "genre" -> stringResource(R.string.mobile_smart_field_genre)
    "tag" -> stringResource(R.string.mobile_smart_field_tag)
    "company" -> stringResource(R.string.mobile_smart_field_company)
    "resolution" -> stringResource(R.string.mobile_resolution)
    "dynamicRange" -> stringResource(R.string.mobile_dynamic_range)
    "collection" -> stringResource(R.string.mobile_smart_field_collection)
    "bookSeries" -> stringResource(R.string.mobile_smart_field_book_series)
    "inList" -> stringResource(R.string.mobile_smart_field_in_list)
    "rootWork" -> stringResource(R.string.mobile_smart_field_root_work)
    "actor" -> stringResource(R.string.mobile_smart_field_actor)
    "director" -> stringResource(R.string.mobile_smart_field_director)
    "author" -> stringResource(R.string.mobile_smart_field_author)
    "narrator" -> stringResource(R.string.mobile_smart_field_narrator)
    "rating" -> stringResource(R.string.mobile_smart_field_rating)
    "myRating" -> stringResource(R.string.mobile_smart_field_my_rating)
    "runtimeMs" -> stringResource(R.string.mobile_smart_field_runtime)
    "year" -> stringResource(R.string.mobile_smart_field_year)
    "addedAt" -> stringResource(R.string.mobile_smart_field_added_at)
    "lastEpisodeAddedAt" -> stringResource(R.string.mobile_smart_field_last_episode_added_at)
    "releaseDate" -> stringResource(R.string.mobile_smart_field_release_date)
    "lastPlayedAt" -> stringResource(R.string.mobile_smart_field_last_played_at)
    "title" -> stringResource(R.string.mobile_smart_field_title)
    else -> field.name
}

@Composable
internal fun opName(op: String, type: String): String = when (op) {
    "anyOf", "eq" -> stringResource(R.string.mobile_smart_op_is)
    "allOf" -> stringResource(R.string.mobile_smart_op_all_of)
    "noneOf" -> stringResource(R.string.mobile_smart_op_is_not)
    "isSet" -> stringResource(R.string.mobile_smart_op_is_set)
    "isNotSet" -> stringResource(R.string.mobile_smart_op_is_not_set)
    "gte" -> stringResource(if (type == "date") R.string.mobile_smart_op_on_or_after else R.string.mobile_smart_op_at_least)
    "lte" -> stringResource(if (type == "date") R.string.mobile_smart_op_on_or_before else R.string.mobile_smart_op_at_most)
    "between" -> stringResource(R.string.mobile_smart_op_between)
    "within" -> stringResource(R.string.mobile_smart_op_within)
    "notWithin" -> stringResource(R.string.mobile_smart_op_not_within)
    "contains" -> stringResource(R.string.mobile_smart_op_contains)
    "notContains" -> stringResource(R.string.mobile_smart_op_not_contains)
    "startsWith" -> stringResource(R.string.mobile_smart_op_starts_with)
    else -> op
}

/** A sort as the Sort row names it: "Rating, highest first". */
@Composable
internal fun sortName(field: WorkField, direction: String): String {
    val ascending = direction == "asc"
    val order = when (field.type) {
        "date" -> if (ascending) R.string.mobile_smart_oldest_first else R.string.mobile_smart_newest_first
        "text" -> if (direction == "desc") R.string.mobile_smart_z_to_a else R.string.mobile_smart_a_to_z
        else -> when {
            field.unit == "ms" -> if (ascending) R.string.mobile_smart_shortest_first else R.string.mobile_smart_longest_first
            ascending -> R.string.mobile_smart_lowest_first
            else -> R.string.mobile_smart_highest_first
        }
    }
    return stringResource(R.string.mobile_smart_sort_named, fieldName(field), stringResource(order))
}

internal fun isPeriod(op: String): Boolean = op == "within" || op == "notWithin"

/** A period value, "P30D", as words: "30 days". */
@Composable
internal fun periodWords(value: String): String {
    val amount = value.drop(1).dropLast(1).toIntOrNull() ?: 0
    val plural = when (value.lastOrNull()) {
        'D' -> R.plurals.mobile_smart_days
        'W' -> R.plurals.mobile_smart_weeks
        'M' -> R.plurals.mobile_smart_months
        else -> R.plurals.mobile_smart_years
    }
    return pluralStringResource(plural, amount, amount)
}

/** A value in milliseconds as minutes. */
internal fun minutesOf(value: String): String {
    val ms = value.toDoubleOrNull() ?: return value
    return NumberFormat.getNumberInstance().apply {
        isGroupingUsed = false
        maximumFractionDigits = 3
    }.format(ms / 60_000)
}

/** The key of a value in a names map. */
internal fun nameKey(field: String, value: String): String = "$field/$value"

/**
 * A rule's values to read: names where the values are chosen, a period in
 * words, a duration in minutes, the two ends of a range joined by [to].
 */
@Composable
internal fun ruleValues(rule: RuleDraft, field: WorkField, names: Map<String, String>, to: String = stringResource(R.string.mobile_smart_and)): String {
    if (isPeriod(rule.op)) return rule.values.firstOrNull()?.let { periodWords(it) }.orEmpty()
    if (rule.arity == -1) return rule.values.joinToString(", ") { names[nameKey(rule.field, it)] ?: it }
    val shown = if (field.unit == "ms") rule.values.map { stringResource(R.string.mobile_smart_minutes, minutesOf(it)) } else rule.values
    return shown.joinToString(" $to ")
}

/** [query]'s rules in a line: "Comedy, no Animation, 1990 to 1999, Unplayed". */
@Composable
internal fun rulesSummary(query: WorkQuery, fields: List<WorkField>, names: Map<String, String>): String {
    val root = query.where ?: return ""
    return phrases(root, fields, names).joinToString(if (root.match == "all") ", " else " ${stringResource(R.string.mobile_smart_or)} ")
}

@Composable
private fun phrases(group: RuleGroup, fields: List<WorkField>, names: Map<String, String>): List<String> {
    val rules = group.rules.orEmpty().filter { it.field != LIBRARY_FIELD }.mapNotNull { rule ->
        fields.firstOrNull { it.field == rule.field }?.let { rulePhrase(rule, it, names) }
    }
    val nested = group.groups.orEmpty().map { inner ->
        val joiner = stringResource(if (inner.match == "all") R.string.mobile_smart_and else R.string.mobile_smart_or)
        "(${phrases(inner, fields, names).joinToString(" $joiner ")})"
    }
    return rules + nested
}

@Composable
private fun rulePhrase(rule: Rule, field: WorkField, names: Map<String, String>): String {
    val arity = field.ops.firstOrNull { it.op == rule.op }?.arity ?: -1
    val draft = RuleDraft(rule.field, rule.op, arity, rule.propertyValues.orEmpty())
    val label = fieldName(field)
    return when (rule.op) {
        "anyOf", "allOf" -> ruleValues(draft, field, names)
        "noneOf" -> stringResource(R.string.mobile_smart_no, ruleValues(draft, field, names))
        "isSet" -> stringResource(R.string.mobile_smart_has, label.lowercase())
        "isNotSet" -> stringResource(R.string.mobile_smart_no, label.lowercase())
        "between" -> {
            val range = ruleValues(draft, field, names, to = stringResource(R.string.mobile_smart_to))
            if (field.field == "year") range else "$label $range"
        }
        else -> "$label ${opName(rule.op, field.type)} ${ruleValues(draft, field, names)}"
    }
}
