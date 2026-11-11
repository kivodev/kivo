package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.api.models.Rule
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.RuleGroupMatch
import com.mmcdole.kivo.api.models.SortDirection
import com.mmcdole.kivo.api.models.SortKey
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldOp
import com.mmcdole.kivo.api.models.WorkFieldType
import com.mmcdole.kivo.api.models.WorkFieldValueSource
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.core.kivoJson
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SmartListsTest {
    private val ops = listOf(WorkFieldOp("anyOf", -1), WorkFieldOp("noneOf", -1), WorkFieldOp("between", 2), WorkFieldOp("within", 1), WorkFieldOp("notWithin", 1), WorkFieldOp("isSet", 0))

    private fun field(name: String, type: WorkFieldType = WorkFieldType.Choice, valueSource: WorkFieldValueSource? = WorkFieldValueSource.Inline, sortable: Boolean = false) = WorkField(
        field = name,
        name = name,
        type = type,
        ops = ops,
        kinds = listOf(WorkKind.Movie, WorkKind.Series),
        sortable = sortable,
        valueSource = valueSource,
    )

    private val fields = listOf(field("library"), field("genre"), field("year", WorkFieldType.Number, null), field("addedAt", WorkFieldType.Date, null, sortable = true), field("playState"), field("resolution"), field("dynamicRange"))

    private fun rule(field: String, op: String, type: WorkFieldType, arity: Int, vararg values: String) = RuleDraft(field, op, arity, type, values.toList())

    @Test
    fun aNewDraftStartsOnMoviesWithoutRules() {
        val draft = SmartLists.draft("", null, fields)
        assertEquals(listOf(WorkKind.Movie), draft.kinds)
        assertEquals(RuleGroupMatch.All, draft.match)
        assertNull(draft.query.where)
    }

    @Test
    fun aSavedQueryComesBackWhole() {
        val query = WorkQuery(
            kinds = listOf(WorkKind.Movie, WorkKind.Series),
            where = RuleGroup(
                RuleGroupMatch.All,
                rules = listOf(Rule("library", "anyOf", listOf("l1")), Rule("genre", "anyOf", listOf("Comedy"))),
                groups = listOf(RuleGroup(RuleGroupMatch.Any, rules = listOf(Rule("year", "between", listOf("1990", "1999"))))),
            ),
            sort = listOf(SortKey("addedAt", SortDirection.Asc)),
            maxWorks = 50,
        )
        val draft = SmartLists.draft("90s", query, fields)
        assertEquals(setOf("l1"), draft.libraries)
        assertEquals(listOf("genre"), draft.rules.map { it.field })
        assertEquals(2, draft.groups.single().rules.single().arity)
        assertEquals(WorkFieldType.Number, draft.groups.single().rules.single().type)
        assertEquals(query.copy(version = null), draft.query)
    }

    @Test
    fun aRootMatchingAnyKeepsItsLibraryRuleAmongTheRules() {
        val query = WorkQuery(listOf(WorkKind.Movie), where = RuleGroup(RuleGroupMatch.Any, rules = listOf(Rule("library", "anyOf", listOf("l1")), Rule("genre", "anyOf", listOf("Comedy")))))
        val draft = SmartLists.draft("", query, fields)
        assertEquals(RuleGroupMatch.Any, draft.match)
        assertTrue(draft.libraries.isEmpty())
        assertEquals(listOf("library", "genre"), draft.rules.map { it.field })
        assertEquals(query.where, draft.query.where)
    }

    @Test
    fun allLibrariesSavesNoRule() {
        val draft = SmartLists.draft("All", null, fields).copy(rules = listOf(rule("genre", "anyOf", WorkFieldType.Choice, -1, "Comedy")))
        assertEquals(listOf("genre"), draft.query.where?.rules?.map { it.field })
    }

    @Test
    fun anUnfinishedRuleIsLeftOutOfTheQueryAndBlocksSaving() {
        val finished = rule("genre", "anyOf", WorkFieldType.Choice, -1, "Comedy")
        val unfinished = rule("year", "between", WorkFieldType.Number, 2, "1990", "")
        val draft = SmartListDraft("Mine", listOf(WorkKind.Movie), rules = listOf(finished, unfinished))
        assertEquals(listOf(finished.toRule()), draft.query.where?.rules)
        assertFalse(draft.isReady)
        assertTrue(draft.copy(rules = listOf(finished)).isReady)
    }

    @Test
    fun aListNeedsANameAndAKindToSave() {
        assertFalse(SmartListDraft(" ", listOf(WorkKind.Movie)).isReady)
        assertFalse(SmartListDraft("Mine", emptyList()).isReady)
    }

    @Test
    fun aGroupWithoutCompleteRulesIsLeftOut() {
        val draft = SmartListDraft("Mine", listOf(WorkKind.Movie), groups = listOf(GroupDraft(RuleGroupMatch.Any, rules = listOf(rule("genre", "anyOf", WorkFieldType.Choice, -1)))))
        assertNull(draft.query.where)
    }

    @Test
    fun aNumberMustBeFiniteAndWellFormed() {
        fun year(value: String) = rule("year", "anyOf", WorkFieldType.Number, 1, value).isComplete
        assertTrue(year("1999"))
        assertTrue(year("6.5"))
        assertTrue(year(" 7 "))
        assertTrue(year("1e3"))
        assertFalse(year(""))
        assertFalse(year("nineteen"))
        assertFalse(year("1f"))
        assertFalse(year("1e999"))
    }

    @Test
    fun aDateIsWrittenInFullWithNumbers() {
        fun date(value: String) = rule("addedAt", "gte", WorkFieldType.Date, 1, value).isComplete
        assertTrue(date("2024-01-31"))
        assertFalse(date("2024-1-31"))
        assertFalse(date("soon"))
    }

    @Test
    fun aPeriodIsPThenOneToFourDigitsAndAUnit() {
        fun within(value: String) = rule("addedAt", "within", WorkFieldType.Date, 1, value).isComplete
        assertTrue(within("P1D"))
        assertTrue(within("P1234Y"))
        assertFalse(within("P0D"))
        assertFalse(within("P12345D"))
        assertFalse(within("P5X"))
        assertFalse(within("30D"))
        assertFalse(within(""))
    }

    @Test
    fun aRuleWithoutValuesIsComplete() {
        assertTrue(rule("genre", "isSet", WorkFieldType.Choice, 0).isComplete)
    }

    @Test
    fun aMalformedValueBlocksSaving() {
        val draft = SmartListDraft("Mine", listOf(WorkKind.Movie), rules = listOf(rule("year", "anyOf", WorkFieldType.Number, 1, "abc")))
        assertFalse(draft.isReady)
        assertNull(draft.query.where)
    }

    @Test
    fun aBlankRuleHasTheFieldsFirstOperatorAndAnEmptySlotForExactValues() {
        assertEquals(listOf("year", "anyOf"), SmartLists.blankRule(field("year", WorkFieldType.Number, null))?.let { listOf(it.field, it.op) })
        val between = field("year", WorkFieldType.Number, null).copy(ops = listOf(WorkFieldOp("between", 2)))
        assertEquals(listOf("", ""), SmartLists.blankRule(between)?.values)
        assertNull(SmartLists.blankRule(field("year").copy(ops = emptyList())))
    }

    @Test
    fun aBlankGroupMatchesAnyWithOneBlankRule() {
        val group = SmartLists.blankGroup(field("genre"))
        assertEquals(RuleGroupMatch.Any, group.match)
        assertEquals(1, group.rules.size)
    }

    @Test
    fun aPeriodStaysInItsValueAndANewOperatorKeepsIt() {
        val period = rule("addedAt", "within", WorkFieldType.Date, 1, "P30D")
        assertEquals(listOf("P30D"), period.withOp(WorkFieldOp("notWithin", 1)).values)
        assertEquals(listOf(""), period.withOp(WorkFieldOp("eq", 1)).values)
    }

    @Test
    fun aNewFieldKeepsAnOperatorItHasAndStartsTheValuesOver() {
        val before = rule("genre", "noneOf", WorkFieldType.Choice, -1, "Comedy")
        val changed = before.withField(field("resolution"))
        assertEquals("noneOf", changed.op)
        assertTrue(changed.values.isEmpty())
        val other = field("rating", WorkFieldType.Number, null).copy(ops = listOf(WorkFieldOp("gte", 1)))
        assertEquals("gte" to listOf(""), before.withField(other).let { it.op to it.values })
        assertEquals(WorkFieldType.Number, before.withField(other).type)
    }

    @Test
    fun aPeriodValueHasAnAmountAndAUnit() {
        assertEquals("30", SmartLists.periodAmount("P30D"))
        assertEquals("", SmartLists.periodAmount(""))
        assertEquals("W", SmartLists.periodUnit("P30W"))
        assertEquals("D", SmartLists.periodUnit(""))
        assertEquals("D", SmartLists.periodUnit(null))
        assertEquals("P4M", SmartLists.period("4", "M"))
        assertEquals("", SmartLists.period("", "M"))
    }

    @Test
    fun aNewerVersionIsRefused() {
        assertEquals(SmartListRefusal.NewerVersion, SmartLists.refusal(WorkQuery(listOf(WorkKind.Movie), version = 2), fields))
        assertNull(SmartLists.refusal(WorkQuery(listOf(WorkKind.Movie), version = 1), fields))
    }

    @Test
    fun aFieldMissingFromTheCatalogIsRefused() {
        val nested = WorkQuery(listOf(WorkKind.Movie), where = RuleGroup(RuleGroupMatch.All, groups = listOf(RuleGroup(RuleGroupMatch.Any, rules = listOf(Rule("mood", "anyOf", listOf("Dark")))))))
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(nested, fields))
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(WorkQuery(listOf(WorkKind.Movie), sort = listOf(SortKey("mood"))), fields))
    }

    @Test
    fun anOperatorTheFieldDoesntHaveIsRefused() {
        val query = WorkQuery(listOf(WorkKind.Movie), where = RuleGroup(RuleGroupMatch.All, rules = listOf(Rule("genre", "soon", listOf("x")))))
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(query, fields))
    }

    @Test
    fun aSortOnAFieldThatIsntSortableIsRefused() {
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(WorkQuery(listOf(WorkKind.Movie), sort = listOf(SortKey("genre"))), fields))
    }

    @Test
    fun aKindMatchOrDirectionALaterServerAddsIsRefused() {
        val saved = kivoJson.decodeFromString(
            WorkQuery.serializer(),
            """{"kinds":["movie","podcast"],"where":{"match":"all","rules":[{"field":"genre","op":"anyOf","values":["Drama"]}]}}""",
        )
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(saved, fields), "saving it would lose the kind")
        val nested = WorkQuery(listOf(WorkKind.Movie), where = RuleGroup(RuleGroupMatch.All, groups = listOf(RuleGroup(RuleGroupMatch.Unknown))))
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(nested, fields))
        val sorted = WorkQuery(listOf(WorkKind.Movie), sort = listOf(SortKey("addedAt", SortDirection.Unknown)))
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(sorted, fields))
        assertNull(SmartLists.refusal(WorkQuery(listOf(WorkKind.Movie), sort = listOf(SortKey("addedAt", SortDirection.Desc))), fields))
    }

    @Test
    fun aSavedQueryGoesBackToTheServerAsItCame() {
        val text = """{"kinds":["movie","series"],"where":{"match":"all","rules":[{"field":"genre","op":"anyOf","values":["Drama"]}],""" +
            """"groups":[{"match":"any","rules":[{"field":"year","op":"between","values":["1990","1999"]}]}]},"sort":[{"field":"addedAt","direction":"desc"}]}"""
        val saved = kivoJson.decodeFromString(WorkQuery.serializer(), text)
        assertNull(SmartLists.refusal(saved, fields + field("year", WorkFieldType.Number, null)))
        assertEquals(text, kivoJson.encodeToString(WorkQuery.serializer(), SmartLists.draft("Mine", saved, fields).query))
    }

    @Test
    fun randomIsASortTheCatalogNeedNotList() {
        assertNull(SmartLists.refusal(WorkQuery(listOf(WorkKind.Movie), sort = listOf(SortKey(SmartLists.RANDOM_SORT))), fields))
    }

    @Test
    fun aFilteredGridBecomesAQueryOnItsLibrary() {
        val grid = GridQuery(
            "l1", sort = GridSort.Runtime, played = WorkPlayState.Unplayed, genres = listOf("Drama"), yearFrom = 1990, yearTo = 1999,
            contentRatings = listOf("R"), resolutions = listOf(Resolution.P720, Resolution.FourK), dynamicRanges = listOf(DynamicRange.Hdr),
        )
        val query = SmartLists.fromGrid(grid, LibraryKind.Movies)
        assertEquals(listOf(WorkKind.Movie), query.kinds)
        assertEquals(
            listOf(
                Rule("library", "anyOf", listOf("l1")),
                Rule("playState", "anyOf", listOf("unplayed")),
                Rule("genre", "anyOf", listOf("Drama")),
                Rule("year", "between", listOf("1990", "1999")),
                Rule("contentRating", "anyOf", listOf("R")),
                Rule("resolution", "anyOf", listOf("4k", "720p")),
                Rule("dynamicRange", "anyOf", listOf("hdr")),
            ),
            query.where?.rules,
        )
        assertEquals(listOf(SortKey("runtimeMs")), query.sort)
        assertEquals(query.where?.rules?.drop(1), SmartLists.gridQuery(grid, LibraryKind.Movies).where?.rules, "the grid's own read has no library rule")
        assertEquals(query.copy(where = null), SmartLists.gridQuery(grid, LibraryKind.Movies).copy(where = null))
    }

    @Test
    fun aGridWithoutFiltersIsAQueryWithoutRules() {
        assertNull(SmartLists.gridQuery(GridQuery("l1"), LibraryKind.Movies).where)
        assertEquals(listOf(Rule("year", "gte", listOf("1990"))), SmartLists.gridQuery(GridQuery("l1", yearFrom = 1990), LibraryKind.Movies).where?.rules)
        assertEquals(listOf(Rule("year", "lte", listOf("1999"))), SmartLists.gridQuery(GridQuery("l1", yearTo = 1999), LibraryKind.Movies).where?.rules)
    }

    @Test
    fun aGridsSortAndKindMapToTheQuerys() {
        assertEquals(listOf(SortKey("title")), SmartLists.fromGrid(GridQuery("l"), LibraryKind.Shows).sort)
        assertEquals(listOf(WorkKind.Series), SmartLists.fromGrid(GridQuery("l"), LibraryKind.Shows).kinds)
        assertEquals(listOf(SortKey("author")), SmartLists.fromGrid(GridQuery("l", sort = GridSort.Author), LibraryKind.Audiobooks).sort)
        assertEquals(listOf(WorkKind.Book), SmartLists.fromGrid(GridQuery("l", sort = GridSort.Author), LibraryKind.Audiobooks).kinds)
        assertEquals(listOf(WorkKind.Video), SmartLists.fromGrid(GridQuery("l"), LibraryKind.Videos).kinds)
    }

    @Test
    fun theValuesToNameAreThoseOfFieldsThatNameThem() {
        val query = WorkQuery(
            listOf(WorkKind.Movie),
            where = RuleGroup(
                RuleGroupMatch.All,
                rules = listOf(Rule("genre", "anyOf", listOf("Comedy")), Rule("year", "anyOf", listOf("1999"))),
                groups = listOf(RuleGroup(RuleGroupMatch.Any, rules = listOf(Rule("genre", "anyOf", listOf("Drama"))))),
            ),
        )
        assertEquals(mapOf("genre" to setOf("Comedy", "Drama")), SmartLists.valuesToName(query, fields))
    }

    @Test
    fun aDraftOnlyEverChangesThroughACopy() {
        val rule = SmartLists.blankRule(field("genre"))!!
        val group = SmartLists.blankGroup(field("year", WorkFieldType.Number, null))
        val draft = SmartListDraft("Mine", listOf(WorkKind.Movie)).addingRule(rule, null).addingGroup(group).withGroupMatch(group.id, RuleGroupMatch.All)
        assertEquals(listOf(rule), draft.rules)
        assertEquals(RuleGroupMatch.All, draft.groups.single().match)
        val changed = draft.changingRule(rule.id) { it.withValues(listOf("Comedy")) }.removingRule(group.rules.single().id)
        assertEquals(listOf("Comedy"), changed.rules.single().values)
        assertTrue(changed.groups.single().rules.isEmpty())
        assertTrue(changed.removingGroup(group.id).groups.isEmpty())
        assertEquals(rule.id, changed.rule(rule.id)?.id)
    }
}
