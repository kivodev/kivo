package parse

import "testing"

func TestBookFolder(t *testing.T) {
	for rel, want := range map[string]string{
		"Andy Weir/Project Hail Mary/Project Hail Mary.m4b": "Andy Weir/Project Hail Mary",
		"Michael J. Sullivan/Heir of Novron/CD1/01.mp3":     "Michael J. Sullivan/Heir of Novron",
		"Michael J. Sullivan/Heir of Novron/Disc 2/01.mp3":  "Michael J. Sullivan/Heir of Novron",
		"Michael J. Sullivan/Heir of Novron/Disk3/01.mp3":   "Michael J. Sullivan/Heir of Novron",
		"Stephen King/11.22.63/Bonus/01.mp3":                "Stephen King/11.22.63/Bonus",
		"Sphere.m4b":                                        "",
		"CD1/01.mp3":                                        "",
	} {
		if got := BookFolder(rel); got != want {
			t.Errorf("BookFolder(%q) = %q, want %q", rel, got, want)
		}
	}
	if Disc("Heir of Novron/Disc 2/01.mp3") != 2 || Disc("Heir of Novron/CD10/01.mp3") != 10 || Disc("Heir/01.mp3") != 0 {
		t.Error("disc numbers")
	}
}
