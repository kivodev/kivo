package health

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/events"
)

// logger returns a logger that logs into a fresh Logs of the given
// capacity, and into out as the server's text log at level.
func logger(capacity int, level slog.Level, out *bytes.Buffer) (*slog.Logger, *Logs) {
	return loggerOn(events.NewBus(), capacity, level, out)
}

// loggerOn is like logger, but its Logs publishes on bus.
func loggerOn(bus *events.Bus, capacity int, level slog.Level, out *bytes.Buffer) (*slog.Logger, *Logs) {
	logs := NewLogs(capacity, bus)
	return slog.New(logs.Handler(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level}))), logs
}

func messages(lines []LogLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Message
	}
	return out
}

func TestLogsKeepTheNewest(t *testing.T) {
	log, logs := logger(3, slog.LevelInfo, &bytes.Buffer{})
	for i := range 5 {
		log.Info(fmt.Sprintf("line %d", i))
	}
	lines := logs.Lines(LogFilter{})
	if got := fmt.Sprint(messages(lines)); got != "[line 2 line 3 line 4]" {
		t.Errorf("kept %s", got)
	}
	if lines[0].Seq != 3 || lines[2].Seq != 5 {
		t.Errorf("seqs %d…%d", lines[0].Seq, lines[2].Seq)
	}
	if got := fmt.Sprint(messages(logs.Lines(LogFilter{After: 4}))); got != "[line 4]" {
		t.Errorf("after 4: %s", got)
	}
}

func TestLogsFilter(t *testing.T) {
	log, logs := logger(10, slog.LevelDebug, &bytes.Buffer{})
	log.Debug("scan: file probed", "file", "Dune.mkv")
	log.Info("scan: finished", "root", "/mnt/movies")
	log.Warn("metadata: TMDb is slow")
	log.Error("store: disk full", "err", errors.New("no space left on device"))
	for _, c := range []struct {
		f    LogFilter
		want string
	}{
		{LogFilter{MinLevel: slog.LevelDebug}, "[scan: file probed scan: finished metadata: TMDb is slow store: disk full]"},
		{LogFilter{MinLevel: slog.LevelWarn}, "[metadata: TMDb is slow store: disk full]"},
		{LogFilter{MinLevel: slog.LevelError}, "[store: disk full]"},
		{LogFilter{MinLevel: slog.LevelDebug, Search: "SCAN:"}, "[scan: file probed scan: finished]"},
		{LogFilter{Search: "/mnt/movies"}, "[scan: finished]"},      // an attribute's value
		{LogFilter{Search: "space left"}, "[store: disk full]"},     // an error's text
		{LogFilter{Search: "dune", MinLevel: slog.LevelInfo}, "[]"}, // both filters
	} {
		if got := fmt.Sprint(messages(logs.Lines(c.f))); got != c.want {
			t.Errorf("%+v: %s, want %s", c.f, got, c.want)
		}
	}
}

// Lines the server's log level drops aren't kept either.
func TestLogsFollowTheLevel(t *testing.T) {
	log, logs := logger(10, slog.LevelInfo, &bytes.Buffer{})
	log.Debug("api: request")
	log.Info("kivo: starting")
	if got := fmt.Sprint(messages(logs.Lines(LogFilter{MinLevel: slog.LevelDebug}))); got != "[kivo: starting]" {
		t.Errorf("kept %s", got)
	}
}

// Lines at Info and above publish logsChanged, with a burst of them making
// one event. Debug lines, which every request logs, publish nothing.
func TestLogsPublish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := events.NewBus()
		defer bus.Close()
		sub := bus.Subscribe(func(events.Scope) bool { return true })
		defer sub.Close()
		heard := func() int {
			n := 0
			for {
				select {
				case e := <-sub.C():
					if e.Kind == events.LogsChanged && e.Scope.AdminOnly {
						n++
					}
				default:
					return n
				}
			}
		}
		log, _ := loggerOn(bus, 10, slog.LevelDebug, &bytes.Buffer{})
		log.Debug("api: request")
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if n := heard(); n != 0 {
			t.Fatalf("a Debug line published %d events", n)
		}
		log.Info("scan: finished")
		log.Warn("metadata: TMDb is slow")
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if n := heard(); n != 1 {
			t.Errorf("two lines published %d events; want 1", n)
		}
	})
}

// The download matches the text log byte for byte, including groups and
// logger attributes.
func TestLogsWriteText(t *testing.T) {
	var out bytes.Buffer
	log, logs := logger(10, slog.LevelDebug, &out)
	log = log.With("component", "scan").WithGroup("run")
	log.Info("scan: finished", "took", 1500*time.Millisecond, slog.Group("seen", "files", 3), "err", errors.New(`open "x": denied`))
	slog.New(logs.Handler(slog.NewTextHandler(&out, nil))).Warn("plain", "n", 42, "at", time.Date(2026, 10, 10, 2, 15, 0, 0, time.UTC))
	var text bytes.Buffer
	if err := WriteText(&text, logs.Lines(LogFilter{})); err != nil {
		t.Fatal(err)
	}
	if text.String() != out.String() {
		t.Errorf("download\n%s\ntext log\n%s", text.String(), out.String())
	}
}

func TestLogsConcurrently(t *testing.T) {
	log, logs := logger(100, slog.LevelInfo, &bytes.Buffer{})
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 50 {
				log.Info("line", "g", g, "i", i)
				logs.Lines(LogFilter{Search: "line"})
			}
		})
	}
	wg.Wait()
	lines := logs.Lines(LogFilter{})
	if len(lines) != 100 || lines[99].Seq != 400 {
		t.Fatalf("%d lines, last %d", len(lines), lines[len(lines)-1].Seq)
	}
	for i := 1; i < len(lines); i++ {
		if lines[i].Seq != lines[i-1].Seq+1 {
			t.Fatalf("seq %d after %d", lines[i].Seq, lines[i-1].Seq)
		}
	}
}
