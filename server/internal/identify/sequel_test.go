package identify

import "testing"

func TestTitleKey(t *testing.T) {
	for _, tc := range []struct{ title, want string }{
		{"Rocky II", "rocky 2"},
		{"Rocky 2", "rocky 2"},
		{"Dune: Part One", "dune part 1"},
		{"Harry Potter and the Deathly Hallows: Part I", "harry potter deathly hallows part 1"},
		{"Kill Bill: Vol. 1", "kill bill vol 1"},
		{"The Lion King 1½", "the lion king 1half"},
		{"Ranma ½", "ranma 1 2"}, // a fraction alone is not a sequel's
		{"Malcolm X", "malcolm 10"},
		{"Generation X Files", "generation x files"}, // a numeral only counts last
		{"Me Myself and I", "me myself i"},           // I only counts after Part and the like
		{"Ocean's Eleven", "oceans eleven"},
		{"V", "v"},
	} {
		if got := titleKey(tc.title); got != tc.want {
			t.Errorf("titleKey(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestDifferentSequel(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		// Different entries in a series.
		{"How to Train Your Dragon 3", "How to Train Your Dragon 2", true},
		{"How to Train Your Dragon 3", "How to Train Your Dragon 2 3D", true},
		{"The Lion King II", "The Lion King 1½", true},
		{"Harry Potter and the Deathly Hallows Part 2", "Harry Potter and the Deathly Hallows Part 3", true},
		{"Harry Potter and the Deathly Hallows Part II", "Harry Potter and the Deathly Hallows Part 1", true},
		{"Rocky III", "Rocky 2", true},
		{"Dune Part One", "Dune: Part Two", true},
		{"21 Jump Street", "22 Jump Street", true},
		{"Apollo 13", "Apollo 18", true},
		{"Stalingrad 2", "Stalingrad", true},
		{"The Lion King", "The Lion King 1½", true},
		{"Evangelion 1.0", "Evangelion 2.0", true},

		// The same entry.
		{"Rocky II", "Rocky 2", false},
		{"Harry Potter and the Deathly Hallows Part II", "Harry Potter and the Deathly Hallows: Part 2", false},
		{"Dune Part One", "Dune: Part 1", false},
		{"Toy Story 1", "Toy Story", false},
		{"Kill Bill Vol 1", "Kill Bill: Vol. 1", false},
		{"Lilo and Stitch 2", "Lilo & Stitch 2: Stitch Has a Glitch", false}, // decided by the titles' likeness
		{"Ranma ½", "Ranma 1/2", false},

		// Numbers that are part of a title.
		{"101 Dalmatians", "101 Dalmatians", false},
		{"101 Dalmatians", "One Hundred and One Dalmatians", false},
		{"2001: A Space Odyssey", "2001 A Space Odyssey", false},
		{"Ocean's Eleven", "Ocean's Twelve", false}, // decided by the titles' likeness
		{"Blade Runner 2049", "Blade Runner 2049", false},
		{"Blade Runner 2049", "Blade Runner", false}, // decided by the titles' likeness
		{"1917", "1917", false},
		{"1917", "1918", false},
		{"21 Jump Street", "21 Jump Street", false},
		{"Evangelion: 1.0 You Are (Not) Alone", "Evangelion 1.0 You Are Not Alone", false},
		{"Malcolm X", "Malcolm X", false},
		{"Star Wars Episode IV A New Hope", "Star Wars: Episode IV - A New Hope", false},
	} {
		if got := differentSequel(titleKey(tc.a), titleKey(tc.b)); got != tc.want {
			t.Errorf("differentSequel(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
		if got := differentSequel(titleKey(tc.b), titleKey(tc.a)); got != tc.want {
			t.Errorf("differentSequel(%q, %q) = %v, want %v", tc.b, tc.a, got, tc.want)
		}
	}
}
