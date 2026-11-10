package metadata

import (
	"reflect"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
)

func TestParseBookPath(t *testing.T) {
	for _, tc := range []struct {
		folder string
		want   bookPath
	}{
		{"Andy Weir/Project Hail Mary", bookPath{title: "Project Hail Mary", authors: []string{"Andy Weir"}}},
		{"James S. A. Corey/The Expanse/Book 1 - Leviathan Wakes",
			bookPath{title: "Leviathan Wakes", series: "The Expanse", position: "1", authors: []string{"James S. A. Corey"}}},
		{"James S. A. Corey/The Expanse/Book 1.5 - The Churn",
			bookPath{title: "The Churn", series: "The Expanse", position: "1.5", authors: []string{"James S. A. Corey"}}},
		{"Brandon Sanderson/Mistborn/Vol. 3 - The Hero of Ages",
			bookPath{title: "The Hero of Ages", series: "Mistborn", position: "3", authors: []string{"Brandon Sanderson"}}},
		{"Jason Anspach & Nick Cole/Galaxy's Edge/Part 1 - Legionnaire & Galactic Outlaws",
			bookPath{title: "Legionnaire & Galactic Outlaws", series: "Galaxy's Edge", position: "1", authors: []string{"Jason Anspach", "Nick Cole"}}},
		{"Kyle Kirrin/Vainqueur the Dragon/1-3 - The Complete Series",
			bookPath{title: "The Complete Series", series: "Vainqueur the Dragon", position: "1-3", authors: []string{"Kyle Kirrin"}}},
		{"Terry Pratchett/Discworld/01. The Colour of Magic",
			bookPath{title: "The Colour of Magic", series: "Discworld", position: "1", authors: []string{"Terry Pratchett"}}},
		{"Joe Abercrombie/First Law World/Best Served Cold",
			bookPath{title: "Best Served Cold", series: "First Law World", authors: []string{"Joe Abercrombie"}}},
		// A bare number before more words is a title, not a position.
		{"Dodie Smith/Dalmatians/101 Dalmatians",
			bookPath{title: "101 Dalmatians", series: "Dalmatians", authors: []string{"Dodie Smith"}}},
		{"Andy Weir/2014 - The Martian {R. C. Bray} [B00B5HZGUG]",
			bookPath{title: "The Martian", narrator: "R. C. Bray", asin: "B00B5HZGUG", year: 2014, authors: []string{"Andy Weir"}}},
		{"Stephen King/The Dark Tower/Book 2 - (1987) - The Drawing of the Three",
			bookPath{title: "The Drawing of the Three", series: "The Dark Tower", position: "2", year: 1987, authors: []string{"Stephen King"}}},
		// Only the three folders nearest the files are read.
		{"Audio/Fantasy/Michael J. Sullivan/Riyria Revelations/Heir of Novron",
			bookPath{title: "Heir of Novron", series: "Riyria Revelations", authors: []string{"Michael J. Sullivan"}}},
		// Without a series folder, "Book N - " is part of the title.
		{"Andy Weir/Book 1 - Artemis", bookPath{title: "Book 1 - Artemis", authors: []string{"Andy Weir"}}},
		{"Sphere", bookPath{title: "Sphere"}},
	} {
		if got := parseBookPath(tc.folder); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got  %+v\n want %+v", tc.folder, got, tc.want)
		}
	}
}

func TestBookFromFoldersThenTags(t *testing.T) {
	inAudible := map[string]string{
		"album": "02 The Drawing of the Three", "artist": "Stephen King", "album_artist": "Stephen King (The Dark Tower)",
		"composer": "Frank Muller", "grouping": "The Dark Tower", "date": "2016-01-12", "description": "The second book.",
		"publisher": "Penguin Audio", "genre": "Audiobook", "series": "Dark Tower", "part": "02",
	}
	got := bookOf("Stephen King/The Dark Tower/Book 2 - The Drawing of the Three/The Drawing of the Three.m4b", inAudible)
	want := catalog.WorkMetadata{
		Title: "The Drawing of the Three", Year: 2016, Overview: "The second book.", ReleaseDate: "2016-01-12",
		Companies: []string{"Penguin Audio"},
		Book: &catalog.BookMetadata{
			Authors: []string{"Stephen King"}, Narrators: []string{"Frank Muller"},
			// The folders' series wins over the tags'.
			Series: []catalog.SeriesRef{{Key: "local:Stephen King/The Dark Tower", Name: "The Dark Tower", Position: "2", Primary: true}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("folders and tags:\n got  %+v %+v\n want %+v %+v", got, *got.Book, want, *want.Book)
	}

	// Without a series folder, the tags give the series and position, and
	// the file name gives the ASIN. Several narrators are split.
	got = bookOf("Peter F. Hamilton/Exodus/Exodus - The Archimedes Engine [B0CTKSG1VB].m4b", map[string]string{
		"artist": "Peter F. Hamilton", "composer": "John Lee, Jane Doe", "series": "Exodus", "part": "1", "date": "2024",
	})
	b := got.Book
	if got.Title != "Exodus" || b.ASIN != "B0CTKSG1VB" || !reflect.DeepEqual(b.Narrators, []string{"John Lee", "Jane Doe"}) ||
		len(b.Series) != 1 || b.Series[0] != (catalog.SeriesRef{Key: "local:Peter F. Hamilton/Exodus", Name: "Exodus", Position: "1", Primary: true}) {
		t.Errorf("tag series: %+v %+v", got, *b)
	}

	// A grouping tag naming the author isn't a series. A folder's year wins
	// over a different year in a tag. A book directly in the library folder
	// takes its title from its album tag and its author from its tags.
	got = bookOf("Sphere.m4b", map[string]string{"album": "Sphere", "artist": "Michael Crichton", "grouping": "Michael Crichton", "date": "1987"})
	if got.Title != "Sphere" || len(got.Book.Series) != 0 || !reflect.DeepEqual(got.Book.Authors, []string{"Michael Crichton"}) || got.Year != 1987 {
		t.Errorf("loose file: %+v %+v", got, *got.Book)
	}
	got = bookOf("Andy Weir/2011 - The Martian/The Martian.m4b", map[string]string{"date": "2014-02-11"})
	if got.Year != 2011 || got.ReleaseDate != "2011" {
		t.Errorf("folder year: %+v %+v", got, *got.Book)
	}
}
