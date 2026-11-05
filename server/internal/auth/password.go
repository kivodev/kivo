package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argonParams are argon2id's settings. They are kept in each hash's PHC
// string, so they can change without invalidating stored hashes.
type argonParams struct {
	memory  uint32 // KiB
	time    uint32
	threads uint8
}

// defaultParams are modest: OWASP's minimum for argon2id. A hash takes
// about 20 ms and 19 MiB, so a small NAS can afford a few at once.
var defaultParams = argonParams{memory: 19 * 1024, time: 2, threads: 1}

const (
	saltLen = 16
	keyLen  = 32
)

var b64 = base64.RawStdEncoding

// errMalformedHash means a stored PHC string doesn't parse.
var errMalformedHash = errors.New("malformed password hash")

// hasher caps concurrent hashing at about half the CPUs, so a burst of sign-ins
// can't take over a small server. A full semaphore fails fast with ErrBusy.
type hasher struct {
	params argonParams
	sem    chan struct{}
	dummy  string // a hash of nothing, verified for unknown users
}

func newHasher(p argonParams) *hasher {
	return &hasher{
		params: p,
		sem:    make(chan struct{}, max(1, runtime.NumCPU()/2)),
		dummy:  encodePHC(p, make([]byte, saltLen), make([]byte, keyLen)),
	}
}

func (h *hasher) acquire() error {
	select {
	case h.sem <- struct{}{}:
		return nil
	default:
		return ErrBusy
	}
}

func (h *hasher) release() { <-h.sem }

// hash returns password's argon2id hash in PHC form.
func (h *hasher) hash(password string) (string, error) {
	if err := h.acquire(); err != nil {
		return "", err
	}
	defer h.release()
	salt := make([]byte, saltLen)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, h.params.time, h.params.memory, h.params.threads, keyLen)
	return encodePHC(h.params, salt, key), nil
}

// verify reports whether password matches phc. An empty phc means there is
// no such user or no password. It is verified against the dummy hash, so it
// costs the same and fails.
func (h *hasher) verify(password, phc string) (bool, error) {
	known := phc != ""
	if !known {
		phc = h.dummy
	}
	p, salt, key, err := decodePHC(phc)
	if err != nil {
		return false, err
	}
	if err := h.acquire(); err != nil {
		return false, err
	}
	defer h.release()
	got := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1 && known, nil
}

func encodePHC(p argonParams, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.time, p.threads, b64.EncodeToString(salt), b64.EncodeToString(key))
}

func decodePHC(s string) (p argonParams, salt, key []byte, err error) {
	f := strings.Split(s, "$")
	var v int
	if len(f) != 6 || f[0] != "" || f[1] != "argon2id" {
		return p, nil, nil, errMalformedHash
	}
	if _, err := fmt.Sscanf(f[2], "v=%d", &v); err != nil || v != argon2.Version {
		return p, nil, nil, errMalformedHash
	}
	if _, err := fmt.Sscanf(f[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil ||
		p.time == 0 || p.threads == 0 {
		return p, nil, nil, errMalformedHash
	}
	if salt, err = b64.DecodeString(f[4]); err != nil {
		return p, nil, nil, errMalformedHash
	}
	if key, err = b64.DecodeString(f[5]); err != nil || len(key) == 0 {
		return p, nil, nil, errMalformedHash
	}
	return p, salt, key, nil
}
