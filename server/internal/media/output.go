package media

import (
	"iter"
	"strconv"
	"time"
)

// TimestampOffset is added to every timestamp a transcode writes, so no
// sample has a negative decode time. B-frame reordering and audio priming
// put the first samples of a file slightly before zero, and fragmented MP4
// can't store that. Segment.Start is on the source's timeline, without the
// offset. Anything aligned to the bytes of a segment, such as a WebVTT
// timestamp map, must add it.
const TimestampOffset = 10 * time.Second

// Output is a running transcode job. Init and Segments read the job's
// output as it is written. Close stops the job and must always be called.
type Output interface {
	// Init returns the initialization segment, waiting until it is written.
	Init() ([]byte, error)
	// Segments yields segments in order, each as soon as it is complete.
	// It may be ranged over once.
	Segments() iter.Seq2[Segment, error]
	// Cues returns the cues of the request's text track i that were read
	// since the last call. The file is read in order. So once a segment
	// has been yielded, every cue starting before its end has been read,
	// except perhaps a cue the job is still converting. After Segments
	// ends, every cue has been read.
	Cues(i int) ([]Cue, error)
	// Pause stops the job reading its input until Resume, keeping the
	// process and anything it holds.
	Pause() error
	Resume() error
	// Diagnostics describes the run, for display only.
	Diagnostics() Diagnostics
	// Close stops the job and returns once its processes are gone.
	Close() error
}

// Segment is one media segment of a transcode. It is one fragment, with
// video and audio together, from one video keyframe to the next. Copied
// video keeps the source's keyframes, so its segments are as long as the
// source's GOPs. Encoded video has its keyframes on the request's grid, so
// each of its segments is one grid step.
type Segment struct {
	Start time.Duration // read from the output, on the source's timeline
	Data  []byte
}

// Cue is one event of a text subtitle track, on the source's timeline.
type Cue struct {
	Start, End time.Duration
	Text       string // WebVTT cue text, as lines with italics, bold and underline as tags
	Settings   string // WebVTT cue settings, such as "line:0" for a cue at the top
}

// Keyframe is one video keyframe of a file.
type Keyframe struct {
	Time time.Duration // presentation time on the source's timeline
}

// Previews describes the sprite sheets a PreviewsRequest made. Frame i shows
// the video at i×Interval. It is on sheet i/(Columns×Rows), at column
// i%Columns and row i/Columns%Rows. Each frame is Width by Height pixels.
type Previews struct {
	Interval time.Duration `json:"interval"`
	Width    int           `json:"width"`
	Height   int           `json:"height"`
	Columns  int           `json:"columns"`
	Rows     int           `json:"rows"`
	Sheets   int           `json:"sheets"`
}

// SheetNames returns the sheets' file names, in order.
func (p Previews) SheetNames() []string {
	out := make([]string, p.Sheets)
	for i := range out {
		out[i] = SheetName(i)
	}
	return out
}

// SheetName returns the file name of sheet i.
func SheetName(i int) string { return "sheet-" + strconv.Itoa(i) + ".webp" }

// PointStep is how far apart a Chromaprint fingerprint's points are. There
// is one point every third of a 4096-sample frame, at 11025 Hz.
const PointStep = 4096 * time.Second / 3 / 11025

// SampleWidth is the width a SampleRequest measures keyframes at.
const SampleWidth = 480

// BlackLevels are the luma levels KeyframeStats.Below counts pixels under.
var BlackLevels = [3]int{20, 26, 32}

// Sample is what a SampleRequest read.
type Sample struct {
	// Points are the audio's raw Chromaprint points, PointStep apart,
	// starting at the request's Start.
	Points []uint32
	// Silences are the audio's quiet spans, under -50 dB and lasting half a
	// second or more, on the file's timeline.
	Silences []Span
	// Keyframes are the video keyframes in order. It is empty unless
	// SampleRequest.Video is set.
	Keyframes []KeyframeStats
}

// Span is a stretch of a file's timeline.
type Span struct {
	Start, End time.Duration
}

// KeyframeStats measures one keyframe's picture. YMin, YLow, YHigh and YMax
// are the luma's minimum, 10th percentile, 90th percentile and maximum, in
// 8-bit limited range. SatLow and SatAvg are the saturation's 10th
// percentile and average.
type KeyframeStats struct {
	At time.Duration // on the file's timeline
	// Below is the percentage of pixels darker than each of BlackLevels.
	Below                   [3]uint8
	YMin, YLow, YHigh, YMax float32
	SatLow, SatAvg          float32
}

// Diagnostics describes one ffmpeg or ffprobe run, for an admin to read.
// Nothing should parse it.
type Diagnostics struct {
	Command  string // the command line, quoted to paste into a shell
	ExitCode int    // -1 while running or after a signal
	Stderr   string // the end of the error output
}
