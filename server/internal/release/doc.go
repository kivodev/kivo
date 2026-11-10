// Package release finds out whether a newer Kivo release is out, for the
// admin's notice. Shortly after startup and then once a day it asks GitHub
// for Kivo's latest release, with a plain HTTPS GET through egress that
// says nothing about the server. A dev build asks too, and hears of a
// release after the one it's built past.
package release
