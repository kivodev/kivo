package id

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewIsVersion7(t *testing.T) {
	u := New()
	if v := u[6] >> 4; v != 7 {
		t.Fatalf("version = %d, want 7", v)
	}
	if v := u[8] >> 6; v != 0b10 {
		t.Fatalf("variant = %b, want 10", v)
	}
	if d := time.Since(u.Time()); d < 0 || d > time.Second {
		t.Fatalf("embedded time off by %v", d)
	}
}

func TestNewSortsByCreation(t *testing.T) {
	prev := New()
	for range 100_000 { // far more than 4096 per millisecond, so the counter overflows
		next := New()
		if bytes.Compare(prev[:], next[:]) >= 0 {
			t.Fatalf("%s not before %s", prev, next)
		}
		prev = next
	}
}

func TestNextClockBackwards(t *testing.T) {
	ms1, s1 := next(1 << 41)
	ms2, s2 := next(1<<41 - 5000)
	if ms2 < ms1 || (ms2 == ms1 && s2 <= s1) {
		t.Fatalf("went backwards: (%d,%d) then (%d,%d)", ms1, s1, ms2, s2)
	}
}

func TestTextRoundTrip(t *testing.T) {
	u := New()
	got, err := Parse(u.String())
	if err != nil || got != u {
		t.Fatalf("Parse(String()) = %v, %v; want %v", got, err, u)
	}

	b, err := json.Marshal(struct{ ID ID }{u})
	if err != nil {
		t.Fatal(err)
	}
	var back struct{ ID ID }
	if err := json.Unmarshal(b, &back); err != nil || back.ID != u {
		t.Fatalf("JSON round trip = %v, %v; want %v", back.ID, err, u)
	}
}

func TestSet(t *testing.T) {
	if got := Set(); got != "[]" {
		t.Fatalf("Set() = %s, want []", got)
	}
	a, b := New(), New()
	want, _ := json.Marshal([]string{hex.EncodeToString(a[:]), hex.EncodeToString(b[:])})
	if got := Set(a, b); got != string(want) {
		t.Fatalf("Set = %s, want %s", got, want)
	}
}

func TestParseRejects(t *testing.T) {
	for _, s := range []string{
		"",
		"0190a6e2-7b3c-7d4e-8f00-00000000000",   // short
		"0190a6e2-7b3c-7d4e-8f00-0000000000000", // long
		"0190a6e27b3c-7d4e-8f00-0000000000000",  // hyphen missing
		"0190a6e2-7b3c-7d4e-8f00-00000000000g",  // not hex
	} {
		if _, err := Parse(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalid", s, err)
		}
	}
}

func TestSQLRoundTrip(t *testing.T) {
	u := New()
	v, err := u.Value()
	if err != nil {
		t.Fatal(err)
	}
	b, ok := v.([]byte)
	if !ok || len(b) != 16 {
		t.Fatalf("Value() = %#v, want a 16-byte blob", v)
	}
	var back ID
	if err := back.Scan(b); err != nil || back != u {
		t.Fatalf("Scan(Value()) = %v, %v; want %v", back, err, u)
	}

	if v, _ := (ID{}).Value(); v != nil {
		t.Fatalf("zero ID Value() = %#v, want nil", v)
	}
	back = u
	if err := back.Scan(nil); err != nil || !back.IsZero() {
		t.Fatalf("Scan(nil) = %v, %v; want zero", back, err)
	}
	if err := back.Scan([]byte{1, 2, 3}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Scan(3 bytes) error = %v, want ErrInvalid", err)
	}
	if err := back.Scan("text"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Scan(string) error = %v, want ErrInvalid", err)
	}
}
