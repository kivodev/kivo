package main

import (
	"testing"

	"github.com/mmcdole/kivo/internal/config"
)

// Open Kivo goes to the first listen address, on this machine when it
// listens on every address.
func TestLocalURL(t *testing.T) {
	for _, c := range []struct {
		boot config.Bootstrap
		want string
	}{
		{config.Bootstrap{Listen: []string{":8484"}}, "http://localhost:8484/"},
		{config.Bootstrap{Listen: []string{"0.0.0.0:8484", ":9000"}}, "http://localhost:8484/"},
		{config.Bootstrap{Listen: []string{"[::]:8484"}}, "http://localhost:8484/"},
		{config.Bootstrap{Listen: []string{"192.168.1.5:8484"}}, "http://192.168.1.5:8484/"},
		{config.Bootstrap{Listen: []string{":8443"}, TLSCert: "c.pem", BasePath: "/kivo"}, "https://localhost:8443/kivo/"},
	} {
		if got := localURL(c.boot); got != c.want {
			t.Errorf("%v: %s, want %s", c.boot.Listen, got, c.want)
		}
	}
}
