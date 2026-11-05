package com.mmcdole.kivo.core

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.Problem
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.minutes
import kotlin.time.Duration.Companion.seconds

class ReachabilityTest {
    private fun refusal(status: Int, code: String) = ProblemException(status, Problem("about:blank", "", status, code, "r"))

    /** A read that fails with each of [failures] in turn, then answers with how many times it was asked. */
    private class Read(private val failures: List<Exception>) {
        var asked = 0

        val flow: Flow<Int> = flow {
            asked++
            failures.getOrNull(asked - 1)?.let { throw it }
            emit(asked)
        }
    }

    private fun TestScope.collect(reachability: Reachability, read: Read): MutableList<Load<Int>> {
        val loads = mutableListOf<Load<Int>>()
        backgroundScope.launch { reachability.load(read.flow).collect { loads += it } }
        runCurrent()
        return loads
    }

    @Test
    fun anAnswerIsLoaded() = runTest {
        val loads = Reachability().load(flow { emit("a"); emit("b") }).toList()
        assertEquals(listOf(Load.Loaded("a"), Load.Loaded("b")), loads)
    }

    @Test
    fun anAnswerKnownShowsFirstAndStaysWhileNoAnswerComes() = runTest {
        val read = Read(List(2) { IOException("offline") })
        val loads = mutableListOf<Load<Int>>()
        backgroundScope.launch { Reachability().load(read.flow, known = 0).collect { loads += it } }
        runCurrent()
        assertEquals(listOf<Load<Int>>(Load.Loaded(0)), loads, "no failure shown over it")
        advanceTimeBy(3.seconds)
        runCurrent()
        assertEquals(listOf<Load<Int>>(Load.Loaded(0), Load.Loaded(3)), loads)
    }

    @Test
    fun aRefusalStillShowsOverAnAnswerKnown() = runTest {
        val loads = Reachability().load(Read(listOf(refusal(404, "notFound"))).flow, known = 0).toList()
        assertEquals(Load.Loaded(0), loads.first())
        assertEquals(2, loads.size)
        assertTrue(loads.last() is Load.Failed)
    }

    @Test
    fun noAnswerIsAskedAgainWithBackoffUntilAnswered() = runTest {
        val reachability = Reachability()
        val read = Read(List(3) { IOException("offline") })
        val loads = collect(reachability, read)
        assertEquals(listOf<Load<Int>>(Load.Failed(RequestProblem.Unreachable)), loads, "shown once while it asks again")
        assertTrue(reachability.unreachable.value)
        advanceTimeBy(999)
        runCurrent()
        assertEquals(1, read.asked, "waits 1 s")
        advanceTimeBy(1)
        runCurrent()
        assertEquals(2, read.asked)
        advanceTimeBy(2.seconds)
        runCurrent()
        assertEquals(3, read.asked, "then 2 s")
        advanceTimeBy(4.seconds)
        runCurrent()
        assertEquals(4, read.asked, "then 4 s")
        assertEquals(listOf(Load.Failed(RequestProblem.Unreachable), Load.Loaded(4)), loads)
        assertFalse(reachability.unreachable.value)
    }

    @Test
    fun aGatewaysDownIsAskedAgain() = runTest {
        val read = Read(listOf(ProblemException(503, null)))
        val loads = collect(Reachability(), read)
        advanceTimeBy(1.seconds)
        runCurrent()
        assertEquals(listOf(Load.Failed(RequestProblem.Unreachable), Load.Loaded(2)), loads)
    }

    @Test
    fun aRefusalIsFinalAndNeverAskedAgain() = runTest {
        for (error in listOf(refusal(404, "notFound"), refusal(403, "forbidden"), refusal(503, "transcodeBusy"), refusal(500, "internal"))) {
            val reachability = Reachability()
            val read = Read(listOf(error))
            val loads = collect(reachability, read)
            advanceTimeBy(10.minutes)
            runCurrent()
            assertEquals(1, read.asked, "${error.code} isn't asked again")
            assertEquals(listOf<Load<Int>>(Load.Failed(RequestProblem.of(error))), loads)
            assertFalse(reachability.unreachable.value, "${error.code} is an answer")
        }
    }

    @Test
    fun signedOutIsFinal() = runTest {
        val read = Read(listOf(refusal(401, "unauthenticated")))
        val loads = collect(Reachability(), read)
        advanceTimeBy(10.minutes)
        runCurrent()
        assertEquals(1, read.asked)
        assertEquals(listOf<Load<Int>>(Load.Failed(RequestProblem.SignedOut)), loads)
    }

    @Test
    fun anAnswerLoadedStaysWhileTheNextGetsNone() = runTest {
        val reachability = Reachability()
        var asked = 0
        val source = flow {
            asked++
            if (asked == 2) throw IOException("offline")
            emit(asked)
            if (asked == 1) throw IOException("dropped")
        }
        val loads = mutableListOf<Load<Int>>()
        backgroundScope.launch { reachability.load(source).collect { loads += it } }
        runCurrent()
        assertEquals(listOf<Load<Int>>(Load.Loaded(1)), loads, "no failure over the answer showing")
        assertTrue(reachability.unreachable.value)
        advanceTimeBy(1.seconds)
        runCurrent()
        assertEquals(listOf<Load<Int>>(Load.Loaded(1)), loads)
        advanceTimeBy(2.seconds)
        runCurrent()
        assertEquals(listOf<Load<Int>>(Load.Loaded(1), Load.Loaded(3)), loads)
        assertFalse(reachability.unreachable.value)
    }

    @Test
    fun aRefusalAfterAnAnswerReplacesIt() = runTest {
        var asked = 0
        val source = flow {
            asked++
            emit(asked)
            throw refusal(404, "notFound")
        }
        val loads = Reachability().load(source).toList()
        assertEquals(Load.Loaded(1), loads.first())
        assertEquals("notFound", loads.last().problemOrNull()?.code)
        assertEquals(1, asked)
    }

    @Test
    fun retryNowAsksAgainAtOnce() = runTest {
        val reachability = Reachability()
        val read = Read(List(5) { IOException("offline") })
        collect(reachability, read)
        advanceTimeBy(1.seconds + 2.seconds)
        runCurrent()
        assertEquals(3, read.asked, "waiting 4 s now")
        reachability.retryNow()
        runCurrent()
        assertEquals(4, read.asked)
    }

    @Test
    fun theFirstAnswerWakesTheOtherReads() = runTest {
        val reachability = Reachability()
        val first = Read(List(3) { IOException("offline") })
        val second = Read(List(1) { IOException("offline") })
        val firstLoads = collect(reachability, first)
        advanceTimeBy(1.seconds + 2.seconds)
        runCurrent()
        assertEquals(3, first.asked, "the first waits 4 s")
        collect(reachability, second)
        advanceTimeBy(1.seconds)
        runCurrent()
        assertEquals(2, second.asked, "the second is answered")
        assertEquals(4, first.asked, "and wakes the first")
        assertEquals(Load.Loaded(4), firstLoads.last())
    }

    @Test
    fun untilAnsweredAsksAgainOnlyWithoutAnAnswer() = runTest {
        val reachability = Reachability()
        var asked = 0
        val answer = backgroundScope.launch {
            assertEquals(3, reachability.untilAnswered { if (++asked < 3) throw IOException("offline") else asked })
        }
        runCurrent()
        advanceTimeBy(3.seconds)
        runCurrent()
        assertTrue(answer.isCompleted)
        assertEquals(3, asked)

        var refused = 0
        assertFailsWith<ProblemException> { reachability.untilAnswered { refused++; throw refusal(404, "notFound") } }
        assertEquals(1, refused, "a refusal is thrown at once")
    }

    @Test
    fun backoffDoublesToItsCap() {
        val live = Backoff(1.seconds, 30.seconds)
        assertEquals(listOf(1, 2, 4, 8, 16, 30, 30).map { it.seconds }, (0..6).map(live::wait))
        assertEquals(30.seconds, live.wait(1_000))
        val transfers = Backoff(15.seconds, 5.minutes)
        assertEquals(listOf(15, 30, 60, 120, 240, 300, 300).map { it.seconds }, (0..6).map(transfers::wait))
    }
}
