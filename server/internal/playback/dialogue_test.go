package playback

import (
	"math/rand/v2"
	"testing"

	"github.com/mmcdole/kivo/internal/media"
)

// TestDialogueBoost checks that a play that can boost its dialogue says
// whether it does, whichever way it's switched. With the boost on, the audio is encoded to AAC with the boost,
// keeping the channels a plain transcode would keep.
func TestDialogueBoost(t *testing.T) {
	mkv, mp4 := "matroska,webm", "mov,mp4,m4a,3gp,3g2,mj2"
	for _, c := range []struct {
		name      string
		in        Input
		channels  int // the boosted audio's channel count
		boostable bool
	}{
		{"5.1 E-AC-3 played directly: to AAC 5.1",
			Input{Caps: safari, Source: Source{Container: mp4, Video: h264(), Audio: audio("eac3", 6), DefaultAudio: true, Remuxable: true}},
			6, true},
		{"5.1 E-AC-3 to stereo speakers, transcoded anyway: mixed down to stereo, nothing else",
			Input{Caps: stereoPhone, Source: Source{Container: mp4, Video: h264(), Audio: audio("eac3", 6), DefaultAudio: true, Remuxable: true}},
			2, true},
		{"TrueHD 7.1 passed through: to AAC 5.1",
			Input{Caps: withAudio(androidTV, "truehd", 8), Source: Source{Container: mkv, Video: h264(), Audio: audio("truehd", 8), DefaultAudio: true, Remuxable: true}},
			6, true},
		{"stereo AAC played directly: AAC again, nothing else",
			Input{Caps: browser, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			2, true},
		{"AC-3 copied into a remux: to AAC",
			Input{Caps: safari, Source: Source{Container: mkv, Video: h264(), Audio: audio("ac3", 6), DefaultAudio: true, Remuxable: true}},
			6, true},
		{"no keyframe index: the video encoded too",
			Input{Caps: browser, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}},
			2, true},
		{"a video transcode anyway, Opus copied: to AAC",
			Input{Caps: browser, Source: Source{Container: mkv, Video: hevc10(), Audio: audio("opus", 2), DefaultAudio: true, Remuxable: true}},
			2, true},
		{"a fallback: nothing else",
			Input{Caps: safari, Fallback: true, Source: Source{Container: mp4, Video: h264(), Audio: audio("eac3", 6), DefaultAudio: true, Remuxable: true}},
			6, true},
		{"a book: no boost",
			Input{Caps: browser, Source: Source{Container: mp4, Audio: audio("aac", 2), DefaultAudio: true}}, 0, false},
		{"a download: no boost",
			Input{Caps: safari, Target: TargetFile, Source: Source{Container: mp4, Video: h264(), Audio: audio("eac3", 6), DefaultAudio: true}}, 0, false},
		{"a client without HLS: no boost",
			Input{Caps: noHLS(assTV), Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}}, 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, on := range []bool{false, true} {
				in := c.in
				in.DialogueBoost = on
				d := decided(in)
				if !c.boostable {
					if d.DialogueBoost != nil {
						t.Fatalf("boost %v: %+v, want none", on, *d.DialogueBoost)
					}
					continue
				}
				if want := (DialogueBoost{Enabled: on}); d.DialogueBoost == nil || *d.DialogueBoost != want {
					t.Fatalf("boost %v: %+v, want %+v", on, d.DialogueBoost, want)
				}
				a, _ := d.Selected(media.Audio)
				if !on {
					if a.Audio != nil && a.Audio.DialogueBoost {
						t.Fatalf("boost off: audio %+v boosted", a.Audio)
					}
					continue
				}
				if a.Method() != Transcode || a.Audio.Codec != media.AAC || !a.Audio.DialogueBoost || a.Audio.Channels != c.channels {
					t.Errorf("boost on: audio %s %+v, want AAC with %d channels, boosted", a.Method(), a.Audio, c.channels)
				}
			}
		})
	}
}

// TestDialogueBoostCopiesVideo checks that a boost never encodes video
// that the play would otherwise copy or play directly. The exception is
// video that can't be copied into the client's stream.
func TestDialogueBoostCopiesVideo(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for range 5000 {
		in := randomInput(r)
		if !streams(in) {
			in.Caps.Subtitles = append(in.Caps.Subtitles, vttStream)
		}
		in.Source.Remuxable = true
		plain := decided(in)
		in.DialogueBoost = true
		d := decided(in)
		if plain.Playable != d.Playable {
			t.Fatalf("a boost changed whether it plays: %v → %v for %+v", plain.Playable, d.Playable, in)
		}
		v0, _ := plain.Selected(media.Video)
		v, _ := d.Selected(media.Video)
		inStream := judgeVideo(in.Caps.Video, videoCase{track: in.Source.Video.Track, delivery: DeliverStream}).ok
		if !v0.Transcoded() && v.Transcoded() && inStream {
			t.Fatalf("a boost encoded the video: %+v for %+v", v, in)
		}
	}
}
