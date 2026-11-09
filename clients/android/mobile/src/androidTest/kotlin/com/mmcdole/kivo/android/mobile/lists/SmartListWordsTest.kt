package com.mmcdole.kivo.android.mobile.lists

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.api.models.Rule
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldOp
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

    private fun field(name: String, type: String, valueSource: String? = null, unit: String? = null) =
        WorkField(name, name, type, ops, listOf("movie"), sortable = true, unit = unit, valueSource = valueSource)

    private val fields = listOf(field("genre", "choice", "inline"), field("tag", "set", "inline"), field("addedAt", "date"), field("runtimeMs", "number", unit = "ms"))

    private fun summaryOf(root: RuleGroup): String {
        var line = ""
        rule.setContent { line = rulesSummary(WorkQuery(listOf("movie"), where = root), fields, emptyMap()) }
        rule.waitForIdle()
        return line
    }

    @Test
    fun aValueListReadsWithOr() {
        assertEquals("Comedy or Drama", summaryOf(RuleGroup("all", rules = listOf(Rule("genre", "anyOf", listOf("Comedy", "Drama"))))))
    }

    @Test
    fun allOfAListReadsWithAnd() {
        assertEquals("Dark and Quiet", summaryOf(RuleGroup("all", rules = listOf(Rule("tag", "allOf", listOf("Dark", "Quiet"))))))
    }

    @Test
    fun topLevelRulesJoinWithCommasEvenMatchingAny() {
        assertEquals("Comedy, Dark", summaryOf(RuleGroup("any", rules = listOf(Rule("genre", "anyOf", listOf("Comedy")), Rule("tag", "anyOf", listOf("Dark"))))))
    }

    @Test
    fun aGroupReadsInParenthesesByItsMatch() {
        val any = RuleGroup("any", rules = listOf(Rule("genre", "anyOf", listOf("Comedy")), Rule("tag", "anyOf", listOf("Dark"))))
        val all = any.copy(match = "all")
        assertEquals("Drama, (Comedy or Dark), (Comedy and Dark)", summaryOf(RuleGroup("all", rules = listOf(Rule("genre", "anyOf", listOf("Drama"))), groups = listOf(any, all))))
    }

    @Test
    fun setRulesNameTheField() {
        assertEquals("Genre is set, Tag isn't set", summaryOf(RuleGroup("all", rules = listOf(Rule("genre", "isSet"), Rule("tag", "isNotSet")))))
    }

    @Test
    fun aDurationAndAPeriodReadInUnits() {
        val root = RuleGroup("all", rules = listOf(Rule("runtimeMs", "gte", listOf("5400000")), Rule("addedAt", "within", listOf("P30D"))))
        assertEquals("Runtime at least 90 min, Date added in the last 30 days", summaryOf(root))
    }
}
