package release

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/egress"
	"github.com/mmcdole/kivo/internal/version"
)

// latestURL is GitHub's latest release of Kivo, which leaves out drafts
// and prereleases.
const latestURL = "https://api.github.com/repos/kivodev/kivo/releases/latest"

// notesURL is where a release's notes are, by its tag. It's made here
// rather than taken from the answer, so the link always goes to GitHub.
const notesURL = "https://github.com/kivodev/kivo/releases/tag/"

const (
	firstCheck = time.Minute    // after startup
	checkEvery = 24 * time.Hour // after that
)

// Options say which version is running and whether to check at all.
type Options struct {
	// Version is the running version, such as 0.4.0 or 0.3.1-dev+56c0018d.
	// One that isn't a semantic version is never checked.
	Version string
	// Check is false when the admin turned the check off.
	Check bool
}

// Release is a published release.
type Release struct {
	Version string // such as 0.5.0
	URL     string // its release notes
}

// getter is the egress client. Tests swap in a fake.
type getter interface {
	Get(ctx context.Context, url string, header http.Header) (egress.Response, error)
}

// Checker keeps the newest release it has seen that is newer than the
// running version.
type Checker struct {
	opt Options
	get getter

	mu    sync.Mutex
	newer *Release
}

// New returns a checker. It asks nothing until Run.
func New(opt Options) *Checker {
	return &Checker{opt: opt, get: egress.New()}
}

// Newer returns the newer release, if a check found one.
func (c *Checker) Newer() (Release, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.newer == nil {
		return Release{}, false
	}
	return *c.newer, true
}

// Run checks a minute after it starts and then once a day, until ctx
// ends. It returns at once when the check is off or the running version
// isn't a semantic version. A failed check is logged and tried again the next day.
func (c *Checker) Run(ctx context.Context) error {
	if _, ok := version.Parse(c.opt.Version); !c.opt.Check || !ok {
		return nil
	}
	t := time.NewTimer(firstCheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := c.check(ctx); err != nil && ctx.Err() == nil {
				slog.WarnContext(ctx, "release: can't check for a new release", "err", err)
			}
			t.Reset(checkEvery)
		}
	}
}

// check asks GitHub for the latest release and keeps it if it's newer than
// the running version.
func (c *Checker) check(ctx context.Context) error {
	resp, err := c.get.Get(ctx, latestURL, http.Header{"Accept": {"application/vnd.github+json"}})
	if err != nil {
		return err
	}
	if resp.Status != http.StatusOK {
		return fmt.Errorf("github answered %d", resp.Status)
	}
	var r struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(resp.Body, &r); err != nil {
		return fmt.Errorf("github's answer: %w", err)
	}
	if r.Draft || r.Prerelease || !version.Newer(r.TagName, c.opt.Version) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.newer = &Release{Version: strings.TrimPrefix(r.TagName, "v"), URL: notesURL + url.PathEscape(r.TagName)}
	return nil
}
