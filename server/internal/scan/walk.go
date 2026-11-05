package scan

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/scan/dbq"
)

// dirState is what a run learned about one listed directory. It is
// committed only at the end of the run.
type dirState struct {
	mtime time.Time
	// localMetadataHash hashes the names and modification times of the
	// directory's local metadata files. Comparing it notices a change.
	localMetadataHash string
}

// walk visits dir and its subdirectories. An unchanged directory is only
// stat'ed: it isn't listed and none of its files are opened, so a rescan of
// an unchanged library doesn't spin up array disks.
func (r *run) walk(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := fs.Stat(r.fsys, dir)
	if err != nil {
		if dir == "." {
			return err
		}
		return nil // it vanished since its parent was listed, and the next run will see that
	}
	if key, ok := keyOf(info); ok {
		if r.visits[key] {
			// Reached again through a link: a loop, or a second path to
			// the same folder. Only the first path is catalogued.
			return r.leaveOut(ctx, dir)
		}
		r.visits[key] = true
	}
	r.seen[dir] = true
	if !r.mustList(dir, info.ModTime()) {
		for _, sub := range r.kids[dir] {
			if err := r.walk(ctx, sub); err != nil {
				return err
			}
		}
		return nil
	}
	subdirs, err := r.list(ctx, dir, info.ModTime())
	if err != nil {
		return err
	}
	for _, sub := range subdirs {
		if err := r.walk(ctx, sub); err != nil {
			return err
		}
	}
	return nil
}

// A directory listed within this long of its mtime may have changed in the
// same tick, so it's listed again next time.
const racyWindow = 2 * time.Second

// mustList reports whether dir has to be listed rather than only stat'ed.
func (r *run) mustList(dir string, mtime time.Time) bool {
	k, ok := r.known[dir]
	// A partial scan's own directory is listed whatever its mtime: a file
	// written in place doesn't change it.
	if r.full || dir == r.dir && dir != "." || !ok || k.Mtime != mtime.UnixMilli() || time.Duration(k.ListedAt-k.Mtime)*time.Millisecond < racyWindow {
		return true
	}
	// A subtitles folder's change is a change to the videos beside it.
	for _, sub := range r.kids[dir] {
		if isSubsDir(path.Base(sub)) {
			if info, err := fs.Stat(r.fsys, sub); err != nil || info.ModTime().UnixMilli() != r.known[sub].Mtime {
				return true
			}
		}
	}
	return false
}

// leaveOut treats dir as absent: whatever was catalogued below it is gone.
func (r *run) leaveOut(ctx context.Context, dir string) error {
	under, err := r.s.deps.Catalog.FilesUnder(ctx, r.folder.ID, dir)
	if err != nil {
		return err
	}
	for _, f := range under {
		if f.Present {
			r.gone = append(r.gone, f)
		}
	}
	return nil
}

// list reads one directory. It records the directory's videos, committed
// at once, with probe jobs for new and changed ones. It attaches subtitle
// files, notices changed NFO and artwork files, and finds files and
// directories that are gone.
func (r *run) list(ctx context.Context, dir string, mtime time.Time) ([]string, error) {
	entries, err := fs.ReadDir(r.fsys, dir)
	if err != nil {
		// The directory is unreadable, often because of a permissions
		// mistake. Everything under it is left as it was, and it's tried
		// again next run.
		slog.WarnContext(ctx, "scan: directory unreadable; nothing under it changes", "dir", join(r.folder.Path, dir), "err", err)
		return nil, nil
	}
	if ignored(entries) {
		entries = nil // treat it as empty, so what was catalogued below it is gone
	}
	var (
		obs      []catalog.FileObservation
		present  = map[string]bool{}
		subtitle []subFile
		metadata []string
		subdirs  []string
	)
	for _, e := range entries {
		name := e.Name()
		rel := join(dir, name)
		if skipName(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if e.Type()&fs.ModeSymlink != 0 {
			// os.Root refuses links that leave the folder, so those are
			// skipped.
			if info, err = fs.Stat(r.fsys, rel); err != nil {
				r.linkOut = true
				r.s.setProblem("symlink:"+r.folder.ID.String(), health.Problem{
					Component: "scan", Code: "symlinkSkipped", Severity: health.Info, RaisedAt: time.Now(),
					Message: fmt.Sprintf("Skipped %s in %s: the symlink points outside the library folder.", rel, r.folder.Path),
				})
				continue
			}
		}
		switch {
		case info.IsDir():
			subdirs = append(subdirs, rel)
			if isSubsDir(name) {
				subtitle = append(subtitle, r.listSubs(rel)...)
			}
		case r.files.isMedia(name):
			present[rel] = true
			obs = append(obs, catalog.FileObservation{RelPath: rel, Size: info.Size(), MTime: info.ModTime()})
		case r.files.isSubtitle(name):
			subtitle = append(subtitle, subFile{rel: rel})
		case r.files.isMetadata(name):
			metadata = append(metadata, fmt.Sprintf("%s@%d", name, info.ModTime().UnixMilli()))
		}
	}
	slices.Sort(metadata)
	hash := strings.Join(metadata, "|")
	_, wasKnown := r.known[dir]
	metadataChanged := wasKnown && r.known[dir].LocalMetadataHash != hash

	catalogued, err := r.s.deps.Catalog.FilesIn(ctx, r.folder.ID, dir)
	if err != nil {
		return nil, err
	}
	err = r.s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		states, err := r.s.deps.Catalog.UpsertFiles(ctx, tx, r.folder.ID, obs)
		if err != nil {
			return err
		}
		var reidentify []id.ID
		for _, st := range states {
			switch {
			case st.Changed:
				r.added++ // probed by the scan's next step
			case metadataChanged:
				reidentify = append(reidentify, st.ID)
			}
			if err := r.s.deps.Catalog.SetExternalSubtitles(ctx, tx, st.ID, attach(st.RelPath, subtitle)); err != nil {
				return err
			}
		}
		if err := r.s.deps.Identifier.Reidentify(ctx, tx, reidentify); err != nil {
			return err
		}
		if metadataChanged && len(states) == 0 && dir != "." {
			// A show folder's NFO and artwork sit above its videos, which
			// are in Season NN/. One video below is enough, because identify
			// decides the whole top folder.
			return r.identifyOneUnder(ctx, tx, dir)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The gone files are those directly in dir that weren't listed, and
	// everything under a known subdirectory that no longer exists. They are
	// marked at the end of the run.
	for _, f := range catalogued {
		if f.Present && !present[f.RelPath] {
			r.gone = append(r.gone, f)
		}
	}
	for _, sub := range r.kids[dir] {
		if !slices.Contains(subdirs, sub) {
			if err := r.leaveOut(ctx, sub); err != nil {
				return nil, err
			}
		}
	}
	r.listed[dir] = dirState{mtime: mtime, localMetadataHash: hash}
	return subdirs, nil
}

// listSubs lists a Subs or Subtitles folder for subtitle files.
func (r *run) listSubs(dir string) []subFile {
	entries, err := fs.ReadDir(r.fsys, dir)
	if err != nil {
		slog.Warn("scan: subtitles folder unreadable", "dir", join(r.folder.Path, dir), "err", err)
		return nil
	}
	if ignored(entries) {
		return nil
	}
	var out []subFile
	for _, e := range entries {
		if !e.IsDir() && r.files.isSubtitle(e.Name()) {
			out = append(out, subFile{rel: join(dir, e.Name()), inSubs: true})
		}
	}
	return out
}

// identifyOneUnder sends the first present video below dir, if there is
// one, back to be identified.
func (r *run) identifyOneUnder(ctx context.Context, tx *store.Tx, dir string) error {
	f, err := dbq.New(tx).FirstFileUnder(ctx, dbq.FirstFileUnderParams{FolderID: r.folder.ID, Dir: dir})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.s.deps.Identifier.Reidentify(ctx, tx, []id.ID{f.ID})
}

// within reports whether dir is top or is below it.
func within(dir, top string) bool {
	return top == "." || dir == top || strings.HasPrefix(dir, top+"/")
}

func join(dir, name string) string {
	if dir == "." {
		return name
	}
	return dir + "/" + name
}
