package scan

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog"
)

// files are the names a library of a given kind catalogues: its media, and
// in a video library also subtitle files and the files local metadata
// comes from.
type files struct {
	media, subtitles, metadata map[string]bool
}

func filesOf(kind catalog.LibraryKind) files {
	switch kind {
	case catalog.Audiobooks:
		return files{media: audioExt, metadata: imageExt}
	default:
		return files{media: videoExt, subtitles: subtitleExt, metadata: metadataExt}
	}
}

func (f files) isMedia(name string) bool    { return f.media[ext(name)] }
func (f files) isSubtitle(name string) bool { return f.subtitles[ext(name)] }
func (f files) isMetadata(name string) bool { return f.metadata[ext(name)] }

var videoExt = setOf(".mkv", ".mp4", ".m4v", ".avi", ".mov", ".wmv", ".ts", ".m2ts", ".mts", ".webm", ".mpg", ".mpeg", ".flv", ".ogv", ".iso", ".vob")

// The audio files an audiobooks library holds instead.
var audioExt = setOf(".m4b", ".m4a", ".mp3", ".aac", ".flac", ".ogg", ".opus")

// Subtitle files next to a video. A .sub is the data half of an .idx pair,
// so only the .idx counts.
var subtitleExt = setOf(".srt", ".ass", ".ssa", ".vtt", ".sup", ".idx")

// Files whose change sends a folder back to identification: NFO metadata
// and local artwork. A book's folder counts only artwork. Its NFO files are
// rip reports, not metadata, and its CUE files repeat the chapters its file
// already has.
var (
	metadataExt = setOf(".nfo", ".jpg", ".jpeg", ".png", ".webp")
	imageExt    = setOf(".jpg", ".jpeg", ".png", ".webp")
)

func setOf(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func ext(name string) string { return strings.ToLower(path.Ext(name)) }

// skipName reports whether a name is never catalogued. That covers hidden
// files, which include the .bookarr- files a downloader leaves while it
// replaces a book, and NAS bookkeeping folders.
func skipName(name string) bool {
	switch strings.ToLower(name) {
	case "@eadir", "#recycle", "$recycle.bin", "lost+found", "#snapshot", ".streams":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// ignoreFile marks a folder that is left out of the library, with
// everything below it.
const ignoreFile = ".ignore"

func ignored(entries []fs.DirEntry) bool {
	return slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return e.Name() == ignoreFile && !e.IsDir() })
}
