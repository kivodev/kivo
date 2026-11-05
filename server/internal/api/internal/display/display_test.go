package display

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
)

// vectors are the shared track vectors: real pickers' tracks with their
// names, language tags, tracks in full and versions.
type vectors struct {
	Pickers []struct {
		Tracks []json.RawMessage `json:"tracks"`
		Names  [][]*string       `json:"names"`
	} `json:"pickers"`
	Languages [][]*string         `json:"languages"`
	Details   [][]json.RawMessage `json:"details"`
	Versions  [][]json.RawMessage `json:"versions"`
}

func readVectors(t *testing.T) vectors {
	t.Helper()
	b, err := os.ReadFile("../../../../../shared/display/vectors/tracks.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// decodeVector decodes a vector's track or version into out, leaving out
// its IDs, which aren't the server's.
func decodeVector(t *testing.T, raw json.RawMessage, out any) {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	var drop func(any)
	drop = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			delete(x, "id")
			for _, e := range x {
				drop(e)
			}
		case []any:
			for _, e := range x {
				drop(e)
			}
		}
	}
	drop(v)
	b, _ := json.Marshal(v)
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatal(err)
	}
}

// pair decodes a vector's input into in and returns its expected output.
func pair(t *testing.T, raw []json.RawMessage, in any) string {
	t.Helper()
	decodeVector(t, raw[0], in)
	var want string
	if err := json.Unmarshal(raw[1], &want); err != nil {
		t.Fatal(err)
	}
	return want
}

func TestVectors(t *testing.T) {
	v := readVectors(t)
	for i, p := range v.Pickers {
		ts := make([]gen.Track, len(p.Tracks))
		for j, raw := range p.Tracks {
			decodeVector(t, raw, &ts[j])
		}
		NameTracks(ts)
		for j, track := range ts {
			if want := *p.Names[j][0]; track.Name != want {
				t.Errorf("picker %d track %d: name %q, want %q", i, j, track.Name, want)
			}
		}
	}
	for _, l := range v.Languages {
		if got := language(l[0]); got != *l[1] {
			t.Errorf("language(%v) = %q, want %q", deref(l[0]), got, *l[1])
		}
	}
	for i, d := range v.Details {
		var track gen.Track
		if want := pair(t, d, &track); fullName(track) != want {
			t.Errorf("track %d in full = %q, want %q", i, fullName(track), want)
		}
	}
	for i, d := range v.Versions {
		var version gen.Version
		if want := pair(t, d, &version); VersionName(version) != want {
			t.Errorf("version %d = %q, want %q", i, VersionName(version), want)
		}
	}
}

func TestNameTracksByType(t *testing.T) {
	eng := "eng"
	ts := []gen.Track{
		{Type: gen.TrackTypeAudio, Codec: "aac", Language: &eng, Audio: &gen.AudioTrackInfo{Channels: 2}},
		{Type: gen.TrackTypeSubtitle, Codec: "subrip", Language: &eng, Subtitle: &gen.SubtitleTrackInfo{Source: gen.SubtitleSourceEmbedded}},
		{Type: gen.TrackTypeAudio, Codec: "aac", Language: &eng, Audio: &gen.AudioTrackInfo{Channels: 2}},
	}
	NameTracks(ts)
	for i, want := range []string{"English · Stereo · 1", "English", "English · Stereo · 2"} {
		if ts[i].Name != want {
			t.Errorf("track %d: name %q, want %q", i, ts[i].Name, want)
		}
	}
	if want := "English · SRT · Embedded"; ts[1].FullName != want {
		t.Errorf("full name %q, want %q", ts[1].FullName, want)
	}
}
