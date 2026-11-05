// Package health holds what the admin pages read about the server's state.
// It defines the Problem type that components report, so no lower layer
// imports api. It has the registry that collects problems and keeps an
// admin's dismissals. It also keeps the newest log lines in memory.
package health
