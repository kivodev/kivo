package media

import "strconv"

// Capabilities is what the startup test found. Accelerators lists the
// accelerators that can encode video. Problems lists those found that
// can't, with the reason. CPU is what the CPU passed, since the CPU isn't
// an accelerator. It is empty until the test has run.
type Capabilities struct {
	Tested       bool
	Accelerators []Caps // best first
	Problems     []AcceleratorProblem
	CPU          Caps
}

// Backend is a family of accelerators, or the CPU.
type Backend string

// Backends.
const (
	Software     Backend = "software"
	VAAPI        Backend = "vaapi"
	QSV          Backend = "qsv"          // Intel Quick Sync Video
	NVENC        Backend = "nvenc"        // NVIDIA
	VideoToolbox Backend = "videotoolbox" // macOS, best effort
)

// Caps is what passed the test on one accelerator, or on the CPU. For the
// CPU, Accelerator is empty.
type Caps struct {
	Accelerator Accelerator
	Backend     Backend
	Name        string // a display name per method: "Intel Quick Sync (QSV)", "Intel VA-API", "NVIDIA NVENC", "CPU"
	Encoders    []EncoderCaps
	// Decoders are the source formats the accelerator decodes itself.
	// Anything else is decoded by the CPU and handed to its encoder.
	Decoders []VideoFormat
	ToneMap  Placement // where HDR is tone-mapped, if anywhere
	BurnIn   Placement // where subtitles are drawn, if anywhere
}

// Encoder returns the encoder for c, if it passed.
func (d Caps) Encoder(c Codec) (EncoderCaps, bool) {
	for _, e := range d.Encoders {
		if e.Codec == c {
			return e, true
		}
	}
	return EncoderCaps{}, false
}

// EncoderCaps is one output codec an accelerator or the CPU encodes.
type EncoderCaps struct {
	Codec Codec
	GOP   GOPMode
}

// GOPMode is how an encoder's keyframes meet the segment grid.
type GOPMode string

const (
	// GridGOP forces an IDR frame at each grid point, with closed GOPs and
	// no keyframes at scene cuts, so each segment is one grid step.
	GridGOP GOPMode = "grid"
	// FixedGOP is for an encoder that doesn't honor forced keyframes. It
	// gets a fixed GOP that is one segment of frames long. Its segments
	// follow the grid only as closely as whole frames allow. At NTSC rates
	// they drift by a fraction of a frame per segment, and they stop
	// following the grid across a change of frame rate.
	FixedGOP GOPMode = "fixed"
)

// VideoFormat is a codec at a bit depth, as a decoder sees it.
type VideoFormat struct {
	Codec    Codec
	BitDepth int
}

func (f VideoFormat) String() string {
	return string(f.Codec) + " " + strconv.Itoa(f.BitDepth) + "-bit"
}

// Placement is where a step of a transcode runs.
type Placement string

// Placements. The zero value means the step can't run.
const (
	OnAccelerator Placement = "accelerator"
	OnCPU         Placement = "cpu" // on the CPU, before the frames go to the accelerator
)

// AcceleratorProblem is an accelerator the server found but can't use.
type AcceleratorProblem struct {
	Accelerator Accelerator
	Backend     Backend
	Name        string
	Reason      ProblemReason
	Detail      string // the error, in the system's or ffmpeg's words
	// Group is the group that owns the device node, such as
	// "render (gid 109)". It is set for NoPermission when known.
	Group string
}

// ProblemReason is a code saying why an accelerator can't be used. Clients
// turn it into words.
type ProblemReason string

// Problem reasons.
const (
	// NoDeviceNode means the GPU is in the machine but its device node
	// isn't visible, as when /dev/dri isn't passed into a container.
	NoDeviceNode ProblemReason = "noDeviceNode"
	// NoPermission means the server can't open the device node. Group
	// names the group to add the server's user to, when it's known.
	NoPermission ProblemReason = "noPermission"
	// NoDriver means the GPU is there but its driver isn't reachable, as
	// when a container runs without the NVIDIA runtime.
	NoDriver ProblemReason = "noDriver"
	// TestFailed means the accelerator opened but no test encode passed.
	TestFailed ProblemReason = "testFailed"
)
