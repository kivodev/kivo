package ffmpeg

import (
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/media"
)

// patterns map ffmpeg's error output to the error set. The first match
// wins. Device failures come before source failures, because a failing
// device also reports I/O errors.
var patterns = []struct {
	text string
	kind error
}{
	// NVENC's per-system session limit, and a GPU that is out of memory or
	// out of encoder sessions.
	{"OpenEncodeSessionEx failed: incompatible client key", media.ErrEncoderBusy},
	{"OpenEncodeSessionEx failed: out of memory", media.ErrEncoderBusy},
	{"CUDA_ERROR_OUT_OF_MEMORY", media.ErrEncoderBusy},
	{"resource allocation failed", media.ErrEncoderBusy}, // VA-API
	{"Device or resource busy", media.ErrEncoderBusy},

	{"Failed to initialise VAAPI connection", media.ErrEncoderFailed},
	{"Device creation failed", media.ErrEncoderFailed},
	{"Error while opening encoder", media.ErrEncoderFailed},
	{"Error initializing an internal MFX session", media.ErrEncoderFailed}, // Quick Sync
	{"device failed", media.ErrEncoderFailed},                              // Quick Sync, mid-stream
	{"GPU Hang", media.ErrEncoderFailed},
	{"hardware device was lost", media.ErrEncoderFailed},
	{"CUDA_ERROR_", media.ErrEncoderFailed},
	{"Impossible to convert between the formats", media.ErrEncoderFailed}, // a filter the device can't feed
	{"Failed to render parameter buffer", media.ErrEncoderFailed},         // a VA-API filter refusing its input, then reported as an I/O error

	// A track that isn't there, or a codec the output can't carry.
	{"matches no streams", media.ErrUnsupported},
	{"codec not currently supported in container", media.ErrUnsupported},
	{"Unknown encoder", media.ErrUnsupported},
	{"No such filter", media.ErrUnsupported}, // an ffmpeg without the bundled build's filters

	// A file that can't be opened is a source failure only when the line
	// names the source. A missing subtitle to burn in, fonts folder or
	// scratch file is a request that can't be met (see classify).
	{"No such file or directory", media.ErrSourceUnreadable},
	{"Permission denied", media.ErrSourceUnreadable},
	{"Input/output error", media.ErrSourceUnreadable},
	{"Stale file handle", media.ErrSourceUnreadable},
	{"Transport endpoint is not connected", media.ErrSourceUnreadable}, // a FUSE mount that died

	{"Invalid data found when processing input", media.ErrSourceCorrupt},
	{"moov atom not found", media.ErrSourceCorrupt},
	{"error reading header", media.ErrSourceCorrupt},
	{"EBML header parsing failed", media.ErrSourceCorrupt},
}

// openErrors are the patterns for a file that can't be opened. They are a
// source failure only when the line names input.
var openErrors = []string{"No such file or directory", "Permission denied"}

// classify maps a failed run's error output to the error set, or to
// fallback when nothing in it is recognized. input is the run's -i
// argument. A file that can't be opened is unreadable when it's the input.
// Any other file that can't be opened is unsupported, so a play steps down
// rather than restarting forever.
func classify(stderr, input string, fallback error) error {
	for _, p := range patterns {
		if !strings.Contains(stderr, p.text) {
			continue
		}
		if slices.Contains(openErrors, p.text) && !namesInput(stderr, p.text, input) {
			return media.ErrUnsupported
		}
		return p.kind
	}
	return fallback
}

// namesInput reports whether a line of stderr with text in it names input.
func namesInput(stderr, text, input string) bool {
	for line := range strings.Lines(stderr) {
		if input != "" && strings.Contains(line, text) && strings.Contains(line, input) {
			return true
		}
	}
	return false
}
