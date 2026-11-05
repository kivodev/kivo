package detect

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMarkersHarness measures marker detection against episodes whose
// chapters mark their intros and credits, with the chapters hidden. It
// also measures another server's markers against the same episodes. It is
// the second half of an offline measurement, not a test.
//
// It runs only when KIVO_MARKERS_HARNESS names a folder holding the
// seasons.json that the markers package's TestMarkersHarness writes. It
// writes its report to the folder's report.txt. KIVO_MARKERS_EXPORT names
// another server's export to add (see loadExport). KIVO_MARKERS_DUMP names
// a file to write each source's markers to.
func TestMarkersHarness(t *testing.T) {
	dir := os.Getenv("KIVO_MARKERS_HARNESS")
	if dir == "" {
		t.Skip("KIVO_MARKERS_HARNESS not set")
	}
	b, err := os.ReadFile(filepath.Join(dir, "seasons.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data harnessData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}

	var report strings.Builder
	leads(&report, data.Seasons)
	detection(&report, data.Seasons)
	var ex *export
	if p := os.Getenv("KIVO_MARKERS_EXPORT"); p != "" {
		ex = loadExport(t, p)
	}
	compareSources(&report, data.Seasons, ex)
	if ex != nil {
		libraryCoverage(&report, data.Library, ex)
	}
	t.Log("\n" + report.String())
	_ = os.WriteFile(filepath.Join(dir, "report.txt"), []byte(report.String()), 0o644)
}

// harnessData is a measurement's seasons and library, as the markers
// harness writes them.
type harnessData struct {
	Seasons []harnessSeason
	Library []libraryFile
}

// harnessSeason is a season's name and its files.
type harnessSeason struct {
	Name  string
	Files []harnessFile
}

// harnessFile is a file as Decide decides from it, with its chapters
// hidden. It also holds its episode's number, the works its version
// covers, its path, the length of the stretch read for its head, and its
// chapters' markers, by kind.
type harnessFile struct {
	File
	Number, Works int
	Path          string
	HeadLength    time.Duration
	Truth         map[string][2]time.Duration
}

// libraryFile is one of Kivo's present episode files: its path and the
// kinds of marker its chapters give it.
type libraryFile struct {
	Path  string
	Kinds []string
}

// files returns the season's files as Decide takes them.
func (s harnessSeason) files() []File {
	out := make([]File, 0, len(s.Files))
	for _, f := range s.Files {
		out = append(out, f.File)
	}
	return out
}

// truths returns, for each of the season's files, the markers its chapters
// give it, by kind.
func (s harnessSeason) truths() []map[string]span {
	out := make([]map[string]span, len(s.Files))
	for i, f := range s.Files {
		out[i] = map[string]span{}
		for k, t := range f.Truth {
			out[i][k] = span{t[0], t[1]}
		}
	}
	return out
}

// leads reports how far the raw matches sit from the chapters' marks. It
// gives the median of chapter start minus match start, and the same for
// the ends.
func leads(w *strings.Builder, all []harnessSeason) {
	var dsi, dei, dsc, dec []time.Duration
	for _, s := range all {
		files, truths := s.files(), s.truths()
		heads := prints(files, func(fp *Fingerprint) *Window { return fp.Head })
		tails := prints(files, func(fp *Fingerprint) *Window { return fp.Tail })
		intros, ends := season(heads, introLimits), season(tails, creditsLimits)
		for i, f := range files {
			if tr, ok := truths[i][Intro]; ok {
				if m, ok := intros[f.ID]; ok && m.partners >= 2 && overlap(tr, m.span) > 0.5 {
					dsi, dei = append(dsi, tr.start-m.start), append(dei, tr.end-m.end)
				}
			}
			if tr, ok := truths[i][Credits]; ok && f.Fingerprint != nil && f.Fingerprint.Tail != nil {
				if m, ok := ends[f.ID]; ok && m.partners >= 2 {
					abs := span{f.Fingerprint.Tail.Start + m.start, f.Fingerprint.Tail.Start + m.end}
					if overlap(tr, abs) > 0.5 {
						dsc, dec = append(dsc, tr.start-abs.start), append(dec, tr.end-abs.end)
					}
				}
			}
		}
	}
	fmt.Fprintf(w, "LEADS (chapter minus raw match, median)\n")
	fmt.Fprintf(w, "  intro:   start %v end %v (n=%d)\n", medianOr(dsi), medianOr(dei), len(dsi))
	fmt.Fprintf(w, "  credits: start %v end %v (n=%d)\n\n", medianOr(dsc), medianOr(dec), len(dsc))
}

func medianOr(v []time.Duration) time.Duration {
	if len(v) == 0 {
		return 0
	}
	return median(v).Round(10 * time.Millisecond)
}

// score is how a source's markers of one kind compare with the chapters'
// markers.
type score struct {
	truths, found, within1, within3, falseSkips, extra, extraTrusted int
	startErr, endErr                                                 []time.Duration
	falses, extras                                                   []string
	// pastNoStory counts guesses past the chapters by more than 3s over
	// keyframes that show no story: black, or text on black.
	pastNoStory int
	// unseen counts the false skips over a stretch with no keyframe, or a
	// stretch the pictures don't cover. Those can't be judged by pictures.
	unseen int
}

// pics are a window's classified keyframes and the stretch they cover.
type pics struct {
	frames   []keyframe
	from, to time.Duration
}

// noStory reports whether the pictures cover the stretch from from to to,
// with at least one keyframe in it, and none of those keyframes shows
// story.
func (p *pics) noStory(from, to time.Duration) bool {
	if p == nil || from < p.from || to > p.to {
		return false
	}
	seen := false
	for _, k := range p.frames {
		if k.at >= from && k.at < to {
			if k.look == story {
				return false
			}
			seen = true
		}
	}
	return seen
}

// covers reports whether the pictures cover from to to with a keyframe.
func (p *pics) covers(from, to time.Duration) bool {
	return p != nil && from >= p.from && to <= p.to &&
		slices.ContainsFunc(p.frames, func(k keyframe) bool { return k.at >= from && k.at < to })
}

// add scores one file's guess against its truth. A skip of more than
// falseSlack beyond what the chapters mark is a false skip, unless the
// pictures show no story over all of it. A guess is extra when the
// season's chapters mark this kind, but not in this file.
func (sc *score) add(kind string, g *span, tr span, hasTruth, trusted bool, duration time.Duration, name string, p *pics) {
	const falseSlack = 3 * time.Second
	if hasTruth {
		sc.truths++
	}
	if g == nil {
		return
	}
	if !hasTruth {
		sc.extra++
		where := "unmarked file"
		if trusted {
			sc.extraTrusted++
			where = "chapters mark others"
		}
		sc.extras = append(sc.extras, fmt.Sprintf("%s: %s %v-%v (%s)", name, kind, g.start.Round(100*time.Millisecond), g.end.Round(100*time.Millisecond), where))
		return
	}
	sc.found++
	ds, de := (g.start - tr.start).Abs(), (g.end - tr.end).Abs()
	sc.startErr, sc.endErr = append(sc.startErr, ds), append(sc.endErr, de)
	if ds <= time.Second && de <= time.Second {
		sc.within1++
	}
	if ds <= 3*time.Second && de <= 3*time.Second {
		sc.within3++
	}
	toEnd := tr.end >= duration-endSnap
	early, late := tr.start-g.start > falseSlack, !toEnd && g.end-tr.end > falseSlack
	if early || late {
		if (!early || p.noStory(g.start+time.Second, tr.start)) && (!late || p.noStory(tr.end, g.end-time.Second)) {
			sc.pastNoStory++
			return
		}
		sc.falseSkips++
		if early && !p.covers(g.start+time.Second, tr.start) || late && !p.covers(tr.end, g.end-time.Second) {
			sc.unseen++
		}
		sc.falses = append(sc.falses, fmt.Sprintf("%s: %s %v-%v, chapters %v-%v", name, kind,
			g.start.Round(100*time.Millisecond), g.end.Round(100*time.Millisecond), tr.start.Round(100*time.Millisecond), tr.end.Round(100*time.Millisecond)))
	}
}

func (sc *score) write(w *strings.Builder, name string) {
	pct := func(n, d int) string {
		if d == 0 {
			return "-"
		}
		return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(d))
	}
	q := func(v []time.Duration, p float64) time.Duration {
		if len(v) == 0 {
			return 0
		}
		v = slices.Clone(v)
		slices.Sort(v)
		return v[int(math.Min(float64(len(v)-1), p*float64(len(v))))].Round(10 * time.Millisecond)
	}
	fmt.Fprintf(w, "  %-22s marked %4d  found %4d (coverage %s)  within 1s %s  within 3s %s  start p50 %v p90 %v  end p50 %v p90 %v  false skips %d (%s of found, %d without pictures)  past chapters over no story %d  extra %d (in marked files %d)\n",
		name, sc.truths, sc.found, pct(sc.found, sc.truths), pct(sc.within1, sc.found), pct(sc.within3, sc.found),
		q(sc.startErr, 0.5), q(sc.startErr, 0.9), q(sc.endErr, 0.5), q(sc.endErr, 0.9),
		sc.falseSkips, pct(sc.falseSkips, sc.found), sc.unseen, sc.pastNoStory, sc.extra, sc.extraTrusted)
}

// marked counts how many of a season's files the chapters mark with a
// kind.
func marked(truths []map[string]span, kind string) int {
	n := 0
	for _, t := range truths {
		if _, ok := t[kind]; ok {
			n++
		}
	}
	return n
}

// trusted reports whether the i-th file's chapters would mark a kind if
// the file had one. That is when they mark another kind, in a season whose
// chapters mark this kind in at least half its files.
func trusted(truths []map[string]span, i int, kind string) bool {
	return len(truths[i]) > 0 && 2*marked(truths, kind) >= len(truths)
}

// detection replays detection on each season with its chapters hidden.
func detection(w *strings.Builder, all []harnessSeason) {
	var intro, credits score
	perSeason := map[string]*[2]score{}
	for _, s := range all {
		truths, found := s.truths(), Decide(s.files())
		ps := &[2]score{}
		perSeason[s.Name] = ps
		for i, f := range s.Files {
			ti, tc := trusted(truths, i, Intro), trusted(truths, i, Credits)
			fn := fmt.Sprintf("%s E%d", s.Name, f.Number)
			ig, cg := spanOf(found[f.ID], Intro), spanOf(found[f.ID], Credits)
			tr, ok := truths[i][Intro]
			for _, sc := range []*score{&intro, &ps[0]} {
				sc.add(Intro, ig, tr, ok, ti, f.Duration, fn, headPics(f))
			}
			tr, ok = truths[i][Credits]
			for _, sc := range []*score{&credits, &ps[1]} {
				sc.add(Credits, cg, tr, ok, tc, f.Duration, fn, tailPics(f))
			}
		}
	}
	fmt.Fprintf(w, "FINGERPRINT DETECTION (chapters hidden)\n")
	intro.write(w, "intro")
	credits.write(w, "credits")
	fmt.Fprintf(w, "\n  by season:\n")
	for _, n := range slices.Sorted(maps.Keys(perSeason)) {
		perSeason[n][0].write(w, n+" intro")
		perSeason[n][1].write(w, n+" credits")
	}
	fmt.Fprintf(w, "\n  past the chapters by more than 3s:\n")
	for _, f := range append(intro.falses, credits.falses...) {
		fmt.Fprintf(w, "    %s\n", f)
	}
	fmt.Fprintf(w, "\n  where the chapters mark none:\n")
	for _, f := range append(intro.extras, credits.extras...) {
		fmt.Fprintf(w, "    %s\n", f)
	}
	fmt.Fprintln(w)
}

// headPics returns a file's classified head keyframes, or nil without
// them.
func headPics(f harnessFile) *pics {
	if f.Fingerprint == nil || f.Fingerprint.Head == nil || len(f.Fingerprint.Head.Keyframes) == 0 {
		return nil
	}
	w := f.Fingerprint.Head
	return &pics{frames: classify(w.Keyframes), from: w.Start, to: w.Start + f.HeadLength}
}

// tailPics returns a file's classified tail keyframes, or nil without
// them.
func tailPics(f harnessFile) *pics {
	if f.Fingerprint == nil || f.Fingerprint.Tail == nil || len(f.Fingerprint.Tail.Keyframes) == 0 {
		return nil
	}
	w := f.Fingerprint.Tail
	return &pics{frames: classify(w.Keyframes), from: w.Start, to: f.Duration}
}

// spanOf returns the span of a file's marker of a kind, if it has one.
func spanOf(ms []Marker, kind string) *span {
	for _, m := range ms {
		if m.Kind == kind {
			return &span{m.Start, m.End}
		}
	}
	return nil
}

// eligible reports whether a file is one whole episode. Every source is
// measured on those files.
func eligible(f harnessFile) bool { return f.Parts == 1 && f.Works == 1 }
