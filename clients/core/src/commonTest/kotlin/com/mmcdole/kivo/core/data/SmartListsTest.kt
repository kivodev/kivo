package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.Rule
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.SortKey
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldOp
import com.mmcdole.kivo.api.models.WorkQuery
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SmartListsTest {
    private val ops = listOf(WorkFieldOp("anyOf", -1), WorkFieldOp("noneOf", -1), WorkFieldOp("between", 2), WorkFieldOp("within", 1), WorkFieldOp("notWithin", 1), WorkFieldOp("isSet", 0))

    private fun field(name: String, type: String = "choice", valueSource: String? = "inline", sortable: Boolean = false) = WorkField(
        field = name,
        name = name,
        type = type,
        ops = ops,
        kinds = listOf("movie", "series"),
        sortable = sortable,
        valueSource = valueSource,
    )

    private val fields = listOf(field("library"), field("genre"), field("year", "number", null), field("addedAt", "date", null, sortable = true), field("playState"), field("resolution"), field("dynamicRange"))

    private fun rule(field: String, op: String, type: String, arity: Int, vararg values: String) = RuleDraft(field, op, arity, type, values.toList())

    @Test
    fun aNewDraftStartsOnMoviesWithoutRules() {
        val draft = SmartLists.draft("", null, fields)
        assertEquals(listOf("movie"), draft.kinds)
        assertEquals("all", draft.match)
        assertNull(draft.query.where)
    }

    @Test
    fun aSavedQueryComesBackWhole() {
        val query = WorkQuery(
            kinds = listOf("movie", "series"),
            where = RuleGroup(
                "all",
                rules = listOf(Rule("library", "anyOf", listOf("l1")), Rule("genre", "anyOf", listOf("Comedy"))),
                groups = listOf(RuleGroup("any", rules = listOf(Rule("year", "between", listOf("1990", "1999"))))),
            ),
            sort = listOf(SortKey("addedAt", "asc")),
            maxWorks = 50,
            collapse = "series",
        )
        val draft = SmartLists.draft("90s", query, fields)
        assertEquals(setOf("l1"), draft.libraries)
        assertEquals(listOf("genre"), draft.rules.map { it.field })
        assertEquals(2, draft.groups.single().rules.single().arity)
        assertEquals("number", draft.groups.single().rules.single().type)
        assertEquals(query.copy(version = null), draft.query)
    }

    @Test
    fun aRootMatchingAnyKeepsItsLibraryRuleAmongTheRules() {
        val query = WorkQuery(listOf("movie"), where = RuleGroup("any", rules = listOf(Rule("library", "anyOf", listOf("l1")), Rule("genre", "anyOf", listOf("Comedy")))))
        val draft = SmartLists.draft("", query, fields)
        assertEquals("any", draft.match)
        assertTrue(draft.libraries.isEmpty())
        assertEquals(listOf("library", "genre"), draft.rules.map { it.field })
        assertEquals(query.where, draft.query.where)
    }

    @Test
    fun allLibrariesSavesNoRule() {
        val draft = SmartLists.draft("All", null, fields).copy(rules = listOf(rule("genre", "anyOf", "choice", -1, "Comedy")))
        assertEquals(listOf("genre"), draft.query.where?.rules?.map { it.field })
    }

    @Test
    fun anUnfinishedRuleIsLeftOutOfTheQueryAndBlocksSaving() {
        val finished = rule("genre", "anyOf", "choice", -1, "Comedy")
        val unfinished = rule("year", "between", "number", 2, "1990", "")
        val draft = SmartListDraft("Mine", listOf("movie"), rules = listOf(finished, unfinished))
        assertEquals(listOf(finished.toRule()), draft.query.where?.rules)
        assertFalse(draft.isReady)
        assertTrue(draft.copy(rules = listOf(finished)).isReady)
    }

    @Test
    fun aListNeedsANameAndAKindToSave() {
        assertFalse(SmartListDraft(" ", listOf("movie")).isReady)
        assertFalse(SmartListDraft("Mine", emptyList()).isReady)
    }

    @Test
    fun aGroupWithoutCompleteRulesIsLeftOut() {
        val draft = SmartListDraft("Mine", listOf("movie"), groups = listOf(GroupDraft("any", rules = listOf(rule("genre", "anyOf", "choice", -1)))))
        assertNull(draft.query.where)
    }

    @Test
    fun aNumberMustBeFiniteAndWellFormed() {
        fun year(value: String) = rule("year", "anyOf", "number", 1, value).isComplete
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
        fun date(value: String) = rule("addedAt", "gte", "date", 1, value).isComplete
        assertTrue(date("2024-01-31"))
        assertFalse(date("2024-1-31"))
        assertFalse(date("soon"))
    }

    @Test
    fun aPeriodIsPThenOneToFourDigitsAndAUnit() {
        fun within(value: String) = rule("addedAt", "within", "date", 1, value).isComplete
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
        assertTrue(rule("genre", "isSet", "choice", 0).isComplete)
    }

    @Test
    fun aMalformedValueBlocksSaving() {
        val draft = SmartListDraft("Mine", listOf("movie"), rules = listOf(rule("year", "anyOf", "number", 1, "abc")))
        assertFalse(draft.isReady)
        assertNull(draft.query.where)
    }

    @Test
    fun aBlankRuleHasTheFieldsFirstOperatorAndAnEmptySlotForExactValues() {
        assertEquals(listOf("year", "anyOf"), SmartLists.blankRule(field("year", "number", null))?.let { listOf(it.field, it.op) })
        val between = field("year", "number", null).copy(ops = listOf(WorkFieldOp("between", 2)))
        assertEquals(listOf("", ""), SmartLists.blankRule(between)?.values)
        assertNull(SmartLists.blankRule(field("year").copy(ops = emptyList())))
    }

    @Test
    fun aBlankGroupMatchesAnyWithOneBlankRule() {
        val group = SmartLists.blankGroup(field("genre"))
        assertEquals("any", group.match)
        assertEquals(1, group.rules.size)
    }

    @Test
    fun aPeriodStaysInItsValueAndANewOperatorKeepsIt() {
        val period = rule("addedAt", "within", "date", 1, "P30D")
        assertEquals(listOf("P30D"), period.withOp(WorkFieldOp("notWithin", 1)).values)
        assertEquals(listOf(""), period.withOp(WorkFieldOp("eq", 1)).values)
    }

    @Test
    fun aNewFieldKeepsAnOperatorItHasAndStartsTheValuesOver() {
        val before = rule("genre", "noneOf", "choice", -1, "Comedy")
        val changed = before.withField(field("resolution"))
        assertEquals("noneOf", changed.op)
        assertTrue(changed.values.isEmpty())
        val other = field("rating", "number", null).copy(ops = listOf(WorkFieldOp("gte", 1)))
        assertEquals("gte" to listOf(""), before.withField(other).let { it.op to it.values })
        assertEquals("number", before.withField(other).type)
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
        assertEquals(SmartListRefusal.NewerVersion, SmartLists.refusal(WorkQuery(listOf("movie"), version = 2), fields))
        assertNull(SmartLists.refusal(WorkQuery(listOf("movie"), version = 1), fields))
    }

    @Test
    fun aFieldMissingFromTheCatalogIsRefused() {
        val nested = WorkQuery(listOf("movie"), where = RuleGroup("all", groups = listOf(RuleGroup("any", rules = listOf(Rule("mood", "anyOf", listOf("Dark")))))))
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(nested, fields))
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(WorkQuery(listOf("movie"), sort = listOf(SortKey("mood"))), fields))
    }

    @Test
    fun anOperatorTheFieldDoesntHaveIsRefused() {
        val query = WorkQuery(listOf("movie"), where = RuleGroup("all", rules = listOf(Rule("genre", "soon", listOf("x")))))
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(query, fields))
    }

    @Test
    fun aSortOnAFieldThatIsntSortableIsRefused() {
        assertEquals(SmartListRefusal.UnknownField, SmartLists.refusal(WorkQuery(listOf("movie"), sort = listOf(SortKey("genre"))), fields))
    }

    @Test
    fun randomIsASortTheCatalogNeedNotList() {
        assertNull(SmartLists.refusal(WorkQuery(listOf("movie"), sort = listOf(SortKey(SmartLists.RANDOM_SORT))), fields))
    }

    @Test
    fun aFilteredGridBecomesAQueryOnItsLibrary() {
        val grid = GridQuery("l1", sort = "runtime", played = "unplayed", resolutions = listOf("720p", "4k"), dynamicRanges = listOf("hdr"))
        val query = SmartLists.fromGrid(grid, "movies")
        assertEquals(listOf("movie"), query.kinds)
        assertEquals(
            listOf(
                Rule("library", "anyOf", listOf("l1")),
                Rule("playState", "anyOf", listOf("unplayed")),
                Rule("resolution", "anyOf", listOf("4k", "720p")),
                Rule("dynamicRange", "anyOf", listOf("hdr")),
            ),
            query.where?.rules,
        )
        assertEquals(listOf(SortKey("runtimeMs")), query.sort)
    }

    @Test
    fun aGridsSortAndKindMapToTheQuerys() {
        assertEquals(listOf(SortKey("title")), SmartLists.fromGrid(GridQuery("l"), "shows").sort)
        assertEquals(listOf("series"), SmartLists.fromGrid(GridQuery("l"), "shows").kinds)
        assertNull(SmartLists.fromGrid(GridQuery("l", sort = "author"), "audiobooks").sort)
        assertEquals(listOf("book"), SmartLists.fromGrid(GridQuery("l", sort = "author"), "audiobooks").kinds)
        assertEquals(listOf("video"), SmartLists.fromGrid(GridQuery("l"), "videos").kinds)
    }

    @Test
    fun theValuesToNameAreThoseOfFieldsThatNameThem() {
        val query = WorkQuery(
            listOf("movie"),
            where = RuleGroup(
                "all",
                rules = listOf(Rule("genre", "anyOf", listOf("Comedy")), Rule("year", "anyOf", listOf("1999"))),
                groups = listOf(RuleGroup("any", rules = listOf(Rule("genre", "anyOf", listOf("Drama"))))),
            ),
        )
        assertEquals(mapOf("genre" to setOf("Comedy", "Drama")), SmartLists.valuesToName(query, fields))
    }

    @Test
    fun aDraftOnlyEverChangesThroughACopy() {
        val rule = SmartLists.blankRule(field("genre"))!!
        val group = SmartLists.blankGroup(field("year", "number", null))
        val draft = SmartListDraft("Mine", listOf("movie")).addingRule(rule, null).addingGroup(group).withGroupMatch(group.id, "all")
        assertEquals(listOf(rule), draft.rules)
        assertEquals("all", draft.groups.single().match)
        val changed = draft.changingRule(rule.id) { it.withValues(listOf("Comedy")) }.removingRule(group.rules.single().id)
        assertEquals(listOf("Comedy"), changed.rules.single().values)
        assertTrue(changed.groups.single().rules.isEmpty())
        assertTrue(changed.removingGroup(group.id).groups.isEmpty())
        assertEquals(rule.id, changed.rule(rule.id)?.id)
    }
}
