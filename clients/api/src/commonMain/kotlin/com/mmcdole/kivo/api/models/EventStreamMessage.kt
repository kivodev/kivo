package com.mmcdole.kivo.api.models

import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.descriptors.buildClassSerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.JsonDecoder
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive

/**
 * A frame of the live-events WebSocket, told apart by its `type`: a
 * [SnapshotMessage] first, then [EventMessage] hints, a [ResyncMessage]
 * after the client fell behind, and a [KeepAliveMessage] every 25 seconds. Written by hand, since openapi-generator's
 * Kotlin multiplatform template can't decode `oneOf`. A frame
 * of a type this client doesn't know decodes as [Unknown], to be ignored.
 */
@Serializable(with = EventStreamMessage.Serializer::class)
sealed interface EventStreamMessage {
    data class Snapshot(val message: SnapshotMessage) : EventStreamMessage

    data class Event(val message: EventMessage) : EventStreamMessage

    data class Resync(val message: ResyncMessage) : EventStreamMessage

    data class KeepAlive(val message: KeepAliveMessage) : EventStreamMessage

    data object Unknown : EventStreamMessage

    object Serializer : KSerializer<EventStreamMessage> {
        override val descriptor: SerialDescriptor = buildClassSerialDescriptor("EventStreamMessage")

        override fun deserialize(decoder: Decoder): EventStreamMessage {
            val input = decoder as? JsonDecoder ?: throw SerializationException("EventStreamMessage is read from JSON")
            val frame = input.decodeJsonElement()
            val type = (frame as? JsonObject)?.get("type")?.jsonPrimitive?.contentOrNull
            return when (type) {
                "snapshot" -> Snapshot(input.json.decodeFromJsonElement(SnapshotMessage.serializer(), frame))
                "event" -> Event(input.json.decodeFromJsonElement(EventMessage.serializer(), frame))
                "resync" -> Resync(input.json.decodeFromJsonElement(ResyncMessage.serializer(), frame))
                "keepAlive" -> KeepAlive(input.json.decodeFromJsonElement(KeepAliveMessage.serializer(), frame))
                else -> Unknown
            }
        }

        /** Clients send nothing on the socket. */
        override fun serialize(encoder: Encoder, value: EventStreamMessage): Unit =
            throw SerializationException("EventStreamMessage is only read")
    }
}
