package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// sendfileSpy stands in for http.ResponseWriter on a plain TCP connection.
// It records each ReadFrom chunk, checks that the reader has the exact shape
// Go's net.sendFile requires (at most one *io.LimitedReader over a
// syscall.Conn), and tracks SetWriteDeadline calls.
type sendfileSpy struct {
	header    http.Header
	status    int
	body      bytes.Buffer
	chunks    []int64
	deadlines []time.Time
	writes    int
	sendfile  bool
}

func newSendfileSpy() *sendfileSpy {
	return &sendfileSpy{header: http.Header{}, sendfile: true}
}

func (s *sendfileSpy) Header() http.Header  { return s.header }
func (s *sendfileSpy) WriteHeader(code int) { s.status = code }

func (s *sendfileSpy) Write(b []byte) (int, error) {
	s.writes++
	return s.body.Write(b)
}

func (s *sendfileSpy) SetWriteDeadline(t time.Time) error {
	s.deadlines = append(s.deadlines, t)
	return nil
}

func (s *sendfileSpy) ReadFrom(r io.Reader) (int64, error) {
	// Mirror net.sendFile: unwrap at most one *io.LimitedReader, then
	// require syscall.Conn directly.
	inner := r
	if lr, ok := r.(*io.LimitedReader); ok {
		if lr.N > readFromChunk {
			s.sendfile = false
		}
		inner = lr.R
	}
	if _, ok := inner.(syscall.Conn); !ok {
		s.sendfile = false
	}
	n, err := io.Copy(&s.body, r)
	s.chunks = append(s.chunks, n)
	return n, err
}

func writeTestFile(t *testing.T, size int) (*os.File, []byte) {
	t.Helper()
	want := make([]byte, size)
	for i := range want {
		want[i] = byte(i*31 + 7)
	}
	p := filepath.Join(t.TempDir(), "media.bin")
	if err := os.WriteFile(p, want, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f, want
}

func TestStatusWriterReadFromSendfileShape(t *testing.T) {
	const size = 2*readFromChunk + readFromChunk/2 // 2.5 chunks: three ReadFrom calls
	f, want := writeTestFile(t, size)
	mod := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("full file through rangeWriter and statusWriter", func(t *testing.T) {
		spy := newSendfileSpy()
		sw := &statusWriter{ResponseWriter: spy, rc: http.NewResponseController(spy)}
		rec := &rangeWriter{ResponseWriter: sw}
		req := httptest.NewRequest(http.MethodGet, "/media", nil)
		http.ServeContent(rec, req, "", mod, f)

		if spy.writes != 0 {
			t.Errorf("Write called %d times; want 0 (should use ReadFrom)", spy.writes)
		}
		if !spy.sendfile {
			t.Error("ReadFrom did not preserve a single *io.LimitedReader over syscall.Conn")
		}
		wantChunks := []int64{readFromChunk, readFromChunk, readFromChunk / 2}
		if len(spy.chunks) != len(wantChunks) {
			t.Fatalf("chunks = %v, want %v", spy.chunks, wantChunks)
		}
		for i, c := range wantChunks {
			if spy.chunks[i] != c {
				t.Errorf("chunk[%d] = %d, want %d", i, spy.chunks[i], c)
			}
		}
		if len(spy.deadlines) != len(wantChunks) {
			t.Errorf("deadlines refreshed %d times, want %d", len(spy.deadlines), len(wantChunks))
		}
		if sw.status != http.StatusOK || !rec.wrote {
			t.Errorf("status=%d wrote=%v, want 200 true", sw.status, rec.wrote)
		}
		if !bytes.Equal(spy.body.Bytes(), want) {
			t.Error("response body mismatch")
		}
	})

	t.Run("partial range across chunk boundary", func(t *testing.T) {
		spy := newSendfileSpy()
		sw := &statusWriter{ResponseWriter: spy, rc: http.NewResponseController(spy)}
		rec := &rangeWriter{ResponseWriter: sw}
		req := httptest.NewRequest(http.MethodGet, "/media", nil)
		// 1.5 chunks starting halfway through the first chunk
		const start = readFromChunk / 2
		const end = start + readFromChunk + readFromChunk/2
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end-1))
		http.ServeContent(rec, req, "", mod, f)

		if spy.writes != 0 || !spy.sendfile {
			t.Fatalf("writes=%d sendfile=%v, want 0 true", spy.writes, spy.sendfile)
		}
		if len(spy.chunks) != 2 || spy.chunks[0] != readFromChunk || spy.chunks[1] != readFromChunk/2 {
			t.Errorf("chunks = %v, want [%d %d]", spy.chunks, readFromChunk, readFromChunk/2)
		}
		if sw.status != http.StatusPartialContent {
			t.Errorf("status = %d, want 206", sw.status)
		}
		if !bytes.Equal(spy.body.Bytes(), want[start:end]) {
			t.Error("range body mismatch")
		}
	})

	t.Run("unsatisfiable range still suppressed", func(t *testing.T) {
		spy := newSendfileSpy()
		sw := &statusWriter{ResponseWriter: spy, rc: http.NewResponseController(spy)}
		rec := &rangeWriter{ResponseWriter: sw}
		req := httptest.NewRequest(http.MethodGet, "/media", nil)
		req.Header.Set("Range", "bytes=99999999-")
		http.ServeContent(rec, req, "", mod, f)

		if rec.status != http.StatusRequestedRangeNotSatisfiable || rec.wrote || spy.body.Len() != 0 {
			t.Errorf("416: status=%d wrote=%v body=%d", rec.status, rec.wrote, spy.body.Len())
		}
	})

	t.Run("unlimited os.File", func(t *testing.T) {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		spy := newSendfileSpy()
		sw := &statusWriter{ResponseWriter: spy, rc: http.NewResponseController(spy)}
		n, err := io.Copy(sw, f)
		if err != nil || n != size {
			t.Fatalf("Copy: n=%d err=%v, want %d nil", n, err, size)
		}
		if spy.writes != 0 || !spy.sendfile || sw.status != http.StatusOK {
			t.Errorf("writes=%d sendfile=%v status=%d", spy.writes, spy.sendfile, sw.status)
		}
		if !bytes.Equal(spy.body.Bytes(), want) {
			t.Error("unlimited copy body mismatch")
		}
	})

	t.Run("fallback without ReaderFrom", func(t *testing.T) {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		sw := &statusWriter{ResponseWriter: rr, rc: http.NewResponseController(rr)}
		n, err := io.Copy(sw, io.LimitReader(f, 4096))
		if err != nil || n != 4096 {
			t.Fatalf("Copy fallback: n=%d err=%v", n, err)
		}
		if sw.status != http.StatusOK || !bytes.Equal(rr.Body.Bytes(), want[:4096]) {
			t.Errorf("status=%d bodyLen=%d", sw.status, rr.Body.Len())
		}
	})
}

func TestStatusWriterReadFromTCP(t *testing.T) {
	const size = 2*readFromChunk + readFromChunk/2
	f, want := writeTestFile(t, size)
	mod := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, rc: http.NewResponseController(w)}
		rec := &rangeWriter{ResponseWriter: sw}
		ff, err := os.Open(f.Name())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer ff.Close()
		http.ServeContent(rec, r, "", mod, ff)
		if rec.status == http.StatusRequestedRangeNotSatisfiable && !rec.wrote {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		}
	}))
	defer ts.Close()

	// Full GET over real TCP (exercises net.sendFile on Linux and macOS).
	resp, err := ts.Client().Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(got, want) {
		t.Fatalf("full GET: status=%d len=%d err=%v", resp.StatusCode, len(got), err)
	}

	// Partial Range GET spanning chunk boundaries.
	const start = readFromChunk / 2
	const end = start + readFromChunk + readFromChunk/2
	req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end-1))
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusPartialContent || !bytes.Equal(got, want[start:end]) {
		t.Fatalf("range GET: status=%d len=%d err=%v", resp.StatusCode, len(got), err)
	}
}
