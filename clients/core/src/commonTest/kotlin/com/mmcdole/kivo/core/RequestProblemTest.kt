package com.mmcdole.kivo.core

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.Problem
import com.mmcdole.kivo.api.result
import io.ktor.client.engine.mock.respond
import io.ktor.client.network.sockets.ConnectTimeoutException
import io.ktor.client.network.sockets.SocketTimeoutException
import io.ktor.client.plugins.HttpRequestTimeoutException
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlinx.serialization.SerializationException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFails

class RequestProblemTest {
    private fun refusal(status: Int, code: String, message: String? = null) =
        ProblemException(status, Problem("about:blank", "", status, code, "r", message = message))

    @Test
    fun aProblemDocumentIsTheServersAnswer() {
        for ((status, code) in listOf(
            400 to "invalidRequest", 403 to "forbidden", 404 to "notFound", 409 to "conflict", 410 to "expired",
            429 to "rateLimited", 500 to "internal",
        )) {
            val error = refusal(status, code)
            assertEquals(RequestProblem.Refused(status, code, null, error.problem), RequestProblem.of(error), "$status $code")
        }
    }

    @Test
    fun theServersOwn503sAreAnswersNotNoAnswer() {
        for (code in listOf("transcodeBusy", "providerUnavailable", "notConfigured", "quotaUsed")) {
            val problem = RequestProblem.of(refusal(503, code))
            assertEquals(code, problem.code)
            assertEquals(503, (problem as RequestProblem.Refused).status)
        }
    }

    @Test
    fun aGatewaysDownWithoutAProblemIsNoAnswer() {
        for (status in 502..504) assertEquals(RequestProblem.Unreachable, RequestProblem.of(ProblemException(status, null)), "$status")
        assertEquals(RequestProblem.Refused(502, "internal", null, refusal(502, "internal").problem), RequestProblem.of(refusal(502, "internal")))
    }

    @Test
    fun anotherAnswerWithoutAProblemIsUnknown() {
        assertEquals(RequestProblem.Refused(500, "unknown", null, null), RequestProblem.of(ProblemException(500, null)))
        assertEquals(RequestProblem.Refused(404, "unknown", null, null), RequestProblem.of(ProblemException(404, null)))
    }

    @Test
    fun a401IsSignedOutUnlessItsAnotherProblem() {
        assertEquals(RequestProblem.SignedOut, RequestProblem.of(refusal(401, "unauthenticated")))
        assertEquals(RequestProblem.SignedOut, RequestProblem.of(ProblemException(401, null)))
        assertEquals("invalidCredentials", RequestProblem.of(refusal(401, "invalidCredentials")).code)
        assertEquals("wrongPassword", RequestProblem.of(refusal(401, "wrongPassword")).code)
    }

    @Test
    fun theServersMessageIsKept() {
        val problem = RequestProblem.of(refusal(410, "playEnded", "Stopped for maintenance"))
        assertEquals("Stopped for maintenance", (problem as RequestProblem.Refused).message)
    }

    @Test
    fun noConnectionOrATimeoutIsNoAnswer() {
        for (error in listOf(
            IOException("no route to host"),
            HttpRequestTimeoutException("http://kivo.test/api/v1/home", 15_000),
            ConnectTimeoutException("connect timed out"),
            SocketTimeoutException("read timed out"),
        )) {
            assertEquals(RequestProblem.Unreachable, RequestProblem.of(error), "$error")
        }
    }

    @Test
    fun aFailureOtherThanTheServersIsUnexpected() {
        assertEquals(RequestProblem.Unexpected, RequestProblem.of(SerializationException("unreadable")))
        assertEquals(RequestProblem.Unexpected, RequestProblem.of(IllegalStateException("bug")))
    }

    @Test
    fun eachKindHasItsCode() {
        assertEquals("network", RequestProblem.Unreachable.code)
        assertEquals("unauthenticated", RequestProblem.SignedOut.code)
        assertEquals("unknown", RequestProblem.Unexpected.code)
    }

    @Test
    fun theConnectionsFailuresAreClassified() = runTest {
        val server = FakeServer()
        val connection = server.connection()
        suspend fun read() = RequestProblem.of(assertFails { connection.works.getHome().result() })
        server.on("GET", "/api/v1/home") { throw IOException("offline") }
        assertEquals(RequestProblem.Unreachable, read(), "offline")
        server.on("GET", "/api/v1/home") { respond("<html>Bad gateway</html>", HttpStatusCode.BadGateway, headersOf(HttpHeaders.ContentType, "text/html")) }
        assertEquals(RequestProblem.Unreachable, read(), "a proxy's 502")
        server.on("GET", "/api/v1/home") { problem(HttpStatusCode.ServiceUnavailable, "transcodeBusy") }
        assertEquals("transcodeBusy", read().code, "the server's 503")
        server.on("GET", "/api/v1/home") { respond("<html>Oops</html>", HttpStatusCode.InternalServerError, headersOf(HttpHeaders.ContentType, "text/html")) }
        assertEquals(RequestProblem.Refused(500, "unknown", null, null), read(), "a proxy's 500")
        server.on("GET", "/api/v1/home") { problem(HttpStatusCode.NotFound, "notFound") }
        assertEquals("notFound", read().code)
    }
}
