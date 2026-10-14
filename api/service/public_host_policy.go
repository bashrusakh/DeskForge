package service

// This file is the one authoritative host policy for the branding URLs the
// public build workflow may fetch. It holds the non-public address table and
// the IANA global-unicast allocation list (isPublicLiteralIP), the literal-host
// canonicalization that keeps the resolver-style IPv4 spellings and at most one
// trailing root-label dot (parseLiteralHostAddr), the localhost-name rule, and
// the hostname-plausibility rule, so the public origin (public-api-server), an
// absolute branding image URL, the dispatch gate, and the stored-data migration
// seam all make the same decision in one place.
// Nothing in this file resolves DNS.

import (
	"net/netip"
	"strconv"
	"strings"
	"unicode"
)

// ianaGlobalUnicastAllocations is the IPv6 half of the literal public-fetch
// policy: the prefixes the IANA IPv6 Global Unicast Address Space registry
// lists as ALLOCATED, transcribed in registry order on 2026-10-14 from the
// registry (last updated 2025-10-10) at
// https://www.iana.org/assignments/ipv6-unicast-address-assignments/
// (ipv6-unicast-address-assignments.csv; the XHTML table carries the same
// rows). The registry's own note is the rule this list implements: "All
// address space in this block not listed in the table below is reserved by
// IANA for future allocation", so an IPv6 literal is public only when it falls
// inside one of these prefixes. The gaps between the rows — 2000::/16 above
// the first row, and every unallocated range between and after them — are
// refused along with every row the registry marks RESERVED. A future
// allocation therefore stays refused until this list is refreshed from the
// registry: the policy fails closed on a stale table, the safe direction for a
// fetch target. IPv4 needs no such list — its complement is the ordinary
// global-unicast space, and every IPv4 Special-Purpose Address Registry row
// that is not Globally Reachable is covered by the nonPublicFetchPrefixes
// table below.
var ianaGlobalUnicastAllocations = []netip.Prefix{
	// 2001::/23 is ALLOCATED but only partially allocated: the
	// nonPublicFetchPrefixes row for the same block refuses it as a whole, and
	// only the special-purpose registry's Globally Reachable more-specifics
	// (globallyReachableFetchExceptions) pass through that row, so listing the
	// block here does not re-admit Teredo, benchmarking, ORCHID, ORCHIDv2 or
	// any other space inside it.
	netip.MustParsePrefix("2001::/23"),      // IANA
	netip.MustParsePrefix("2001:200::/23"),  // APNIC
	netip.MustParsePrefix("2001:400::/23"),  // ARIN
	netip.MustParsePrefix("2001:600::/23"),  // RIPE NCC
	netip.MustParsePrefix("2001:800::/22"),  // RIPE NCC
	netip.MustParsePrefix("2001:c00::/23"),  // APNIC
	netip.MustParsePrefix("2001:e00::/23"),  // APNIC
	netip.MustParsePrefix("2001:1200::/23"), // LACNIC
	netip.MustParsePrefix("2001:1400::/22"), // RIPE NCC
	netip.MustParsePrefix("2001:1800::/23"), // ARIN
	netip.MustParsePrefix("2001:1a00::/23"), // RIPE NCC
	netip.MustParsePrefix("2001:1c00::/22"), // RIPE NCC
	netip.MustParsePrefix("2001:2000::/19"), // RIPE NCC
	netip.MustParsePrefix("2001:4000::/23"), // RIPE NCC
	netip.MustParsePrefix("2001:4200::/23"), // AFRINIC
	netip.MustParsePrefix("2001:4400::/23"), // APNIC
	netip.MustParsePrefix("2001:4600::/23"), // RIPE NCC
	netip.MustParsePrefix("2001:4800::/23"), // ARIN
	netip.MustParsePrefix("2001:4a00::/23"), // RIPE NCC
	netip.MustParsePrefix("2001:4c00::/23"), // RIPE NCC
	netip.MustParsePrefix("2001:5000::/20"), // RIPE NCC
	netip.MustParsePrefix("2001:8000::/19"), // APNIC
	netip.MustParsePrefix("2001:a000::/20"), // APNIC
	netip.MustParsePrefix("2001:b000::/20"), // APNIC
	// 2002::/16 is ALLOCATED but refused on top: 6to4 embeds an IPv4
	// destination this literal policy cannot verify (nonPublicFetchPrefixes).
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2003::/18"), // RIPE NCC
	netip.MustParsePrefix("2400::/12"), // APNIC
	netip.MustParsePrefix("2410::/12"), // APNIC
	netip.MustParsePrefix("2600::/12"), // ARIN
	netip.MustParsePrefix("2610::/23"), // ARIN
	netip.MustParsePrefix("2620::/23"), // ARIN
	netip.MustParsePrefix("2630::/12"), // ARIN
	netip.MustParsePrefix("2800::/12"), // LACNIC
	netip.MustParsePrefix("2a00::/12"), // RIPE NCC
	netip.MustParsePrefix("2a10::/12"), // RIPE NCC
	netip.MustParsePrefix("2c00::/12"), // AFRINIC
}

// nonPublicFetchPrefixes is the single authoritative address policy behind the
// public-fetch host rule. A literal address inside any of these blocks is not a
// public fetch target for the build runner.
//
// Provenance: every block below was checked on 2026-10-14 against the live IANA
// registries — the IPv4 and IPv6 Special-Purpose Address Space registries (IPv6
// last updated 2025-10-09), the IPv6 Address Space registry (2025-10-23), and
// the IPv6 Global Unicast Address Space registry (2025-10-10). It contains every
// special-purpose registry row that is not Globally Reachable plus multicast,
// the transition/deprecated forms that embed an IPv4 destination this literal
// policy cannot verify (::/96, 64:ff9b::/96, 64:ff9b:1::/48, 2002::/16,
// 2001::/32 Teredo), and every prefix the Global Unicast Address Space registry
// marks RESERVED inside the 2000::/3 global-unicast block. The IPv4
// Special-Purpose registry is fully covered by the IPv4 rows, including the
// 192.0.0.0/24 parent row (which contains two Globally Reachable anycast /32s
// that are refused with their parent, fail-closed). It is deliberately not an
// exhaustive implementation of Internet routing or a general SSRF framework. It
// makes no DNS lookup and makes no claim that a hostname is public or reachable,
// which is finally constrained at the workflow fetch boundary.
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

	// IPv6 special-purpose, transition, and non-unicast blocks. Blocks outside
	// the 2000::/3 global-unicast block are listed as well: the allocation list
	// above already refuses them as unlisted, and the rows keep the policy
	// readable as one table.
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
	// 2001::/23 is the IETF protocol-assignments block: its registry row is
	// Globally Reachable only "[u]nless allowed by a more specific allocation",
	// so the whole block is refused and only the registry's Globally Reachable
	// more-specific allocations (globallyReachableFetchExceptions) pass through.
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001::/32"),      // Teredo (transition, embeds IPv4)
	netip.MustParsePrefix("2001:2::/48"),    // benchmarking
	netip.MustParsePrefix("2001:10::/28"),   // deprecated (previously ORCHID), terminated 2014-03
	netip.MustParsePrefix("2001:20::/28"),   // ORCHIDv2 (see globallyReachableFetchExceptions)
	netip.MustParsePrefix("64:ff9b::/96"),   // NAT64 well-known prefix (embeds IPv4)
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64 (embeds IPv4)
	netip.MustParsePrefix("2002::/16"),      // 6to4 (embeds IPv4)

	// Prefixes the IANA IPv6 Global Unicast Address Space registry marks
	// RESERVED inside the 2000::/3 block itself (checked 2026-10-14): the
	// explicit reserved staircase above the last RIR allocation (2c00::/12) and
	// 3ffe::/16, the 6bone range returned to IANA (RFC 5156). The allocation
	// list above already refuses this space as unlisted; the rows keep it
	// refused as explicit non-public blocks too.
	netip.MustParsePrefix("2d00::/8"),
	netip.MustParsePrefix("2e00::/7"),
	netip.MustParsePrefix("3000::/5"),
	netip.MustParsePrefix("3800::/6"),
	netip.MustParsePrefix("3c00::/7"),
	netip.MustParsePrefix("3e00::/8"),
	netip.MustParsePrefix("3f00::/9"),
	netip.MustParsePrefix("3f80::/10"),
	netip.MustParsePrefix("3fc0::/11"),
	netip.MustParsePrefix("3fe0::/12"),
	netip.MustParsePrefix("3ff0::/13"),
	netip.MustParsePrefix("3ff8::/14"),
	netip.MustParsePrefix("3ffc::/15"),
	netip.MustParsePrefix("3ffe::/16"),
}

// globallyReachableFetchExceptions lists the more-specific allocations inside
// the 2001::/23 IETF protocol-assignments block that the IANA IPv6
// Special-Purpose Address Space registry (checked 2026-10-14) marks Globally
// Reachable: the parent block is refused as a whole per its "unless allowed by a
// more specific allocation" footnote, and these registry-listed blocks are the
// only addresses inside it that are let through. 2001:20::/28 ORCHIDv2 is
// deliberately not listed even though the registry copies RFC 7343's allocation
// fields: RFC 7343 designs ORCHIDv2 as an identifier space that is "non-routable
// at the IP layer, by design" and "should not appear in actual IPv6 headers", so
// it is not a fetch destination, and refusing it fail-closed keeps the behavior
// this policy had before the parent block was added.
var globallyReachableFetchExceptions = []netip.Prefix{
	netip.MustParsePrefix("2001:1::1/128"),   // Port Control Protocol anycast (RFC 7723)
	netip.MustParsePrefix("2001:1::2/128"),   // TURN anycast (RFC 8155)
	netip.MustParsePrefix("2001:1::3/128"),   // DNS-SD SRP anycast (RFC 9665)
	netip.MustParsePrefix("2001:3::/32"),     // AMT (RFC 7450)
	netip.MustParsePrefix("2001:4:112::/48"), // AS112-v6 (RFC 7535)
	netip.MustParsePrefix("2001:30::/28"),    // DRIP DETs (RFC 9374)
}

// isPublicLiteralIP reports whether addr is a literal public fetch target. An
// IPv4 address is public when it is outside every nonPublicFetchPrefixes block,
// because the complement of those blocks is the ordinary global unicast space.
// An IPv6 address is public only when it lies inside ianaGlobalUnicastAllocations,
// the prefixes the IANA IPv6 Global Unicast Address Space registry lists as
// ALLOCATED — the registry reserves every address in 2000::/3 not listed there
// for future allocation — and is not refused on top by nonPublicFetchPrefixes.
// That second check refuses the allocated-but-unsuitable rows: documentation
// (2001:db8::/32 inside the 2001:c00::/23 row), 6to4 (2002::/16), and Teredo,
// benchmarking, the terminated ORCHID and ORCHIDv2 inside the partially
// allocated 2001::/23 block. The other non-public rows (NAT64, the deprecated
// IPv4-compatible form, multicast) are already refused by the allocation list,
// which does not contain them. A table block still refuses its addresses unless
// the address is one of the registry's more-specific globally reachable
// allocations (globallyReachableFetchExceptions). An IPv4-mapped IPv6 literal
// such as ::ffff:127.0.0.1 is unmapped first and classified as its IPv4
// address. An invalid address is not public, and a zoned address is refused
// (fail closed) because prefix containment cannot classify one.
func isPublicLiteralIP(addr netip.Addr) bool {
	if !addr.IsValid() || addr.Zone() != "" {
		return false
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if addr.Is4() {
		return !containsAnyPrefix(nonPublicFetchPrefixes, addr)
	}
	if !containsAnyPrefix(ianaGlobalUnicastAllocations, addr) {
		return false
	}
	if !containsAnyPrefix(nonPublicFetchPrefixes, addr) {
		return true
	}
	return containsAnyPrefix(globallyReachableFetchExceptions, addr)
}

// containsAnyPrefix reports whether addr lies inside any prefix in prefixes.
func containsAnyPrefix(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// canonicalizeHost lowercases host and strips at most one trailing dot — the
// DNS root label a resolver treats as naming the same host ("127.0.0.1." and
// "LOCALHOST." are the hosts "127.0.0.1" and "localhost"). A second trailing dot
// is not a root label: it leaves an empty label, which the literal and
// plausibility rules below refuse instead of silently reading the host as its
// single-dot spelling and dispatching a host the workflow fetcher cannot
// resolve (getaddrinfo("example.com..") fails).
func canonicalizeHost(host string) string {
	host = strings.ToLower(host)
	if strings.HasSuffix(host, ".") {
		host = host[:len(host)-1]
	}
	return host
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
// hyphen are refused. canonicalizeHost has already stripped at most one
// trailing root-label dot, so a second trailing dot leaves an empty final label
// here and is refused.
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
