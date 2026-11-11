package auth

import (
	"crypto/ed25519"
	"encoding/binary"
)

// ProbeClaim is what the server says about itself to a probe. It holds the
// probe's challenge, the server's ID, the host the probe reached, and
// whether the probe came from a local network.
type ProbeClaim struct {
	Challenge []byte
	ServerID  string
	Host      string
	Local     bool
}

// SignProbe is the server's proof of claim: an Ed25519 signature by key
// over "kivo-probe-v2", then the challenge, server ID and host, each
// preceded by its length as two big-endian bytes, then one byte, which is 1
// when local and 0 otherwise.
func SignProbe(key ed25519.PrivateKey, c ProbeClaim) []byte {
	msg := []byte("kivo-probe-v2")
	for _, f := range [][]byte{c.Challenge, []byte(c.ServerID), []byte(c.Host)} {
		msg = binary.BigEndian.AppendUint16(msg, uint16(len(f)))
		msg = append(msg, f...)
	}
	local := byte(0)
	if c.Local {
		local = 1
	}
	return ed25519.Sign(key, append(msg, local))
}
