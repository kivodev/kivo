package com.mmcdole.kivo.android.mobile.lists

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.SortKey
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.RuleDraft
import com.mmcdole.kivo.core.data.SmartListDraft
import com.mmcdole.kivo.core.data.SmartListRefusal
import com.mmcdole.kivo.core.data.SmartLists
import com.mmcdole.kivo.core.session.KivoSession
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

    /** The fields a new rule starts from; in the group [groupId], or the root's. */
    data class NewRule(val groupId: String?) : BuilderPage

    /** The fields a new group's first rule starts from. */
    data object NewGroup : BuilderPage

    data class EditingRule(val id: String) : BuilderPage
}

data class BuilderUiState(
    /** The catalog's fields of the types this app can edit; null until read. */
    val fields: List<WorkField>? = null,
    val draft: SmartListDraft? = null,
    /** Why the saved query must be left alone, when it must. */
    val refusal: SmartListRefusal? = null,
    /** Why the catalog couldn't be read. */
    val failure: RequestProblem? = null,
    /** Why Save was refused, until it's dismissed. */
    val problem: RequestProblem? = null,
    val page: BuilderPage = BuilderPage.Form,
    /** How many works match; null before the first answer or with no kinds. */
    val matching: Int? = null,
    val matchingProblem: RequestProblem? = null,
    /** Chosen values' names, by [SmartLists.nameKey]. */
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
                val refusal = query?.let { SmartLists.refusal(it, catalog) }
                mutable.update {
                    it.copy(fields = catalog, failure = null, refusal = refusal, draft = if (refusal != null) null else SmartLists.draft(name, query, catalog))
                }
                if (query != null && refusal == null) {
                    val names = session.library.smartListNames(query, catalog)
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

    fun setName(name: String) = changeDraft { it.named(name) }

    /** Turns [kind] on or off, keeping the catalog's order [offered]. */
    fun toggleKind(kind: String, offered: List<String>) = changeDraft { draft ->
        val others = if (kind in draft.kinds) draft.kinds - kind else draft.kinds + kind
        draft.showing(offered.filter { it in others })
    }

    fun clearLibraries() = changeDraft { it.inLibraries(emptySet()) }

    fun toggleLibrary(id: String) = changeDraft { draft ->
        draft.inLibraries(if (id in draft.libraries) draft.libraries - id else draft.libraries + id)
    }

    fun show(page: BuilderPage) = mutable.update { it.copy(page = page) }

    /** Opens the fields [page] starts from, when the kinds offer any. */
    fun chooseField(page: BuilderPage) {
        val state = mutable.value
        val draft = state.draft ?: return
        if (offeredFields(state.fields ?: return, draft.kinds, null).isNotEmpty()) show(page)
    }

    /** A new rule on [field], opened at once; in [groupId], or the root's. */
    fun addRule(field: WorkField, groupId: String?) {
        val rule = SmartLists.blankRule(field) ?: return
        changeDraft { it.addingRule(rule, groupId) }
        show(BuilderPage.EditingRule(rule.id))
    }

    fun removeRule(id: String) = changeDraft { it.removingRule(id) }

    fun changeRule(id: String, change: (RuleDraft) -> RuleDraft) = changeDraft { it.changingRule(id, change) }

    /** A new group matching any, with a new rule on [field] opened at once. */
    fun addGroup(field: WorkField) {
        val group = SmartLists.blankGroup(field)
        changeDraft { it.addingGroup(group) }
        group.rules.firstOrNull()?.let { show(BuilderPage.EditingRule(it.id)) }
    }

    fun removeGroup(id: String) = changeDraft { it.removingGroup(id) }

    fun setGroupMatch(id: String, match: String) = changeDraft { it.withGroupMatch(id, match) }

    fun setSort(sort: List<SortKey>) = changeDraft { it.sorted(sort) }

    fun noteName(field: String, value: WorkFieldValue) = mutable.update { it.copy(names = it.names + (SmartLists.nameKey(field, value.value) to value.name)) }

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
    field.field == kept || (field.field != SmartLists.LIBRARY_FIELD && field.kinds.any { it in kinds })
}
