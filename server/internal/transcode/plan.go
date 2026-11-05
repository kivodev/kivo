package transcode

import (
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

const (
	// gridSegment is the spacing of the grid that encoded video's
	// keyframes are forced onto. It's short because a far seek waits for
	// one whole segment to encode and download.
	gridSegment = 2 * time.Second
	// targetSegment is the length a remux segment reaches before the next
	// source keyframe ends it.
	targetSegment = 6 * time.Second
	// maxSegment is the longest a segment may get. Planning cuts a segment
	// at the last keyframe before this length.
	maxSegment = 10 * time.Second
	// maxKeyframeGap is the longest gap between keyframes that a remux
	// accepts. Copied video can only be cut at the source's keyframes, so
	// a longer gap makes a segment too long to seek in. It's above 10.43 s,
	// which is x264's default GOP of 250 frames at 23.976 fps, the most
	// common encode.
	maxKeyframeGap = 20 * time.Second
)

// span is one planned segment. Part is the part it's in, Start is where it
// begins on that part's timeline, and At is where it begins on the parts'
// joined timeline.
type span struct {
	Part            int
	Start, Duration time.Duration
	At              time.Duration
}

// planGrid cuts a file of the given duration into gridSegment-long
// segments counted from zero, which is the grid encoded video puts its
// keyframes on. A job always starts at a grid point, so its keyframes fall
// on the same grid.
func planGrid(duration time.Duration) []span {
	var out []span
	for start := time.Duration(0); start < duration; start += gridSegment {
		out = append(out, span{Start: start, Duration: min(gridSegment, duration-start), At: start})
	}
	if len(out) == 0 {
		out = append(out, span{Duration: duration})
	}
	return out
}

// planKeyframes cuts a file of the given duration at its keyframes. A
// segment ends at the first keyframe at least targetSegment after it
// began. It ends earlier, at the last keyframe before maxSegment, when the
// next keyframe would pass maxSegment. That way a short GOP followed by a
// long one doesn't make one very long segment. It's a pure function of its
// inputs, so a playlist is the same every time it's planned.
func planKeyframes(duration time.Duration, keys []media.Keyframe) []span {
	var cuts []time.Duration
	for _, k := range keys {
		if k.Time > 0 && k.Time < duration && (len(cuts) == 0 || k.Time > cuts[len(cuts)-1]) {
			cuts = append(cuts, k.Time)
		}
	}
	cuts = append(cuts, duration)

	var out []span
	start := time.Duration(0)
	for i, c := range cuts {
		if c == duration {
			break
		}
		next := cuts[i+1]
		if c-start >= targetSegment || next-start > maxSegment {
			out = append(out, span{Start: start, Duration: c - start, At: start})
			start = c
		}
	}
	return append(out, span{Start: start, Duration: duration - start, At: start})
}

// segmentAt returns the planned segment containing t on the joined timeline.
func segmentAt(plan []span, t time.Duration) int {
	i := 0
	for i+1 < len(plan) && plan[i+1].At <= t {
		i++
	}
	return i
}

// segmentIn returns the segment of part p containing t on the part's own
// timeline, allowing for the small difference between a container's
// keyframe time and the one measured from the output.
func segmentIn(plan []span, p int, t time.Duration) int {
	const slack = 50 * time.Millisecond
	i := 0
	for i < len(plan) && plan[i].Part < p {
		i++
	}
	for i+1 < len(plan) && plan[i+1].Part == p && plan[i+1].Start <= t+slack {
		i++
	}
	return i
}
