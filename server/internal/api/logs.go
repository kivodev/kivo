package api

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/health"
)

const (
	defaultLogLimit = 500
	maxLogLimit     = 5000
)

// logLevels are the levels the API names, keyed by their wire value. The
// wire values are the spec's words, such as "warning". Config's log_level
// takes slog's words, such as "warn".
var logLevels = map[gen.LogLevel]slog.Level{
	gen.LogLevelDebug: slog.LevelDebug, gen.LogLevelInfo: slog.LevelInfo, gen.LogLevelWarning: slog.LevelWarn, gen.LogLevelError: slog.LevelError,
}

func levelName(l slog.Level) gen.LogLevel {
	switch {
	case l >= slog.LevelError:
		return gen.LogLevelError
	case l >= slog.LevelWarn:
		return gen.LogLevelWarning
	case l >= slog.LevelInfo:
		return gen.LogLevelInfo
	}
	return gen.LogLevelDebug
}

func (h *handlers) ListLogs(_ context.Context, req gen.ListLogsRequestObject) (gen.ListLogsResponseObject, error) {
	p := req.Params
	f := health.LogFilter{MinLevel: slog.LevelInfo}
	if p.Level != nil {
		lv, ok := logLevels[*p.Level]
		if !ok {
			return nil, invalid("unknown level", field("level", "invalid"))
		}
		f.MinLevel = lv
	}
	if p.Q != nil {
		f.Search = *p.Q
	}
	pg, err := ownPagingOf(p.After, p.Limit, defaultLogLimit, maxLogLimit)
	if err != nil {
		return nil, err
	}
	if f.After, err = cursor(pg, parseSeq, "the cursor is invalid"); err != nil {
		return nil, err
	}

	lines := h.deps.Logs.Lines(f)
	out := gen.LogLineFeed{Items: make([]gen.LogLine, 0, min(len(lines), pg.limit)), Total: pg.total(len(lines))}
	switch {
	case pg.first():
		lines = lines[max(0, len(lines)-pg.limit):]
	case len(lines) > pg.limit:
		lines = lines[:pg.limit]
		out.Next = ptr(strconv.FormatInt(lines[pg.limit-1].Seq, 10))
	}
	for _, l := range lines {
		attrs := make([]gen.LogAttr, len(l.Attrs))
		for i, a := range l.Attrs {
			attrs[i] = gen.LogAttr{Key: a.Key, Value: a.Value.String()}
		}
		out.Items = append(out.Items, gen.LogLine{
			ID: strconv.FormatInt(l.Seq, 10), LoggedAt: l.Time.UTC(), Level: levelName(l.Level), Message: l.Message, Attributes: attrs,
		})
	}
	return gen.ListLogs200JSONResponse(out), nil
}

// parseSeq reads a log line's sequence number.
func parseSeq(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err == nil && n < 0 {
		err = strconv.ErrRange
	}
	return n, err
}

func (h *handlers) GetLogFile(context.Context, gen.GetLogFileRequestObject) (gen.GetLogFileResponseObject, error) {
	var b strings.Builder
	if err := health.WriteText(&b, h.deps.Logs.Lines(health.LogFilter{MinLevel: slog.LevelDebug})); err != nil {
		return nil, err
	}
	disposition := fmt.Sprintf(`attachment; filename="kivo-log-%s.txt"`, time.Now().UTC().Format("20060102-150405"))
	return gen.GetLogFile200TextResponse{
		Body: b.String(), Headers: gen.GetLogFile200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
