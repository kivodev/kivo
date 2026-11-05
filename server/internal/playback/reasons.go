package playback

// Reason is a code for why a track isn't delivered unchanged in the file,
// or why choosing it starts a new play. Clients translate it into a
// message such as "this device can't play HEVC Main 10". The values are
// part of the API.
type Reason string

// Why a track isn't delivered unchanged, by the declared field it failed.
// The track's Unmet names the field.
const (
	ReasonContainerUnsupported       Reason = "containerUnsupported"
	ReasonVideoCodecUnsupported      Reason = "videoCodecUnsupported"
	ReasonVideoCodecTagUnsupported   Reason = "videoCodecTagUnsupported"
	ReasonVideoProfileUnsupported    Reason = "videoProfileUnsupported"
	ReasonVideoLevelUnsupported      Reason = "videoLevelUnsupported"
	ReasonVideoBitDepthUnsupported   Reason = "videoBitDepthUnsupported"
	ReasonVideoResolutionUnsupported Reason = "videoResolutionUnsupported"
	ReasonVideoFramerateUnsupported  Reason = "videoFramerateUnsupported"
	ReasonVideoBitrateUnsupported    Reason = "videoBitrateUnsupported"
	ReasonVideoRefFramesUnsupported  Reason = "videoRefFramesUnsupported"
	ReasonVideoInterlacedUnsupported Reason = "videoInterlacedUnsupported"
	ReasonVideoAnamorphicUnsupported Reason = "videoAnamorphicUnsupported"
	ReasonVideoHdrUnsupported        Reason = "videoHdrUnsupported"
	ReasonAudioCodecUnsupported      Reason = "audioCodecUnsupported"
	ReasonAudioChannelsUnsupported   Reason = "audioChannelsUnsupported"
	ReasonAudioSampleRateUnsupported Reason = "audioSampleRateUnsupported"
	ReasonAudioBitDepthUnsupported   Reason = "audioBitDepthUnsupported"
	ReasonAudioProfileUnsupported    Reason = "audioProfileUnsupported"
	ReasonSubtitleCodecUnsupported   Reason = "subtitleCodecUnsupported"
	// ReasonTrackSelectionUnsupported means the audio track isn't the
	// file's default, and the client plays only that one from a file.
	ReasonTrackSelectionUnsupported Reason = "trackSelectionUnsupported"
	// ReasonStreamCodecUnsupported means the codec plays from the file but
	// not in the stream.
	ReasonStreamCodecUnsupported Reason = "streamCodecUnsupported"
)

// Why a track isn't delivered unchanged, by the setting or limit that
// applies.
const (
	ReasonDirectPlayDisabled   Reason = "directPlayDisabled"
	ReasonBurnSubtitlesEnabled Reason = "burnSubtitlesEnabled"
	ReasonClientQualityLimit   Reason = "clientQualityLimit"
	ReasonRemoteQualityLimit   Reason = "remoteQualityLimit"
	ReasonBandwidthLimit       Reason = "bandwidthLimit"
	ReasonDialogueBoost        Reason = "dialogueBoost" // the audio is encoded to boost dialogue
	ReasonKeyframeIndexMissing Reason = "keyframeIndexMissing"
	ReasonSubtitlePreparing    Reason = "subtitlePreparing"
	ReasonFallback             Reason = "fallback"        // the play retries one whose playback failed, transcoding every track
	ReasonDownloadQuality      Reason = "downloadQuality" // the original is larger than a download at the chosen quality
)

// Why a video transcode runs on the CPU though the server has an
// accelerator.
const (
	ReasonAcceleratorFailed      Reason = "acceleratorFailed" // the accelerator was busy or failed during this play
	ReasonNoAccelerator          Reason = "noAccelerator"     // the server's accelerator setting is none
	ReasonAcceleratorUnsupported Reason = "acceleratorUnsupported"
)

// Refusal is a code for why a play or a download can't happen. The values
// are part of the API.
type Refusal string

const (
	RefuseNoTracks Refusal = "noTracks"
	// RefuseFileUnplayable means the work plays only as its file here, as
	// a book does and as everything does on a client without a stream, and
	// the file doesn't play.
	RefuseFileUnplayable Refusal = "fileUnplayable"
	// RefuseSubtitleUnplayable means the chosen subtitle is an image file
	// that can be neither drawn nor burned in.
	RefuseSubtitleUnplayable  Refusal = "subtitleUnplayable"
	RefuseKeyframeIndex       Refusal = "keyframeIndexMissing"
	RefuseDownloadsNotAllowed Refusal = "downloadsNotAllowed"
	RefuseTranscodeFailed     Refusal = "transcodeFailed" // the job failed as it started, or a download's conversion failed
	// The refusals below, along with transcodeFailed and storageFull, say
	// why a download's preparation failed.
	RefuseSourceUnreadable     Refusal = "sourceUnreadable"     // the file couldn't be read, after retrying
	RefuseSubtitlesUnavailable Refusal = "subtitlesUnavailable" // its subtitle couldn't be prepared
	RefusePlanUnreadable       Refusal = "planUnreadable"       // the server can't read the download's stored decision
	// The server is busy, and the client should try again later.
	RefuseTranscodeLimit Refusal = "transcodeLimit" // the server is at its limit of simultaneous video transcodes
	RefuseStorageFull    Refusal = "storageFull"    // the transcode disk is below its free-space floor
)
