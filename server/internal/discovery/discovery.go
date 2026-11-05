package discovery

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"sync"

	"github.com/brutella/dnssd"

	"github.com/mmcdole/kivo/internal/config"
)

// ServiceType is the DNS-SD service type the server advertises.
const ServiceType = "_kivo._tcp"

// Deps describe the server a Responder advertises: its settings and
// version, and where it's served.
type Deps struct {
	// Settings hold the server's ID, name and local networks.
	Settings *config.Store
	Version  string
	// Listen are the addresses the server listens on. With none, nothing
	// is advertised.
	Listen []string
	// TLS is whether the server serves HTTPS on those addresses.
	TLS bool
	// BasePath is the path the server is served under, used in its URLs.
	// It is empty for the root, and otherwise has a leading slash and no
	// trailing slash.
	BasePath string
}

// Responder advertises the server on the local network and lists its local
// URLs.
type Responder struct {
	deps    Deps
	renamed chan struct{}
}

// New returns a Responder for the server d describes.
func New(d Deps) *Responder {
	return &Responder{deps: d, renamed: make(chan struct{}, 1)}
}

// Renamed tells the advertisement that the server's name changed, so the
// server is announced again under the new name. It doesn't block.
func (r *Responder) Renamed() {
	select {
	case r.renamed <- struct{}{}:
	default:
	}
}

// Advertise announces the server over multicast DNS as a ServiceType
// service on its LAN interfaces, named by its serverName setting. When ctx
// ends it says goodbye. Its TXT record holds the server's ID (id), version
// (v), base path (path) and whether it serves HTTPS (tls). With no LAN
// interface, or no port reachable from one, there's nothing to advertise
// and it returns nil at once.
func (r *Responder) Advertise(ctx context.Context) error {
	if err := r.advertise(ctx, lanIfaces(interfaces())); err != nil {
		return err
	}
	return nil
}

func (r *Responder) advertise(ctx context.Context, ifaces []string) error {
	port, ok := listenPort(r.deps.Listen)
	if !ok || len(ifaces) == 0 {
		return nil
	}
	rp, err := dnssd.NewResponder()
	if err != nil {
		return err
	}
	cfg := r.service(port, ifaces)
	svc, err := dnssd.NewService(cfg)
	if err != nil {
		return err
	}
	h, err := rp.Add(svc)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Go(func() { r.rename(ctx, rp, h, cfg) })
	if err := rp.Respond(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// rename announces the service again under the server's new name each time
// the server is renamed, until ctx ends.
func (r *Responder) rename(ctx context.Context, rp dnssd.Responder, h dnssd.ServiceHandle, cfg dnssd.Config) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.renamed:
		}
		name := r.deps.Settings.Get().ServerName
		if name == cfg.Name {
			continue
		}
		cfg.Name = name
		svc, err := dnssd.NewService(cfg)
		if err == nil {
			rp.Remove(h)
			h, err = rp.Add(svc)
		}
		if err != nil {
			slog.WarnContext(ctx, "discovery: can't announce the new name", "name", name, "err", err)
			return
		}
	}
}

// service describes the server as a DNS-SD service on port, at ifaces. It
// uses its own host name, "kivo-" plus the end of the server's ID, so the
// host's own name is left to the host.
func (r *Responder) service(port int, ifaces []string) dnssd.Config {
	id := r.deps.Settings.ServerID().String()
	tls := "0"
	if r.deps.TLS {
		tls = "1"
	}
	return dnssd.Config{
		Name:   r.deps.Settings.Get().ServerName,
		Type:   ServiceType,
		Host:   "kivo-" + id[len(id)-6:],
		Port:   port,
		Ifaces: ifaces,
		Text:   map[string]string{"id": id, "v": r.deps.Version, "path": r.deps.BasePath, "tls": tls},
	}
}

// listenPort returns the first port the server listens on that a LAN
// device can reach. That is a port on every address, or on an address that
// isn't loopback.
func listenPort(listen []string) (int, bool) {
	for _, l := range listen {
		host, p, err := net.SplitHostPort(l)
		port, perr := strconv.ParseUint(p, 10, 16)
		if err != nil || perr != nil || port == 0 {
			continue
		}
		if host == "" {
			return int(port), true
		}
		if ip, err := netip.ParseAddr(host); err == nil && !ip.IsLoopback() {
			return int(port), true
		}
	}
	return 0, false
}
