package service

// This file is the one authoritative host policy for the branding URLs the
// public build workflow may fetch. It holds the non-public address table
// (isPublicLiteralIP), the literal-host canonicalization that keeps the
// resolver-style IPv4 spellings and trailing dots (parseLiteralHostAddr), the
// localhost-name rule, and the hostname-plausibility rule, so the public origin
// (public-api-server), an absolute branding image URL, the dispatch gate, and
// the stored-data migration seam all make the same decision in one place.
// Nothing in this file resolves DNS.

import (
	"net/netip"
	"strconv"
	"strings"
	"unicode"
)

// nonPublicFetchPrefixes is the single authoritative address policy behind the
// public-fetch host rule. A literal address inside any of these blocks is not a
// public fetch target for the build runner.
//
// This is the non-public special-purpose policy relevant to a *literal public
// fetch target* for this contract: it is drawn from the IANA IPv4 and IPv6
// Special-Purpose Address Registries plus multicast and the transition and
// deprecated forms that embed an IPv4 destination this literal policy cannot
// verify (::/96, 64:ff9b::/96, 64:ff9b:1::/48, 2002::/16). It is deliberately
// not an exhaustive implementation of Internet routing or a general SSRF
// framework. It makes no DNS lookup and makes no claim that a hostname is
// public or reachable, which is finally constrained at the workflow fetch
// boundary.
var nonPublicFetchPrefixes = []netip.Prefix{
	// IPv4 special-purpose and non-unicast blocks.
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network" / unspecified
	netip.MustParsePrefix("127.0.0.0/8"),     // loopback
	netip.MustParsePrefix("10.0.0.0/8"),      // RFC 1918 private
	netip.MustParsePrefix("172.16.0.0/12"),   // RFC 1918 private
	netip.MustParsePrefix("192.168.0.0/16"),  // RFC 1918 private
	netip.MustParsePrefix("169.254.0.0/16"),  // link-local
	netip.MustParsePrefix("100.64.0.0/10"),   // shared address space (RFC 6598)
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("192.88.99.0/24"),  // 6to4 relay anycast (deprecated)
	netip.MustParsePrefix("224.0.0.0/4"),     // multicast
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved (covers 255.255.255.255)

	// IPv6 special-purpose, transition, and non-unicast blocks.
	netip.MustParsePrefix("::/128"),  // unspecified
	netip.MustParsePrefix("::1/128"), // loopback
	// ::/96 is the deprecated IPv4-compatible address form (::a.b.c.d). It is
	// not a routable public IPv6 destination, and it embeds an IPv4 destination
	// this literal policy cannot verify, so it is refused fail-closed rather
	// than being unmapped into an accepted IPv4 address.
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("fe80::/10"),      // link-local
	netip.MustParsePrefix("fc00::/7"),       // unique-local (RFC 4193)
	netip.MustParsePrefix("ff00::/8"),       // multicast
	netip.MustParsePrefix("100::/64"),       // discard-only
	netip.MustParsePrefix("100:0:0:1::/64"), // dummy IPv6 prefix (RFC 9780)
	netip.MustParsePrefix("2001:db8::/32"),  // documentation
	netip.MustParsePrefix("3fff::/20"),      // documentation (RFC 9637)
	netip.MustParsePrefix("5f00::/16"),      // Segment Routing (SRv6) SIDs (RFC 9602)
	netip.MustParsePrefix("2001::/32"),      // Teredo
	netip.MustParsePrefix("2001:2::/48"),    // benchmarking
	netip.MustParsePrefix("2001:10::/28"),   // ORCHID
	netip.MustParsePrefix("2001:20::/28"),   // ORCHIDv2
	netip.MustParsePrefix("64:ff9b::/96"),   // NAT64 well-known prefix (embeds IPv4)
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64 (embeds IPv4)
	netip.MustParsePrefix("2002::/16"),      // 6to4 (embeds IPv4)
}

// isPublicLiteralIP reports whether addr is a literal public fetch target: a
// valid, zoned-free address outside every block in nonPublicFetchPrefixes. An
// IPv4-mapped IPv6 literal such as ::ffff:127.0.0.1 is unmapped first and
// classified as its IPv4 address. An invalid address is not public, and a zoned
// address is refused (fail closed) because prefix containment cannot classify
// one.
func isPublicLiteralIP(addr netip.Addr) bool {
	if !addr.IsValid() || addr.Zone() != "" {
		return false
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	for _, prefix := range nonPublicFetchPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// canonicalizeHost lowercases host and strips trailing dots, the spellings a
// resolver treats as naming the same host ("127.0.0.1." and "LOCALHOST." are the
// hosts "127.0.0.1" and "localhost").
func canonicalizeHost(host string) string {
	return strings.TrimRight(strings.ToLower(host), ".")
}

// parseLiteralHostAddr parses a canonical host as a literal IP address. It
// accepts everything netip.ParseAddr understands (dotted-quad IPv4, IPv6) plus
// the inet_aton IPv4 spellings a resolver also accepts but netip rejects:
// decimal, octal, and hex integers with one to four dot-separated parts
// ("2130706433", "0x7f000001", "0177.0.0.1", "127.1"). It reports false when
// the host is not a literal.
func parseLiteralHostAddr(canonicalHost string) (netip.Addr, bool) {
	if addr, err := netip.ParseAddr(canonicalHost); err == nil {
		return addr, true
	}
	return parseLooseIPv4(canonicalHost)
}

// parseLooseIPv4 interprets host as an IPv4 literal in every textual form an
// inet_aton-style resolver accepts. A single part may be up to 32 bits; with two
// parts the final part absorbs 24 bits; with three, 16 bits; with four, each
// part is one octet. Each part may be decimal, octal (leading "0"), or hex
// ("0x"). It reports false when host is not such a literal.
func parseLooseIPv4(host string) (netip.Addr, bool) {
	parts := strings.Split(host, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return netip.Addr{}, false
	}
	values := make([]uint64, len(parts))
	for i, part := range parts {
		value, ok := parseLooseIPv4Part(part)
		if !ok {
			return netip.Addr{}, false
		}
		values[i] = value
	}
	var raw uint64
	switch len(values) {
	case 1:
		if values[0] > 0xFFFFFFFF {
			return netip.Addr{}, false
		}
		raw = values[0]
	case 2:
		if values[0] > 0xFF || values[1] > 0xFFFFFF {
			return netip.Addr{}, false
		}
		raw = values[0]<<24 | values[1]
	case 3:
		if values[0] > 0xFF || values[1] > 0xFF || values[2] > 0xFFFF {
			return netip.Addr{}, false
		}
		raw = values[0]<<24 | values[1]<<16 | values[2]
	default:
		for _, value := range values {
			if value > 0xFF {
				return netip.Addr{}, false
			}
		}
		raw = values[0]<<24 | values[1]<<16 | values[2]<<8 | values[3]
	}
	return netip.AddrFrom4([4]byte{byte(raw >> 24), byte(raw >> 16), byte(raw >> 8), byte(raw)}), true
}

// parseLooseIPv4Part parses one inet_aton numeric IPv4 part. A "0x"/"0X" prefix
// selects base 16, a bare leading "0" selects base 8, and anything else is
// base 10. It returns false for an empty part or a non-numeric one.
func parseLooseIPv4Part(part string) (uint64, bool) {
	if part == "" {
		return 0, false
	}
	base := 10
	switch {
	case strings.HasPrefix(part, "0x") || strings.HasPrefix(part, "0X"):
		part = part[2:]
		base = 16
	case len(part) > 1 && part[0] == '0':
		part = part[1:]
		base = 8
	}
	if part == "" {
		return 0, false
	}
	value, err := strconv.ParseUint(part, base, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// isLocalhostName reports whether a canonical host is the name "localhost" or a
// subdomain of it. These names are refused without being resolved.
func isLocalhostName(canonicalHost string) bool {
	return canonicalHost == "localhost" || strings.HasSuffix(canonicalHost, ".localhost")
}

// isNonPublicFetchHost applies the literal public-fetch host policy to a
// hostname taken from an absolute URL. It reports true — the host is refused —
// when the canonical host is empty, is the localhost name, is a literal address
// isPublicLiteralIP classifies as non-public, or is neither a recognizable
// literal nor a syntactically plausible hostname (ambiguous forms fail closed).
//
// A plausible non-literal hostname reports false here but is not thereby proven
// public: no DNS lookup happens at create or dispatch, and that name is finally
// constrained at the workflow fetch boundary.
func isNonPublicFetchHost(host string) bool {
	name := canonicalizeHost(host)
	if name == "" || isLocalhostName(name) {
		return true
	}
	if addr, ok := parseLiteralHostAddr(name); ok {
		return !isPublicLiteralIP(addr)
	}
	return !isPlausibleHostname(name)
}

// isPlausibleHostname reports whether host is a syntactically plausible DNS
// hostname, so the public-host rule can fail closed on a host that is neither a
// recognizable IP literal nor a hostname a resolver would treat as a name.
// Unicode letters are accepted for IDN hosts and underscores for the hosts that
// use them; empty and over-long labels and labels that begin or end with a
// hyphen are refused. Any trailing dot has already been stripped by
// canonicalizeHost.
func isPlausibleHostname(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, char := range label {
			if unicode.IsLetter(char) || unicode.IsNumber(char) || char == '-' || char == '_' {
				continue
			}
			return false
		}
	}
	return true
}
