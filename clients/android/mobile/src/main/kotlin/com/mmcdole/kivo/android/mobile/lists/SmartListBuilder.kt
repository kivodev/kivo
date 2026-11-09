package com.mmcdole.kivo.android.mobile.lists

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.ListRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.SmartListBuilderRoute
import com.mmcdole.kivo.android.mobile.downloads.Swipeable
import com.mmcdole.kivo.android.mobile.settings.Choice
import com.mmcdole.kivo.android.mobile.settings.ChoiceRow
import com.mmcdole.kivo.android.mobile.settings.FailureRow
import com.mmcdole.kivo.android.mobile.settings.Footnote
import com.mmcdole.kivo.android.mobile.settings.Label
import com.mmcdole.kivo.android.mobile.settings.SettingRow
import com.mmcdole.kivo.android.mobile.settings.clear
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Loading
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.android.mobile.ui.wideWidth
import com.mmcdole.kivo.api.models.SortKey
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.core.data.RuleDraft
import com.mmcdole.kivo.core.data.SmartListDraft
import com.mmcdole.kivo.core.data.SmartListRefusal
import com.mmcdole.kivo.core.data.SmartLists
import kotlinx.coroutines.delay
import kotlinx.serialization.json.Json

/** The route opening the builder: on a new list from [query] and [name], or with [listId] on that list. */
internal fun smartListBuilderRoute(name: String, query: WorkQuery?, listId: String? = null) =
    SmartListBuilderRoute(listId, name, query?.let { Json.encodeToString(WorkQuery.serializer(), it) })

/**
 * The smart list builder, a page over the tabs: the name, what to show,
 * which libraries, the rules and groups, the sort, and how many works match,
 * driven by the server's field catalog. A saved query this app can't edit
 * whole is left alone, and says so.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SmartListBuilderScreen(session: ServerSession, route: SmartListBuilderRoute, nav: MobileNav) {
    val model = viewModel {
        SmartListBuilderViewModel(session.session, route.listId, route.name, route.query?.let { Json.decodeFromString(WorkQuery.serializer(), it) })
    }
    val state by model.state.collectAsStateWithLifecycle()
    val draft = state.draft
    val page = state.page
    val rule = (page as? BuilderPage.EditingRule)?.let { draft?.rule(it.id) }
    val fields = state.fields
    val refusal = state.refusal
    BackHandler(page != BuilderPage.Form) { model.show(BuilderPage.Form) }
    when {
        page == BuilderPage.Kinds && draft != null && fields != null -> KindsPage(draft, offeredKinds(fields), model)
        page == BuilderPage.Libraries && draft != null -> LibrariesPage(draft, state, model)
        rule != null && fields != null && draft != null -> RulePage(rule, offeredFields(fields, draft.kinds, rule.field), state.names, model)
        else -> Scaffold(
            topBar = {
                TopAppBar(
                    title = { Text(stringResource(if (route.listId == null) R.string.mobile_smart_new else R.string.mobile_smart_edit), maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    navigationIcon = { TextButton(onClick = nav::back, Modifier.testTag("smartListCancel")) { Text(stringResource(R.string.mobile_cancel)) } },
                    actions = {
                        if (draft != null) {
                            TextButton(
                                onClick = { model.save { made -> if (made != null) nav.replaceBuilder(ListRoute(made.id, made.name)) else nav.back() } },
                                enabled = draft.isReady,
                                modifier = Modifier.testTag("smartListSave"),
                            ) { Text(stringResource(R.string.mobile_smart_save)) }
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background, scrolledContainerColor = MaterialTheme.colorScheme.background),
                )
            },
        ) { padding ->
            Box(Modifier.padding(padding).fillMaxSize()) {
                val failure = state.failure
                when {
                    refusal != null -> Message(
                        stringResource(R.string.mobile_smart_refused),
                        stringResource(if (refusal == SmartListRefusal.NewerVersion) R.string.mobile_smart_refused_newer else R.string.mobile_smart_refused_unknown),
                    )
                    draft != null && fields != null -> BuilderForm(draft, fields, state, model)
                    failure != null -> Column { FailureRow(sentence(failure), model::load) }
                    else -> Loading()
                }
            }
        }
    }
    state.problem?.let { problem ->
        AlertDialog(
            onDismissRequest = model::dismissProblem,
            text = { Text(nameProblem(problem)) },
            confirmButton = { TextButton(onClick = model::dismissProblem) { Text(stringResource(R.string.mobile_ok)) } },
        )
    }
}

/** The builder's form: Name, Show and Libraries, the rules, the groups, Sort and the live count; each rule is a row opening its own page. */
@Composable
private fun BuilderForm(draft: SmartListDraft, fields: List<WorkField>, state: BuilderUiState, model: SmartListBuilderViewModel) {
    Column(Modifier.wideWidth().fillMaxSize().verticalScroll(rememberScrollState()).imePadding().padding(bottom = space(6))) {
        OutlinedTextField(
            value = draft.name,
            onValueChange = model::setName,
            label = { Text(stringResource(R.string.mobile_list_name)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().padding(horizontal = space(4), vertical = space(2)).testTag("smartListName"),
        )
        SettingRow(stringResource(R.string.mobile_show), "smartListShow", detail = draft.kinds.map { kindName(it) }.joinToString(", ")) { model.show(BuilderPage.Kinds) }
        // A root matching any would OR the libraries with the rules.
        if (draft.match == "all") {
            SettingRow(stringResource(R.string.mobile_smart_libraries), "smartListLibraries", detail = librariesLabel(draft, state)) { model.show(BuilderPage.Libraries) }
        }
        Label(stringResource(if (draft.match == "any") R.string.mobile_smart_match_any else R.string.mobile_smart_match_all))
        RuleRows(draft.rules, fields, state.names, model)
        ActionRow(stringResource(R.string.mobile_smart_add_rule), "addRule") { model.addRule(null) }
        ActionRow(stringResource(R.string.mobile_smart_add_group), "addGroup", model::addGroup)
        for (group in draft.groups) {
            key(group.id) {
                GroupHeader(group.match) { model.setGroupMatch(group.id, it) }
                RuleRows(group.rules, fields, state.names, model)
                ActionRow(stringResource(R.string.mobile_smart_add_rule), "addRule") { model.addRule(group.id) }
                SettingRow(stringResource(R.string.mobile_smart_remove_group), "removeGroup", danger = true) { model.removeGroup(group.id) }
            }
        }
        SortRow(draft, fields, model)
        matchingLine(draft, state)?.let { line -> Box(Modifier.testTag("smartListCount")) { Footnote(line) } }
    }
}

@Composable
private fun librariesLabel(draft: SmartListDraft, state: BuilderUiState): String =
    if (draft.libraries.isEmpty()) stringResource(R.string.mobile_all) else state.libraries.filter { it.id in draft.libraries }.joinToString(", ") { it.name }

/** The live count as a line, null before the first answer or with no kinds. */
@Composable
private fun matchingLine(draft: SmartListDraft, state: BuilderUiState): String? {
    state.matchingProblem?.let { return sentence(it) }
    val total = state.matching ?: return null
    return stringResource(if (total == 1) R.string.mobile_smart_matches else R.string.mobile_smart_match, smartCount(total, draft.kinds))
}

@Composable
private fun ActionRow(label: String, tag: String, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(label, color = MaterialTheme.colorScheme.primary) },
        colors = clear(),
        modifier = Modifier.testTag(tag).clickable(onClick = onClick),
    )
}

/** A section's rules, each opening its page and swiping away. */
@Composable
private fun RuleRows(rules: List<RuleDraft>, fields: List<WorkField>, names: Map<String, String>, model: SmartListBuilderViewModel) {
    for (rule in rules) {
        key(rule.id) {
            Swipeable(onDelete = { model.removeRule(rule.id) }) {
                RuleRow(rule, fields.firstOrNull { it.field == rule.field }, names) { model.show(BuilderPage.EditingRule(rule.id)) }
            }
        }
    }
}

/** A rule's row: "Genre is Comedy", or "Choose" until it's complete. */
@Composable
private fun RuleRow(rule: RuleDraft, field: WorkField?, names: Map<String, String>, onOpen: () -> Unit) {
    val muted = MaterialTheme.colorScheme.onSurfaceVariant
    ListItem(
        headlineContent = {
            if (field != null) {
                Row(horizontalArrangement = Arrangement.spacedBy(space(2))) {
                    Text(fieldName(field), maxLines = 1)
                    Text(opName(rule.op, field.type), color = muted, maxLines = 1)
                    Text(
                        if (rule.isComplete) ruleValues(rule, field, names) else stringResource(R.string.mobile_smart_choose),
                        Modifier.weight(1f),
                        color = if (rule.isComplete) MaterialTheme.colorScheme.onSurface else muted,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        },
        trailingContent = { Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = muted) },
        // Opaque, over the swipe's red.
        colors = ListItemDefaults.colors(containerColor = MaterialTheme.colorScheme.background),
        modifier = Modifier.testTag("rule:${rule.field}").clickable(onClick = onOpen),
    )
}

/** A group's heading, a menu of what the group matches. */
@Composable
private fun GroupHeader(match: String, onMatch: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    Box(Modifier.padding(start = space(4), top = space(4))) {
        Row(Modifier.clickable { open = true }.testTag("groupMatch"), verticalAlignment = Alignment.CenterVertically) {
            Text(
                stringResource(if (match == "all") R.string.mobile_smart_match_all else R.string.mobile_smart_match_any),
                style = MaterialTheme.typography.labelLarge,
                color = MaterialTheme.colorScheme.primary,
            )
            Icon(Icons.Default.ArrowDropDown, null, tint = MaterialTheme.colorScheme.primary)
        }
        DropdownMenu(open, onDismissRequest = { open = false }) {
            for ((value, label) in listOf("any" to R.string.mobile_smart_match_any, "all" to R.string.mobile_smart_match_all)) {
                DropdownMenuItem(
                    text = { Text(stringResource(label)) },
                    onClick = {
                        open = false
                        onMatch(value)
                    },
                    trailingIcon = { if (value == match) Icon(Icons.Default.Check, null) },
                )
            }
        }
    }
}

/** The sorts the kinds allow: each sortable field in both directions, its own first, the saved one if it isn't among them, and Random. */
@Composable
private fun SortRow(draft: SmartListDraft, fields: List<WorkField>, model: SmartListBuilderViewModel) {
    fun direction(key: SortKey) = key.direction ?: fields.firstOrNull { it.field == key.field }?.defaultDirection ?: "asc"
    val current = draft.sort.firstOrNull()
    val currentTag = current?.let { key -> if (key.field == SmartLists.RANDOM_SORT) key.field else "${key.field}:${direction(key)}" } ?: "title:asc"
    val choices = buildList {
        for (field in offeredFields(fields, draft.kinds, current?.field).filter { it.sortable }) {
            val way = field.defaultDirection ?: "asc"
            val other = if (way == "asc") "desc" else "asc"
            add(Triple("${field.field}:$way", sortName(field, way), listOf(SortKey(field.field))))
            add(Triple("${field.field}:$other", sortName(field, other), listOf(SortKey(field.field, other))))
        }
        val saved = current?.let { key -> fields.firstOrNull { it.field == key.field } }
        if (current != null && saved != null && none { it.first == currentTag }) add(0, Triple(currentTag, sortName(saved, direction(current)), draft.sort))
        add(Triple(SmartLists.RANDOM_SORT, stringResource(R.string.mobile_smart_random), listOf(SortKey(SmartLists.RANDOM_SORT))))
    }
    ChoiceRow(stringResource(R.string.mobile_smart_sort), choices.map { Choice(it.first, it.second) }, currentTag, "smartListSort") { tag ->
        model.setSort(choices.firstOrNull { it.first == tag }?.third.orEmpty())
    }
}

/** A pushed page of the builder: back to the form. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SubPage(title: String, model: SmartListBuilderViewModel, content: @Composable () -> Unit) {
    Scaffold(topBar = { PageTopBar(title, { model.show(BuilderPage.Form) }) }) { padding ->
        Column(Modifier.padding(padding).wideWidth().fillMaxSize().verticalScroll(rememberScrollState()).imePadding()) { content() }
    }
}

@Composable
private fun CheckRow(title: String, chosen: Boolean, tag: String, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(title) },
        trailingContent = { if (chosen) Icon(Icons.Default.Check, null, tint = MaterialTheme.colorScheme.primary) },
        colors = clear(),
        modifier = Modifier.testTag(tag).clickable(onClick = onClick),
    )
}

/** Show: the kinds the catalog offers, in its order, any number of them. */
@Composable
private fun KindsPage(draft: SmartListDraft, offered: List<String>, model: SmartListBuilderViewModel) {
    SubPage(stringResource(R.string.mobile_show), model) {
        for (kind in offered) {
            CheckRow(kindName(kind), kind in draft.kinds, "kind:$kind") { model.toggleKind(kind, offered) }
        }
    }
}

/** Libraries: All, or any of the libraries the user sees. */
@Composable
private fun LibrariesPage(draft: SmartListDraft, state: BuilderUiState, model: SmartListBuilderViewModel) {
    SubPage(stringResource(R.string.mobile_smart_libraries), model) {
        CheckRow(stringResource(R.string.mobile_all), draft.libraries.isEmpty(), "library:all", model::clearLibraries)
        for (library in state.libraries) {
            CheckRow(library.name, library.id in draft.libraries, "library:${library.name}") { model.toggleLibrary(library.id) }
        }
    }
}

/** A rule's page: its field, its operator, then values by the operator's arity, chosen from the field's values or typed. */
@Composable
private fun RulePage(rule: RuleDraft, fields: List<WorkField>, names: Map<String, String>, model: SmartListBuilderViewModel) {
    val field = fields.firstOrNull { it.field == rule.field }
    var search by rememberSaveable(rule.id, rule.field) { mutableStateOf("") }
    var found by remember(rule.id, rule.field) { mutableStateOf<List<WorkFieldValue>>(emptyList()) }
    LaunchedEffect(rule.id, rule.field, search) {
        if (field?.valueSource == null) return@LaunchedEffect
        if (search.isNotEmpty()) delay(READ_DELAY_MS)
        model.values(field.field, search.trim())?.let { found = it }
    }
    SubPage(field?.let { fieldName(it) }.orEmpty(), model) {
        ChoiceRow(stringResource(R.string.mobile_smart_field), fields.map { Choice(it.field, fieldName(it)) }, rule.field, "ruleField") { chosen ->
            fields.firstOrNull { it.field == chosen }?.let { picked -> if (picked.field != rule.field) model.changeRule(rule.id) { it.withField(picked) } }
        }
        if (field != null) {
            ChoiceRow(stringResource(R.string.mobile_smart_condition), field.ops.map { Choice(it.op, opName(it.op, field.type)) }, rule.op, "ruleOp") { chosen ->
                field.ops.firstOrNull { it.op == chosen }?.let { picked -> model.changeRule(rule.id) { it.withOp(picked) } }
            }
            if (rule.arity != 0) RuleValues(rule, field, names, search, found, { search = it }, model)
        }
    }
}

@Composable
private fun RuleValues(
    rule: RuleDraft,
    field: WorkField,
    names: Map<String, String>,
    search: String,
    found: List<WorkFieldValue>,
    onSearch: (String) -> Unit,
    model: SmartListBuilderViewModel,
) {
    when {
        SmartLists.isPeriod(rule.op) -> PeriodValue(rule, model)
        field.valueSource != null -> ValueChooser(rule, field, names, search, found, onSearch, model)
        else -> {
            for (index in rule.values.indices) {
                key(index) { TypedValue(rule, field, index, model) }
            }
            if (field.type == "date") Footnote(stringResource(R.string.mobile_smart_date_note))
            if (field.unit == "ms") Footnote(stringResource(R.string.mobile_smart_minutes_note))
        }
    }
}

/** A typed value; a duration is typed in minutes and held in milliseconds. */
@Composable
private fun TypedValue(rule: RuleDraft, field: WorkField, index: Int, model: SmartListBuilderViewModel) {
    val minutes = field.unit == "ms"
    // Kept as typed: the rule's value is trimmed, and a duration's is converted.
    var typed by rememberSaveable(rule.id, rule.field, rule.op, index) {
        mutableStateOf(rule.values[index].let { if (minutes && it.isNotEmpty()) minutesOf(it) else it })
    }
    OutlinedTextField(
        value = typed,
        onValueChange = { text ->
            typed = text
            val value = text.trim().let { if (minutes) it.toDoubleOrNull()?.let { number -> (number * 60_000).toInt().toString() }.orEmpty() else it }
            model.changeRule(rule.id) { it.copy(values = it.values.toMutableList().also { values -> values[index] = value }) }
        },
        label = {
            Text(
                when {
                    rule.arity == 2 -> stringResource(if (index == 0) R.string.mobile_smart_from else R.string.mobile_smart_until)
                    field.type == "date" -> stringResource(R.string.mobile_smart_date_format)
                    else -> stringResource(R.string.mobile_smart_value)
                },
            )
        },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = if (field.type == "number") KeyboardType.Decimal else KeyboardType.Text),
        modifier = Modifier.fillMaxWidth().padding(horizontal = space(4), vertical = space(2)).testTag("ruleValue$index"),
    )
}

/** A period: a number of days, weeks, months or years, held as P30D. */
@Composable
private fun PeriodValue(rule: RuleDraft, model: SmartListBuilderViewModel) {
    var unit by rememberSaveable(rule.id) { mutableStateOf(SmartLists.periodUnit(rule.values.firstOrNull())) }
    OutlinedTextField(
        value = SmartLists.periodAmount(rule.values.firstOrNull().orEmpty()),
        onValueChange = { text ->
            val digits = text.filter(Char::isDigit)
            model.changeRule(rule.id) { it.withValues(listOf(SmartLists.period(digits, unit))) }
        },
        label = { Text(stringResource(R.string.mobile_smart_number)) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = Modifier.fillMaxWidth().padding(horizontal = space(4), vertical = space(2)).testTag("ruleValue0"),
    )
    val units = listOf(
        Choice("D", stringResource(R.string.mobile_smart_unit_days)),
        Choice("W", stringResource(R.string.mobile_smart_unit_weeks)),
        Choice("M", stringResource(R.string.mobile_smart_unit_months)),
        Choice("Y", stringResource(R.string.mobile_smart_unit_years)),
    )
    ChoiceRow(stringResource(R.string.mobile_smart_unit), units, unit, "ruleUnit") { picked ->
        unit = picked
        model.changeRule(rule.id) { current ->
            val value = current.values.firstOrNull().orEmpty()
            if (value.isEmpty()) current else current.withValues(listOf(SmartLists.period(SmartLists.periodAmount(value), picked)))
        }
    }
}

/** A rule's values chosen from the field's: its inline choices, or for a field with many, the ones chosen and what a search finds. */
@Composable
private fun ValueChooser(
    rule: RuleDraft,
    field: WorkField,
    names: Map<String, String>,
    search: String,
    found: List<WorkFieldValue>,
    onSearch: (String) -> Unit,
    model: SmartListBuilderViewModel,
) {
    val searches = field.valueSource == "search"
    if (searches) {
        OutlinedTextField(
            value = search,
            onValueChange = onSearch,
            label = { Text(stringResource(R.string.mobile_smart_search)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().padding(horizontal = space(4), vertical = space(2)).testTag("valueSearch"),
        )
        if (rule.values.isNotEmpty()) {
            Label(stringResource(R.string.mobile_smart_chosen))
            for (value in rule.values) {
                CheckRow(names[SmartLists.nameKey(field.field, value)] ?: value, true, "chosen:$value") {
                    model.changeRule(rule.id) { it.copy(values = it.values - value) }
                }
            }
        }
    }
    for (value in found.filter { !searches || it.value !in rule.values }) {
        val chosen = value.value in rule.values
        CheckRow(value.name, chosen, "value:${value.name}") {
            model.noteName(field.field, value)
            model.changeRule(rule.id) { it.copy(values = if (chosen) it.values - value.value else it.values + value.value) }
        }
    }
}
