package discovery

import (
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// LocalURLs returns the server's URLs on its local networks, ordered for a
// device at near. For each port the server listens on with every address,
// there is a URL for each IPv4 address of its network interfaces. Each
// single address the server listens on also gets a URL. lanAddrs picks and
// orders the interface addresses.
func (r *Responder) LocalURLs(near netip.Addr) []string {
	var local []netip.Prefix
	for _, n := range r.deps.Settings.Get().LocalNetworks {
		if p, err := netip.ParsePrefix(n); err == nil {
			local = append(local, p.Masked())
		}
	}
	addrs := lanAddrs(interfaces(), near, local)

	scheme := "http"
	if r.deps.TLS {
		scheme = "https"
	}
	out := []string{}
	add := func(ip netip.Addr, port uint16) {
		u := (&url.URL{Scheme: scheme, Host: netip.AddrPortFrom(ip, port).String(), Path: r.deps.BasePath}).String()
		if !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	for _, l := range r.deps.Listen {
		host, p, err := net.SplitHostPort(l)
		port, perr := strconv.ParseUint(p, 10, 16)
		if err != nil || perr != nil {
			continue
		}
		var ip netip.Addr
		if host != "" {
			if ip, err = netip.ParseAddr(host); err != nil {
				continue // a host name says nothing about addresses
			}
		}
		switch ip = ip.Unmap(); {
		case !ip.IsValid() || ip.IsUnspecified():
			for _, a := range addrs {
				add(a, uint16(port))
			}
		case !ip.IsLoopback():
			add(ip, uint16(port))
		}
	}
	return out
}

// netIface is a network interface that is up, with its name, its addresses
// and whether it carries multicast.
type netIface struct {
	name      string
	addrs     []netip.Prefix
	multicast bool
}

// interfaces lists the host's interfaces that are up and not loopback.
func interfaces() []netIface {
	ifs, _ := net.Interfaces()
	var out []netIface
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		ni := netIface{name: i.Name, multicast: i.Flags&net.FlagMulticast != 0}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				ip, _ := netip.AddrFromSlice(n.IP)
				bits, _ := n.Mask.Size()
				ni.addrs = append(ni.addrs, netip.PrefixFrom(ip.Unmap(), bits))
			}
		}
		out = append(out, ni)
	}
	return out
}

// Interface name prefixes that no device on the LAN reaches. Container and
// VM bridges and virtual Ethernet pairs are never listed. VPN tunnels are
// listed only for an asker on that network.
var (
	virtualIfaces = []string{"docker", "br-", "veth", "virbr", "cni", "flannel", "podman"}
	vpnIfaces     = []string{"tailscale", "wg"}
)

// lanIfaces returns the interfaces the server is advertised on. Those are
// the interfaces that carry multicast and have an IPv4 address a LAN device
// can reach. Virtual interfaces and VPN tunnels are left out.
func lanIfaces(ifs []netIface) []string {
	var out []string
	for _, i := range ifs {
		name := strings.ToLower(i.name)
		hasPrefix := func(p string) bool { return strings.HasPrefix(name, p) }
		if !i.multicast || name == "lo" || slices.ContainsFunc(virtualIfaces, hasPrefix) || slices.ContainsFunc(vpnIfaces, hasPrefix) {
			continue
		}
		if slices.ContainsFunc(i.addrs, func(p netip.Prefix) bool {
			ip := p.Addr()
			return ip.Is4() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
		}) {
			out = append(out, i.name)
		}
	}
	return out
}

// lanAddrs returns the IPv4 addresses a device at near could reach the
// server by. Virtual interfaces, loopback and link-local addresses are left
// out. The asker's own network comes first, then addresses in the
// configured local networks, then the others. Each group keeps interface
// order.
func lanAddrs(ifs []netIface, near netip.Addr, local []netip.Prefix) []netip.Addr {
	type ranked struct {
		addr netip.Addr
		rank int
	}
	var cs []ranked
	for _, i := range ifs {
		name := strings.ToLower(i.name)
		hasPrefix := func(p string) bool { return strings.HasPrefix(name, p) }
		if name == "lo" || slices.ContainsFunc(virtualIfaces, hasPrefix) {
			continue
		}
		vpn := slices.ContainsFunc(vpnIfaces, hasPrefix)
		for _, p := range i.addrs {
			ip := p.Addr()
			if !ip.Is4() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			mine := p.Masked().Contains(near)
			inLocal := slices.ContainsFunc(local, func(l netip.Prefix) bool { return l.Contains(ip) })
			switch {
			case mine:
				cs = append(cs, ranked{ip, 0})
			case vpn:
			case inLocal:
				cs = append(cs, ranked{ip, 1})
			default:
				cs = append(cs, ranked{ip, 2})
			}
		}
	}
	slices.SortStableFunc(cs, func(a, b ranked) int { return a.rank - b.rank })
	out := make([]netip.Addr, len(cs))
	for i, c := range cs {
		out[i] = c.addr
	}
	return out
}
