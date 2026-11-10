package scan

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/tasks"
)

// memFS is a directory tree in memory that counts listings and opens, so a
// test can prove an unchanged rescan does neither. Adding or removing an
// entry updates its directory's mtime, as real filesystems do.
type memFS struct {
	mu    sync.Mutex
	nodes map[string]*node
	kids  map[string]map[string]bool // entries by directory
	lists int
	opens int
	// onList, if set, runs before each listing. A crash test cancels there.
	onList func(dir string)
}

type node struct {
	dir   bool
	data  []byte
	mtime time.Time
}

func newMemFS() *memFS {
	return &memFS{nodes: map[string]*node{".": {dir: true, mtime: time.Now().Add(-time.Hour)}}, kids: map[string]map[string]bool{}}
}

// write adds or replaces a file and creates its directories, all dated at.
func (m *memFS) write(p, data string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mkdirs(path.Dir(p), at)
	m.nodes[p] = &node{data: []byte(data), mtime: at}
	m.link(p)
	m.touch(path.Dir(p), at)
}

func (m *memFS) link(p string) {
	d := path.Dir(p)
	if m.kids[d] == nil {
		m.kids[d] = map[string]bool{}
	}
	m.kids[d][p] = true
}

// touch moves a directory's mtime forward, whatever the entry's own date,
// as adding or removing an entry does on a real filesystem.
func (m *memFS) touch(dir string, at time.Time) {
	n := m.nodes[dir]
	n.mtime = later(at, n.mtime.Add(time.Second))
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (m *memFS) mkdirs(dir string, at time.Time) {
	if _, ok := m.nodes[dir]; ok {
		return
	}
	m.mkdirs(path.Dir(dir), at)
	m.nodes[dir] = &node{dir: true, mtime: at}
	m.link(dir)
	m.touch(path.Dir(dir), at)
}

// remove deletes a file or a whole directory.
func (m *memFS) remove(p string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.nodes {
		if k == p || strings.HasPrefix(k, p+"/") {
			delete(m.nodes, k)
			delete(m.kids, k)
		}
	}
	delete(m.kids[path.Dir(p)], p)
	m.touch(path.Dir(p), at)
}

func (m *memFS) rename(from, to string, at time.Time) {
	m.mu.Lock()
	n := m.nodes[from]
	m.mu.Unlock()
	m.remove(from, at)
	m.write(to, string(n.data), n.mtime)
	m.mu.Lock()
	m.touch(path.Dir(to), at)
	m.mu.Unlock()
}

func (m *memFS) counts() (lists, opens int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lists, m.opens
}

func (m *memFS) resetCounts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lists, m.opens = 0, 0
}

func (m *memFS) Open(name string) (fs.File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opens++
	n, ok := m.nodes[name]
	if !ok || n.dir {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &memFile{Reader: bytes.NewReader(n.data), info: info{path.Base(name), n}}, nil
}

func (m *memFS) Stat(name string) (fs.FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[name]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return info{path.Base(name), n}, nil
}

func (m *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if m.onList != nil {
		m.onList(name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lists++
	if n, ok := m.nodes[name]; !ok || !n.dir {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	var out []fs.DirEntry
	for p := range m.kids[name] {
		out = append(out, fs.FileInfoToDirEntry(info{path.Base(p), m.nodes[p]}))
	}
	slices.SortFunc(out, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return out, nil
}

type memFile struct {
	*bytes.Reader
	info info
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *memFile) Close() error               { return nil }

var _ io.ReaderAt = (*memFile)(nil)

type info struct {
	name string
	n    *node
}

func (i info) Name() string       { return i.name }
func (i info) Size() int64        { return int64(len(i.n.data)) }
func (i info) ModTime() time.Time { return i.n.mtime }
func (i info) IsDir() bool        { return i.n.dir }
func (i info) Sys() any           { return nil }
func (i info) Mode() fs.FileMode {
	if i.n.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}

// fakeProber returns a fixed result or err, after running hook if it is
// set.
type fakeProber struct {
	err  error
	hook func(path string)
	// bad maps file names to the errors their probes return.
	bad map[string]error
	// subtitles maps file names to their subtitle tracks.
	subtitles map[string][]media.Track
	tags      map[string]string // the tags of every file

	mu     sync.Mutex
	probed []string // the names probed, in order
}

func (p *fakeProber) Probe(_ context.Context, path string) (media.ProbeResult, error) {
	if p.hook != nil {
		p.hook(path)
	}
	name := filepath.Base(path)
	p.mu.Lock()
	p.probed = append(p.probed, name)
	err := cmp.Or(p.bad[name], p.err)
	p.mu.Unlock()
	if err != nil {
		return media.ProbeResult{}, err
	}
	return media.ProbeResult{
		Container: "matroska,webm", Duration: time.Hour, Start: 1400 * time.Millisecond,
		Tracks: append([]media.Track{{Index: 0, Kind: media.Video, Codec: "h264"}, {Index: 1, Kind: media.Audio, Codec: "aac", Language: "eng"}},
			p.subtitles[name]...),
		Tags: p.tags,
	}, nil
}

// took returns the names probed since it was last called, sorted.
func (p *fakeProber) took() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := slices.Sorted(slices.Values(p.probed))
	p.probed = nil
	return out
}

// noMedia is the media for the real previews and subtitles services that a
// probe asks. Those services only ask for work, so it makes nothing.
type noMedia struct{}

func (noMedia) Previews(context.Context, media.PreviewsRequest) (media.Previews, error) {
	return media.Previews{}, errors.ErrUnsupported
}

func (noMedia) Subtitles(context.Context, media.SubtitlesRequest) error {
	return errors.ErrUnsupported
}

// noPreviews and noSubtitles drop what a probe asks of previews and
// subtitles.
type (
	noPreviews  struct{}
	noSubtitles struct{}
)

func (noPreviews) Probed(*store.Tx, catalog.Library, id.ID)                   {}
func (noSubtitles) Probed(*store.Tx, catalog.Library, id.ID, []catalog.Track) {}

// fakeTasks records the items asked for, by task.
type fakeTasks struct {
	mu    sync.Mutex
	asked map[string][]id.ID
}

func (f *fakeTasks) Enqueue(task string, items ...id.ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.asked == nil {
		f.asked = map[string][]id.ID{}
	}
	f.asked[task] = append(f.asked[task], items...)
}

// EnqueueFor records the items asked for task, as Enqueue does.
func (f *fakeTasks) EnqueueFor(_ id.ID, task string, items ...id.ID) { f.Enqueue(task, items...) }

// of returns the items asked for task, in order.
func (f *fakeTasks) of(task string) []id.ID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked[task])
}

func (f *fakeTasks) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = nil
}

type harness struct {
	t          *testing.T
	b          *catalogtest.Builder
	settings   *config.Store
	tasks      *fakeTasks
	s          *Scanner
	prob       *fakeProber
	folders    map[string]*memFS
	down       map[string]bool // folder paths that fail to open
	mu         sync.Mutex
	identified []id.ID // the libraries Scanned was called for
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, b: catalogtest.New(t), tasks: &fakeTasks{}, prob: &fakeProber{}, folders: map[string]*memFS{}, down: map[string]bool{}}
	settings, err := config.Open(ctx, h.b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	h.settings = settings
	h.s = New(h.b.DB, Deps{
		Catalog: h.b.Catalog, Tasks: h.tasks, Prober: h.prob, Settings: settings, Identifier: h, Previews: noPreviews{}, Subtitles: noSubtitles{},
	}, Options{
		OpenFolder: func(p string) (fs.FS, func() error, error) {
			p = filepath.ToSlash(strings.TrimPrefix(p, filepath.VolumeName(p))) // as the test named it
			if h.down[p] || h.folders[p] == nil {
				return nil, nil, errors.New("no such device")
			}
			return h.folders[p], func() error { return nil }, nil
		},
	})
	return h
}

// Scanned stands in for the identify package's Scanned and the task it asks.
// It records every file of lib that is probed but not identified as
// identified at its generation.
func (h *harness) Scanned(ctx context.Context, lib catalog.Library) error {
	h.mu.Lock()
	h.identified = append(h.identified, lib.ID)
	h.mu.Unlock()
	rows, err := h.b.DB.Reader().QueryContext(ctx, `SELECT f.id, f.rel_path, f.generation FROM media_file f JOIN library_folder r ON r.id = f.folder_id
		WHERE r.library_id = ? AND f.status = 'present' AND f.probed = f.generation AND f.probe_error IS NULL AND f.identified IS NOT f.generation`, lib.ID)
	if err != nil {
		return err
	}
	var files []catalog.FileState
	for rows.Next() {
		var f catalog.FileState
		if err := rows.Scan(&f.ID, &f.RelPath, &f.Generation); err != nil {
			rows.Close()
			return err
		}
		files = append(files, f)
	}
	rows.Close()
	return h.b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		for _, f := range files {
			if _, err := tx.ExecContext(ctx, `UPDATE media_file SET identified = generation WHERE id = ? AND generation = ?`, f.ID, f.Generation); err != nil {
				return err
			}
		}
		return nil
	})
}

// Reidentify stands in for the identify package's Reidentify. It records
// files as not identified.
func (h *harness) Reidentify(ctx context.Context, tx *store.Tx, files []id.ID) error {
	for _, f := range files {
		if _, err := tx.ExecContext(ctx, `UPDATE media_file SET identified = NULL WHERE id = ?`, f); err != nil {
			return err
		}
	}
	return nil
}

// library adds a library over the given folder paths, each backed by a memFS.
// The folders are named /like/this, and made absolute for the OS.
func (h *harness) library(kind catalog.LibraryKind, paths ...string) catalog.Library {
	h.t.Helper()
	abs := make([]string, len(paths))
	for i, p := range paths {
		if h.folders[p] == nil {
			h.folders[p] = newMemFS()
		}
		abs[i] = catalogtest.Abs(p)
	}
	lib, err := h.s.AddLibrary(context.Background(), catalog.LibraryInput{Name: "L " + paths[0], Kind: kind, Folders: abs})
	if err != nil {
		h.t.Fatal(err)
	}
	l, err := h.b.Catalog.Library(context.Background(), lib)
	if err != nil {
		h.t.Fatal(err)
	}
	return l
}

// scan walks the whole of folder, without probing.
func (h *harness) scan(folder id.ID) error {
	return h.s.scanFolder(context.Background(), folder, ".")
}

func (h *harness) mustScan(folder id.ID) {
	h.t.Helper()
	if err := h.scan(folder); err != nil {
		h.t.Fatal(err)
	}
}

// scanDir walks one directory of folder and what changed below it.
func (h *harness) scanDir(folder id.ID, dir string) {
	h.t.Helper()
	if err := h.s.scanFolder(context.Background(), folder, dir); err != nil {
		h.t.Fatal(err)
	}
}

// scanLibrary does one item of the scan task: the library's walk, its
// probes and its identify step.
func (h *harness) scanLibrary(lib id.ID) {
	h.t.Helper()
	if err := h.s.scan(context.Background(), lib); err != nil {
		h.t.Fatal(err)
	}
}

// probeAll probes every library's files not probed at their generation, as
// a library's scan does after its walk.
func (h *harness) probeAll() {
	h.t.Helper()
	libs, err := h.b.Catalog.Libraries(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	for _, l := range libs {
		if err := h.s.probeAll(context.Background(), l); err != nil {
			h.t.Fatal(err)
		}
	}
}

// unprobed lists a library's files left to probe.
func (h *harness) unprobed(lib id.ID) []id.ID {
	h.t.Helper()
	files, err := h.s.q.Unprobed(context.Background(), lib)
	if err != nil {
		h.t.Fatal(err)
	}
	return files
}

// unidentified lists, by path, a folder's files that are probed but not
// identified at their generation. Those are the files the library's scan
// is to identify.
func (h *harness) unidentified(folder id.ID) []id.ID {
	h.t.Helper()
	return h.ids(`SELECT id FROM media_file WHERE folder_id = ? AND status = 'present'
		AND probed = generation AND probe_error IS NULL AND identified IS NOT generation ORDER BY rel_path`, folder)
}

// identifyAll records every probed file as identified, as identify would.
func (h *harness) identifyAll() {
	h.t.Helper()
	h.exec(`UPDATE media_file SET identified = generation WHERE probed = generation AND probe_error IS NULL`)
}

// remove does one item of catalog's library-changes task, as the runner
// would.
func (h *harness) remove(item id.ID) {
	h.t.Helper()
	for _, t := range h.b.Catalog.Tasks() {
		if t.Name == catalog.TaskLibraryChanges {
			if err := t.Work.(tasks.Items).Do(context.Background(), item); err != nil {
				h.t.Fatal(err)
			}
			return
		}
	}
	h.t.Fatalf("no task %s", catalog.TaskLibraryChanges)
}

func (h *harness) ids(q string, args ...any) []id.ID {
	h.t.Helper()
	rows, err := h.b.DB.Reader().Query(q, args...)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var out []id.ID
	for rows.Next() {
		var s id.ID
		if err := rows.Scan(&s); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// sortIDs sorts ids in place, for comparing sets.
func sortIDs(ids []id.ID) []id.ID {
	slices.SortFunc(ids, func(a, b id.ID) int { return strings.Compare(a.String(), b.String()) })
	return ids
}

func (h *harness) exec(q string, args ...any) {
	h.t.Helper()
	err := h.b.DB.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

// files returns the folder's files by path, with presence.
func (h *harness) files(folder id.ID) map[string]catalog.FileRef {
	h.t.Helper()
	fs, err := h.b.Catalog.FilesUnder(context.Background(), folder, "")
	if err != nil {
		h.t.Fatal(err)
	}
	out := map[string]catalog.FileRef{}
	for _, f := range fs {
		out[f.RelPath] = f
	}
	return out
}

var old = time.Now().Add(-24 * time.Hour) // settled files
