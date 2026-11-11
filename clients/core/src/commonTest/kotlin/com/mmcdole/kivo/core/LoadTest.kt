package com.mmcdole.kivo.core

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class LoadTest {
    @Test
    fun theAnswerIsTheFirstLoadedOnceLoadingIsPast() = runTest {
        val read: Flow<Load<String>> = flowOf(Load.Loading, Load.Loaded("first"), Load.Loaded("second"))

        assertEquals("first", read.answer())
    }

    @Test
    fun theAnswerStopsTheReadOnceItHasOne() = runTest {
        var readOn = false
        val read: Flow<Load<String>> = flow {
            emit(Load.Loaded("first"))
            readOn = true
        }

        assertEquals("first", read.answer())
        assertFalse(readOn)
    }

    @Test
    fun aFailureIsNoAnswerAndIsNotWaitedOut() = runTest {
        // A server that isn't answering fails first and is asked again: the answer doesn't wait for it.
        val read = Reachability().load(flow<String> { throw IOException("no route") })

        assertNull(read.answer())
        assertNull(flowOf<Load<String>>(Load.Loading, Load.Failed(RequestProblem.Unexpected), Load.Loaded("late")).answer())
    }

    @Test
    fun aReadThatEndsWithoutAnsweringIsNoAnswer() = runTest {
        assertNull(flowOf<Load<String>>(Load.Loading).answer())
        assertNull(flowOf<Load<String>>().answer())
    }

    @Test
    fun cancellingWhileItWaitsCancelsTheReadAndIsNotAnAnswer() = runTest {
        var cancelled = false
        val read: Flow<Load<String>> = flow {
            emit(Load.Loading)
            try {
                awaitCancellation()
            } finally {
                cancelled = true
            }
        }
        val answer = async { read.answer() }
        runCurrent()

        answer.cancel()

        assertFailsWith<CancellationException> { answer.await() }
        assertTrue(cancelled)
    }
}
