package api

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
)

// metrics are counted here and written in Prometheus's text format by
// hand. A handful of counters and gauges don't justify the client library.
type metrics struct {
	mu       sync.Mutex
	requests map[requestKey]*requestStat
}

type requestKey struct {
	route string
	class string // 2xx, 4xx, …
}

type requestStat struct {
	count   int64
	seconds float64
}

func newMetrics() *metrics { return &metrics{requests: map[requestKey]*requestStat{}} }

func (m *metrics) observe(route string, status int, d time.Duration) {
	k := requestKey{route: route, class: fmt.Sprintf("%dxx", status/100)}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.requests[k]
	if st == nil {
		st = &requestStat{}
		m.requests[k] = st
	}
	st.count++
	st.seconds += d.Seconds()
}

func (h *handlers) GetMetrics(ctx context.Context, _ gen.GetMetricsRequestObject) (gen.GetMetricsResponseObject, error) {
	s := h.Server
	var b strings.Builder
	s.metrics.mu.Lock()
	keys := slices.SortedFunc(maps.Keys(s.metrics.requests), func(a, b requestKey) int {
		return strings.Compare(a.route+a.class, b.route+b.class)
	})
	b.WriteString("# HELP kivo_http_requests_total HTTP requests by route pattern and status class.\n")
	b.WriteString("# TYPE kivo_http_requests_total counter\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "kivo_http_requests_total{route=%q,status=%q} %d\n", k.route, k.class, s.metrics.requests[k].count)
	}
	b.WriteString("# HELP kivo_http_request_seconds_total Time spent serving requests, by route pattern and status class.\n")
	b.WriteString("# TYPE kivo_http_request_seconds_total counter\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "kivo_http_request_seconds_total{route=%q,status=%q} %g\n", k.route, k.class, s.metrics.requests[k].seconds)
	}
	s.metrics.mu.Unlock()

	busy := s.deps.Tasks.Busy()
	b.WriteString("# HELP kivo_task_items Items in flight now, by task.\n")
	b.WriteString("# TYPE kivo_task_items gauge\n")
	for _, t := range slices.Sorted(maps.Keys(busy)) {
		fmt.Fprintf(&b, "kivo_task_items{task=%q} %d\n", t, busy[t])
	}
	plays := map[string]int{}
	for _, p := range s.deps.Playback.Plays() {
		plays[string(p.Decision.Summary())]++
	}
	b.WriteString("# HELP kivo_plays What's playing, by its method as clients show it: directPlay, directStream or transcode.\n")
	b.WriteString("# TYPE kivo_plays gauge\n")
	for _, k := range slices.Sorted(maps.Keys(plays)) {
		fmt.Fprintf(&b, "kivo_plays{summary=%q} %d\n", k, plays[k])
	}
	seeks, seekTime := s.deps.Playback.SeekTimes()
	b.WriteString("# HELP kivo_seek_first_frame_seconds Time from a seek to the first frame, as clients report it.\n")
	b.WriteString("# TYPE kivo_seek_first_frame_seconds summary\n")
	fmt.Fprintf(&b, "kivo_seek_first_frame_seconds_sum %g\nkivo_seek_first_frame_seconds_count %d\n", seekTime.Seconds(), seeks)
	b.WriteString("# HELP kivo_problems Open problems on the problems page.\n")
	b.WriteString("# TYPE kivo_problems gauge\n")
	fmt.Fprintf(&b, "kivo_problems %d\n", len(s.deps.Problems.Open(ctx)))
	return gen.GetMetrics200TextResponse(b.String()), nil
}
