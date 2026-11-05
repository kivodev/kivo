package detect

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// exportMark is one of an export's markers on an episode.
type exportMark struct {
	kind  string
	s     span
	final bool
}

// export is another server's episodes and their markers, as exported to a
// file named for the server. It holds the server's files, by key, and its
// markers, by episode.
type export struct {
	name     string
	files    map[string]int // a file's key: its episode's ID
	episodes map[int]bool
	marks    map[int][]exportMark
}

// loadExport reads an export. Its lines are tab-separated, either
// "F, episode ID, file path" or
// "M, episode ID, intro or credits, start ms, end ms, 1 if final".
// It returns nil when the file doesn't exist.
func loadExport(t *testing.T, path string) *export {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	ex := &export{name: strings.ToUpper(name[:1]) + name[1:], files: map[string]int{}, episodes: map[int]bool{}, marks: map[int][]exportMark{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		c := strings.Split(sc.Text(), "\t")
		if len(c) < 3 {
			continue
		}
		ep, _ := strconv.Atoi(c[1])
		switch {
		case c[0] == "F":
			ex.files[fileKey(c[2])] = ep
			ex.episodes[ep] = true
		case c[0] == "M" && len(c) >= 6:
			from, _ := strconv.Atoi(c[3])
			to, _ := strconv.Atoi(c[4])
			ex.marks[ep] = append(ex.marks[ep], exportMark{kind: c[2], s: span{time.Duration(from) * time.Millisecond, time.Duration(to) * time.Millisecond}, final: c[5] == "1"})
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return ex
}

// fileKey returns a path's last three parts: series, season and file. They
// are the same wherever a server mounts the share.
func fileKey(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	return strings.Join(parts[max(0, len(parts)-3):], "/")
}

// guesses returns an export's markers for the file at path. The intro is
// the first intro marker. The credits are the final credits marker, or
// else the last one. The other credits markers are returned too.
func (ex *export) guesses(path string) (intro, credits *span, others []span, known bool) {
	ep, ok := ex.files[fileKey(path)]
	if !ok {
		return nil, nil, nil, false
	}
	var cs []exportMark
	for _, m := range ex.marks[ep] {
		switch m.kind {
		case Intro:
			if intro == nil || m.s.start < intro.start {
				s := m.s
				intro = &s
			}
		case Credits:
			cs = append(cs, m)
		}
	}
	slices.SortFunc(cs, func(a, b exportMark) int { return int(a.s.start - b.s.start) })
	pick := len(cs) - 1
	for i, m := range cs {
		if m.final {
			pick = i
		}
	}
	for i, m := range cs {
		if i == pick {
			s := m.s
			credits = &s
		} else {
			others = append(others, m.s)
		}
	}
	return intro, credits, others, true
}

// srcKivo names Kivo's markers among the sources compared.
const srcKivo = "Kivo"

// row is one eligible episode file and each source's markers on it.
type row struct {
	name     string
	path     string
	duration time.Duration
	truth    map[string]span
	trusted  map[string]bool
	guess    map[string]map[string]*span // source: kind: span
	pics     map[string]*pics            // kind: the pictures where it is
	listed   bool                        // the export lists the file
	others   []span                      // the export's other credits markers
}

// compareSources scores each source's intros and credits on the same
// eligible files. It then scores them again on the files where Kivo and
// the export both have a marker.
func compareSources(w *strings.Builder, all []harnessSeason, ex *export) {
	sources := []string{srcKivo}
	if ex != nil {
		sources = append(sources, ex.name)
	}
	var rows []row
	skipped := 0
	for _, s := range all {
		truths, found := s.truths(), Decide(s.files())
		for i, f := range s.Files {
			if !eligible(f) {
				skipped++
				continue
			}
			r := row{
				name: fmt.Sprintf("%s E%d", s.Name, f.Number), path: f.Path, duration: f.Duration, truth: truths[i],
				trusted: map[string]bool{}, guess: map[string]map[string]*span{},
				pics: map[string]*pics{Intro: headPics(f), Credits: tailPics(f)},
			}
			for _, k := range []string{Intro, Credits} {
				r.trusted[k] = trusted(truths, i, k)
			}
			r.guess[srcKivo] = map[string]*span{Intro: spanOf(found[f.ID], Intro), Credits: spanOf(found[f.ID], Credits)}
			if ex != nil {
				in, cr, others, known := ex.guesses(f.Path)
				r.guess[ex.name] = map[string]*span{Intro: in, Credits: cr}
				r.listed, r.others = known, others
			}
			rows = append(rows, r)
		}
	}
	fmt.Fprintf(w, "SOURCES ON THE SAME EPISODES (%d whole-episode files; %d multi-part or multi-episode files left out)\n", len(rows), skipped)
	if ex != nil {
		n := 0
		for _, r := range rows {
			if !r.listed {
				n++
			}
		}
		fmt.Fprintf(w, "  files %s doesn't list: %d\n", ex.name, n)
	}
	for _, k := range []string{Intro, Credits} {
		fmt.Fprintf(w, "\n  %s, all:\n", k)
		scoreRows(w, rows, sources, k, func(row) bool { return true })
		if ex != nil {
			fmt.Fprintf(w, "\n  %s, Kivo and %s both have one, the chapters mark one:\n", k, ex.name)
			scoreRows(w, rows, []string{srcKivo, ex.name}, k, func(r row) bool {
				_, ok := r.truth[k]
				return ok && r.guess[srcKivo][k] != nil && r.guess[ex.name][k] != nil
			})
		}
	}
	if ex != nil {
		otherCredits(w, ex.name, rows)
	}
	fmt.Fprintln(w)
	if p := os.Getenv("KIVO_MARKERS_DUMP"); p != "" {
		dumpRows(p, rows, sources)
	}
}

// dumpRows writes each row's marks, in seconds, one line per kind. A line
// holds the name, path, duration, kind, the chapters' start and end, and
// then each source's start and end. A missing mark is left empty.
func dumpRows(path string, rows []row, sources []string) {
	var b strings.Builder
	sec := func(s *span) string {
		if s == nil {
			return "\t"
		}
		return fmt.Sprintf("%.2f\t%.2f", s.start.Seconds(), s.end.Seconds())
	}
	for _, r := range rows {
		for _, k := range []string{Intro, Credits} {
			var tr *span
			if t, ok := r.truth[k]; ok {
				tr = &t
			}
			fmt.Fprintf(&b, "%s\t%s\t%.2f\t%s\t%s", r.name, r.path, r.duration.Seconds(), k, sec(tr))
			for _, src := range sources {
				fmt.Fprintf(&b, "\t%s", sec(r.guess[src][k]))
			}
			b.WriteString("\n")
		}
	}
	_ = os.WriteFile(path, []byte(b.String()), 0o644)
}

// scoreRows writes each source's score over the rows that keep accepts,
// followed by its false skips.
func scoreRows(w *strings.Builder, rows []row, sources []string, kind string, keep func(row) bool) {
	scores := make([]score, len(sources))
	for _, r := range rows {
		if !keep(r) {
			continue
		}
		tr, ok := r.truth[kind]
		for i, src := range sources {
			scores[i].add(kind, r.guess[src][kind], tr, ok, r.trusted[kind], r.duration, r.name, r.pics[kind])
		}
	}
	for i, src := range sources {
		scores[i].write(w, src)
	}
	for i, src := range sources {
		for _, f := range scores[i].falses {
			fmt.Fprintf(w, "    %s %s\n", src, f)
		}
	}
}

// otherCredits reports the export's credits markers other than the one
// scored. It sorts them into those in the chapters' credits, those over no
// story, and those over story.
func otherCredits(w *strings.Builder, name string, rows []row) {
	var eps, n, inside, noStory, overStory int
	var lines []string
	for _, r := range rows {
		if len(r.others) == 0 {
			continue
		}
		eps++
		for _, o := range r.others {
			n++
			tr, ok := r.truth[Credits]
			p := r.pics[Credits]
			switch {
			case ok && o.start >= tr.start-3*time.Second && o.end <= tr.end+3*time.Second:
				inside++
			case p.noStory(o.start+time.Second, o.end-time.Second):
				noStory++
			default:
				overStory++
				lines = append(lines, fmt.Sprintf("%s: other credits %v-%v, chapters %v", r.name, o.start.Round(100*time.Millisecond), o.end.Round(100*time.Millisecond), tr))
			}
		}
	}
	fmt.Fprintf(w, "\n  %s's other credits markers: %d on %d files; in the chapters' credits %d, over no story %d, elsewhere %d\n", name, n, eps, inside, noStory, overStory)
	for _, l := range lines {
		fmt.Fprintf(w, "    %s\n", l)
	}
}

// libraryCoverage counts Kivo's episode files, the files whose chapters
// mark an intro or credits, and the files the export has markers for.
func libraryCoverage(w *strings.Builder, files []libraryFile, ex *export) {
	type counts struct{ files, chIntro, chCredits, listed, intro, credits, eitherIntro, eitherCredits int }
	var c counts
	for _, f := range files {
		c.files++
		ch := map[string]bool{}
		for _, k := range f.Kinds {
			ch[k] = true
		}
		if ch[Intro] {
			c.chIntro++
		}
		if ch[Credits] {
			c.chCredits++
		}
		in, cr, others, known := ex.guesses(f.Path)
		if known {
			c.listed++
		}
		pi, pc := in != nil, cr != nil || len(others) > 0
		if pi {
			c.intro++
		}
		if pc {
			c.credits++
		}
		if pi || ch[Intro] {
			c.eitherIntro++
		}
		if pc || ch[Credits] {
			c.eitherCredits++
		}
	}
	pIntro, pCredits := 0, 0
	for _, ms := range ex.marks {
		hasI := slices.ContainsFunc(ms, func(m exportMark) bool { return m.kind == Intro })
		hasC := slices.ContainsFunc(ms, func(m exportMark) bool { return m.kind == Credits })
		if hasI {
			pIntro++
		}
		if hasC {
			pCredits++
		}
	}
	pct := func(n int) string { return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(max(c.files, 1))) }
	fmt.Fprintf(w, "LIBRARY COVERAGE (Kivo's present episode files: %d; %s lists %d of them)\n", c.files, ex.name, c.listed)
	fmt.Fprintf(w, "  chapters mark an intro:   %d (%s)   credits: %d (%s)\n", c.chIntro, pct(c.chIntro), c.chCredits, pct(c.chCredits))
	fmt.Fprintf(w, "  %-25s %d (%s)   credits: %d (%s)\n", ex.name+" has an intro:", c.intro, pct(c.intro), c.credits, pct(c.credits))
	fmt.Fprintf(w, "  either:                   %d (%s)   credits: %d (%s)\n", c.eitherIntro, pct(c.eitherIntro), c.eitherCredits, pct(c.eitherCredits))
	fmt.Fprintf(w, "  %s's own count: %d episodes, %d with an intro, %d with credits\n", ex.name, len(ex.episodes), pIntro, pCredits)
}
