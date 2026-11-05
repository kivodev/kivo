// Package egress is the one way the server fetches from the internet. It
// speaks only HTTPS, and only to public addresses. The check runs on the
// address actually dialled, after DNS and again for every redirect. So a
// provider response or a DNS answer can't point a request at the LAN or at
// the server itself. Proxy environment variables are ignored, and every
// response is bounded in time and size. Its Limiter spaces an outside
// service's requests and tracks the service's outage, resting it after
// failures or a refused key. The package uses only the standard library.
package egress
