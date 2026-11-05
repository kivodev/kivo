package playback

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/media"
)

// capsSummary is the client's capability report in one line, for the play
// start's log: where it plays HEVC (as a file, in a stream, or neither),
// the HDR formats, audio codecs and subtitle codecs it reported, its
// largest picture, and whether it selects tracks inside a file. It tells a
// decision's reasons apart from a client that reported less than it can
// do.
func capsSummary(c Capabilities) string {
	var hevc, ranges, audio, subs []string
	maxW, maxH := 0, 0
	for _, v := range c.Video {
		if v.Codec == media.HEVC {
			hevc = append(hevc, list(v.Delivery))
		}
		ranges = append(ranges, list(v.HDR))
		if len(v.DolbyVision) > 0 {
			ranges = append(ranges, "dv"+dolbyVisionList(v.DolbyVision))
		}
		maxW, maxH = max(maxW, v.MaxWidth), max(maxH, v.MaxHeight)
	}
	for _, a := range c.Audio {
		audio = append(audio, string(a.Codec))
	}
	for _, s := range c.Subtitles {
		subs = append(subs, string(s.Codec)+":"+list(s.Delivery))
	}
	selection := c.File != nil && c.File.TrackSelection
	return fmt.Sprintf("hevc=%s hdr=%s audio=%s subtitles=%s max=%dx%d selection=%t",
		capsList(hevc), capsList(ranges), capsList(audio), capsList(subs), maxW, maxH, selection)
}

// capsList is a summary's list: sorted, each value once, or "none".
func capsList(s []string) string {
	s = slices.DeleteFunc(s, func(v string) bool { return v == "" })
	if len(s) == 0 {
		return "none"
	}
	slices.Sort(s)
	return strings.Join(slices.Compact(s), ",")
}
