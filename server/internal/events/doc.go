// Package events pushes live-update hints to subscribers through bounded
// buffers. Delivery is never promised. Kinds that come in bursts are sent
// at most once per interval. The package also closes a credential's
// live-event sockets when the credential is revoked.
package events
