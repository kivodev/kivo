package playback

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

// tv is what the TV plays offline: MP4 and MKV, and it draws WebVTT, ASS
// and PGS itself.
var tv = alike([]string{"mp4", "mkv"}, true, []VideoEntry{safari.Video[0], safari.Video[1]}, []AudioEntry{{Codec: "aac", MaxChannels: 2}},
	vttFile, assFile, pgsFile)

// anime is an episode of a Japanese series. It has these tracks:
//   - the English dub, flagged default, and the Japanese original;
//   - full English subtitles as PGS, English signs as ASS, and French as
//     SubRip;
//   - a Spanish SubRip file and an Italian PGS file next to it;
//   - German subtitles downloaded for it.
type anime struct {
	work, file               id.ID
	eng, jpn                 id.ID // audio tracks
	pgs, signs, fre          id.ID // embedded subtitle tracks
	spanish, italian, german id.ID // subtitle files
}

func (f *fixture) dualAudioEpisode() anime {
	f.t.Helper()
	series := f.b.Series("Steins;Gate")
	e := f.b.Episode(f.folder, series, 1, 1)
	f.probeWith(e.File, "mkv", 24*time.Minute, 4_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "eng", Default: true},
		media.Track{Index: 2, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "jpn"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "hdmv_pgs_subtitle", Language: "eng"},
		media.Track{Index: 4, Kind: media.Subtitle, Codec: "ass", Language: "eng", Forced: true},
		media.Track{Index: 5, Kind: media.Subtitle, Codec: "subrip", Language: "fre"},
	)
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		if err := f.b.Catalog.SetExternalSubtitles(ctx, tx, e.File, []catalog.SubtitleFile{
			{RelPath: "Show/S01E01.es.srt", Format: "srt", Language: "es"},
			{RelPath: "Show/S01E01.it.sup", Format: "sup", Language: "it"},
		}); err != nil {
			return err
		}
		_, err := f.b.Catalog.AddDownloadedSubtitle(ctx, tx, e.File, catalog.SubtitleFile{RelPath: "x/s01e01.de.srt", Format: "srt", Language: "de"})
		return err
	})
	cf, err := f.b.Catalog.File(ctx, e.File)
	if err != nil {
		f.t.Fatal(err)
	}
	a := anime{work: e.Work, file: e.File, eng: cf.Tracks[1].ID, jpn: cf.Tracks[2].ID, pgs: cf.Tracks[3].ID, signs: cf.Tracks[4].ID, fre: cf.Tracks[5].ID}
	for _, x := range cf.SubtitleFiles {
		switch x.Language {
		case "es":
			a.spanish = x.ID
		case "it":
			a.italian = x.ID
		case "de":
			a.german = x.ID
		}
	}
	return a
}

// converts lists the conversions the fixture's media ran.
func (f *fixture) converts() []media.ConvertRequest {
	f.media.mu.Lock()
	defer f.media.mu.Unlock()
	return slices.Clone(f.media.converts)
}

// A user with no audio language starts a dual-audio anime with the file's
// default track, the English dub, and its English signs. When it's
// downloaded in Japanese with full English subtitles, every subtitle track
// comes down with it as a file the TV draws. Each track is extracted
// before the download is ready, and the converted file keeps the audio's
// language.
func TestADownloadHasItsAudioAndEverySubtitle(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	a := f.dualAudioEpisode()

	pref, err := f.svc.Preferred(ctx, f.v, a.work)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range pref.Versions {
		if tr.Audio != a.eng || tr.Subtitle != a.signs {
			t.Fatalf("a play starts with %+v, want the English dub %v and its signs %v", tr, a.eng, a.signs)
		}
	}

	hold := make(chan struct{})
	f.extractions.set(nil, hold)
	d, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: a.work, AudioTrack: &a.jpn, SubtitleTrack: &a.pgs, Caps: tv})
	if err != nil {
		t.Fatal(err)
	}
	if selectedOf(d.Decision, media.Audio) != a.jpn || selectedOf(d.Decision, media.Subtitle) != a.pgs {
		t.Fatalf("audio %v and subtitle %v, want the Japanese %v and the English PGS %v", selectedOf(d.Decision, media.Audio), selectedOf(d.Decision, media.Subtitle), a.jpn, a.pgs)
	}
	if d.Decision.Delivery != DeliverConversion {
		t.Fatalf("the Japanese audio isn't the file's default, so it's converted: %+v", d.Decision)
	}
	// The embedded tracks aren't extracted yet, so the preparation
	// extracts them first.
	f.await(d.ID, DownloadPreparing)
	for f.extractions.count() == 0 {
		time.Sleep(time.Millisecond)
	}
	if got := f.converts(); len(got) != 0 {
		t.Fatalf("converted before its subtitles were extracted: %+v", got)
	}
	close(hold)
	d = f.await(d.ID, DownloadReady)

	want := map[id.ID]media.SubtitleFormat{
		a.pgs: media.PGS, a.signs: media.ASS, a.fre: media.VTT, a.spanish: media.VTT, a.italian: media.PGS, a.german: media.VTT,
	}
	got := map[id.ID]media.SubtitleFormat{}
	for _, s := range d.ExternalSubtitles {
		if !s.Ready {
			t.Errorf("subtitle %v isn't ready", s.Track)
		}
		got[s.Track] = s.Format
	}
	if len(got) != len(want) {
		t.Fatalf("subtitles %+v, want every track: %v", d.ExternalSubtitles, want)
	}
	for track, format := range want {
		if got[track] != format {
			t.Errorf("subtitle %v as %q, want %q", track, got[track], format)
		}
	}
	for _, s := range d.ExternalSubtitles {
		if _, err := f.dl.Subtitle(ctx, f.v, d.ID, s.Name); err != nil && s.Track != a.spanish && s.Track != a.italian && s.Track != a.german {
			t.Errorf("subtitle %s: %v", s.Name, err)
		}
	}
	c := f.converts()
	if len(c) != 1 || len(c[0].Audio) != 1 || c[0].Audio[0].Track != 2 || c[0].Audio[0].Language != "jpn" || len(c[0].Text) != 0 || c[0].BurnIn != nil {
		t.Fatalf("the conversion %+v", c)
	}
}

// A client that draws none of a file's subtitles, like AVPlayer on its
// own, gets every text track in the converted file, each with its
// language, and the chosen PGS track burned in. A client that draws PGS
// and ASS gets those as files, and the remaining text tracks in the
// converted file.
func TestADownloadCarriesTheTextTracksItDoesntDeliverAsFiles(t *testing.T) {
	tests := []struct {
		name      string
		subtitles []SubtitleEntry
		files     int // how many subtitles are delivered as files
		text      []string
		burned    bool
	}{
		{"draws none", nil, 0, []string{"en", "fr", "es", "de"}, true},
		{"draws PGS and ASS", []SubtitleEntry{pgsFile, assFile}, 3, []string{"fr", "es", "de"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, 0)
			f.downloads()
			a := f.dualAudioEpisode()
			f.extract(a.file, "", false)
			caps := iPhone
			caps.Subtitles = tt.subtitles
			d, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: a.work, AudioTrack: &a.jpn, SubtitleTrack: &a.pgs, Caps: caps})
			if err != nil {
				t.Fatal(err)
			}
			d = f.await(d.ID, DownloadReady)
			if selectedOf(d.Decision, media.Audio) != a.jpn || selectedOf(d.Decision, media.Subtitle) != a.pgs || len(d.ExternalSubtitles) != tt.files {
				t.Fatalf("download %+v", d)
			}
			c := f.converts()
			if len(c) != 1 {
				t.Fatalf("conversions %+v", c)
			}
			var langs []string
			for _, x := range c[0].Text {
				langs = append(langs, x.Language)
			}
			if !slices.Equal(langs, tt.text) {
				t.Errorf("text tracks in %q, want %q", langs, tt.text)
			}
			if burned := c[0].BurnIn != nil; burned != tt.burned {
				t.Errorf("burned in: %v, want %v", burned, tt.burned)
			}
			// Each carried text track is in the file after the video and the
			// audio, at its stream index there.
			var indexes []int
			for _, tr := range d.Decision.Tracks {
				if tr.Type == media.Subtitle && tr.Delivery == DeliverFile {
					indexes = append(indexes, tr.Index)
				}
			}
			if want := []int{2, 3, 4, 5}[:len(tt.text)]; !slices.Equal(indexes, want) {
				t.Errorf("text tracks at %v, want %v", indexes, want)
			}
			// The file's embedded tracks are read from the source, and the
			// others from their own files.
			for _, x := range c[0].Text {
				if own := x.Language == "en" || x.Language == "fr"; own != (x.Path == "") {
					t.Errorf("text track %+v", x)
				}
			}
		})
	}
}

// A file whose only subtitles are PGS, as Steins;Gate's are, downloads in
// Japanese with its English PGS track as a file for the TV.
func TestAPGSOnlyDownload(t *testing.T) {
	f := newFixture(t, 0)
	f.downloads()
	m := f.b.Movie(f.folder, "Your Name", 2016)
	f.probeWith(m.File, "mkv", 2*time.Hour, 4_000_000,
		media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8},
		media.Track{Index: 1, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "eng", Default: true},
		media.Track{Index: 2, Kind: media.Audio, Codec: "aac", Channels: 2, Language: "jpn"},
		media.Track{Index: 3, Kind: media.Subtitle, Codec: "hdmv_pgs_subtitle", Language: "eng"},
	)
	f.extract(m.File, "", false)
	cf, _ := f.b.Catalog.File(ctx, m.File)
	jpn, pgs := cf.Tracks[2].ID, cf.Tracks[3].ID
	d, err := f.dl.Request(ctx, f.p, f.v, DownloadRequest{Work: m.Work, AudioTrack: &jpn, SubtitleTrack: &pgs, Caps: tv})
	if err != nil {
		t.Fatal(err)
	}
	d = f.await(d.ID, DownloadReady)
	if selectedOf(d.Decision, media.Audio) != jpn || selectedOf(d.Decision, media.Subtitle) != pgs ||
		len(d.ExternalSubtitles) != 1 || d.ExternalSubtitles[0].Format != media.PGS || !d.ExternalSubtitles[0].Ready {
		t.Fatalf("download %+v", d)
	}
}
