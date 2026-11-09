package com.mmcdole.kivo.android.mobile.lists

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.SortKey
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldOp
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.session.SessionLibrary
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** How long the rules or a value search rest before they're read. */
internal const val READ_DELAY_MS = 350L

/** The field types this app can edit. */
private val FieldTypes = setOf("choice", "set", "person", "number", "date", "text")

/** Where the builder is: its form, or a page the form opened. */
sealed interface BuilderPage {
    data object Form : BuilderPage

    data object Kinds : BuilderPage

    data object Libraries : BuilderPage

    data class EditingRule(val id: String) : BuilderPage
}

data class BuilderUiState(
    /** The catalog's fields of the types this app can edit; null until read. */
    val fields: List<WorkField>? = null,
    val draft: SmartListDraft? = null,
    /** The saved query is one this app must leave alone. */
    val refused: Boolean = false,
    /** Why the catalog couldn't be read. */
    val failure: RequestProblem? = null,
    /** Why Save was refused, until it's dismissed. */
    val problem: RequestProblem? = null,
    val page: BuilderPage = BuilderPage.Form,
    /** How many works match; null before the first answer or with no kinds. */
    val matching: Int? = null,
    val matchingProblem: RequestProblem? = null,
    /** Chosen values' names, by [nameKey]. */
    val names: Map<String, String> = emptyMap(),
    val libraries: List<Library> = emptyList(),
)

/**
 * The smart list builder: a new list from [query] and [name], or the list
 * [listId] on its saved query. The live count follows the complete rules a
 * moment after they stop changing.
 */
@OptIn(FlowPreview::class)
class SmartListBuilderViewModel(
    private val session: KivoSession,
    private val listId: String?,
    private val name: String,
    private val query: WorkQuery?,
) : ViewModel() {
    private val mutable = MutableStateFlow(BuilderUiState())
    val state: StateFlow<BuilderUiState> = mutable.asStateFlow()

    init {
        load()
        viewModelScope.launch {
            session.library.libraries().collect { load -> load.getOrNull()?.let { read -> mutable.update { it.copy(libraries = read) } } }
        }
        viewModelScope.launch {
            mutable.map { it.draft?.query }.distinctUntilChanged().debounce(READ_DELAY_MS).collectLatest(::count)
        }
    }

    private suspend fun count(query: WorkQuery?) {
        if (query == null || query.kinds.isEmpty()) {
            mutable.update { it.copy(matching = null, matchingProblem = null) }
            return
        }
        try {
            val total = session.library.countMatching(query)
            mutable.update { it.copy(matching = total, matchingProblem = null) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            mutable.update { it.copy(matchingProblem = RequestProblem.of(e)) }
        }
    }

    /** Reads the catalog and starts the draft, or refuses the saved query. */
    fun load() {
        viewModelScope.launch {
            try {
                val catalog = session.library.workFields().filter { it.type in FieldTypes }
                val refused = query != null && refusesEditing(query, catalog)
                mutable.update {
                    it.copy(fields = catalog, failure = null, refused = refused, draft = if (refused) null else smartListDraft(name, query, catalog))
                }
                if (query != null && !refused) {
                    val names = readNames(query, catalog, session.library)
                    mutable.update { it.copy(names = it.names + names) }
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                mutable.update { it.copy(failure = RequestProblem.of(e)) }
            }
        }
    }

    private fun changeDraft(change: (SmartListDraft) -> SmartListDraft) = mutable.update { state -> state.copy(draft = state.draft?.let(change)) }

    fun setName(name: String) = changeDraft { it.copy(name = name) }

    /** Turns [kind] on or off, keeping the catalog's order [offered]. */
    fun toggleKind(kind: String, offered: List<String>) = changeDraft { draft ->
        val others = if (kind in draft.kinds) draft.kinds - kind else draft.kinds + kind
        draft.copy(kinds = offered.filter { it in others })
    }

    fun clearLibraries() = changeDraft { it.copy(libraries = emptySet()) }

    fun toggleLibrary(id: String) = changeDraft { draft ->
        draft.copy(libraries = if (id in draft.libraries) draft.libraries - id else draft.libraries + id)
    }

    fun show(page: BuilderPage) = mutable.update { it.copy(page = page) }

    /** A new rule on the first field the kinds offer, opened at once; in [groupId], or the root's. */
    fun addRule(groupId: String?) {
        val state = mutable.value
        val fields = state.fields ?: return
        val draft = state.draft ?: return
        val field = offeredFields(fields, draft.kinds, null).firstOrNull() ?: return
        val op = field.ops.firstOrNull() ?: return
        val rule = RuleDraft(field.field, op.op, op.arity, ruleSlots(op.arity))
        changeDraft {
            if (groupId == null) {
                it.copy(rules = it.rules + rule)
            } else {
                it.copy(groups = it.groups.map { group -> if (group.id == groupId) group.copy(rules = group.rules + rule) else group })
            }
        }
        show(BuilderPage.EditingRule(rule.id))
    }

    fun removeRule(id: String) = changeDraft { draft ->
        draft.copy(rules = draft.rules.filter { it.id != id }, groups = draft.groups.map { group -> group.copy(rules = group.rules.filter { it.id != id }) })
    }

    fun changeRule(id: String, change: (RuleDraft) -> RuleDraft) = changeDraft { it.changingRule(id, change) }

    /** A new group matching any, with a new rule opened at once. */
    fun addGroup() {
        val group = GroupDraft()
        changeDraft { it.copy(groups = it.groups + group) }
        addRule(group.id)
    }

    fun removeGroup(id: String) = changeDraft { draft -> draft.copy(groups = draft.groups.filter { it.id != id }) }

    fun setGroupMatch(id: String, match: String) = changeDraft { draft ->
        draft.copy(groups = draft.groups.map { if (it.id == id) it.copy(match = match) else it })
    }

    fun setSort(sort: List<SortKey>) = changeDraft { it.copy(sort = sort) }

    fun noteName(field: String, value: WorkFieldValue) = mutable.update { it.copy(names = it.names + (nameKey(field, value.value) to value.name)) }

    /** The values of [field] a search finds, null when they couldn't be read. */
    suspend fun values(field: String, search: String): List<WorkFieldValue>? = try {
        session.library.workFieldValues(field, search.ifEmpty { null }, null)
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        null
    }

    fun dismissProblem() = mutable.update { it.copy(problem = null) }

    /** Saves the list; [onSaved] with the new list, or null after an edit. */
    fun save(onSaved: (WorkList?) -> Unit) {
        val draft = mutable.value.draft ?: return
        viewModelScope.launch {
            try {
                val trimmed = draft.name.trim()
                if (listId != null) {
                    session.library.updateList(listId, trimmed, draft.query)
                    onSaved(null)
                } else {
                    onSaved(session.library.createSmartList(trimmed, draft.query))
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                mutable.update { it.copy(problem = RequestProblem.of(e)) }
            }
        }
    }
}

/** The kinds the catalog offers, in its order. */
fun offeredKinds(fields: List<WorkField>): List<String> = fields.flatMap { it.kinds }.distinct()

/** The fields a rule can use for the [kinds] showing, and the one a rule already has, [kept]; `library` is never offered. */
fun offeredFields(fields: List<WorkField>, kinds: List<String>, kept: String?): List<WorkField> = fields.filter { field ->
    field.field == kept || (field.field != LIBRARY_FIELD && field.kinds.any { it in kinds })
}

/** A new field keeps the rule's operator if it has it, its first if not; the values start over. */
fun withField(rule: RuleDraft, field: WorkField): RuleDraft {
    val op = field.ops.firstOrNull { it.op == rule.op } ?: field.ops.firstOrNull() ?: return rule
    return rule.copy(field = field.field, op = op.op, arity = op.arity, values = ruleSlots(op.arity))
}

/** A new operator keeps values it reads the same way. */
fun withOp(rule: RuleDraft, op: WorkFieldOp): RuleDraft {
    val sameShape = rule.arity == op.arity && isPeriod(rule.op) == isPeriod(op.op)
    return rule.copy(op = op.op, arity = op.arity, values = if (sameShape) rule.values else ruleSlots(op.arity))
}

/** Reads the names of [query]'s chosen values, as a names map. */
internal suspend fun readNames(query: WorkQuery, fields: List<WorkField>, library: SessionLibrary): Map<String, String> {
    val wanted = mutableMapOf<String, MutableSet<String>>()
    fun collect(group: RuleGroup) {
        for (rule in group.rules.orEmpty()) {
            if (fields.firstOrNull { it.field == rule.field }?.valueSource != null) wanted.getOrPut(rule.field) { mutableSetOf() } += rule.propertyValues.orEmpty()
        }
        group.groups.orEmpty().forEach(::collect)
    }
    query.where?.let(::collect)
    val names = mutableMapOf<String, String>()
    for ((field, values) in wanted) {
        try {
            for (found in library.workFieldValues(field, null, values.toList())) names[nameKey(field, found.value)] = found.name
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // The values show as saved.
        }
    }
    return names
}
