package com.mmcdole.kivo.android.mobile.lists

import com.mmcdole.kivo.api.models.Rule
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.SortKey
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldOp
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.core.data.GridQuery
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SmartListDraftTest {
    private fun field(name: String, type: String = "choice", valueSource: String? = "inline") = WorkField(
        field = name,
        name = name,
        type = type,
        ops = listOf(WorkFieldOp("anyOf", -1), WorkFieldOp("noneOf", -1), WorkFieldOp("between", 2), WorkFieldOp("within", 1), WorkFieldOp("notWithin", 1), WorkFieldOp("isSet", 0)),
        kinds = listOf("movie", "series"),
        sortable = false,
        valueSource = valueSource,
    )

    private val fields = listOf(field("library"), field("genre"), field("year", "number", null), field("addedAt", "date", null), field("playState"), field("resolution"), field("dynamicRange"))

    @Test
    fun aNewDraftStartsOnMoviesWithoutRules() {
        val draft = smartListDraft("", null, fields)
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
        val draft = smartListDraft("90s", query, fields)
        assertEquals(setOf("l1"), draft.libraries)
        assertEquals(listOf("genre"), draft.rules.map { it.field })
        assertEquals(2, draft.groups.single().rules.single().arity)
        assertEquals(query.copy(version = null), draft.query)
    }

    @Test
    fun aRootMatchingAnyKeepsItsLibraryRuleAmongTheRules() {
        val query = WorkQuery(listOf("movie"), where = RuleGroup("any", rules = listOf(Rule("library", "anyOf", listOf("l1")), Rule("genre", "anyOf", listOf("Comedy")))))
        val draft = smartListDraft("", query, fields)
        assertEquals("any", draft.match)
        assertTrue(draft.libraries.isEmpty())
        assertEquals(listOf("library", "genre"), draft.rules.map { it.field })
        assertEquals(query.where, draft.query.where)
    }

    @Test
    fun allLibrariesSavesNoRule() {
        val draft = smartListDraft("All", null, fields).copy(rules = listOf(RuleDraft("genre", "anyOf", -1, listOf("Comedy"))))
        assertEquals(listOf("genre"), draft.query.where?.rules?.map { it.field })
    }

    @Test
    fun anUnfinishedRuleIsLeftOutOfTheQueryAndBlocksSaving() {
        val finished = RuleDraft("genre", "anyOf", -1, listOf("Comedy"))
        val unfinished = RuleDraft("year", "between", 2, listOf("1990", ""))
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
        val draft = SmartListDraft("Mine", listOf("movie"), groups = listOf(GroupDraft(rules = listOf(RuleDraft("genre", "anyOf", -1, emptyList())))))
        assertNull(draft.query.where)
    }

    @Test
    fun aPeriodStaysInItsValueAndANewOperatorKeepsIt() {
        val rule = RuleDraft("addedAt", "within", 1, listOf("P30D"))
        assertEquals(listOf("P30D"), withOp(rule, WorkFieldOp("notWithin", 1)).values)
        assertEquals(listOf(""), withOp(rule, WorkFieldOp("eq", 1)).values)
    }

    @Test
    fun aNewFieldKeepsAnOperatorItHasAndStartsTheValuesOver() {
        val rule = RuleDraft("genre", "noneOf", -1, listOf("Comedy"))
        val changed = withField(rule, field("resolution"))
        assertEquals("noneOf", changed.op)
        assertTrue(changed.values.isEmpty())
        val other = field("rating", "number", null).copy(ops = listOf(WorkFieldOp("gte", 1)))
        assertEquals("gte" to listOf(""), withField(rule, other).let { it.op to it.values })
    }

    @Test
    fun libraryIsNeverOfferedAndAKeptFieldAlwaysIs() {
        val series = field("bookSeries").copy(kinds = listOf("book"))
        val all = fields + series
        assertFalse(offeredFields(all, listOf("movie"), null).any { it.field == "library" })
        assertFalse(offeredFields(all, listOf("movie"), null).any { it.field == "bookSeries" })
        assertTrue(offeredFields(all, listOf("movie"), "bookSeries").any { it.field == "bookSeries" })
    }

    @Test
    fun theSortsAreTheKindsOwnAndTheOneAlreadyChosen() {
        val rating = field("rating", "number", null).copy(sortable = true)
        val pages = field("pages", "number", null).copy(kinds = listOf("book"), sortable = true)
        val sorts = { kept: String? -> offeredFields(listOf(rating, pages), listOf("movie"), kept).filter { it.sortable }.map { it.field } }
        assertEquals(listOf("rating"), sorts(null))
        assertEquals(listOf("rating", "pages"), sorts("pages"))
    }

    @Test
    fun aNewerVersionIsRefused() {
        assertTrue(refusesEditing(WorkQuery(listOf("movie"), version = 2), fields))
        assertFalse(refusesEditing(WorkQuery(listOf("movie"), version = 1), fields))
    }

    @Test
    fun aFieldMissingFromTheCatalogIsRefused() {
        val nested = WorkQuery(listOf("movie"), where = RuleGroup("all", groups = listOf(RuleGroup("any", rules = listOf(Rule("mood", "anyOf", listOf("Dark")))))))
        assertTrue(refusesEditing(nested, fields))
        assertTrue(refusesEditing(WorkQuery(listOf("movie"), sort = listOf(SortKey("mood"))), fields))
    }

    @Test
    fun randomIsASortTheCatalogNeedNotList() {
        assertFalse(refusesEditing(WorkQuery(listOf("movie"), sort = listOf(SortKey(RANDOM_SORT))), fields))
    }

    @Test
    fun aFilteredGridBecomesAQueryOnItsLibrary() {
        val grid = GridQuery("l1", sort = "runtime", played = "unplayed", resolutions = listOf("720p", "4k"), dynamicRanges = listOf("hdr"))
        val query = smartListQuery(grid, "movies")
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
        assertEquals(listOf(SortKey("title")), smartListQuery(GridQuery("l"), "shows").sort)
        assertEquals(listOf("series"), smartListQuery(GridQuery("l"), "shows").kinds)
        assertNull(smartListQuery(GridQuery("l", sort = "author"), "audiobooks").sort)
        assertEquals(listOf("book"), smartListQuery(GridQuery("l", sort = "author"), "audiobooks").kinds)
        assertEquals(listOf("video"), smartListQuery(GridQuery("l"), "videos").kinds)
    }
}
