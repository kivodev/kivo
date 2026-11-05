package playback

import (
	"testing"

	"github.com/mmcdole/kivo/internal/media"
)

// Migration (downloads): remove after every client has updated past this release.
func TestALegacyPlanMovesToItsQuality(t *testing.T) {
	legacy := DownloadQuality{Name: "720p", Height: 720, Bitrate: 4_000_000}
	in := Input{Caps: stereoPhone, Target: TargetFile, Quality: legacy,
		Source: Source{Container: "matroska,webm", Bitrate: 8_000_000, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}}
	in.Source.Tracks = []SourceTrack{*in.Source.Video, *in.Source.Audio}
	p := plan{Input: in, Decision: Decide(in)}

	ready := currentPlan(p, true)
	if v, _ := ready.Decision.Selected(media.Video); ready.Input.Quality.Name != "standard" || v.Video.Height != 720 || v.Video.Bitrate != 4_000_000 {
		t.Fatalf("a ready download keeps its decision: %+v %+v", ready.Input.Quality, v.Video)
	}
	queued := currentPlan(p, false)
	if v, _ := queued.Decision.Selected(media.Video); queued.Input.Quality != quality("standard") || v.Video.Height != 1080 ||
		v.Video.Bitrate != 3_000_000 || v.Video.Fidelity != media.FidelityStandard {
		t.Fatalf("a queued download is decided again: %+v %+v", queued.Input.Quality, v.Video)
	}
	if again := currentPlan(queued, false); again.Input.Quality != queued.Input.Quality {
		t.Fatalf("a current plan stays as it is: %+v", again.Input.Quality)
	}
}
