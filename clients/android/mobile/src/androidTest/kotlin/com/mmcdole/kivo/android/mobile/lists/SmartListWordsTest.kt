package com.mmcdole.kivo.android.mobile.lists

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.api.models.Rule
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.RuleGroupMatch
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldOp
import com.mmcdole.kivo.api.models.WorkFieldType
import com.mmcdole.kivo.api.models.WorkFieldValueSource
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkQuery
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals
import org.junit.Rule as TestRule

// A smart list's rules as the line under its name reads them.

@RunWith(AndroidJUnit4::class)
class SmartListWordsTest {
    @get:TestRule
    val rule = createComposeRule()

    private val ops = listOf(
        WorkFieldOp("anyOf", -1),
        WorkFieldOp("allOf", -1),
        WorkFieldOp("noneOf", -1),
        WorkFieldOp("isSet", 0),
        WorkFieldOp("isNotSet", 0),
        WorkFieldOp("gte", 1),
        WorkFieldOp("within", 1),
    )

    private fun field(name: String, type: WorkFieldType, valueSource: WorkFieldValueSource? = null, unit: String? = null) =
        WorkField(name, name, type, ops, listOf(WorkKind.Movie), sortable = true, unit = unit, valueSource = valueSource)

    private val fields = listOf(field("genre", WorkFieldType.Choice, WorkFieldValueSource.Inline), field("tag", WorkFieldType.Set, WorkFieldValueSource.Inline), field("addedAt", WorkFieldType.Date), field("runtimeMs", WorkFieldType.Number, unit = "ms"))

    private fun summaryOf(root: RuleGroup): String {
        var line = ""
        rule.setContent { line = rulesSummary(WorkQuery(listOf(WorkKind.Movie), where = root), fields, emptyMap()) }
        rule.waitForIdle()
        return line
    }

    @Test
    fun aValueListReadsWithOr() {
        assertEquals("Comedy or Drama", summaryOf(RuleGroup(RuleGroupMatch.All, rules = listOf(Rule("genre", "anyOf", listOf("Comedy", "Drama"))))))
    }

    @Test
    fun allOfAListReadsWithAnd() {
        assertEquals("Dark and Quiet", summaryOf(RuleGroup(RuleGroupMatch.All, rules = listOf(Rule("tag", "allOf", listOf("Dark", "Quiet"))))))
    }

    @Test
    fun topLevelRulesJoinWithCommasEvenMatchingAny() {
        assertEquals("Comedy, Dark", summaryOf(RuleGroup(RuleGroupMatch.Any, rules = listOf(Rule("genre", "anyOf", listOf("Comedy")), Rule("tag", "anyOf", listOf("Dark"))))))
    }

    @Test
    fun aGroupReadsInParenthesesByItsMatch() {
        val any = RuleGroup(RuleGroupMatch.Any, rules = listOf(Rule("genre", "anyOf", listOf("Comedy")), Rule("tag", "anyOf", listOf("Dark"))))
        val all = any.copy(match = RuleGroupMatch.All)
        assertEquals("Drama, (Comedy or Dark), (Comedy and Dark)", summaryOf(RuleGroup(RuleGroupMatch.All, rules = listOf(Rule("genre", "anyOf", listOf("Drama"))), groups = listOf(any, all))))
    }

    @Test
    fun setRulesNameTheField() {
        assertEquals("Genre is set, Tag isn't set", summaryOf(RuleGroup(RuleGroupMatch.All, rules = listOf(Rule("genre", "isSet"), Rule("tag", "isNotSet")))))
    }

    @Test
    fun aDurationAndAPeriodReadInUnits() {
        val root = RuleGroup(RuleGroupMatch.All, rules = listOf(Rule("runtimeMs", "gte", listOf("5400000")), Rule("addedAt", "within", listOf("P30D"))))
        assertEquals("Runtime at least 90 min, Date added in the last 30 days", summaryOf(root))
    }
}
