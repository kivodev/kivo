package media

import "errors"

// Every failure is one of these errors, and each calls for one response.
// Match them with errors.Is. Use errors.As with *Error to get the
// diagnostics.
var (
	// ErrSourceUnreadable is a transient I/O failure. Retry later.
	ErrSourceUnreadable = errors.New("source unreadable")
	// ErrSourceCorrupt means ffmpeg or a parser found the file broken.
	ErrSourceCorrupt = errors.New("source corrupt")
	// ErrUnsupported means this rendition can't be produced. Fail the job.
	ErrUnsupported = errors.New("unsupported")
	// ErrEncoderBusy means the accelerator has no capacity.
	ErrEncoderBusy = errors.New("encoder busy")
	// ErrEncoderFailed is an accelerator or encoder failure.
	ErrEncoderFailed = errors.New("encoder failed")
	// ErrTimeout means the caller's deadline passed.
	ErrTimeout = errors.New("timed out")
	// ErrCancelled means the caller cancelled, or closed the output.
	ErrCancelled = errors.New("cancelled")
)

// Error is a failed run or a refused request. It unwraps to Kind, one of
// the errors above.
type Error struct {
	Op          string // what was being done, such as "probe /movies/a.mkv"
	Kind        error
	Detail      string // the cause in a few words, such as ffmpeg's last error line
	Diagnostics Diagnostics
}

func (e *Error) Error() string {
	s := e.Kind.Error() + ": " + e.Op
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	return s
}

func (e *Error) Unwrap() error { return e.Kind }
