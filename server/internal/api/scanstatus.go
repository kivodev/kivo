package api

import (
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/tasks"
)

// libraryScanStatus builds a library's scan status from the scans in
// flight and the scans asked for. It reports the step the scan is on, how
// far it has got, and the time left in the step at the rate so far
// ("Reading files · 1,240 of 3,100 · about 4 minutes left"). A scan that
// was asked for counts as scanning before it starts.
func libraryScanStatus(l query.AdminLibrary, scans []tasks.Item, asked []id.ID, now time.Time) gen.LibraryScanStatus {
	s := gen.LibraryScanStatus{LibraryID: l.ID, State: gen.LibraryScanStateIdle}
	for _, f := range l.Folders {
		if !f.Reachable {
			s.State = gen.LibraryScanStateUnreachable
		}
	}
	if slices.Contains(asked, l.ID) {
		s.State = gen.LibraryScanStateScanning
	}
	for _, it := range scans {
		if it.ID != l.ID {
			continue
		}
		s.State = gen.LibraryScanStateScanning
		if it.Step == "" {
			break
		}
		p := &gen.ScanProgress{Step: gen.ScanStep(it.Step), Done: it.Done, Total: it.Total}
		if it.Done > 0 {
			per := now.Sub(it.Since) / time.Duration(it.Done)
			p.RemainingMs = ptr(int((per * time.Duration(it.Total-it.Done)).Milliseconds()))
		}
		s.Progress = p
	}
	return s
}
