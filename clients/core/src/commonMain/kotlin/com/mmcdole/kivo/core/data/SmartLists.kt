package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.Rule
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.SortKey
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldOp
import com.mmcdole.kivo.api.models.WorkQuery
import kotlin.uuid.ExperimentalUuidApi
import kotlin.uuid.Uuid

/** Why a builder must leave a saved query alone. */
enum class SmartListRefusal { NewerVersion, UnknownField }

/**
 * A rule as the builder holds it: a field, an operator, the field's type and
 * values by the operator's arity, with an empty slot for each exact value
 * not typed yet.
 */
data class RuleDraft(
    val field: String,
    val op: String,
    val arity: Int,
    val type: String,
    val values: List<String>,
    val id: String = newId(),
) {
    /** Whether it has every value its operator takes, each well formed. */
    val isComplete: Boolean
        get() = when {
            arity == 0 -> true
            if (arity < 0) values.isEmpty() else values.size != arity -> false
            else -> values.all(::validValue)
        }

    internal fun toRule() = Rule(field, op, values.ifEmpty { null })

    /** A new field keeps the rule's operator if it has it, its first if not; the values start over. */
    fun withField(field: WorkField): RuleDraft {
        val op = field.ops.firstOrNull { it.op == this.op } ?: field.ops.firstOrNull() ?: return this
        return copy(field = field.field, op = op.op, arity = op.arity, type = field.type, values = slots(op.arity))
    }

    /** A new operator keeps values it reads the same way. */
    fun withOp(op: WorkFieldOp): RuleDraft {
        val sameShape = arity == op.arity && SmartLists.isPeriod(this.op) == SmartLists.isPeriod(op.op)
        return copy(op = op.op, arity = op.arity, values = if (sameShape) values else slots(op.arity))
    }

    fun withValues(values: List<String>): RuleDraft = copy(values = values)

    private fun validValue(value: String): Boolean = when {
        SmartLists.isPeriod(op) -> PERIOD_FORM.matches(value)
        type == "number" -> NUMBER_FORM.matches(value.trim()) && value.trim().toDouble().isFinite()
        type == "date" -> DATE_FORM.matches(value)
        else -> value.isNotBlank()
    }
}

/** A group of rules the root holds. */
data class GroupDraft(
    val match: String,
    val rules: List<RuleDraft> = emptyList(),
    val id: String = newId(),
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
            val chosen = if (libraries.isEmpty()) emptyList() else listOf(Rule(SmartLists.LIBRARY_FIELD, ANY_OF, libraries.sorted()))
            val rootRules = chosen + rules.filter { it.isComplete }.map { it.toRule() }
            val rootGroups = groups.mapNotNull { group ->
                group.rules.filter { it.isComplete }.map { it.toRule() }.takeIf { it.isNotEmpty() }?.let { RuleGroup(group.match, it) }
            }
            val root = if (rootRules.isEmpty() && rootGroups.isEmpty()) null else RuleGroup(match, rootRules.ifEmpty { null }, rootGroups.ifEmpty { null })
            return WorkQuery(kinds, where = root, sort = sort.ifEmpty { null }, maxWorks = maxWorks, collapse = collapse)
        }

    /** The rule [id], in the root or a group. */
    fun rule(id: String): RuleDraft? = (rules + groups.flatMap { it.rules }).firstOrNull { it.id == id }

    fun named(name: String): SmartListDraft = copy(name = name)

    fun showing(kinds: List<String>): SmartListDraft = copy(kinds = kinds)

    fun inLibraries(libraries: Set<String>): SmartListDraft = copy(libraries = libraries)

    fun sorted(sort: List<SortKey>): SmartListDraft = copy(sort = sort)

    /** This draft with [rule] added to the root, or to [groupId]. */
    fun addingRule(rule: RuleDraft, groupId: String?): SmartListDraft =
        if (groupId == null) {
            copy(rules = rules + rule)
        } else {
            copy(groups = groups.map { if (it.id == groupId) it.copy(rules = it.rules + rule) else it })
        }

    /** This draft with the rule [id] replaced by [change]'s result. */
    fun changingRule(id: String, change: (RuleDraft) -> RuleDraft): SmartListDraft = copy(
        rules = rules.map { if (it.id == id) change(it) else it },
        groups = groups.map { group -> group.copy(rules = group.rules.map { if (it.id == id) change(it) else it }) },
    )

    fun removingRule(id: String): SmartListDraft = copy(
        rules = rules.filter { it.id != id },
        groups = groups.map { group -> group.copy(rules = group.rules.filter { it.id != id }) },
    )

    fun addingGroup(group: GroupDraft): SmartListDraft = copy(groups = groups + group)

    fun removingGroup(id: String): SmartListDraft = copy(groups = groups.filter { it.id != id })

    fun withGroupMatch(id: String, match: String): SmartListDraft = copy(groups = groups.map { if (it.id == id) it.copy(match = match) else it })
}

/** Building and reading the smart list builder's rules. */
object SmartLists {
    /** The sort key that isn't a field. */
    const val RANDOM_SORT = "random"

    /** The field the Libraries row saves its choice under. */
    const val LIBRARY_FIELD = "library"

    /** A draft of [query] named [name], or of a new list when null. */
    fun draft(name: String, query: WorkQuery?, fields: List<WorkField>): SmartListDraft {
        val base = SmartListDraft(name, query?.kinds ?: NEW_LIST_KINDS, sort = query?.sort.orEmpty(), maxWorks = query?.maxWorks, collapse = query?.collapse)
        val root = query?.where ?: return base
        val saved = root.rules.orEmpty()
        val libraryAt = if (root.match == "all") saved.indexOfFirst { it.field == LIBRARY_FIELD && it.op == ANY_OF } else -1
        return base.copy(
            match = root.match,
            libraries = saved.getOrNull(libraryAt)?.propertyValues.orEmpty().toSet(),
            rules = saved.filterIndexed { index, _ -> index != libraryAt }.map { ruleDraft(it, fields) },
            groups = root.groups.orEmpty().map { group -> GroupDraft(group.match, group.rules.orEmpty().map { ruleDraft(it, fields) }) },
        )
    }

    /** [rule] as the builder holds it, typed by its field in [fields]. */
    fun ruleDraft(rule: Rule, fields: List<WorkField>): RuleDraft {
        val field = fields.firstOrNull { it.field == rule.field }
        val arity = field?.ops?.firstOrNull { it.op == rule.op }?.arity ?: -1
        return RuleDraft(rule.field, rule.op, arity, field?.type.orEmpty(), rule.propertyValues.orEmpty())
    }

    /** Why a builder can't edit [query], or null if it can. */
    fun refusal(query: WorkQuery, fields: List<WorkField>): SmartListRefusal? {
        if ((query.version ?: 0) > QUERY_VERSION) return SmartListRefusal.NewerVersion
        val rules = query.where?.let(::rulesOf).orEmpty()
        val knownRules = rules.all { rule -> fields.any { it.field == rule.field && it.ops.any { op -> op.op == rule.op } } }
        val knownSorts = query.sort.orEmpty().all { key -> key.field == RANDOM_SORT || fields.any { it.field == key.field && it.sortable } }
        return if (knownRules && knownSorts) null else SmartListRefusal.UnknownField
    }

    private fun rulesOf(group: RuleGroup): List<Rule> = group.rules.orEmpty() + group.groups.orEmpty().flatMap(::rulesOf)

    /** A rule on [field] with its first operator, its values not chosen yet; null when it has no operators. */
    fun blankRule(field: WorkField): RuleDraft? = field.ops.firstOrNull()?.let { op -> RuleDraft(field.field, op.op, op.arity, field.type, slots(op.arity)) }

    /** A group matching any, with one blank rule on [field]. */
    fun blankGroup(field: WorkField): GroupDraft = GroupDraft("any", listOfNotNull(blankRule(field)))

    /** The query for [grid], a library of [libraryKind]: the kinds it lists, its library, its filters and sort, for Save as Smart List. */
    fun fromGrid(grid: GridQuery, libraryKind: String?): WorkQuery {
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
        return WorkQuery(listOf(kind), where = RuleGroup("all", rules), sort = gridSortKeys(grid.sort ?: "title"))
    }

    /** The sort key of a grid's [sort], null for one a query has no field for. */
    private fun gridSortKeys(sort: String): List<SortKey>? = when (sort) {
        "author" -> null
        "runtime" -> listOf(SortKey("runtimeMs"))
        else -> listOf(SortKey(sort))
    }

    /** Whether [op] takes a period: a count of days, weeks, months or years, held as P30D. */
    fun isPeriod(op: String): Boolean = op == "within" || op == "notWithin"

    /** The digits of a period value, "30" of "P30D"; empty for an empty one. */
    fun periodAmount(value: String): String = value.drop(1).dropLast(1)

    /** The unit of a period value, D, W, M or Y; D for none yet. */
    fun periodUnit(value: String?): String = value?.lastOrNull()?.toString() ?: "D"

    /** The period value of [amount] digits in [unit], empty for no digits. */
    fun period(amount: String, unit: String): String = if (amount.isEmpty()) "" else "P$amount$unit"

    /** The key of a [value] of [field] in a names map. */
    fun nameKey(field: String, value: String): String = "$field/$value"

    /** Each field among [fields] that names its values, with [query]'s values of it. */
    internal fun valuesToName(query: WorkQuery, fields: List<WorkField>): Map<String, Set<String>> {
        val named = fields.filter { it.valueSource != null }.map { it.field }.toSet()
        return query.where?.let(::rulesOf).orEmpty()
            .filter { it.field in named }
            .groupBy { it.field }
            .mapValues { (_, rules) -> rules.flatMap { it.propertyValues.orEmpty() }.toSet() }
    }
}

private const val ANY_OF = "anyOf"

/** The query format this app knows. */
private const val QUERY_VERSION = 1

/** What a new list shows. */
private val NEW_LIST_KINDS = listOf("movie")

private val PERIOD_FORM = Regex("P[1-9][0-9]{0,3}[DWMY]")
private val DATE_FORM = Regex("[0-9]{4}-[0-9]{2}-[0-9]{2}")
private val NUMBER_FORM = Regex("[+-]?([0-9]+\\.?[0-9]*|\\.[0-9]+)([eE][+-]?[0-9]+)?")

/** An empty slot for each exact value, none where any number is taken. */
private fun slots(arity: Int): List<String> = if (arity > 0) List(arity) { "" } else emptyList()

@OptIn(ExperimentalUuidApi::class)
private fun newId(): String = Uuid.random().toString()
