package ffmpeg

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

func TestParseProbe(t *testing.T) {
	b, err := os.ReadFile(fixture(t, "h264.mkv.probe.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseProbe(b)
	if err != nil {
		t.Fatal(err)
	}
	want := media.ProbeResult{
		Container: "matroska,webm",
		Start:     -128 * time.Millisecond,
		Duration:  8128 * time.Millisecond,
		Bitrate:   23340,
		Tracks: []media.Track{
			{Index: 0, Kind: media.Video, Codec: media.H264, Profile: "Main", Level: 10,
				Width: 64, Height: 64, BitDepth: 8, FrameRate: 10, Range: media.SDR, References: 1, PixelFormat: "yuv420p"},
			{Index: 1, Kind: media.Audio, Codec: media.AAC, Profile: "LC", Channels: 1, ChannelLayout: "mono", SampleRate: 8000},
		},
		Chapters: []media.Chapter{
			{Start: 0, End: 2400 * time.Millisecond, Title: "Intro"},
			{Start: 2400 * time.Millisecond, End: 8 * time.Second, Title: "Main"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestParseProbeTracks(t *testing.T) {
	// Fields as ffprobe prints them for a Dolby Vision profile 8.1 file
	// with an interlaced anamorphic second video, 24-bit audio describing
	// the picture, a forced
	// subtitle and an attached cover.
	b := []byte(`{"format": {"format_name": "matroska,webm"}, "streams": [
		{"index": 0, "codec_type": "video", "codec_name": "hevc", "profile": "Main 10", "level": 153,
		 "codec_tag_string": "hvc1", "refs": 1, "field_order": "progressive", "sample_aspect_ratio": "1:1",
		 "width": 3840, "height": 2160, "pix_fmt": "yuv420p10le", "avg_frame_rate": "24000/1001",
		 "color_transfer": "smpte2084", "disposition": {"default": 1},
		 "side_data_list": [{"side_data_type": "DOVI configuration record", "dv_profile": 8,
		   "dv_bl_signal_compatibility_id": 1, "el_present_flag": 0}]},
		{"index": 1, "codec_type": "audio", "codec_name": "eac3", "level": -99, "sample_rate": "48000", "channels": 6,
		 "bits_per_raw_sample": "24", "channel_layout": "5.1(side)", "disposition": {"visual_impaired": 1},
		 "tags": {"LANGUAGE": "eng", "BPS": "640000"}},
		{"index": 6, "codec_type": "video", "codec_name": "mpeg2video", "codec_tag_string": "[0][0][0][0]", "refs": 1,
		 "field_order": "tt", "sample_aspect_ratio": "32:27", "width": 720, "height": 480, "pix_fmt": "yuv420p",
		 "avg_frame_rate": "30000/1001"},
		{"index": 2, "codec_type": "subtitle", "codec_name": "subrip",
		 "disposition": {"forced": 1, "hearing_impaired": 1}, "tags": {"language": "eng", "title": "Forced"}},
		{"index": 3, "codec_type": "video", "codec_name": "mjpeg", "width": 600, "height": 900,
		 "pix_fmt": "yuvj444p", "bits_per_raw_sample": "8", "avg_frame_rate": "0/0", "r_frame_rate": "90000/1",
		 "color_transfer": "arib-std-b67", "disposition": {"attached_pic": 1}},
		{"index": 4, "codec_type": "attachment", "codec_name": "ttf", "tags": {"filename": "font.ttf"}},
		{"index": 5, "codec_type": "data", "codec_name": "bin_data"}
	]}`)
	got, err := parseProbe(b)
	if err != nil {
		t.Fatal(err)
	}
	want := []media.Track{
		{Index: 0, Kind: media.Video, Codec: media.HEVC, Profile: "Main 10", Level: 153, Default: true,
			Width: 3840, Height: 2160, BitDepth: 10, FrameRate: 24000.0 / 1001, Range: media.HDR10, DolbyVisionProfile: 8,
			DolbyVisionCompat: 1, Tag: "hvc1", References: 1, PixelFormat: "yuv420p10le"},
		{Index: 1, Kind: media.Audio, Codec: "eac3", Language: "eng", Bitrate: 640000, VisualImpaired: true,
			Channels: 6, ChannelLayout: "5.1(side)", SampleRate: 48000, BitDepth: 24},
		{Index: 6, Kind: media.Video, Codec: "mpeg2video", Width: 720, Height: 480, BitDepth: 8, FrameRate: 30000.0 / 1001,
			Range: media.SDR, References: 1, Interlaced: true, Anamorphic: true, PixelFormat: "yuv420p"},
		{Index: 2, Kind: media.Subtitle, Codec: "subrip", Language: "eng", Title: "Forced", Forced: true, HearingImpaired: true},
		{Index: 3, Kind: media.Video, Codec: "mjpeg", AttachedPicture: true,
			Width: 600, Height: 900, BitDepth: 8, FrameRate: 90000, Range: media.HLG, PixelFormat: "yuvj444p"},
		{Index: 4, Kind: media.Attachment, Codec: "ttf"},
		{Index: 5, Kind: media.Data, Codec: "bin_data"},
	}
	if !reflect.DeepEqual(got.Tracks, want) {
		t.Errorf("got  %+v\nwant %+v", got.Tracks, want)
	}
}

func TestParseProbeRejectsGarbage(t *testing.T) {
	if _, err := parseProbe([]byte("not json")); err == nil {
		t.Error("parsed garbage")
	}
}

func TestParseProbeBookTags(t *testing.T) {
	// Format tags as ffprobe prints them for an inAudible m4b, with keys in
	// the case the file uses. The encoder and brand tags aren't kept.
	b := []byte(`{"format": {"format_name": "mov,mp4,m4a,3gp,3g2,mj2", "tags": {
		"major_brand": "M4A ", "title": "Exodus", "album": "Exodus: The Archimedes Engine (Unabridged)",
		"artist": "Peter F. Hamilton", "album_artist": "Peter F. Hamilton", "composer": "John Lee",
		"date": "2024", "genre": "Audiobook", "comment": "A comment", "description": "The blurb",
		"SERIES": "Exodus", "PART": "1", "AUDIBLE_ASIN": "B0CTKSG1VB", "PUBLISHER": "Random House Audio",
		"track": "1/1", "encoder": "inAudible 1.97"}}}`)
	got, err := parseProbe(b)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"album": "Exodus: The Archimedes Engine (Unabridged)", "artist": "Peter F. Hamilton",
		"album_artist": "Peter F. Hamilton", "composer": "John Lee", "date": "2024", "genre": "Audiobook",
		"description": "The blurb", "series": "Exodus", "part": "1", "asin": "B0CTKSG1VB",
		"publisher": "Random House Audio", "track": "1/1",
	}
	if !reflect.DeepEqual(got.Tags, want) {
		t.Errorf("tags %v\nwant %v", got.Tags, want)
	}

	// An MP3 has a comment, not a description, and MP4's movement tags
	// name the series and part.
	b = []byte(`{"format": {"tags": {"comment": "Read by Jeff Hays", "mvnm": "Riyria", "mvin": "3", "disc": "1/2"}}}`)
	if got, err = parseProbe(b); err != nil {
		t.Fatal(err)
	}
	want = map[string]string{"description": "Read by Jeff Hays", "series": "Riyria", "part": "3", "disc": "1/2"}
	if !reflect.DeepEqual(got.Tags, want) {
		t.Errorf("tags %v\nwant %v", got.Tags, want)
	}
}
