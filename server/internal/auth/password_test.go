package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	h := newHasher(defaultParams)
	phc, err := h.hash("hunter22")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash %q isn't in PHC form", phc)
	}
	if again, _ := h.hash("hunter22"); again == phc {
		t.Fatal("two hashes of one password are equal; the salt isn't random")
	}
	for pw, want := range map[string]bool{"hunter22": true, "hunter23": false, "": false} {
		if ok, err := h.verify(pw, phc); err != nil || ok != want {
			t.Errorf("verify(%q) = %v, %v; want %v", pw, ok, err, want)
		}
	}
}

func TestOldParamsStillVerify(t *testing.T) {
	old := newHasher(argonParams{memory: 8 * 1024, time: 1, threads: 1})
	phc, err := old.hash("hunter22")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := newHasher(defaultParams).verify("hunter22", phc); err != nil || !ok {
		t.Fatalf("verify with the stored params = %v, %v", ok, err)
	}
}

func TestDummyHashNeverMatches(t *testing.T) {
	h := newHasher(defaultParams)
	if ok, err := h.verify("", ""); err != nil || ok {
		t.Fatalf("verify against the dummy = %v, %v", ok, err)
	}
}

func TestMalformedHash(t *testing.T) {
	h := newHasher(defaultParams)
	for _, phc := range []string{
		"plain",
		"$argon2i$v=19$m=19456,t=2,p=1$AAAA$AAAA",
		"$argon2id$v=16$m=19456,t=2,p=1$AAAA$AAAA",
		"$argon2id$v=19$m=19456,t=0,p=1$AAAA$AAAA",
		"$argon2id$v=19$m=19456,t=2,p=1$!!$AAAA",
		"$argon2id$v=19$m=19456,t=2,p=1$AAAA$",
	} {
		if _, err := h.verify("x", phc); !errors.Is(err, errMalformedHash) {
			t.Errorf("%q: err = %v, want errMalformedHash", phc, err)
		}
	}
}

func TestFullSemaphoreIsBusy(t *testing.T) {
	h := newHasher(defaultParams)
	for range cap(h.sem) {
		if err := h.acquire(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.hash("x"); !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	h.release()
	if _, err := h.hash("x"); err != nil {
		t.Fatal(err)
	}
}
