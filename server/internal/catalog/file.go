package catalog

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

// File is a media file with its probe results.
type File struct {
	ID, Folder, Library id.ID
	FolderPath, RelPath string
	Size                int64
	MTime               time.Time
	Generation          int64
	Present             bool
	Probe               ProbeSummary
	Tracks              []Track        // embedded tracks, ordered by stream index
	SubtitleFiles       []SubtitleFile // external subtitles, then downloaded ones, each ordered by path
	Chapters            []Chapter
	Attachments         []Attachment
}

// Path is where the file is on disk.
func (f File) Path() string {
	return filepath.Join(f.FolderPath, filepath.FromSlash(f.RelPath))
}

// FileStatus is whether a file is at its path, as the last scan found it.
type FileStatus string

// The file statuses.
const (
	FilePresent FileStatus = "present"
	FileGone    FileStatus = "gone" // a scan didn't find it; kept until it returns or is pruned
)

// ProbeSummary is what probing a file found, besides tracks, chapters and
// attachments.
type ProbeSummary struct {
	Container string
	Duration  time.Duration
	Start     time.Duration // the first timestamp, which is zero for most files but often not for MPEG-TS
	Bitrate   int
	EdgeHash  []byte
	OSHash    uint64
	// Tags are the container's tags, keyed by media.ProbeResult's names.
	// Books are described from them.
	Tags map[string]string
}

// Probe is everything probing a file found, as SetProbe stores it.
type Probe struct {
	Summary     ProbeSummary
	Tracks      []Track // embedded tracks, ordered by stream index
	Chapters    []Chapter
	Attachments []Attachment
}

// Track is one audio, video or subtitle track embedded in a file.
type Track struct {
	ID              id.ID  // what clients choose the track by
	Index           int    // the container's stream index
	Type            string // video, audio or subtitle
	Codec           string
	Language        string
	Title           string
	Default         bool
	Forced          bool
	HearingImpaired bool
	Info            json.RawMessage // everything else the probe found, as media.Track's JSON
}

// SubtitleFile is a subtitle file outside the media: an external one next
// to the video, or one downloaded for it.
type SubtitleFile struct {
	ID              id.ID  // what clients choose the track by
	RelPath         string // relative to the library folder, or to data/subtitles/ for a downloaded subtitle
	Format          string // the file's extension: srt, ass, ssa, vtt, sup or idx
	Language        string // a BCP 47 tag, when the file name gives one
	Title           string
	Default         bool
	Forced          bool
	HearingImpaired bool
	Downloaded      bool // kept in data/subtitles/
}

// Track sources, as media_track.source records them.
const (
	SourceEmbedded   = "embedded"
	SourceExternal   = "external" // a subtitle file next to the video in the library
	SourceDownloaded = "downloaded"
)

// Chapter is one chapter of a file.
type Chapter struct {
	Index      int
	Start, End time.Duration
	Title      string
}

// Attachment is a file a container carries besides its tracks, such as a
// font for its styled subtitles.
type Attachment struct {
	Kind     AttachmentKind // judged from its bytes
	Filename string
	MimeType string // as the container declares it
	Size     int64
	SHA256   string // lowercase hex
}

// AttachmentKind is what an attachment holds.
type AttachmentKind string

// The attachment kinds.
const (
	AttachmentFont  AttachmentKind = "font"
	AttachmentImage AttachmentKind = "image"
	AttachmentOther AttachmentKind = "other"
)

// FileObservation is what a scan saw at a path.
type FileObservation struct {
	RelPath string
	Size    int64
	MTime   time.Time
}

// FileState is a file after UpsertFiles. Changed means the file is new or
// its content changed, so it needs probing at this generation.
type FileState struct {
	ID         id.ID
	RelPath    string
	Generation int64
	Changed    bool
}

// FileRef is a file's identity and presence, which a scan uses to mark
// files gone.
type FileRef struct {
	ID      id.ID
	RelPath string
	Present bool
}

// UpsertFiles records what a scan saw under a folder. A new file, or a file
// whose size or modification time changed, gets a new generation and comes
// back Changed so it is probed. A gone file seen again at its path becomes
// present again. UpsertFiles returns ErrNotFound for a folder that is being
// removed or whose library is being deleted.
func (c *Catalog) UpsertFiles(ctx context.Context, tx *store.Tx, folder id.ID, obs []FileObservation) ([]FileState, error) {
	q := dbq.New(tx)
	if active, err := q.FolderActive(ctx, folder); err != nil || active == 0 {
		return nil, notFound(cmp.Or(err, sql.ErrNoRows))
	}
	out := make([]FileState, 0, len(obs))
	var returned []id.ID
	for _, o := range obs {
		mtime := o.MTime.UnixMilli()
		cur, err := q.FileByPath(ctx, dbq.FileByPathParams{FolderID: folder, RelPath: o.RelPath})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			f := id.New()
			err := q.InsertFile(ctx, dbq.InsertFileParams{ID: f, FolderID: folder, RelPath: o.RelPath, Size: o.Size, Mtime: mtime})
			if err != nil {
				return nil, fmt.Errorf("record %s: %w", o.RelPath, err)
			}
			out = append(out, FileState{ID: f, RelPath: o.RelPath, Generation: 1, Changed: true})
			continue
		case err != nil:
			return nil, fmt.Errorf("record %s: %w", o.RelPath, err)
		}
		changed := cur.Size != o.Size || cur.Mtime != mtime
		gen := cur.Generation
		if changed || FileStatus(cur.Status) == FileGone {
			gen, err = q.UpdateFileStat(ctx, dbq.UpdateFileStatParams{ID: cur.ID, Size: o.Size, Mtime: mtime, Bump: changed})
			if err != nil {
				return nil, fmt.Errorf("record %s: %w", o.RelPath, err)
			}
			if FileStatus(cur.Status) == FileGone {
				returned = append(returned, cur.ID)
			}
		}
		out = append(out, FileState{ID: cur.ID, RelPath: o.RelPath, Generation: gen, Changed: changed})
	}
	if err := c.refreshFiles(ctx, tx, returned); err != nil {
		return nil, err
	}
	return out, nil
}

// MarkGone hides files that a scan found missing from folder. Their rows
// stay until deleted, so a file that moved or comes back keeps everything
// attached to it. A file is marked only while it's still present at the
// path it was missing from. A file that a probe has meanwhile moved to its
// new path stays present.
func (c *Catalog) MarkGone(ctx context.Context, tx *store.Tx, folder id.ID, files []FileRef) error {
	q := dbq.New(tx)
	now := sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true}
	var gone []id.ID
	for _, f := range files {
		n, err := q.MarkGone(ctx, dbq.MarkGoneParams{Now: now, ID: f.ID, FolderID: folder, RelPath: f.RelPath})
		if err != nil {
			return err
		}
		if n > 0 {
			gone = append(gone, f.ID)
		}
	}
	return c.refreshFiles(ctx, tx, gone)
}

// DeleteFiles deletes files for good, with every version they're part of.
// The works stay until DeleteEmptyWorks finds them with nothing left.
func (c *Catalog) DeleteFiles(ctx context.Context, tx *store.Tx, files []id.ID) error {
	if len(files) == 0 {
		return nil
	}
	q := dbq.New(tx)
	versions, works, err := versionsOfFiles(ctx, q, files)
	if err != nil {
		return err
	}
	if err := deleteVersions(ctx, q, versions); err != nil {
		return err
	}
	if err := q.DeleteFiles(ctx, id.Set(files...)); err != nil {
		return err
	}
	return c.refreshLibraries(ctx, tx, works)
}

// DeleteMissing deletes one missing file at an admin's request, in its own
// write. Like DeleteFiles, it deletes every version the file is part of. It
// returns ErrPresent if the file is present again, even if the admin saw it
// missing.
func (c *Catalog) DeleteMissing(ctx context.Context, file id.ID) error {
	err := c.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		f, err := q.GetFile(ctx, file)
		if err != nil {
			return notFound(err)
		}
		if FileStatus(f.MediaFile.Status) == FilePresent {
			return fmt.Errorf("%w: file %s", ErrPresent, file)
		}
		return c.DeleteFiles(ctx, tx, []id.ID{file})
	})
	return err
}

// DeleteAllMissing deletes every file that is missing when an admin asks,
// as DeleteMissing does for one file. It writes one batch at a time. It
// deletes the files that went missing at or before the moment it's called,
// including that millisecond.
func (c *Catalog) DeleteAllMissing(ctx context.Context) error {
	now := time.Now()
	err := c.db.Drain(ctx, func(ctx context.Context, tx *store.Tx) (int, error) {
		return c.PruneGone(ctx, tx, now)
	})
	return err
}

// PruneGone deletes one batch of the files gone at or before t, as
// DeleteFiles does, and returns how many it deleted. It is a store.Drain
// step.
func (c *Catalog) PruneGone(ctx context.Context, tx *store.Tx, t time.Time) (int, error) {
	files, err := dbq.New(tx).GoneBatch(ctx, dbq.GoneBatchParams{Before: t.UnixMilli(), Limit: store.BatchSize})
	if err != nil {
		return 0, err
	}
	return len(files), c.DeleteFiles(ctx, tx, files)
}

// Relink merges a newly found file into a gone file with the same content.
// The gone file keeps its ID, and with it its versions and everything
// attached. It takes the found file's place on disk, and the found file's
// row is deleted.
func (c *Catalog) Relink(ctx context.Context, tx *store.Tx, gone, found id.ID) error {
	q := dbq.New(tx)
	g, err := q.GetFile(ctx, gone)
	if err != nil {
		return notFound(err)
	}
	f, err := q.GetFile(ctx, found)
	if err != nil {
		return notFound(err)
	}
	v, err := q.VersionsOfFiles(ctx, id.Set(found))
	if err != nil {
		return err
	}
	if len(v) > 0 {
		return fmt.Errorf("%w: the found file already belongs to a version", ErrInvalid)
	}
	if f.LibraryID != g.LibraryID {
		// A version's parts stay in one library, so a move across libraries
		// must not carry versions with other parts along.
		parts, err := partsWith(ctx, q, gone)
		if err != nil {
			return err
		}
		if parts > 1 {
			return fmt.Errorf("%w: a multi-part version can't move to another library", ErrInvalid)
		}
	}
	if err := q.DeleteFiles(ctx, id.Set(found)); err != nil {
		return err
	}
	err = q.MoveFile(ctx, dbq.MoveFileParams{ID: gone, FolderID: f.MediaFile.FolderID, RelPath: f.MediaFile.RelPath, Size: f.MediaFile.Size, Mtime: f.MediaFile.Mtime})
	if err != nil {
		return err
	}
	return c.refreshFiles(ctx, tx, []id.ID{gone})
}

// partsWith counts the parts of the version a file belongs to.
func partsWith(ctx context.Context, q *dbq.Queries, file id.ID) (int, error) {
	vs, err := q.VersionsOfFiles(ctx, id.Set(file))
	if err != nil || len(vs) == 0 {
		return 0, err
	}
	parts, err := q.VersionParts(ctx, vs[0])
	return len(parts), err
}

// SetProbe stores a file's probe results if the file is still at the
// generation that was probed. Otherwise it changes nothing and returns
// ErrStale. It replaces the file's embedded tracks, chapters and
// attachments. An embedded track keeps its ID when the file had a track of
// the same type and codec at the same stream index, so re-probing an
// unchanged file keeps every ID a client holds.
func (c *Catalog) SetProbe(ctx context.Context, tx *store.Tx, file id.ID, gen int64, p Probe) error {
	q := dbq.New(tx)
	if err := setSummary(ctx, q, file, gen, p.Summary); err != nil {
		return err
	}
	if err := setTracks(ctx, q, file, p.Tracks); err != nil {
		return err
	}
	if err := setChapters(ctx, q, file, p.Chapters); err != nil {
		return err
	}
	if err := setAttachments(ctx, q, file, p.Attachments); err != nil {
		return err
	}
	return c.refreshFiles(ctx, tx, []id.ID{file}) // work_library holds copies of the runtime and video keys
}

// setSummary stores a file's probe summary if it's still at generation gen.
func setSummary(ctx context.Context, q *dbq.Queries, file id.ID, gen int64, p ProbeSummary) error {
	n, err := q.SetProbe(ctx, dbq.SetProbeParams{
		ID: file, Generation: gen, Container: nullString(p.Container),
		DurationMs: sql.NullInt64{Int64: p.Duration.Milliseconds(), Valid: p.Duration > 0},
		Bitrate:    nullInt(p.Bitrate), StartMs: p.Start.Milliseconds(), EdgeHash: p.EdgeHash,
		OsHash: sql.NullInt64{Int64: int64(p.OSHash), Valid: p.OSHash != 0}, Tags: tagsJSON(p.Tags),
	})
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := q.GetFile(ctx, file); err != nil {
		return notFound(err)
	}
	return fmt.Errorf("%w: file %s", ErrStale, file)
}

// setTracks replaces a file's embedded tracks. A track found again with the
// same type and codec at its stream index keeps its ID.
func setTracks(ctx context.Context, q *dbq.Queries, file id.ID, tracks []Track) error {
	old, err := q.EmbeddedTracks(ctx, file)
	if err != nil {
		return err
	}
	type key struct {
		index      int64
		typ, codec string
	}
	keep := map[key]id.ID{}
	for _, t := range tracks {
		keep[key{int64(t.Index), t.Type, t.Codec}] = id.ID{}
	}
	var stale []id.ID
	for _, o := range old {
		k := key{o.StreamIndex.Int64, o.Type, o.Codec}
		if _, ok := keep[k]; ok {
			keep[k] = o.ID
		} else {
			stale = append(stale, o.ID)
		}
	}
	if len(stale) > 0 {
		if err := q.DeleteTracks(ctx, id.Set(stale...)); err != nil {
			return err
		}
	}
	for _, t := range tracks {
		info := nullString(string(t.Info))
		if same := keep[key{int64(t.Index), t.Type, t.Codec}]; !same.IsZero() {
			err = q.UpdateEmbeddedTrack(ctx, dbq.UpdateEmbeddedTrackParams{
				ID: same, Language: nullString(t.Language), Title: nullString(t.Title), IsDefault: t.Default,
				Forced: t.Forced, HearingImpaired: t.HearingImpaired, Info: info,
			})
		} else {
			err = q.InsertEmbeddedTrack(ctx, dbq.InsertEmbeddedTrackParams{
				ID: id.New(), FileID: file, StreamIndex: sql.NullInt64{Int64: int64(t.Index), Valid: true}, Type: t.Type,
				Codec: t.Codec, Language: nullString(t.Language), Title: nullString(t.Title), IsDefault: t.Default,
				Forced: t.Forced, HearingImpaired: t.HearingImpaired, Info: info,
			})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func setChapters(ctx context.Context, q *dbq.Queries, file id.ID, chapters []Chapter) error {
	if err := q.DeleteChapters(ctx, file); err != nil {
		return err
	}
	for _, ch := range chapters {
		err := q.InsertChapter(ctx, dbq.InsertChapterParams{
			FileID: file, Idx: int64(ch.Index), StartMs: ch.Start.Milliseconds(), EndMs: ch.End.Milliseconds(),
			Title: nullString(ch.Title),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func setAttachments(ctx context.Context, q *dbq.Queries, file id.ID, attachments []Attachment) error {
	if err := q.DeleteAttachments(ctx, file); err != nil {
		return err
	}
	for i, a := range attachments {
		err := q.InsertAttachment(ctx, dbq.InsertAttachmentParams{
			FileID: file, Idx: int64(i), Kind: string(a.Kind), Filename: a.Filename, MimeType: nullString(a.MimeType),
			Size: a.Size, Sha256: a.SHA256,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// File returns a file with its probe results.
func (c *Catalog) File(ctx context.Context, file id.ID) (File, error) {
	row, err := c.q.GetFile(ctx, file)
	if err != nil {
		return File{}, notFound(err)
	}
	f := fileFrom(row.MediaFile, row.FolderPath, row.LibraryID)
	tracks, err := c.q.ListTracks(ctx, file)
	if err != nil {
		return File{}, err
	}
	for _, t := range tracks {
		if t.Source != SourceEmbedded {
			f.SubtitleFiles = append(f.SubtitleFiles, SubtitleFile{
				ID: t.ID, RelPath: t.RelPath.String, Format: SubtitleFormat(t.RelPath.String),
				Language: t.Language.String, Title: t.Title.String, Default: t.IsDefault, Forced: t.Forced,
				HearingImpaired: t.HearingImpaired, Downloaded: t.Source == SourceDownloaded,
			})
			continue
		}
		f.Tracks = append(f.Tracks, Track{
			ID: t.ID, Index: int(t.StreamIndex.Int64), Type: t.Type, Codec: t.Codec, Language: t.Language.String,
			Title: t.Title.String, Default: t.IsDefault, Forced: t.Forced, HearingImpaired: t.HearingImpaired,
			Info: []byte(t.Info.String),
		})
	}
	chapters, err := c.q.ListChapters(ctx, file)
	if err != nil {
		return File{}, err
	}
	for _, ch := range chapters {
		f.Chapters = append(f.Chapters, Chapter{
			Index: int(ch.Idx), Start: time.Duration(ch.StartMs) * time.Millisecond,
			End: time.Duration(ch.EndMs) * time.Millisecond, Title: ch.Title.String,
		})
	}
	attachments, err := c.q.ListAttachments(ctx, file)
	if err != nil {
		return File{}, err
	}
	for _, a := range attachments {
		f.Attachments = append(f.Attachments, Attachment{
			Kind: AttachmentKind(a.Kind), Filename: a.Filename, MimeType: a.MimeType.String, Size: a.Size, SHA256: a.Sha256,
		})
	}
	return f, nil
}

// Generation cheaply reads a file's current generation. It returns
// ErrNotFound when the file has no row.
func (c *Catalog) Generation(ctx context.Context, file id.ID) (int64, error) {
	gen, err := c.q.FileGeneration(ctx, file)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: file %s", ErrNotFound, file)
	}
	return gen, err
}

// Generations reads the current generation of each given file that still
// has a row.
func (c *Catalog) Generations(ctx context.Context, files []id.ID) (map[id.ID]int64, error) {
	rows, err := c.q.FileGenerations(ctx, id.Set(files...))
	gens := make(map[id.ID]int64, len(rows))
	for _, r := range rows {
		gens[r.ID] = r.Generation
	}
	return gens, err
}

// FileAt returns the file at an absolute path in a library folder, whether
// present or gone. It returns ErrNotFound when no library has a file there.
func (c *Catalog) FileAt(ctx context.Context, abs string) (File, error) {
	libs, err := c.Libraries(ctx)
	if err != nil {
		return File{}, err
	}
	for _, l := range libs {
		for _, folder := range l.Folders {
			rel, ok := strings.CutPrefix(abs, strings.TrimSuffix(folder.Path, "/")+"/")
			if !ok {
				continue
			}
			row, err := c.q.FileByPath(ctx, dbq.FileByPathParams{FolderID: folder.ID, RelPath: rel})
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return File{}, err
			}
			return c.File(ctx, row.ID)
		}
	}
	return File{}, fmt.Errorf("%w: %s", ErrNotFound, abs)
}

// FilesByHash returns the files with this size and edge hash, whether
// present or gone. These are the files a moved file may have been. Tracks,
// chapters and attachments aren't loaded.
func (c *Catalog) FilesByHash(ctx context.Context, size int64, edgeHash []byte) ([]File, error) {
	rows, err := c.q.FilesByHash(ctx, dbq.FilesByHashParams{Size: size, EdgeHash: edgeHash})
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(rows))
	for _, r := range rows {
		out = append(out, fileFrom(r.MediaFile, r.FolderPath, r.LibraryID))
	}
	return out, nil
}

func fileFrom(r dbq.MediaFile, folderPath string, lib id.ID) File {
	return File{
		ID: r.ID, Folder: r.FolderID, Library: lib, FolderPath: folderPath, RelPath: r.RelPath,
		Size: r.Size, MTime: time.UnixMilli(r.Mtime), Generation: r.Generation, Present: FileStatus(r.Status) == FilePresent,
		Probe: ProbeSummary{
			Container: r.Container.String, Duration: time.Duration(r.DurationMs.Int64) * time.Millisecond,
			Start: time.Duration(r.StartMs) * time.Millisecond, Bitrate: int(r.Bitrate.Int64), EdgeHash: r.EdgeHash, OSHash: uint64(r.OsHash.Int64),
			Tags: tagsOf(r.Tags),
		},
	}
}

// tagsJSON returns a file's tags as they are stored: a JSON object, or NULL
// when there are none.
func tagsJSON(tags map[string]string) sql.NullString {
	if len(tags) == 0 {
		return sql.NullString{}
	}
	b, _ := json.Marshal(tags)
	return sql.NullString{String: string(b), Valid: true}
}

func tagsOf(s sql.NullString) map[string]string {
	var out map[string]string
	if s.Valid {
		_ = json.Unmarshal([]byte(s.String), &out) // written by tagsJSON
	}
	return out
}

// FilesUnder returns the files at or below dir, whether present or gone,
// ordered by path. dir is relative to the folder, and "" or "." means the
// whole folder.
func (c *Catalog) FilesUnder(ctx context.Context, folder id.ID, dir string) ([]FileRef, error) {
	if dir == "." {
		dir = ""
	}
	rows, err := c.q.FilesUnder(ctx, dbq.FilesUnderParams{FolderID: folder, Dir: dir})
	if err != nil {
		return nil, err
	}
	return fileRefs(rows), nil
}

// FilesIn returns the files directly in dir, whether present or gone,
// ordered by path. dir is relative to the folder, and "" or "." means its
// top level.
func (c *Catalog) FilesIn(ctx context.Context, folder id.ID, dir string) ([]FileRef, error) {
	if dir == "." {
		dir = ""
	}
	rows, err := c.q.FilesIn(ctx, dbq.FilesInParams{FolderID: folder, Dir: dir})
	if err != nil {
		return nil, err
	}
	return fileRefs(rows), nil
}

// fileRefRow is the row type that the FilesUnder and FilesIn queries return.
type fileRefRow = struct {
	ID      id.ID
	RelPath string
	Status  string
}

func fileRefs[R ~fileRefRow](rows []R) []FileRef {
	out := make([]FileRef, len(rows))
	for i, r := range rows {
		r := fileRefRow(r)
		out[i] = FileRef{ID: r.ID, RelPath: r.RelPath, Present: FileStatus(r.Status) == FilePresent}
	}
	return out
}

// AttachmentLibraries returns the libraries holding a file that carries an
// attachment with this SHA-256, in lowercase hex. It decides who may fetch
// the attachment.
func (c *Catalog) AttachmentLibraries(ctx context.Context, sha256 string) ([]id.ID, error) {
	return c.q.AttachmentLibraries(ctx, sha256)
}

// SetExternalSubtitles sets a video's external subtitles, the subtitle
// files next to it. One still at its path keeps its track ID. Downloaded
// subtitles are left alone.
func (c *Catalog) SetExternalSubtitles(ctx context.Context, tx *store.Tx, file id.ID, tracks []SubtitleFile) error {
	q := dbq.New(tx)
	cur, err := q.ListTracks(ctx, file)
	if err != nil {
		return err
	}
	found := map[string]bool{}
	for _, t := range tracks {
		found[t.RelPath] = true
	}
	var gone []id.ID
	for _, t := range cur {
		if t.Source == SourceExternal && !found[t.RelPath.String] {
			gone = append(gone, t.ID)
		}
	}
	if len(gone) > 0 {
		if err := q.DeleteTracks(ctx, id.Set(gone...)); err != nil {
			return err
		}
	}
	for _, t := range tracks {
		t.Downloaded = false
		if _, err := putSubtitleFile(ctx, q, file, t); err != nil {
			return err
		}
	}
	return nil
}

// AddDownloadedSubtitle attaches a subtitle downloaded for a video, stored
// at t.RelPath under data/subtitles/, and returns its track ID.
func (c *Catalog) AddDownloadedSubtitle(ctx context.Context, tx *store.Tx, file id.ID, t SubtitleFile) (id.ID, error) {
	t.Downloaded = true
	return putSubtitleFile(ctx, dbq.New(tx), file, t)
}

// DropReplacedSubtitles removes the downloaded subtitles of a video that
// the subtitle keep replaces. Those are the ones in the same language and
// with the same forced flag. It returns them so their files can be deleted
// too.
func (c *Catalog) DropReplacedSubtitles(ctx context.Context, tx *store.Tx, file, keep id.ID) ([]SubtitleFile, error) {
	q := dbq.New(tx)
	cur, err := q.ListTracks(ctx, file)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(cur, func(t dbq.MediaTrack) bool { return t.ID == keep && t.Source == SourceDownloaded })
	if i < 0 {
		return nil, fmt.Errorf("%w: downloaded subtitle %s", ErrNotFound, keep)
	}
	k := cur[i]
	var out []SubtitleFile
	var gone []id.ID
	for _, t := range cur {
		if t.Source == SourceDownloaded && t.ID != keep && t.Language == k.Language && t.Forced == k.Forced {
			out = append(out, SubtitleFile{ID: t.ID, RelPath: t.RelPath.String, Language: t.Language.String, Title: t.Title.String,
				Forced: t.Forced, HearingImpaired: t.HearingImpaired, Downloaded: true})
			gone = append(gone, t.ID)
		}
	}
	if len(gone) > 0 {
		if err := q.DeleteTracks(ctx, id.Set(gone...)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func putSubtitleFile(ctx context.Context, q *dbq.Queries, file id.ID, t SubtitleFile) (id.ID, error) {
	source := SourceExternal
	if t.Downloaded {
		source = SourceDownloaded
	}
	got, err := q.PutSubtitleFile(ctx, dbq.PutSubtitleFileParams{
		ID: id.New(), FileID: file, Source: source, RelPath: nullString(t.RelPath), Codec: SubtitleCodec(t.Format),
		Language: nullString(t.Language), Title: nullString(t.Title), IsDefault: t.Default,
		Forced: t.Forced, HearingImpaired: t.HearingImpaired,
	})
	if err != nil {
		return id.ID{}, err
	}
	return got, nil
}

// SubtitleFormat returns the format of the subtitle file at a path, which
// is its extension in lowercase without the dot.
func SubtitleFormat(p string) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(p)), ".")
}

// SubtitleCodec returns the codec name a probe would give a subtitle
// format, such as subrip for srt.
func SubtitleCodec(format string) string {
	if c, ok := subtitleCodecs[format]; ok {
		return c
	}
	return format
}

var subtitleCodecs = map[string]string{
	"srt": "subrip", "vtt": "webvtt", "ass": "ass", "ssa": "ass", "sup": "hdmv_pgs_subtitle", "idx": "dvd_subtitle",
}
