package ffmpeg

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

const (
	// stderrTail is how much of the error output a run keeps.
	stderrTail = 16 << 10
	// waitDelay bounds how long reaping waits, after the process group is
	// killed, for anything that escaped it and still holds a pipe.
	waitDelay = 5 * time.Second
	// maxStdout bounds the output a run returns whole. Probe results and
	// tiny frames are kilobytes.
	maxStdout = 16 << 20
)

// proc is one running ffmpeg or ffprobe. It runs in its own process group,
// which is killed when its context ends. It also dies with the server, by
// the parent-death signal on Linux and by its output pipe closing
// elsewhere.
type proc struct {
	cmd     *exec.Cmd
	parent  context.Context // the caller's context
	cancel  context.CancelFunc
	stderr  *tail
	stdout  io.ReadCloser
	stdin   io.WriteCloser // nil unless asked for
	command string
	input   string // the -i argument, as ffmpeg names it in its errors

	closed   atomic.Bool
	exitCode atomic.Int64
	waitOnce sync.Once
	waitErr  error
}

// spawn says how a process runs.
type spawn struct {
	stdin bool // the caller writes to the process's input
	// background runs the process at low OS priority, below playback. It
	// is for analysis and other work no one is waiting on.
	background bool
	// log, when set, is called with every line of the error output, which
	// holds the results of measuring filters.
	log func(line string)
}

// start starts bin with args. The process is killed when ctx ends.
func start(ctx context.Context, op, bin string, args []string, how spawn) (*proc, error) {
	jobCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(jobCtx, bin, args...)
	// Errors are classified by their stderr text, so the text is always in
	// the C locale.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	prepare(cmd)
	cmd.WaitDelay = waitDelay
	p := &proc{cmd: cmd, parent: ctx, cancel: cancel, stderr: &tail{}, command: quote(append([]string{bin}, args...))}
	if i := slices.Index(args, "-i"); i >= 0 && i+1 < len(args) {
		p.input = args[i+1]
	}
	p.exitCode.Store(-1)
	cmd.Stderr = p.stderr
	if how.log != nil {
		cmd.Stderr = io.MultiWriter(p.stderr, &lines{fn: how.log})
	}
	fail := func(err error) (*proc, error) {
		cancel()
		e := &media.Error{Op: op, Kind: media.ErrEncoderFailed, Detail: err.Error(), Diagnostics: p.diagnostics()}
		// A context that has already ended stops the launch itself.
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			e.Kind = media.ErrTimeout
		case ctx.Err() != nil:
			e.Kind = media.ErrCancelled
		}
		return nil, e
	}
	var err error
	if p.stdout, err = cmd.StdoutPipe(); err != nil {
		return fail(err)
	}
	if how.stdin {
		if p.stdin, err = cmd.StdinPipe(); err != nil {
			return fail(err)
		}
	}
	if err := cmd.Start(); err != nil {
		return fail(err)
	}
	contain(cmd.Process)
	if how.background {
		// Set the priority as soon as the process exists, before it starts
		// any threads, which inherit it.
		lowerPriority(cmd.Process)
	}
	return p, nil
}

// wait reaps the process once. Later calls return the same result.
func (p *proc) wait() error {
	p.waitOnce.Do(func() {
		p.waitErr = p.cmd.Wait()
		if ps := p.cmd.ProcessState; ps != nil {
			p.exitCode.Store(int64(ps.ExitCode()))
		}
		p.cancel()
	})
	return p.waitErr
}

// close kills the process group and reaps it.
func (p *proc) close() {
	p.closed.Store(true)
	p.cancel()
	_ = p.wait()
}

func (p *proc) diagnostics() media.Diagnostics {
	return media.Diagnostics{Command: p.command, ExitCode: int(p.exitCode.Load()), Stderr: withoutKeyEchoes(p.stderr.String())}
}

// withoutKeyEchoes drops the bundled ffmpeg's answers to the pause and
// resume keys, such as "Transcoding is paused. Press [u] to resume.", and
// the blank lines around them. They report pacing, not something that went
// wrong.
func withoutKeyEchoes(s string) string {
	var b strings.Builder
	for line := range strings.Lines(s) {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "Transcoding ") && (strings.Contains(t, "paused") || strings.Contains(t, "resum")) {
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

// failure explains why the run failed. The caller's context and Close take
// precedence, then the error output, then fallback. cause is the error the
// reader saw, if any.
func (p *proc) failure(op string, fallback, cause error) error {
	d := p.diagnostics()
	e := &media.Error{Op: op, Diagnostics: d, Detail: lastLine(d.Stderr)}
	switch {
	case errors.Is(p.parent.Err(), context.DeadlineExceeded):
		e.Kind = media.ErrTimeout
	case p.parent.Err() != nil || p.closed.Load():
		e.Kind = media.ErrCancelled
	default:
		e.Kind = classify(d.Stderr, p.input, fallback)
	}
	if e.Detail == "" && cause != nil {
		e.Detail = cause.Error()
	}
	return e
}

// run runs bin to completion and returns its output.
func run(ctx context.Context, op string, fallback error, bin string, args []string, how spawn) ([]byte, error) {
	p, err := start(ctx, op, bin, args, how)
	if err != nil {
		return nil, err
	}
	out, readErr := io.ReadAll(io.LimitReader(p.stdout, maxStdout+1))
	if readErr == nil && len(out) > maxStdout {
		readErr = fmt.Errorf("more than %d bytes of output", maxStdout)
		p.cancel()
	}
	if err := p.wait(); err != nil || readErr != nil {
		return nil, p.failure(op, fallback, cmp.Or(readErr, err))
	}
	return out, nil
}

// Version returns the first line of bin -version, such as "ffmpeg version
// 8.1.3-Jellyfin". bin runs as every other call does, with no console
// window on Windows.
func Version(ctx context.Context, bin string) (string, error) {
	out, err := run(ctx, "check version", media.ErrUnsupported, bin, []string{"-hide_banner", "-version"}, spawn{})
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(first), nil
}

// lines passes each whole line written to it to fn.
type lines struct {
	fn      func(string)
	partial []byte
}

func (l *lines) Write(p []byte) (int, error) {
	l.partial = append(l.partial, p...)
	for {
		i := bytes.IndexAny(l.partial, "\r\n")
		if i < 0 {
			break
		}
		if i > 0 {
			l.fn(string(l.partial[:i]))
		}
		l.partial = l.partial[i+1:]
	}
	return len(p), nil
}

// tail keeps the last stderrTail bytes written to it.
type tail struct {
	mu        sync.Mutex
	b         []byte
	truncated bool
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if over := len(t.b) - stderrTail; over > 0 {
		t.b = append(t.b[:0], t.b[over:]...)
		t.truncated = true
	}
	return len(p), nil
}

// String returns the kept output, starting at a line boundary.
func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.b
	if i := bytes.IndexByte(b, '\n'); t.truncated && i >= 0 {
		b = b[i+1:]
	}
	return string(b)
}

// lastLine returns the last non-empty line of s.
func lastLine(s string) string {
	s = strings.TrimRight(s, "\n\r\t ")
	return strings.TrimSpace(s[strings.LastIndexByte(s, '\n')+1:])
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// quote renders argv as a POSIX shell command line.
func quote(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		if shellSafe.MatchString(a) {
			q[i] = a
		} else {
			q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(q, " ")
}
