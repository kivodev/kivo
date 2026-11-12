package com.mmcdole.kivo.android.settings

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import com.mmcdole.kivo.core.Done
import com.mmcdole.kivo.core.RequestProblem
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/** A change the server refused or never got: the row it was made on, why, and how to try it again. */
class FailedSave(val row: String, val problem: RequestProblem, val retry: () -> Unit)

/**
 * Sends a page's changes to the server as they're made, keeping the last
 * that failed ([failed]) for its row to show with Retry. A change that
 * works says nothing.
 */
class Saves(private val scope: CoroutineScope) {
    var failed by mutableStateOf<FailedSave?>(null)
        private set

    /** Makes [change], made on [row]; when it fails, [undo] puts the row back as it was. */
    fun save(row: String, undo: () -> Unit = {}, change: suspend () -> Done<*>) {
        failed = null
        scope.launch {
            val problem = change().problemOrNull() ?: return@launch
            undo()
            failed = FailedSave(row, problem) { save(row, undo, change) }
        }
    }

    /** The failure to show on [row], if its last change failed. */
    fun on(row: String): FailedSave? = failed?.takeIf { it.row == row }
}
