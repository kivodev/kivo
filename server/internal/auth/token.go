package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// A token's body is 32 random bytes, sent base64url-encoded and stored only
// as their SHA-256. It carries no ID, so a leaked database yields no usable
// token and a token reveals nothing about its owner. Invite links and
// device code secrets are bare bodies. A credential (a session, an API key
// or a cast token) is its kind's prefix followed by a body, and only the
// body is hashed.
const tokenLen = 32

var tokenEncoding = base64.RawURLEncoding

// tokenHash is the stored form of a token.
type tokenHash = [sha256.Size]byte

// newBareToken returns a new token body and its hash.
func newBareToken() (string, tokenHash) {
	raw := make([]byte, tokenLen)
	_, _ = rand.Read(raw)
	return tokenEncoding.EncodeToString(raw), sha256.Sum256(raw)
}

// hashBareToken returns the hash of a token body. It returns false when
// token is malformed.
func hashBareToken(token string) (tokenHash, bool) {
	raw, err := tokenEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenLen {
		return tokenHash{}, false
	}
	return sha256.Sum256(raw), true
}

// credentialKind is the prefix that says which table a credential is in.
type credentialKind string

const (
	sessionCredential credentialKind = "kivo_s_"
	apiKeyCredential  credentialKind = "kivo_k_"
	castCredential    credentialKind = "kivo_c_"
)

// newCredential returns a new credential of kind k and the hash of its
// body.
func newCredential(k credentialKind) (string, tokenHash) {
	body, hash := newBareToken()
	return string(k) + body, hash
}

// parseCredential returns a credential's kind and the hash of its body. It
// returns false when the token has no known prefix or a malformed body.
func parseCredential(token string) (credentialKind, tokenHash, bool) {
	for _, k := range []credentialKind{sessionCredential, apiKeyCredential, castCredential} {
		if body, ok := strings.CutPrefix(token, string(k)); ok {
			hash, ok := hashBareToken(body)
			return k, hash, ok
		}
	}
	return "", tokenHash{}, false
}
