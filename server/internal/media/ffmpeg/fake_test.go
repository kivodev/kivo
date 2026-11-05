package ffmpeg

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The test binary doubles as a fake ffmpeg and ffprobe. When run with
// KIVO_FAKE_FFMPEG set, it acts out that mode instead of running tests.
const (
	fakeMode    = "KIVO_FAKE_FFMPEG"
	fakeFile    = "KIVO_FAKE_FILE"    // the fixture a mode replays
	fakePidFile = "KIVO_FAKE_PIDFILE" // where the grandchild mode writes its child's pid
	fakeRules   = "KIVO_FAKE_HW"      // the hardware mode's rules
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeMode); mode != "" {
		os.Exit(fake(mode))
	}
	code := m.Run()
	if corpus.dir != "" {
		os.RemoveAll(corpus.dir)
	}
	os.Exit(code)
}

// fake acts as ffmpeg would in one situation and returns its exit code.
func fake(mode string) int {
	if mode == "hardware" {
		return fakeAccelerators(os.Args[1:])
	}
	var data []byte
	if name := os.Getenv(fakeFile); name != "" {
		var err error
		if data, err = os.ReadFile(name); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	init, frags := splitBoxes(data)
	switch mode {
	case "output": // succeed, writing the fixture
		os.Stdout.Write(data)
		return 0
	case "fail": // fail at once, with the fixture as error output
		os.Stderr.Write(data)
		return 1
	case "fail-after-segment": // write one segment, then fail
		os.Stdout.Write(init)
		os.Stdout.Write(frags[0])
		fmt.Fprintln(os.Stderr, "[matroska,webm @ 0x55d0c0] Invalid data found when processing input")
		return 1
	case "hang": // write the init segment, then nothing
		os.Stdout.Write(init)
	case "ignore-term": // like hang, but ignoring polite signals
		signal.Ignore(syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		os.Stdout.Write(init)
	case "grandchild": // like hang, with a child of its own holding stderr
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), fakeMode+"=sleep")
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		os.WriteFile(os.Getenv(fakePidFile), fmt.Append(nil, child.Process.Pid), 0o644)
		os.Stdout.Write(init)
	case "sleep":
	case "keys": // write a fragment per resume key, like a paced job
		os.Stdout.Write(init)
		in := bufio.NewReader(os.Stdin)
		for len(frags) > 0 {
			switch k, err := in.ReadByte(); {
			case err != nil:
				return 1
			case k == pauseKey:
				fmt.Fprintln(os.Stderr, "Transcoding is paused. Press [u] to resume.")
			case k == resumeKey:
				os.Stdout.Write(frags[0])
				frags = frags[1:]
			}
		}
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unknown fake mode", mode)
		return 2
	}
	time.Sleep(time.Hour)
	return 0
}

// fakeAccelerators acts as ffmpeg would on some machine's GPUs, following
// the rules in KIVO_FAKE_HW. The rule "a+b=file;…" means that a run whose
// command line contains both a and b writes file, as fragmented MP4, to
// stdout. For a .txt file, the run fails with the file as error output
// instead. The first rule that matches wins. A run that matches no rule
// succeeds. If it writes a clip, the clip is an empty file. If it writes to
// stdout, it writes KIVO_FAKE_FILE.
func fakeAccelerators(args []string) int {
	command := strings.Join(args, " ")
	result := os.Getenv(fakeFile)
	for rule := range strings.SplitSeq(os.Getenv(fakeRules), ";") {
		match, file, _ := strings.Cut(rule, "=")
		if match != "" && !slices.ContainsFunc(strings.Split(match, "+"), func(m string) bool { return !strings.Contains(command, m) }) {
			result = file
			break
		}
	}
	data, err := os.ReadFile(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if strings.HasSuffix(result, ".txt") {
		os.Stderr.Write(data)
		return 1
	}
	if out := args[len(args)-1]; out != "pipe:1" {
		if err := os.WriteFile(out, nil, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	os.Stdout.Write(data)
	return 0
}

// splitBoxes splits fragmented MP4 into everything up to and including
// moov, and then each moof with its mdat. Anything else gives nothing.
func splitBoxes(b []byte) (init []byte, frags [][]byte) {
	var cur []byte
	for len(b) >= 8 {
		n := binary.BigEndian.Uint32(b)
		if n < 8 || int64(n) > int64(len(b)) {
			return nil, nil
		}
		box, typ := b[:n], string(b[4:8])
		b = b[n:]
		switch {
		case init == nil && typ != "moov":
			cur = append(cur, box...)
		case typ == "moov":
			init, cur = append(cur, box...), nil
		case typ == "mdat":
			frags, cur = append(frags, append(cur, box...)), nil
		default:
			cur = append(cur, box...)
		}
	}
	return init, frags
}

// useFake makes the test binary act out mode, replaying file, for the rest
// of the test.
func useFake(t *testing.T, mode, file string) *Runner {
	t.Setenv(fakeMode, mode)
	t.Setenv(fakeFile, file)
	t.Setenv("GORACE", "atexit_sleep_ms=0") // a race-enabled fake would linger a second on exit
	return New(os.Args[0], os.Args[0])
}

// fixture returns the absolute path of a file in media's testdata.
func fixture(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
