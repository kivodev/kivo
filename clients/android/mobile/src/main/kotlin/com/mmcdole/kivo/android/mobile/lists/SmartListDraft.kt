package com.mmcdole.kivo.android.mobile.lists

import com.mmcdole.kivo.android.library.GridValues
import com.mmcdole.kivo.api.models.Rule
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.SortKey
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.core.data.GridQuery
import java.util.UUID

/** The sort key that isn't a field. */
internal const val RANDOM_SORT = "random"

/** The field the Libraries row saves its choice under. */
internal const val LIBRARY_FIELD = "library"

private const val ANY_OF = "anyOf"

/** The query format this app knows. */
private const val QUERY_VERSION = 1

/** What a new list shows. */
private val DefaultKinds = listOf("movie")

/** An empty slot for each exact value, none where any number is taken. */
internal fun ruleSlots(arity: Int): List<String> = if (arity > 0) List(arity) { "" } else emptyList()

/**
 * A rule as the builder holds it: a field, an operator and values by the
 * operator's arity, with an empty slot for each exact value not typed yet.
 */
data class RuleDraft(
    val field: String,
    val op: String,
    val arity: Int,
    val values: List<String>,
    val id: String = UUID.randomUUID().toString(),
) {
    /** Whether every value it takes is there. */
    val isComplete: Boolean get() = if (arity < 0) values.isNotEmpty() else values.size == arity && values.all { it.isNotEmpty() }

    internal fun toRule() = Rule(field, op, values.ifEmpty { null })
}

/** A group of rules the root holds; Add Group starts one matching any. */
data class GroupDraft(
    val match: String = "any",
    val rules: List<RuleDraft> = emptyList(),
    val id: String = UUID.randomUUID().toString(),
)

/**
 * What the smart list builder edits: the name, the kinds, the libraries, the
 * rules and groups, and the sort. The max works and collapse a query carries
 * are kept untouched, and so is its sort until another is chosen.
 */
data class SmartListDraft(
    val name: String,
    val kinds: List<String>,
    /** The libraries of the `library` rule, empty for every library. */
    val libraries: Set<String> = emptySet(),
    val match: String = "all",
    val rules: List<RuleDraft> = emptyList(),
    val groups: List<GroupDraft> = emptyList(),
    val sort: List<SortKey> = emptyList(),
    val maxWorks: Int? = null,
    val collapse: String? = null,
) {
    val isReady: Boolean
        get() = name.isNotBlank() && kinds.isNotEmpty() && (rules + groups.flatMap { it.rules }).all { it.isComplete }

    /** The query of the complete rules, for the count and for saving. */
    val query: WorkQuery
        get() {
            val chosen = if (libraries.isEmpty()) emptyList() else listOf(Rule(LIBRARY_FIELD, ANY_OF, libraries.sorted()))
            val rootRules = chosen + rules.filter { it.isComplete }.map { it.toRule() }
            val rootGroups = groups.mapNotNull { group ->
                group.rules.filter { it.isComplete }.map { it.toRule() }.takeIf { it.isNotEmpty() }?.let { RuleGroup(group.match, it) }
            }
            val root = if (rootRules.isEmpty() && rootGroups.isEmpty()) null else RuleGroup(match, rootRules.ifEmpty { null }, rootGroups.ifEmpty { null })
            return WorkQuery(kinds, where = root, sort = sort.ifEmpty { null }, maxWorks = maxWorks, collapse = collapse)
        }

    /** The rule [id], in the root or a group. */
    fun rule(id: String): RuleDraft? = (rules + groups.flatMap { it.rules }).firstOrNull { it.id == id }

    /** This draft with the rule [id] replaced by [change]'s result. */
    fun changingRule(id: String, change: (RuleDraft) -> RuleDraft): SmartListDraft = copy(
        rules = rules.map { if (it.id == id) change(it) else it },
        groups = groups.map { group -> group.copy(rules = group.rules.map { if (it.id == id) change(it) else it }) },
    )
}

/** A draft of [query] named [name], or of a new list when null. */
fun smartListDraft(name: String, query: WorkQuery?, fields: List<WorkField>): SmartListDraft {
    val base = SmartListDraft(name, query?.kinds ?: DefaultKinds, sort = query?.sort.orEmpty(), maxWorks = query?.maxWorks, collapse = query?.collapse)
    val root = query?.where ?: return base
    val saved = root.rules.orEmpty()
    val libraryAt = if (root.match == "all") saved.indexOfFirst { it.field == LIBRARY_FIELD && it.op == ANY_OF } else -1
    return base.copy(
        match = root.match,
        libraries = saved.getOrNull(libraryAt)?.propertyValues.orEmpty().toSet(),
        rules = saved.filterIndexed { index, _ -> index != libraryAt }.map { draftOf(it, fields) },
        groups = root.groups.orEmpty().map { group -> GroupDraft(group.match, group.rules.orEmpty().map { draftOf(it, fields) }) },
    )
}

private fun draftOf(rule: Rule, fields: List<WorkField>): RuleDraft {
    val op = fields.firstOrNull { it.field == rule.field }?.ops?.firstOrNull { it.op == rule.op }
    return RuleDraft(rule.field, rule.op, op?.arity ?: -1, rule.propertyValues.orEmpty())
}

/** Whether a builder must leave [query] alone: a newer version, or a field missing from [fields]. */
fun refusesEditing(query: WorkQuery, fields: List<WorkField>): Boolean {
    if ((query.version ?: 0) > QUERY_VERSION) return true
    val known = fields.map { it.field }.toSet()
    val inRules = query.where?.let(::ruleFields).orEmpty()
    val inSort = query.sort.orEmpty().map { it.field }.filter { it != RANDOM_SORT }
    return !known.containsAll(inRules + inSort)
}

private fun ruleFields(group: RuleGroup): List<String> =
    group.rules.orEmpty().map { it.field } + group.groups.orEmpty().flatMap(::ruleFields)

/** The query for [grid], a library of [libraryKind]: the kinds it lists, its library, its filters and sort, for Save as Smart List. */
fun smartListQuery(grid: GridQuery, libraryKind: String?): WorkQuery {
    val rules = buildList {
        add(Rule(LIBRARY_FIELD, ANY_OF, listOf(grid.libraryId)))
        grid.played?.let { add(Rule("playState", ANY_OF, listOf(it))) }
        grid.resolutions?.takeIf { it.isNotEmpty() }?.let { add(Rule("resolution", ANY_OF, it.sorted())) }
        grid.dynamicRanges?.takeIf { it.isNotEmpty() }?.let { add(Rule("dynamicRange", ANY_OF, it.sorted())) }
    }
    val kind = when (libraryKind) {
        "movies" -> "movie"
        "shows" -> "series"
        "audiobooks" -> "book"
        else -> "video"
    }
    return WorkQuery(listOf(kind), where = RuleGroup("all", rules), sort = gridSortKeys(grid.sort ?: GridValues.TITLE))
}

/** The sort key of a grid's [sort], null for one a query has no field for. */
private fun gridSortKeys(sort: String): List<SortKey>? = when (sort) {
    GridValues.AUTHOR -> null
    GridValues.RUNTIME -> listOf(SortKey("runtimeMs"))
    else -> listOf(SortKey(sort))
}
