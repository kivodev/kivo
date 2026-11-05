package discovery

import (
	"context"
	"maps"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brutella/dnssd"
	"github.com/miekg/dns"
	"golang.org/x/net/ipv4"

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/store/storetest"
)

// newResponder returns a Responder for a server at version 1.2.3, served as
// d says, with its own settings.
func newResponder(t *testing.T, d Deps) (*Responder, *config.Store) {
	t.Helper()
	settings, err := config.Open(context.Background(), storetest.New(t), config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	d.Settings, d.Version = settings, "1.2.3"
	return New(d), settings
}

// The service is named after the server and uses its own host name and the
// port the server listens on. Its TXT record holds the ID, version, base
// path and scheme.
func TestService(t *testing.T) {
	r, settings := newResponder(t, Deps{BasePath: "/kivo", Listen: []string{":8484"}, TLS: true})
	id := settings.ServerID().String()
	got := r.service(8484, []string{"eth0"})
	want := map[string]string{"id": id, "v": "1.2.3", "path": "/kivo", "tls": "1"}
	if got.Name != "Kivo" || got.Type != "_kivo._tcp" || got.Host != "kivo-"+id[len(id)-6:] || got.Port != 8484 ||
		!slices.Equal(got.Ifaces, []string{"eth0"}) || !maps.Equal(got.Text, want) {
		t.Errorf("service %+v", got)
	}
}

// The port advertised is one a device on the LAN reaches.
func TestListenPort(t *testing.T) {
	for _, c := range []struct {
		listen []string
		port   int
		ok     bool
	}{
		{[]string{":8484"}, 8484, true},
		{[]string{"127.0.0.1:9000", "0.0.0.0:8484"}, 8484, true},
		{[]string{"[::1]:9000", "192.168.1.5:8080"}, 8080, true},
		{[]string{"127.0.0.1:9000"}, 0, false},
		{[]string{"nas.local:8484"}, 0, false},
		{nil, 0, false},
	} {
		if port, ok := listenPort(c.listen); port != c.port || ok != c.ok {
			t.Errorf("%v: %d %v", c.listen, port, ok)
		}
	}
}

// The server is advertised only where a LAN device can hear it. It isn't
// advertised on bridges, virtual Ethernet pairs or VPN tunnels, or where
// there's no multicast or no IPv4 address.
func TestLANIfaces(t *testing.T) {
	pfx := netip.MustParsePrefix
	ifs := []netIface{
		{name: "lo", addrs: []netip.Prefix{pfx("127.0.0.1/8")}, multicast: true},
		{name: "docker0", addrs: []netip.Prefix{pfx("172.17.0.1/16")}, multicast: true},
		{name: "br-3f2a9c", addrs: []netip.Prefix{pfx("172.18.0.1/16")}, multicast: true},
		{name: "veth12ab", addrs: []netip.Prefix{pfx("10.200.0.1/24")}, multicast: true},
		{name: "tailscale0", addrs: []netip.Prefix{pfx("100.64.0.7/32")}, multicast: true},
		{name: "wg0", addrs: []netip.Prefix{pfx("10.8.0.1/24")}},
		{name: "tun0", addrs: []netip.Prefix{pfx("10.9.0.1/24")}},
		{name: "eth0", addrs: []netip.Prefix{pfx("10.0.0.1/24"), pfx("fe80::1/64")}, multicast: true},
		{name: "eth1", addrs: []netip.Prefix{pfx("fe80::2/64"), pfx("169.254.9.9/16")}, multicast: true},
		{name: "br0", addrs: []netip.Prefix{pfx("192.168.50.2/24")}, multicast: true},
	}
	if got := lanIfaces(ifs); !slices.Equal(got, []string{"eth0", "br0"}) {
		t.Errorf("advertised on %v", got)
	}
}

// When advertised on a LAN interface of this host, the server announces
// itself, answers a query for the service type, and says goodbye when it
// stops.
func TestAdvertise(t *testing.T) {
	ifaces := lanIfaces(interfaces())
	if len(ifaces) == 0 {
		t.Skip("no LAN interface with multicast")
	}
	ifi, err := net.InterfaceByName(ifaces[0])
	if err != nil {
		t.Fatal(err)
	}
	group, err := net.ListenMulticastUDP("udp4", ifi, dnssd.AddrIPv4LinkLocalMulticast)
	if err != nil {
		t.Fatal(err)
	}
	defer group.Close()
	_ = group.SetDeadline(time.Now().Add(20 * time.Second))

	r, settings := newResponder(t, Deps{Listen: []string{":8484"}})
	id := settings.ServerID().String()
	instance := "Kivo." + ServiceType + ".local."
	ctx, stop := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := r.advertise(ctx, ifaces[:1]); err != nil {
			t.Errorf("advertise: %v", err)
		}
	})
	defer func() {
		stop()
		wg.Wait()
	}()

	// The announcement: the service's PTR, SRV and TXT.
	announced := awaitResponse(t, group, func(m *dns.Msg) bool { return ptrTo(m, instance) != nil })
	checkService(t, announced, instance, id)

	// A query from a port other than 5353 is answered by unicast to it.
	q := new(dns.Msg)
	q.SetQuestion(ServiceType+".local.", dns.TypePTR)
	answer := query(t, ifi, q)
	if ptrTo(answer, instance) == nil {
		t.Errorf("the query's answer %v", answer)
	}

	stop()
	bye := awaitResponse(t, group, func(m *dns.Msg) bool {
		ptr := ptrTo(m, instance)
		return ptr != nil && ptr.Hdr.Ttl == 0
	})
	if bye == nil {
		t.Error("no goodbye")
	}
}

// awaitResponse reads mDNS messages from conn until a response matches.
func awaitResponse(t *testing.T, conn *net.UDPConn, match func(*dns.Msg) bool) *dns.Msg {
	t.Helper()
	buf := make([]byte, 9000)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			t.Fatalf("waiting for a response: %v", err)
		}
		m := new(dns.Msg)
		if m.Unpack(buf[:n]) == nil && m.Response && match(m) {
			return m
		}
	}
}

// query sends q to the mDNS group out of ifi, from a port of its own, and
// returns the answer.
func query(t *testing.T, ifi *net.Interface, q *dns.Msg) *dns.Msg {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := ipv4.NewPacketConn(conn).SetMulticastInterface(ifi); err != nil {
		t.Fatal(err)
	}
	b, err := q.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.WriteTo(b, dnssd.AddrIPv4LinkLocalMulticast); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	return awaitResponse(t, conn, func(*dns.Msg) bool { return true })
}

func ptrTo(m *dns.Msg, instance string) *dns.PTR {
	for _, rr := range slices.Concat(m.Answer, m.Extra) {
		if p, ok := rr.(*dns.PTR); ok && strings.EqualFold(p.Ptr, instance) {
			return p
		}
	}
	return nil
}

// checkService checks that m names the instance at port 8484 on the
// server's own host, with its TXT record.
func checkService(t *testing.T, m *dns.Msg, instance, id string) {
	t.Helper()
	var srv *dns.SRV
	var txt []string
	for _, rr := range slices.Concat(m.Answer, m.Extra) {
		switch rr := rr.(type) {
		case *dns.SRV:
			if strings.EqualFold(rr.Hdr.Name, instance) {
				srv = rr
			}
		case *dns.TXT:
			if strings.EqualFold(rr.Hdr.Name, instance) {
				txt = rr.Txt
			}
		}
	}
	if srv == nil || srv.Port != 8484 || srv.Target != "kivo-"+id[len(id)-6:]+".local." {
		t.Errorf("SRV %v", srv)
	}
	slices.Sort(txt)
	if want := []string{"id=" + id, "path=", "tls=0", "v=1.2.3"}; !slices.Equal(txt, want) {
		t.Errorf("TXT %q, want %q", txt, want)
	}
}

// Local URLs list only addresses a LAN device can reach. They include the
// server's own networks, but not Docker's or libvirt's bridges, or a VPN
// the asker isn't on.
func TestLANAddrs(t *testing.T) {
	pfx := netip.MustParsePrefix
	ifs := []netIface{
		{name: "lo", addrs: []netip.Prefix{pfx("127.0.0.1/8")}},
		{name: "docker0", addrs: []netip.Prefix{pfx("172.17.0.1/16")}},
		{name: "br-3f2a9c", addrs: []netip.Prefix{pfx("172.18.0.1/16")}},
		{name: "veth12ab", addrs: []netip.Prefix{pfx("10.200.0.1/24")}},
		{name: "virbr0", addrs: []netip.Prefix{pfx("192.168.122.1/24")}},
		{name: "cni0", addrs: []netip.Prefix{pfx("10.88.0.1/16")}},
		{name: "flannel.1", addrs: []netip.Prefix{pfx("10.244.0.0/32")}},
		{name: "podman0", addrs: []netip.Prefix{pfx("10.89.0.1/24")}},
		{name: "tailscale0", addrs: []netip.Prefix{pfx("100.64.0.7/32")}},
		{name: "wg0", addrs: []netip.Prefix{pfx("10.8.0.1/24")}},
		{name: "eth0", addrs: []netip.Prefix{pfx("10.0.0.1/24"), pfx("fe80::1/64"), pfx("169.254.9.9/16")}},
		{name: "eth1", addrs: []netip.Prefix{pfx("192.168.50.2/24")}},
	}
	addr := netip.MustParseAddr
	for _, c := range []struct {
		name  string
		near  netip.Addr
		local []netip.Prefix
		want  []string
	}{
		{"a phone on eth1's network", addr("192.168.50.30"), nil, []string{"192.168.50.2", "10.0.0.1"}},
		{"local networks first", addr("203.0.113.9"), []netip.Prefix{pfx("192.168.50.0/24")}, []string{"192.168.50.2", "10.0.0.1"}},
		{"the asker's network before local networks", addr("10.0.0.20"), []netip.Prefix{pfx("192.168.50.0/24")}, []string{"10.0.0.1", "192.168.50.2"}},
		{"a VPN only to an asker on it", addr("10.8.0.5"), nil, []string{"10.8.0.1", "10.0.0.1", "192.168.50.2"}},
		{"never a bridge, even to a container on it", addr("172.17.0.5"), nil, []string{"10.0.0.1", "192.168.50.2"}},
	} {
		var got []string
		for _, a := range lanAddrs(ifs, c.near, c.local) {
			got = append(got, a.String())
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
