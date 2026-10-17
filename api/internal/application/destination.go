package application

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// ErrDestinationNotAllowed marks a webhook destination that resolves to an
// address the deployment refuses to contact. Retrying cannot fix it.
var ErrDestinationNotAllowed = errors.New("destination address is not allowed")

// DestinationPolicy decides which network addresses webhook deliveries may
// reach. Endpoint URLs are tenant-controlled, so without it a tenant could
// make the worker call internal services or cloud metadata endpoints.
//
// The policy is enforced twice: on the URL when an endpoint is saved (early,
// readable errors for literal addresses and localhost names) and on the
// resolved address of every outbound connection (the authoritative check,
// which also covers DNS rebinding and hostnames that change after saving).
type DestinationPolicy struct {
	// AllowPrivateNetworks admits loopback and private ranges (RFC 1918,
	// unique local IPv6) so local receivers work in development. Link-local,
	// metadata and other special-purpose ranges stay blocked regardless.
	AllowPrivateNetworks bool
}

// blockedPrefixes covers special-purpose ranges from the IANA IPv4 and IPv6
// registries that are not globally reachable, plus ranges commonly used for
// infrastructure. Loopback and private ranges are handled separately because
// development may allow them.
var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8",       // "this network"
	"100.64.0.0/10",   // carrier-grade NAT; also Alibaba Cloud metadata
	"169.254.0.0/16",  // link-local; AWS, GCP and Azure metadata
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // TEST-NET-1
	"192.88.99.0/24",  // deprecated 6to4 relay anycast
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // TEST-NET-2
	"203.0.113.0/24",  // TEST-NET-3
	"224.0.0.0/4",     // multicast
	"240.0.0.0/4",     // reserved, including 255.255.255.255
	"::/128",          // unspecified
	"64:ff9b:1::/48",  // local-use IPv4/IPv6 translation
	"100::/64",        // discard-only
	"2001::/23",       // IETF protocol assignments, including Teredo
	"2001:db8::/32",   // documentation
	"3fff::/20",       // documentation
	"5f00::/16",       // segment routing SIDs
	"fe80::/10",       // link-local
	"fec0::/10",       // deprecated site-local
	"ff00::/8",        // multicast
)

var privatePrefixes = mustPrefixes(
	"10.0.0.0/8",
	"127.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"::1/128",
	"fc00::/7", // unique local; includes AWS IPv6 metadata fd00:ec2::254
)

var (
	nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")
	sixToFour   = netip.MustParsePrefix("2002::/16")
)

// AllowAddr reports whether a connection to addr is permitted. IPv6 forms
// that embed an IPv4 address (mapped, NAT64, 6to4) are judged by that IPv4
// address so they cannot smuggle a blocked target.
func (p DestinationPolicy) AllowAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.WithZone("").Unmap()
	if addr.Is6() {
		raw := addr.As16()
		switch {
		case nat64Prefix.Contains(addr):
			return p.AllowAddr(netip.AddrFrom4([4]byte(raw[12:16])))
		case sixToFour.Contains(addr):
			return p.AllowAddr(netip.AddrFrom4([4]byte(raw[2:6])))
		}
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	for _, prefix := range privatePrefixes {
		if prefix.Contains(addr) {
			return p.AllowPrivateNetworks
		}
	}
	return true
}

// CheckHost validates the host part of an endpoint URL before it is saved.
// Literal addresses are checked against the policy and localhost names are
// rejected outside development. Other hostnames are accepted here; their
// resolved addresses are checked when the worker connects.
func (p DestinationPolicy) CheckHost(host string) error {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if addr, err := netip.ParseAddr(host); err == nil {
		if !p.AllowAddr(addr) {
			return fmt.Errorf("%w: %s", ErrDestinationNotAllowed, addr)
		}
		return nil
	}
	if !p.AllowPrivateNetworks && (host == "localhost" || strings.HasSuffix(host, ".localhost")) {
		return fmt.Errorf("%w: %s", ErrDestinationNotAllowed, host)
	}
	return nil
}

func mustPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, len(values))
	for i, value := range values {
		prefixes[i] = netip.MustParsePrefix(value)
	}
	return prefixes
}
