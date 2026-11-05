// Package users manages people. It holds users, their library grants,
// rating ceilings, stream limits and playback preferences. It also holds
// each user's own order of the libraries they see, and which ones they
// pin. It is the only constructor of Viewer, and it never stores
// credentials.
package users
