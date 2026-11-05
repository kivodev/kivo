package com.mmcdole.kivo.api

import com.mmcdole.kivo.api.apis.SetupApi
import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import io.ktor.serialization.kotlinx.json.json
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class ProblemExceptionTest {
    private fun api(status: HttpStatusCode, body: String, type: String) = SetupApi(
        "http://kivo.test",
        HttpClient(MockEngine { respond(body, status, headersOf(HttpHeaders.ContentType, type)) }) {
            install(ContentNegotiation) { json() }
        },
    )

    @Test
    fun aProblemDocumentBecomesTheTypedError() = runTest {
        val problem = """{"type":"about:blank","title":"Not found","status":404,"code":"notFound","requestId":"r1"}"""
        val error = assertFailsWith<ProblemException> {
            api(HttpStatusCode.NotFound, problem, "application/problem+json").getServer().result()
        }
        assertEquals(404, error.status)
        assertEquals("notFound", error.code)
        assertEquals("r1", error.problem?.requestId)
    }

    @Test
    fun anotherBodyLeavesTheProblemOut() = runTest {
        val error = assertFailsWith<ProblemException> {
            api(HttpStatusCode.BadGateway, "<html>bad gateway</html>", "text/html").getServer().result()
        }
        assertEquals(502, error.status)
        assertEquals("unknown", error.code)
        assertNull(error.problem)
    }

    @Test
    fun aSuccessIsItsBody() = runTest {
        val info = """{"id":"s1","name":"Kivo","version":"1.0"}"""
        assertEquals("s1", api(HttpStatusCode.OK, info, "application/json").getServer().result().id)
    }
}
