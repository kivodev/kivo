// Package release finds out whether a newer Kivo release is out, for the
// admin's notice. Shortly after startup and then once a day it asks GitHub
// for Kivo's latest release, with a plain HTTPS GET through egress that
// says nothing about the server. A build that isn't a release, such as
// dev, never asks.
package release
