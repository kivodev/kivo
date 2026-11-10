package id

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ID is a UUIDv7 (RFC 9562). The zero value means "no ID".
type ID [16]byte

// ErrInvalid is returned when text or a database value isn't a valid ID.
var ErrInvalid = errors.New("invalid id")

var (
	mu     sync.Mutex
	lastMS int64
	seq    uint16 // 12-bit counter in rand_a, so IDs from one process sort by creation
)

// New returns a new UUIDv7. IDs created by one process sort in creation order,
// even within the same millisecond.
func New() ID {
	ms, s := next(time.Now().UnixMilli())

	var u ID
	if _, err := rand.Read(u[8:]); err != nil {
		panic("id: crypto/rand failed: " + err.Error()) // never happens on supported platforms
	}
	u[0] = byte(ms >> 40)
	u[1] = byte(ms >> 32)
	u[2] = byte(ms >> 24)
	u[3] = byte(ms >> 16)
	u[4] = byte(ms >> 8)
	u[5] = byte(ms)
	u[6] = 0x70 | byte(s>>8) // version 7
	u[7] = byte(s)
	u[8] = 0x80 | u[8]&0x3f // variant 10
	return u
}

// next returns the timestamp and counter for a new ID. When the clock hasn't
// moved, or moved backwards, the counter increases. When the counter
// overflows, the timestamp advances by one millisecond so order still
// holds.
func next(now int64) (int64, uint16) {
	mu.Lock()
	defer mu.Unlock()
	if now > lastMS {
		lastMS = now
		var b [2]byte
		_, _ = rand.Read(b[:])
		seq = (uint16(b[0])<<8 | uint16(b[1])) & 0x3ff // start low, leaving room to count up
		return lastMS, seq
	}
	seq++
	if seq > 0xfff {
		lastMS++
		seq = 0
	}
	return lastMS, seq
}

// Parse reads the canonical 36-character form.
func Parse(s string) (ID, error) {
	var u ID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return ID{}, fmt.Errorf("%w: parse %q", ErrInvalid, s)
	}
	src := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	if _, err := hex.Decode(u[:], []byte(src)); err != nil {
		return ID{}, fmt.Errorf("%w: parse %q", ErrInvalid, s)
	}
	return u, nil
}

// String returns the canonical lowercase form, xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx.
func (u ID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], u[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], u[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], u[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], u[8:10])
	b[23] = '-'
	hex.Encode(b[24:], u[10:])
	return string(b[:])
}

// Set returns ids as one JSON array of their hex forms. A statement binds
// it as one parameter and reads it with
// `SELECT unhex(value) FROM json_each(...)`.
func Set(ids ...ID) string {
	b := make([]byte, 0, 2+35*len(ids))
	b = append(b, '[')
	for i, u := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '"')
		b = hex.AppendEncode(b, u[:])
		b = append(b, '"')
	}
	return string(append(b, ']'))
}

// IsZero reports whether u is the zero ID.
func (u ID) IsZero() bool { return u == ID{} }

// Time returns the creation time encoded in a UUIDv7.
func (u ID) Time() time.Time {
	ms := int64(u[0])<<40 | int64(u[1])<<32 | int64(u[2])<<24 | int64(u[3])<<16 | int64(u[4])<<8 | int64(u[5])
	return time.UnixMilli(ms)
}

// MarshalText implements encoding.TextMarshaler.
func (u ID) MarshalText() ([]byte, error) { return []byte(u.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (u *ID) UnmarshalText(b []byte) error {
	v, err := Parse(string(b))
	if err != nil {
		return err
	}
	*u = v
	return nil
}

// Value stores the ID as a 16-byte blob. The zero ID is stored as NULL.
func (u ID) Value() (driver.Value, error) {
	if u.IsZero() {
		return nil, nil
	}
	return u[:], nil
}

// Scan reads a 16-byte blob. NULL scans as the zero ID.
func (u *ID) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*u = ID{}
		return nil
	case []byte:
		if len(v) != 16 {
			return fmt.Errorf("%w: scan %d-byte blob", ErrInvalid, len(v))
		}
		copy(u[:], v)
		return nil
	default:
		return fmt.Errorf("%w: scan %T", ErrInvalid, src)
	}
}
