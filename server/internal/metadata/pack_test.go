package metadata

import (
	"bytes"
	"testing"
)

func TestPackRoundTrips(t *testing.T) {
	body := bytes.Repeat([]byte(`{"title":"Alien","cast":[]}`), 1000)
	packed := pack(body)
	if len(packed) >= len(body)/5 {
		t.Errorf("packed %d bytes to %d", len(body), len(packed))
	}
	for _, in := range [][]byte{packed, body} { // and a row cached before packing
		out, err := unpack(in)
		if err != nil || !bytes.Equal(out, body) {
			t.Fatalf("unpack: %v", err)
		}
	}
}
