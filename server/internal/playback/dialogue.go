package playback

import "github.com/mmcdole/kivo/internal/media"

// DialogueBoost describes a play's dialogue boost. Cost is what the boost
// changes besides the dialogue, whether it's on or off, so a client can
// show it beside the switch. Cost is empty when nothing else changes.
type DialogueBoost struct {
	Enabled bool
	Cost    DialogueBoostCost
}

// DialogueBoostCost is what a dialogue boost changes about a play besides
// the dialogue. The values are part of the API.
type DialogueBoostCost string

const (
	// CostAudioAsAAC means audio that would play in its own format, either
	// directly or passed through to a receiver, is encoded to AAC instead.
	CostAudioAsAAC DialogueBoostCost = "audioAsAac"
	// CostVideoTranscode means the video is encoded too, because it can't
	// be copied into a stream.
	CostVideoTranscode DialogueBoostCost = "videoTranscode"
)

// boostable reports whether a play can boost its dialogue. Only a stream
// of a video with audio qualifies, because the server encodes the audio
// and sends it with the video copied. A download keeps the original audio.
func (in Input) boostable() bool {
	return in.Target == TargetStream && in.Caps.streams() && in.Source.Video != nil && in.Source.Audio != nil
}

// boosts reports whether a play boosts its dialogue.
func (in Input) boosts() bool {
	return in.DialogueBoost && in.boostable()
}

// dialogueBoost builds a play's dialogue boost. It compares the play's
// decision with the decision it would get with the boost switched the
// other way.
func dialogueBoost(in Input, d Decision) *DialogueBoost {
	other := in
	other.DialogueBoost = !in.DialogueBoost
	with, without := d, decide(other)
	if !in.DialogueBoost {
		with, without = without, d
	}
	return &DialogueBoost{Enabled: in.DialogueBoost, Cost: boostCost(with, without, in.Source.Audio.Track)}
}

// boostCost is what a boost changes besides the dialogue. It compares a
// play's decision with the boost to its decision without it.
func boostCost(with, without Decision, audio media.Track) DialogueBoostCost {
	v, _ := with.Selected(media.Video)
	plain, _ := without.Selected(media.Video)
	if v.Transcoded() && !plain.Transcoded() {
		return CostVideoTranscode
	}
	if a, _ := without.Selected(media.Audio); !a.Transcoded() && audio.Codec != media.AAC {
		return CostAudioAsAAC
	}
	return ""
}
