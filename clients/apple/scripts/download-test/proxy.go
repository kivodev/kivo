// Command proxy sits between the Simulator and a local Kivo server for the
// download UI test: it slows download files to a set rate so a transfer
// can be interrupted, drops the connection once part way through the first
// large file (a network drop), logs each request with its Range header,
// and stops the server on POST /__stop so the test can play downloads with
// no server. POST /__unreachable drops every connection, as a dropped
// route does, until POST /__reachable, with the server still running.
package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

var dropped atomic.Bool

type slowWriter struct {
	http.ResponseWriter
	rate    int
	dropAt  int64
	written *int64
}

func (w slowWriter) Write(b []byte) (int, error) {
	n := 0
	for len(b) > 0 {
		chunk := min(len(b), w.rate/10)
		m, err := w.ResponseWriter.Write(b[:chunk])
		n += m
		*w.written += int64(m)
		if err != nil {
			return n, err
		}
		if f, ok := w.ResponseWriter.(http.Flusher); ok {
			f.Flush()
		}
		if w.dropAt > 0 && *w.written >= w.dropAt && dropped.CompareAndSwap(false, true) {
			log.Printf("DROP after %d bytes", *w.written)
			panic(http.ErrAbortHandler)
		}
		b = b[chunk:]
		time.Sleep(100 * time.Millisecond)
	}
	return n, nil
}

func main() {
	listen := flag.String("listen", ":18490", "")
	backend := flag.String("backend", "http://127.0.0.1:18484", "")
	rate := flag.Int("rate", 1_500_000, "bytes per second for download files")
	dropAt := flag.Int64("drop-after", 8_000_000, "drop the first file over this size once, after this many bytes")
	pidfile := flag.String("pidfile", "", "the server's pid, killed by POST /__stop")
	flag.Parse()
	target, _ := url.Parse(*backend)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	var unreachable atomic.Bool
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__stop":
			b, _ := os.ReadFile(*pidfile)
			out, err := exec.Command("kill", strings.TrimSpace(string(b))).CombinedOutput()
			unreachable.Store(true)
			log.Printf("STOP server %s %v", out, err)
			io.WriteString(w, "stopped\n")
			return
		case "/__unreachable", "/__reachable":
			unreachable.Store(r.URL.Path == "/__unreachable")
			log.Printf("%s", strings.ToUpper(r.URL.Path[2:]))
			io.WriteString(w, r.URL.Path[2:]+"\n")
			return
		}
		if unreachable.Load() {
			// As a closed port, the connection drops.
			if hj, ok := w.(http.Hijacker); ok {
				if c, _, err := hj.Hijack(); err == nil {
					c.Close()
					return
				}
			}
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		log.Printf("%s %s range=%q if-range=%q", r.Method, r.URL.Path, r.Header.Get("Range"), r.Header.Get("If-Range"))
		if strings.Contains(r.URL.Path, "/downloads/") && strings.Contains(r.URL.Path, "/files/") {
			var written int64
			w = slowWriter{w, *rate, *dropAt, &written}
		}
		proxy.ServeHTTP(w, r)
	})
	log.Fatal(http.ListenAndServe(*listen, nil))
}
