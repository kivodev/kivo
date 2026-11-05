package com.mmcdole.kivo.api.models

import kotlinx.serialization.json.Json
import kotlin.test.Test
import kotlin.test.assertEquals

class EventStreamMessageTest {
    private val json = Json { ignoreUnknownKeys = true }

    private fun decode(frame: String) = json.decodeFromString(EventStreamMessage.serializer(), frame)

    @Test
    fun framesDecodeByType() {
        assertEquals(
            EventStreamMessage.Snapshot(SnapshotMessage(SnapshotMessage.Type.snapshot, serverVersion = "1.2.0")),
            decode("""{"type":"snapshot","serverVersion":"1.2.0"}"""),
        )
        val event = decode("""{"type":"event","kind":"libraryChanged","libraryIds":["0190d8a5-0000-7000-8000-000000000001"]}""")
        assertEquals(
            listOf("0190d8a5-0000-7000-8000-000000000001"),
            (event as EventStreamMessage.Event).message.libraryIds,
        )
        assertEquals("libraryChanged", event.message.kind)
        assertEquals(EventStreamMessage.Resync(ResyncMessage(ResyncMessage.Type.resync)), decode("""{"type":"resync"}"""))
        assertEquals(
            EventStreamMessage.KeepAlive(KeepAliveMessage(KeepAliveMessage.Type.keepAlive)),
            decode("""{"type":"keepAlive"}"""),
        )
    }

    @Test
    fun newKindsAndFieldsAreIgnored() {
        assertEquals(EventStreamMessage.Unknown, decode("""{"type":"somethingNew","x":1}"""))
        val event = decode("""{"type":"event","kind":"somethingNew","extra":true}""")
        assertEquals("somethingNew", (event as EventStreamMessage.Event).message.kind)
    }

    @Test
    fun framesRoundTrip() {
        val frame = EventStreamMessage.Event(EventMessage(EventMessage.Type.event, kind = "tasksChanged"))
        assertEquals(frame, decode(json.encodeToString(EventStreamMessage.serializer(), frame)))
    }
}
