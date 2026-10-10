package service

import (
	"net"
	"net/netip"
	"sort"
)

// ServerAddressCandidate is one server-side interface address offered to the
// admin UI for prefilling the Custom Client Builder's server fields
// (issue #82). The label carries where the address came from so the UI can
// make clear these are the server's own interface addresses — in Docker/NAT
// setups a bridge address may not be reachable from clients.
type ServerAddressCandidate struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Family    string `json:"family"` // "ipv4" | "ipv6"
}

// NetworkInterface is a testable snapshot of one host network interface.
// Production uses SystemNetworkInterfaces; tests inject synthetic snapshots so
// enumeration never depends on a live network.
type NetworkInterface struct {
	Name  string
	Flags net.Flags
	Addrs []netip.Addr
}

// NetworkEnumerator returns the host's network interface snapshots.
type NetworkEnumerator func() ([]NetworkInterface, error)

// SystemNetworkInterfaces enumerates the host's interfaces via net.Interfaces.
func SystemNetworkInterfaces() ([]NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	snapshots := make([]NetworkInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			// One unreadable interface must not hide the remaining candidates.
			continue
		}
		snapshot := NetworkInterface{Name: iface.Name, Flags: iface.Flags}
		for _, addr := range addrs {
			if parsed, ok := parseInterfaceAddr(addr); ok {
				snapshot.Addrs = append(snapshot.Addrs, parsed)
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// parseInterfaceAddr extracts the IP from the address kinds net.Interface.Addrs
// returns and unmaps IPv4-mapped IPv6 so the family label matches the address.
func parseInterfaceAddr(address net.Addr) (netip.Addr, bool) {
	var ip net.IP
	switch v := address.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		return netip.Addr{}, false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// ListServerAddressCandidates enumerates the server's own interface addresses
// and reduces them to the labeled candidates returned by
// GET /admin/config/server_addresses. Resolution is fully server-side; no
// outbound network call of any kind is made.
//
// Filtering rules (the documented contract):
//   - interfaces that are down or loopback are skipped entirely;
//   - addresses that are loopback, unspecified (0.0.0.0 / ::), multicast, or
//     link-local (169.254.0.0/16, fe80::/10) are skipped — none of them is a
//     usable RustDesk endpoint for a client.
//
// Remaining unicast addresses (public, RFC1918, and IPv6 unique-local) are
// kept — including Docker bridge/container addresses, which the UI labels as
// the server's own interface address. Candidates are ordered deterministically
// (IPv4 first, then interface name, then address) so the UI list is stable.
func ListServerAddressCandidates(enumerate NetworkEnumerator) ([]ServerAddressCandidate, error) {
	ifaces, err := enumerate()
	if err != nil {
		return nil, err
	}
	return filterServerAddressCandidates(ifaces), nil
}

func filterServerAddressCandidates(ifaces []NetworkInterface) []ServerAddressCandidate {
	candidates := make([]ServerAddressCandidate, 0)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		for _, addr := range iface.Addrs {
			if !addr.IsValid() ||
				addr.IsLoopback() ||
				addr.IsUnspecified() ||
				addr.IsMulticast() ||
				addr.IsLinkLocalUnicast() ||
				addr.IsLinkLocalMulticast() {
				continue
			}
			family := "ipv6"
			if addr.Is4() {
				family = "ipv4"
			}
			candidates = append(candidates, ServerAddressCandidate{
				Interface: iface.Name,
				Address:   addr.String(),
				Family:    family,
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Family != candidates[j].Family {
			return candidates[i].Family == "ipv4"
		}
		if candidates[i].Interface != candidates[j].Interface {
			return candidates[i].Interface < candidates[j].Interface
		}
		return candidates[i].Address < candidates[j].Address
	})
	return candidates
}
