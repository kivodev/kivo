package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"golang.org/x/term"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// resetPassword sets a user's password and signs out all of the user's
// sessions. It is for whoever runs the server, as the way back in for an
// admin who is locked out. The password is asked for twice on a terminal,
// or read as one line from stdin. It is never taken from the arguments,
// which shell history and ps keep. It works while the server runs. Sessions
// are checked in the database on each request, so the old ones stop at
// once, and an open live-events socket closes at its next recheck.
func resetPassword(configPath, username string, stdin *os.File, out io.Writer) error {
	boot, err := config.LoadBootstrap(configPath)
	if err != nil {
		return err
	}
	if err := runAsOwner(boot.DatabasePath()); err != nil {
		return err
	}
	ctx := context.Background()
	db, settings, err := openDatabase(ctx, boot)
	if err != nil {
		return err
	}
	defer db.Close()
	u := users.New(db)
	all, err := u.Users(ctx)
	if err != nil {
		return err
	}
	// Usernames compare case-insensitively, but an exact match wins.
	i := slices.IndexFunc(all, func(x users.User) bool { return x.Username == username })
	if i < 0 {
		i = slices.IndexFunc(all, func(x users.User) bool { return strings.EqualFold(x.Username, username) })
	}
	if i < 0 {
		return fmt.Errorf("no user named %q", username)
	}
	password, err := readPassword(stdin, out)
	if err != nil {
		return err
	}
	// This process isn't the running server, so nobody listens on this bus
	// and no socket is open on it.
	a := auth.New(db, auth.Deps{Users: u, Settings: settings, Activity: activity.New(db, events.NewBus())})
	if err := a.SetPassword(ctx, auth.Caller{}, all[i].ID, password); err != nil {
		return err
	}
	fmt.Fprintf(out, "Set %s's password and signed out all of their sessions.\n", all[i].Username)
	return nil
}

// readPassword asks for a new password twice, without echo, when in is a
// terminal. Otherwise it reads one line of in.
func readPassword(in *os.File, out io.Writer) (string, error) {
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && (!errors.Is(err, io.EOF) || line == "") {
			return "", errors.New("no password on stdin")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	var typed [2]string
	for i, prompt := range []string{"New password: ", "Again: "} {
		fmt.Fprint(out, prompt)
		p, err := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if err != nil {
			return "", err
		}
		typed[i] = string(p)
	}
	if typed[0] != typed[1] {
		return "", errors.New("the passwords don't match")
	}
	return typed[0], nil
}

// openDatabase opens the server's database and its settings, for a command
// run alongside the server. The database must already exist, because
// opening a missing one would create an empty one.
func openDatabase(ctx context.Context, boot config.Bootstrap) (*store.DB, *config.Store, error) {
	if _, err := os.Stat(boot.DatabasePath()); err != nil {
		return nil, nil, err
	}
	db, err := store.Open(ctx, boot.DatabasePath())
	if err != nil {
		return nil, nil, err
	}
	settings, err := config.Open(ctx, db, config.Options{})
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	return db, settings, nil
}
