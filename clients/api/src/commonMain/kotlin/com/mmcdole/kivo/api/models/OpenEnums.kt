package com.mmcdole.kivo.api.models

import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder

// The contract's enums the apps branch on, written by hand: the generator
// maps each schema here (`make generate` in server/). Every other enum
// stays an open string. A value a later server adds decodes as Unknown, so
// an older app keeps working.

/** A track's type. */
@Serializable(with = TrackType.Serializer::class)
enum class TrackType(public override val value: String) : OpenEnum {
    Video("video"), Audio("audio"), Subtitle("subtitle"), Unknown("");

    internal object Serializer : OpenEnumSerializer<TrackType>("TrackType", entries, Unknown)
}

/** How a choice of a play shows: inside the `file`, in the `stream`, as an `external` file, `burn`ed in, or `none` for off. */
@Serializable(with = TrackDelivery.Serializer::class)
enum class TrackDelivery(public override val value: String) : OpenEnum {
    File("file"), Stream("stream"), External("external"), Burn("burn"), None("none"), Unknown("");

    internal object Serializer : OpenEnumSerializer<TrackDelivery>("TrackDelivery", entries, Unknown)
}

/** How a play's media is fetched: the original `file` or the HLS `stream`. */
@Serializable(with = PlayDelivery.Serializer::class)
enum class PlayDelivery(public override val value: String) : OpenEnum {
    File("file"), Stream("stream"), Unknown("");

    internal object Serializer : OpenEnumSerializer<PlayDelivery>("PlayDelivery", entries, Unknown)
}

/** What a download fetches: the original `file`, or a `conversion` made for it. */
@Serializable(with = DownloadDelivery.Serializer::class)
enum class DownloadDelivery(public override val value: String) : OpenEnum {
    File("file"), Conversion("conversion"), Unknown("");

    internal object Serializer : OpenEnumSerializer<DownloadDelivery>("DownloadDelivery", entries, Unknown)
}

/** When subtitles show: `none` never, `forced` only forced tracks, `smart` when the audio is in another language, or `always`. */
@Serializable(with = SubtitleMode.Serializer::class)
enum class SubtitleMode(public override val value: String) : OpenEnum {
    None("none"), Forced("forced"), Smart("smart"), Always("always"), Unknown("");

    internal object Serializer : OpenEnumSerializer<SubtitleMode>("SubtitleMode", entries, Unknown)
}

/** What a work is: `video` an item of a videos library and `book` an item of an audiobooks library. */
@Serializable(with = WorkKind.Serializer::class)
enum class WorkKind(public override val value: String) : OpenEnum {
    Movie("movie"), Series("series"), Season("season"), Episode("episode"), Extra("extra"), Video("video"), Book("book"), Unknown("");

    override fun toString(): String = value

    companion object {
        /** The one [value] names; [Unknown] for none or one this version doesn't know. */
        fun of(value: String?): WorkKind = entries.firstOrNull { it != Unknown && it.value == value } ?: Unknown
    }

    internal object Serializer : OpenEnumSerializer<WorkKind>("WorkKind", entries, Unknown)
}

/** What a library holds. */
@Serializable(with = LibraryKind.Serializer::class)
enum class LibraryKind(public override val value: String) : OpenEnum {
    Movies("movies"), Shows("shows"), Videos("videos"), Audiobooks("audiobooks"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<LibraryKind>("LibraryKind", entries, Unknown)
}

/** What a list is: `manual` is works the user added, `smart` the works rules select. */
@Serializable(with = ListKind.Serializer::class)
enum class ListKind(public override val value: String) : OpenEnum {
    Manual("manual"), Smart("smart"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<ListKind>("ListKind", entries, Unknown)
}

/** The order a manual list is shown in: `custom` the user's own, `title` A to Z, `releaseDate` newest first, `rating` highest first, `dateAdded` latest added to the list first. */
@Serializable(with = ListSort.Serializer::class)
enum class ListSort(public override val value: String) : OpenEnum {
    Custom("custom"), Title("title"), ReleaseDate("releaseDate"), Rating("rating"), DateAdded("dateAdded"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<ListSort>("ListSort", entries, Unknown)
}

/** What a Home row shows. */
@Serializable(with = HomeRowKind.Serializer::class)
enum class HomeRowKind(public override val value: String) : OpenEnum {
    ContinueWatching("continueWatching"), ContinueListening("continueListening"), RecentlyAdded("recentlyAdded"), List("list"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<HomeRowKind>("HomeRowKind", entries, Unknown)
}

/** Whether a work is identified: `identifying` is provisional, its first identification pending, and `unmatched` provisional with no confident match. */
@Serializable(with = MatchState.Serializer::class)
enum class MatchState(public override val value: String) : OpenEnum {
    Matched("matched"), Identifying("identifying"), Unmatched("unmatched"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<MatchState>("MatchState", entries, Unknown)
}

/** Where a collection comes from: `provider` a movie collection, `folder` a videos-library folder. */
@Serializable(with = CollectionSource.Serializer::class)
enum class CollectionSource(public override val value: String) : OpenEnum {
    Provider("provider"), Folder("folder"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<CollectionSource>("CollectionSource", entries, Unknown)
}

/** A video's resolution class, largest first. */
@Serializable(with = Resolution.Serializer::class)
enum class Resolution(public override val value: String) : OpenEnum {
    FourK("4k"), P1080("1080p"), P720("720p"), Sd("sd"), Unknown("");

    override fun toString(): String = value

    companion object {
        /** The one [value] names; [Unknown] for none or one this version doesn't know. */
        fun of(value: String?): Resolution = entries.firstOrNull { it != Unknown && it.value == value } ?: Unknown
    }

    internal object Serializer : OpenEnumSerializer<Resolution>("Resolution", entries, Unknown)
}

/** What a client runs on. */
@Serializable(with = ClientPlatform.Serializer::class)
enum class ClientPlatform(public override val value: String) : OpenEnum {
    AndroidTv("androidTv"), AppleTv("appleTv"), IPhone("iPhone"), IPad("iPad"), AndroidPhone("androidPhone"), Browser("browser"), Cast("cast"),
    Tizen("tizen"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<ClientPlatform>("ClientPlatform", entries, Unknown)
}

/** A `browser` session or an `app`'s. */
@Serializable(with = SessionKind.Serializer::class)
enum class SessionKind(public override val value: String) : OpenEnum {
    Browser("browser"), App("app"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<SessionKind>("SessionKind", entries, Unknown)
}

/** Why a track isn't delivered unchanged in the file. */
@Serializable(with = TrackReason.Serializer::class)
enum class TrackReason(public override val value: String) : OpenEnum {
    ContainerUnsupported("containerUnsupported"), VideoCodecUnsupported("videoCodecUnsupported"),
    VideoCodecTagUnsupported("videoCodecTagUnsupported"), VideoProfileUnsupported("videoProfileUnsupported"),
    VideoLevelUnsupported("videoLevelUnsupported"), VideoBitDepthUnsupported("videoBitDepthUnsupported"),
    VideoResolutionUnsupported("videoResolutionUnsupported"), VideoFrameRateUnsupported("videoFrameRateUnsupported"),
    VideoBitrateUnsupported("videoBitrateUnsupported"), VideoRefFramesUnsupported("videoRefFramesUnsupported"),
    VideoInterlacedUnsupported("videoInterlacedUnsupported"), VideoAnamorphicUnsupported("videoAnamorphicUnsupported"),
    VideoHdrUnsupported("videoHdrUnsupported"), AudioCodecUnsupported("audioCodecUnsupported"), AudioChannelsUnsupported("audioChannelsUnsupported"),
    AudioSampleRateUnsupported("audioSampleRateUnsupported"), AudioBitDepthUnsupported("audioBitDepthUnsupported"),
    AudioProfileUnsupported("audioProfileUnsupported"), SubtitleCodecUnsupported("subtitleCodecUnsupported"),
    TrackSelectionUnsupported("trackSelectionUnsupported"), StreamCodecUnsupported("streamCodecUnsupported"),
    DirectPlayDisabled("directPlayDisabled"), BurnSubtitlesEnabled("burnSubtitlesEnabled"), ClientQualityLimit("clientQualityLimit"),
    RemoteQualityLimit("remoteQualityLimit"), BandwidthLimit("bandwidthLimit"), DialogueBoost("dialogueBoost"),
    KeyframeIndexMissing("keyframeIndexMissing"), SubtitlePreparing("subtitlePreparing"), Fallback("fallback"), DownloadQuality("downloadQuality"),
    Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<TrackReason>("TrackReason", entries, Unknown)
}

/** Whether all of a group's rules and groups must match, or any. */
@Serializable(with = RuleGroupMatch.Serializer::class)
enum class RuleGroupMatch(public override val value: String) : OpenEnum {
    All("all"), Any("any"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<RuleGroupMatch>("RuleGroupMatch", entries, Unknown)
}

/** A sort key's direction. */
@Serializable(with = SortDirection.Serializer::class)
enum class SortDirection(public override val value: String) : OpenEnum {
    Asc("asc"), Desc("desc"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<SortDirection>("SortDirection", entries, Unknown)
}

/** What a work field's values are. */
@Serializable(with = WorkFieldType.Serializer::class)
enum class WorkFieldType(public override val value: String) : OpenEnum {
    Choice("choice"), Set("set"), Person("person"), Number("number"), Date("date"), Text("text"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<WorkFieldType>("WorkFieldType", entries, Unknown)
}

/** Whether a field's values are few, `inline`, or found with `search`. */
@Serializable(with = WorkFieldValueSource.Serializer::class)
enum class WorkFieldValueSource(public override val value: String) : OpenEnum {
    Inline("inline"), Search("search"), Unknown("");

    override fun toString(): String = value

    internal object Serializer : OpenEnumSerializer<WorkFieldValueSource>("WorkFieldValueSource", entries, Unknown)
}

/** An enum case named on the wire by its [value], which each enum overrides as public: an override is otherwise as internal as this. */
internal interface OpenEnum {
    val value: String
}

/** An enum as its wire string, decoding a string it doesn't know as [unknown]. */
internal open class OpenEnumSerializer<T>(
    name: String,
    private val values: List<T>,
    private val unknown: T,
) : KSerializer<T> where T : Enum<T>, T : OpenEnum {
    override val descriptor: SerialDescriptor = PrimitiveSerialDescriptor("com.mmcdole.kivo.api.models.$name", PrimitiveKind.STRING)

    override fun deserialize(decoder: Decoder): T {
        val value = decoder.decodeString()
        return values.firstOrNull { it != unknown && it.value == value } ?: unknown
    }

    override fun serialize(encoder: Encoder, value: T) = encoder.encodeString(value.value)
}
