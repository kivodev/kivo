package playback

// DialogueBoost describes a play's dialogue boost.
type DialogueBoost struct {
	Enabled bool
}

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
