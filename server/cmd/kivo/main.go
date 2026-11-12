// Command kivo is the Kivo media server. It is the only place where
// components and their tasks are wired together.
//
//	kivo [-config path]                     run the server
//	kivo restore [-config path] <snapshot>  restore the database from a snapshot
//	kivo healthcheck [-config path]         exit 0 if the local server answers /healthz
//	kivo reset-password [-config path] <username>
//	                                        set a user's password and sign out its sessions
//	kivo version                            print the version
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mmcdole/kivo/internal/config"
	kivoversion "github.com/mmcdole/kivo/internal/version"
)

// version and commit are set at build time, with -ldflags "-X
// main.version=… -X main.commit=…": version for a release build, commit,
// the short commit hash, for any other. init turns them into the version
// the server reports.
var version, commit string

func init() {
	version = kivoversion.Of(version, commit)
}

func main() {
	if err := command(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kivo:", err)
		alert(err)
		os.Exit(1)
	}
}

func command(args []string) error {
	cmd := "serve"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("kivo "+cmd, flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to server.toml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch cmd {
	case "serve":
		return serve(*configPath)
	case "restore":
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: kivo restore [-config path] <snapshot>")
		}
		return restore(*configPath, fs.Arg(0))
	case "healthcheck":
		return healthcheck(*configPath)
	case "reset-password":
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: kivo reset-password [-config path] <username>")
		}
		return resetPassword(*configPath, fs.Arg(0), os.Stdin, os.Stdout)
	case "version":
		fmt.Println("kivo", version)
		return nil
	}
	return fmt.Errorf("unknown command %q", cmd)
}
