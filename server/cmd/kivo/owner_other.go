//go:build !unix

package main

// runAsOwner does nothing where there's no root to run as.
func runAsOwner(string) error { return nil }
