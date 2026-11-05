package main

// capsFor builds a client's capabilities: video and audio it plays from
// files in containers and, with stream, in a stream too; WebVTT shown in a
// stream, and the subtitle codecs it draws as files.
func capsFor(containers []string, stream bool, video []map[string]any, audio []string, externals ...string) map[string]any {
	delivery := []string{"file"}
	var subtitles []map[string]any
	if stream {
		delivery = append(delivery, "stream")
		subtitles = append(subtitles, map[string]any{"codec": "webvtt", "delivery": []string{"stream"}})
	}
	for _, c := range externals {
		subtitles = append(subtitles, map[string]any{"codec": c, "delivery": []string{"external"}})
	}
	var v, a []map[string]any
	for _, e := range video {
		entry := map[string]any{"delivery": delivery}
		for k, x := range e {
			entry[k] = x
		}
		v = append(v, entry)
	}
	for _, c := range audio {
		a = append(a, map[string]any{"codec": c, "delivery": delivery, "maxChannels": 2})
	}
	return map[string]any{"file": map[string]any{"containers": containers}, "video": v, "audio": a, "subtitles": subtitles}
}

// h264 is a video entry for H.264, of these profiles when given.
func h264(profiles ...string) []map[string]any {
	e := map[string]any{"codec": "h264"}
	if len(profiles) > 0 {
		e["profiles"] = profiles
	}
	return []map[string]any{e}
}

// played is the part of a play's or a download's answer the checks read.
type played struct {
	ID, State, Delivery, URL string
	StartMs                  int
	Tracks                   []playedTrack
	Previews                 struct {
		Ready bool
		URL   string
	}
}

type playedTrack struct {
	TrackID, Type, Delivery, Codec, Reason, URL string
	Selected                                    bool
	Transcode                                   *struct{}
	Fonts                                       []string
}

// selected is the selected track of a type.
func (p played) selected(kind string) playedTrack {
	for _, t := range p.Tracks {
		if t.Type == kind && t.Selected && t.TrackID != "off" {
			return t
		}
	}
	return playedTrack{}
}

// summary is the play's label: its video's.
func (p played) summary() string {
	v := p.selected("video")
	switch {
	case v.Transcode != nil:
		return "transcode"
	case v.Delivery == "file":
		return "directPlay"
	}
	return "directStream"
}

// externals are the subtitle tracks delivered external, as files.
func (p played) externals() []playedTrack {
	var out []playedTrack
	for _, t := range p.Tracks {
		if t.Delivery == "external" {
			out = append(out, t)
		}
	}
	return out
}
