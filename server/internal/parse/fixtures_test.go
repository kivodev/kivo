package parse

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// naming is the vendored fixture directory (server/testdata/naming).
var naming = filepath.Join("..", "..", "testdata", "naming")

// fixtureToday is the date the fixtures parse on. It only bounds years.
var fixtureToday = Date{2026, 1, 1}

// fixture is one vendored test case, reduced to the fields a Guess models.
// A nil field isn't checked.
type fixture struct {
	source, name string
	kind         Kind
	// anyKind marks a fixture that doesn't say whether it's a movie or an
	// episode. A library always declares its kind, so the fixture passes if
	// either kind parses it right.
	anyKind bool
	title   *string
	year    *int
	seasons []int // one season, or several for a season pack
	// episodes are the expected numbers. anitomy lists a range by its ends,
	// so rangeEnds compares only the first and last.
	episodes  []int
	rangeEnds bool
	special   bool
	split     string
	group     *string
	checksum  *string
	revision  *int
}

// loadGuessit reads one of guessit's cross-parser files.
func loadGuessit(t testing.TB, source string) []fixture {
	t.Helper()
	var f struct {
		Entries []struct {
			ReleaseName string         `json:"release_name"`
			Expected    map[string]any `json:"expected"`
		}
	}
	readJSON(t, filepath.Join(naming, "guessit", source+".json"), &f)
	var out []fixture
	for _, e := range f.Entries {
		x := fixture{source: source, name: e.ReleaseName, kind: Movies}
		for k, v := range e.Expected {
			switch k {
			case "title":
				s := v.(string)
				x.title = &s
			case "year":
				n := int(v.(float64))
				x.year = &n
			case "season":
				x.seasons = numbers(v)
				x.kind = Shows
			case "episode":
				x.episodes = numbers(v)
				x.kind = Shows
			}
		}
		switch e.Expected["type"] {
		case "episode":
			x.kind = Shows
		case "movie":
			x.kind = Movies
		default:
			x.anyKind = x.kind == Movies
		}
		out = append(out, x)
	}
	return out
}

// loadAnitomy reads anitomy's test data. Every entry is an anime release in
// a shows library.
func loadAnitomy(t testing.TB) []fixture {
	t.Helper()
	var f []struct {
		Input  string         `json:"input"`
		Output map[string]any `json:"output"`
	}
	readJSON(t, filepath.Join(naming, "anitomy", "data.json"), &f)
	var out []fixture
	for _, e := range f {
		x := fixture{source: "anitomy", name: e.Input, kind: Shows}
		for k, v := range e.Output {
			s, isString := v.(string)
			switch k {
			case "title":
				x.title = &s
			case "year":
				n, _ := strconv.Atoi(s)
				x.year = &n
			case "season":
				x.seasons = anitomyNumbers(v, nil)
			case "episode":
				x.episodes = anitomyNumbers(v, &x)
				x.rangeEnds = !isString
			case "release_group":
				// Only a leading [Group] is parsed. Identification doesn't
				// use groups, and trailing ones are ambiguous with titles.
				if strings.HasPrefix(e.Input, "[") {
					x.group = &s
				}
			case "file_checksum":
				s = strings.ToUpper(s)
				x.checksum = &s
			case "release_version":
				if isString {
					n, _ := strconv.Atoi(s)
					x.revision = &n
				}
			}
		}
		out = append(out, x)
	}
	return out
}

func readJSON(t testing.TB, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// numbers reads a JSON number or list of numbers.
func numbers(v any) []int {
	switch v := v.(type) {
	case float64:
		return []int{int(v)}
	case []any:
		var out []int
		for _, e := range v {
			out = append(out, int(e.(float64)))
		}
		return out
	}
	return nil
}

// anitomyNumbers reads anitomy's string numbers, such as "01", "02.5" (a
// special), "4a" (a split part), or a list of them. Splits and specials are
// recorded in x.
func anitomyNumbers(v any, x *fixture) []int {
	var ss []string
	switch v := v.(type) {
	case string:
		ss = []string{v}
	case []any:
		for _, e := range v {
			ss = append(ss, e.(string))
		}
	}
	var out []int
	for _, s := range ss {
		whole, frac, isFrac := strings.Cut(s, ".")
		if x != nil && isFrac && frac != "" {
			x.special = true
		}
		digits := strings.TrimRightFunc(whole, func(r rune) bool { return r < '0' || r > '9' })
		if x != nil && digits != whole {
			x.split = strings.ToLower(whole[len(digits):])
		}
		n, _ := strconv.Atoi(digits)
		out = append(out, n)
	}
	return out
}

// mismatches lists the fields where g differs from x.
func (x fixture) mismatches(g Guess) []string {
	var out []string
	if x.title != nil && strings.ReplaceAll(Key(*x.title), " ", "") != strings.ReplaceAll(Key(g.Title), " ", "") {
		out = append(out, fmt.Sprintf("title %q, want %q", g.Title, *x.title))
	}
	if x.year != nil && g.Year != *x.year {
		out = append(out, fmt.Sprintf("year %d, want %d", g.Year, *x.year))
	}
	if x.seasons != nil && !slices.Equal(x.seasons, []int{g.Season}) {
		out = append(out, fmt.Sprintf("season %d, want %v", g.Season, x.seasons))
	}
	if x.episodes != nil {
		got := g.Episodes
		if got == nil {
			got = g.Absolute
		}
		ok := slices.Equal(got, x.episodes)
		if x.rangeEnds && len(got) > 0 {
			ok = slices.Equal([]int{got[0], got[len(got)-1]}, x.episodes)
		}
		if !ok || g.Special != x.special || x.split != "" && g.SplitPart != x.split {
			out = append(out, fmt.Sprintf("episodes %v special %v split %q, want %v special %v split %q",
				got, g.Special, g.SplitPart, x.episodes, x.special, x.split))
		}
	}
	if x.group != nil && g.Group != *x.group {
		out = append(out, fmt.Sprintf("group %q, want %q", g.Group, *x.group))
	}
	if x.checksum != nil && g.Checksum != *x.checksum {
		out = append(out, fmt.Sprintf("checksum %q, want %q", g.Checksum, *x.checksum))
	}
	if x.revision != nil && g.Revision != *x.revision {
		out = append(out, fmt.Sprintf("revision %d, want %d", g.Revision, *x.revision))
	}
	return out
}

// loadDivergences reads testdata/divergences.txt. It has one
// "source<TAB>name<TAB>reason" line for each fixture Kivo deliberately
// parses differently.
func loadDivergences(t *testing.T) map[[2]string]string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "divergences.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[[2]string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 || parts[2] == "" {
			t.Fatalf("divergences.txt:%d: want source<TAB>name<TAB>reason", n)
		}
		out[[2]string{parts[0], parts[1]}] = parts[2]
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestFixtures runs the vendored naming fixtures. Every failure must be
// listed in testdata/divergences.txt with a reason, and every listed entry
// must still fail, so the file stays honest.
func TestFixtures(t *testing.T) {
	var all []fixture
	for _, src := range []string{"ptt", "thcolin", "go-ptn", "ptn"} {
		all = append(all, loadGuessit(t, src)...)
	}
	all = append(all, loadAnitomy(t)...)
	div := loadDivergences(t)
	p := Parser{Today: fixtureToday}
	pass := 0
	seen := map[[2]string]bool{}
	for _, x := range all {
		key := [2]string{x.source, x.name}
		seen[key] = true
		bad := x.mismatches(p.Parse(x.name, x.kind))
		if x.anyKind && len(bad) > 0 && len(x.mismatches(p.Parse(x.name, Shows))) == 0 {
			bad = nil
		}
		_, listed := div[key]
		switch {
		case len(bad) == 0:
			pass++
			if listed {
				t.Errorf("%s %q passes now; remove it from divergences.txt", x.source, x.name)
			}
		case !listed:
			t.Errorf("%s %q (%s): %s\n\tadd to divergences.txt: %s\t%s\t<reason>", x.source, x.name, x.kind, strings.Join(bad, "; "), x.source, x.name)
		}
	}
	for key := range div {
		if !seen[key] {
			t.Errorf("divergences.txt lists %s %q, which no fixture has", key[0], key[1])
		}
	}
	t.Logf("fixtures: %d of %d pass (%.1f%%), %d divergences", pass, len(all), 100*float64(pass)/float64(len(all)), len(div))
}
