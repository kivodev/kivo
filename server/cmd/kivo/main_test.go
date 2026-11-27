package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A word left over after the flags is refused before any command runs, so
// a command put after a flag doesn't start the server. The config doesn't
// load, so a command that did run would fail at once instead of serving.
func TestCommandRefusesStrayArguments(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "server.toml")
	if err := os.WriteFile(bad, []byte("not_a_key = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-config", bad, "restore", "snap.db"}, `unexpected "restore": put the command before its flags`},
		{[]string{"version", "extra"}, `unexpected argument "extra"`},
		{[]string{"healthcheck", "-config", bad, "extra"}, `unexpected argument "extra"`},
		{[]string{"restore", "a.db", "b.db"}, `unexpected argument "b.db"`},
	} {
		err := command(c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: %v, want %q", c.args, err, c.want)
		}
	}
}

func TestCommandHelp(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"restore", "-h"}} {
		if err := command(args); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%q: %v, want flag.ErrHelp", args, err)
		}
	}
}
