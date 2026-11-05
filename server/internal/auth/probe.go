package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
)

// ProbeIDLen is the length of a session's probe ID.
const ProbeIDLen = 16

// probeID returns the probe ID of the session with token hash h. It is the
// first ProbeIDLen bytes of SHA-256("kivo-probe-id" ‖ h). An app sends it
// with the unauthenticated server probe, and it reveals nothing of the
// token.
func probeID(h tokenHash) []byte {
	sum := sha256.Sum256(append([]byte("kivo-probe-id"), h[:]...))
	return sum[:ProbeIDLen]
}

// ProbeClaim is what the server says about itself to a probe. It holds the
// probe's challenge, the server's ID, the host the probe reached, and
// whether the probe came from a local network.
type ProbeClaim struct {
	Challenge []byte
	ServerID  string
	Host      string
	Local     bool
}

// Prove returns the server's proof of claim to the app whose session has
// the probe ID probe. The proof is ProbeMAC keyed by the session's token
// hash. It returns false when no session has that probe ID.
func (s *Service) Prove(ctx context.Context, probe []byte, claim ProbeClaim) ([]byte, bool, error) {
	key, err := s.q.ProbeKey(ctx, probe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return ProbeMAC(key, claim), true, nil
}

// ProbeMAC is HMAC-SHA256 keyed by key. It covers "kivo-probe-v1", then the
// challenge, server ID and host, each preceded by its length as two
// big-endian bytes. It ends with one byte, which is 1 when local and 0
// otherwise.
func ProbeMAC(key []byte, c ProbeClaim) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("kivo-probe-v1"))
	for _, f := range [][]byte{c.Challenge, []byte(c.ServerID), []byte(c.Host)} {
		m.Write(binary.BigEndian.AppendUint16(nil, uint16(len(f))))
		m.Write(f)
	}
	local := byte(0)
	if c.Local {
		local = 1
	}
	m.Write([]byte{local})
	return m.Sum(nil)
}
