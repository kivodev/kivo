package health

import (
	"context"
	"time"
)

// Severity orders problems on the problems page.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"
	}
	return "unknown"
}

// Problem is one open issue, written for an admin reading the problems
// page. Its message reads like "library /mnt/tv unreachable for 2 h", not
// like an error string.
type Problem struct {
	Component string // the reporting package, such as "scan"
	Code      string // a stable code, for clients to translate and for dismissals
	Message   string
	RaisedAt  time.Time
	Severity  Severity
}

// Reporter is implemented by every component that can have problems.
type Reporter interface {
	Health(ctx context.Context) []Problem
}
