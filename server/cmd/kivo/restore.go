package main

import (
	"context"
	"fmt"

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/store"
)

// restore replaces the database with a snapshot. The snapshot is either
// the one taken before migrating, for rolling back an upgrade together with
// the previous binary, or a nightly backup. It refuses while the server is
// running.
func restore(configPath, snapshot string) error {
	boot, err := config.LoadBootstrap(configPath)
	if err != nil {
		return err
	}
	dst := boot.DatabasePath()
	if err := store.Restore(context.Background(), dst, snapshot); err != nil {
		return err
	}
	fmt.Printf("Restored %s from %s. Start the server with the release that matches it.\n", dst, snapshot)
	return nil
}
