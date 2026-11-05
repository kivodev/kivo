package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// A token is 32 random bytes, sent base64url-encoded and stored only as its
// SHA-256. It carries no ID, so a leaked database yields no usable token and
// a token reveals nothing about its owner. Session and API key tokens, invite
// links and device code secrets are all tokens.
const tokenLen = 32

var tokenEncoding = base64.RawURLEncoding

// tokenHash is the stored form of a token.
type tokenHash = [sha256.Size]byte

// newToken returns a new token and its hash.
func newToken() (string, tokenHash) {
	raw := make([]byte, tokenLen)
	_, _ = rand.Read(raw)
	return tokenEncoding.EncodeToString(raw), sha256.Sum256(raw)
}

// hashToken returns the hash of token. It returns false when token is
// malformed.
func hashToken(token string) (tokenHash, bool) {
	raw, err := tokenEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenLen {
		return tokenHash{}, false
	}
	return sha256.Sum256(raw), true
}
