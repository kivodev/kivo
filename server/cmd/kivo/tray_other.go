//go:build !windows

package main

import (
	"context"

	"github.com/mmcdole/kivo/internal/config"
)

// withTray runs serve. Only Windows has a tray.
func withTray(ctx context.Context, _ config.Bootstrap, serve func(context.Context) error) error {
	return serve(ctx)
}

// alert does nothing, since the error is already on stderr.
func alert(error) {}
