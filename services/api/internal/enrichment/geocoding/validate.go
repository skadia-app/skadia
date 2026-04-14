package geocoding

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
)

// validationBypassForTest is set ONLY by tests that need to dispatch requests
// against httptest.NewServer (which binds to 127.0.0.1 and would otherwise be
// rejected by the SSRF guard). Production code never flips this. The helper
// that flips it lives in a `_test.go` file so the symbol is unreachable from
// non-test builds.
var validationBypassForTest atomic.Bool

// ErrInvalidBaseURL is returned by ValidateBaseURL when the configured
// provider URL is rejected as unsafe.
//
// Handler/UI code may inspect this via errors.Is to surface a user-friendly
// error; admin-level log messages should include the wrapped detail for
// diagnostics.
var ErrInvalidBaseURL = errors.New("invalid geocoding base URL")

// ValidateBaseURL checks that a user-supplied external-provider endpoint URL
// is safe to dispatch requests to. It enforces the following rules and
// therefore blocks Server-Side-Request-Forgery (SSRF) vectors on multi-tenant
// self-host instances:
//
//  1. Scheme MUST be "http" or "https" (no file://, gopher://, …).
//  2. Host MUST be present and non-empty.
//  3. Hostname MUST NOT be "localhost" (case-insensitive) nor any form of the
//     *.local or *.localhost suffix.
//  4. If the hostname is a numeric IP literal it MUST NOT fall into any
//     private / loopback / link-local / ULA range:
//     - IPv4: 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16,
//       169.254.0.0/16, 0.0.0.0/8, 100.64.0.0/10 (CGNAT).
//     - IPv6: ::1, fc00::/7 (ULA), fe80::/10 (link-local), ::ffff:0:0/96
//       (IPv4-mapped — we re-validate the mapped v4 against v4 rules).
//
// There is deliberately NO escape hatch for private IPs. Self-hosting an own
// Photon/Nominatim instance inside the same private network is not a
// supported use case today — the external-fallback feature exists only for
// the public komoot.io / openstreetmap.org endpoints. Removing the flag
// keeps the attack surface closed.
//
// KNOWN GAP: this validator does NOT resolve hostnames to check whether a
// public-looking DNS name points at a private IP. A DNS-rebinding attacker
// that controls the DNS response could still reach internal hosts. Full
// defence requires resolve-and-recheck at connect time (custom net.Dialer
// with RFC1918 screening). This is intentionally out of scope for the
// initial fix; it is documented as a known limitation. Mitigations that
// remain in place: the provider is opt-in (default "off"), outbound calls
// are timeout-bounded (5s), and responses are parsed with a narrow JSON
// schema — so even a successful rebind yields no useful data exfiltration.
func ValidateBaseURL(raw string) error {
	if validationBypassForTest.Load() {
		return nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		// Empty is fine — provider falls back to its public default.
		return nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: parse: %v", ErrInvalidBaseURL, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: scheme %q not allowed (only http, https)", ErrInvalidBaseURL, u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: host is empty", ErrInvalidBaseURL)
	}

	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" ||
		strings.HasSuffix(lowerHost, ".localhost") ||
		strings.HasSuffix(lowerHost, ".local") {
		return fmt.Errorf("%w: host %q is a local/loopback name", ErrInvalidBaseURL, host)
	}

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOrReservedIP(ip) {
			return fmt.Errorf("%w: host %q resolves to a private/reserved IP range", ErrInvalidBaseURL, host)
		}
	}

	return nil
}

// isPrivateOrReservedIP reports whether ip falls into a range we must block
// as an SSRF target. We test each family explicitly rather than using
// ip.IsPrivate() alone because Go's standard helpers miss some ranges we
// care about (100.64.0.0/10 CGNAT, 0.0.0.0/8) and conflate IPv4-mapped v6
// with native v6.
func isPrivateOrReservedIP(ip net.IP) bool {
	// IPv4-mapped IPv6 (::ffff:a.b.c.d) — test as v4.
	if v4 := ip.To4(); v4 != nil {
		return isPrivateV4(v4)
	}

	// Native IPv6.
	if ip.IsLoopback() { // ::1
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() { // fe80::/10, ff02::/16
		return true
	}
	if ip.IsPrivate() { // fc00::/7 (ULA) per net.IP.IsPrivate
		return true
	}
	if ip.IsUnspecified() { // ::
		return true
	}
	return false
}

// isPrivateV4 checks IPv4 against ranges that must never be reachable from a
// user-configured external geocoder URL.
func isPrivateV4(ip net.IP) bool {
	if ip.IsLoopback() { // 127.0.0.0/8
		return true
	}
	if ip.IsPrivate() { // 10/8, 172.16/12, 192.168/16
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() { // 169.254/16, 224/4 for multicast (link-local subset)
		return true
	}
	if ip.IsUnspecified() { // 0.0.0.0
		return true
	}
	// 0.0.0.0/8 ("current network"): IsUnspecified only catches 0.0.0.0 exactly.
	if ip[0] == 0 {
		return true
	}
	// 100.64.0.0/10 — CGNAT, RFC 6598. Not covered by IsPrivate.
	if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
		return true
	}
	return false
}
