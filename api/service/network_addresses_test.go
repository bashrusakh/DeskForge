package service

import (
	"errors"
	"net"
	"net/netip"
	"reflect"
	"testing"
)

func addr(t *testing.T, value string) netip.Addr {
	t.Helper()
	parsed, err := netip.ParseAddr(value)
	if err != nil {
		t.Fatalf("parse addr %q: %v", value, err)
	}
	return parsed
}

// The synthetic list exercises every documented filter class plus the classes
// that must be kept (RFC1918, Docker bridge, IPv6 unique-local/global).
func syntheticInterfaces(t *testing.T) []NetworkInterface {
	return []NetworkInterface{
		{
			Name:  "lo",
			Flags: net.FlagUp | net.FlagLoopback,
			Addrs: []netip.Addr{addr(t, "127.0.0.1"), addr(t, "::1")},
		},
		{
			Name:  "down0",
			Flags: net.FlagMulticast, // not up
			Addrs: []netip.Addr{addr(t, "10.0.0.5")},
		},
		{
			Name:  "eth0",
			Flags: net.FlagUp | net.FlagMulticast,
			Addrs: []netip.Addr{
				addr(t, "192.168.1.66"),
				addr(t, "fe80::1"),
				addr(t, "169.254.10.20"),
				addr(t, "2001:db8::5"),
				addr(t, "224.0.0.1"),
				addr(t, "0.0.0.0"),
				addr(t, "::"),
			},
		},
		{
			Name:  "docker0",
			Flags: net.FlagUp,
			Addrs: []netip.Addr{addr(t, "172.17.0.1")},
		},
		{
			Name:  "v6only",
			Flags: net.FlagUp,
			Addrs: []netip.Addr{addr(t, "fd00::1")},
		},
	}
}

func TestListServerAddressCandidatesFiltersAndLabels(t *testing.T) {
	candidates, err := ListServerAddressCandidates(func() ([]NetworkInterface, error) {
		return syntheticInterfaces(t), nil
	})
	if err != nil {
		t.Fatalf("ListServerAddressCandidates: %v", err)
	}
	want := []ServerAddressCandidate{
		{Interface: "docker0", Address: "172.17.0.1", Family: "ipv4"},
		{Interface: "eth0", Address: "192.168.1.66", Family: "ipv4"},
		{Interface: "eth0", Address: "2001:db8::5", Family: "ipv6"},
		{Interface: "v6only", Address: "fd00::1", Family: "ipv6"},
	}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("candidates = %+v, want %+v", candidates, want)
	}
}

func TestListServerAddressCandidatesEmptyWhenNothingUsable(t *testing.T) {
	candidates, err := ListServerAddressCandidates(func() ([]NetworkInterface, error) {
		return []NetworkInterface{
			{Name: "lo", Flags: net.FlagUp | net.FlagLoopback, Addrs: []netip.Addr{addr(t, "127.0.0.1")}},
			{Name: "down0", Flags: 0, Addrs: []netip.Addr{addr(t, "10.0.0.5")}},
		}, nil
	})
	if err != nil {
		t.Fatalf("ListServerAddressCandidates: %v", err)
	}
	if candidates == nil || len(candidates) != 0 {
		t.Fatalf("candidates = %+v, want empty non-nil slice", candidates)
	}
}

func TestListServerAddressCandidatesPropagatesEnumeratorError(t *testing.T) {
	sentinel := errors.New("enumerate failed")
	candidates, err := ListServerAddressCandidates(func() ([]NetworkInterface, error) {
		return nil, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
	if candidates != nil {
		t.Fatalf("candidates = %+v, want nil on error", candidates)
	}
}

func TestParseInterfaceAddr(t *testing.T) {
	ipNet := &net.IPNet{IP: net.ParseIP("192.168.1.66"), Mask: net.CIDRMask(24, 32)}
	ipAddr := &net.IPAddr{IP: net.ParseIP("::1")}
	if got, ok := parseInterfaceAddr(ipNet); !ok || got.String() != "192.168.1.66" {
		t.Fatalf("parseInterfaceAddr(IPNet) = %v, %v", got, ok)
	}
	// IPv4-mapped IPv6 must unmap so the family label stays "ipv4".
	mapped := &net.IPNet{IP: net.ParseIP("::ffff:10.1.2.3"), Mask: net.CIDRMask(96, 128)}
	if got, ok := parseInterfaceAddr(mapped); !ok || got.String() != "10.1.2.3" || !got.Is4() {
		t.Fatalf("parseInterfaceAddr(mapped) = %v, %v", got, ok)
	}
	if got, ok := parseInterfaceAddr(ipAddr); !ok || got.String() != "::1" {
		t.Fatalf("parseInterfaceAddr(IPAddr) = %v, %v", got, ok)
	}
	if _, ok := parseInterfaceAddr(&net.UnixAddr{Name: "/run/x", Net: "unix"}); ok {
		t.Fatal("parseInterfaceAddr(UnixAddr) = ok, want !ok")
	}
}
