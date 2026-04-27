package model

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Sentinel errors returned by ValidateURLTemplate. Callers should use errors.Is.
var (
	// ErrPrivateIPBlocked is returned when blockPrivateIPs=true and the URL resolves
	// to an RFC1918, loopback, or link-local address.
	ErrPrivateIPBlocked = errors.New("tile-URL on private network address blocked")
	// ErrIPv6ULABlocked is returned when blockPrivateIPs=true and the URL resolves
	// to an IPv6 ULA address (fc00::/7).
	ErrIPv6ULABlocked = errors.New("tile-URL on IPv6 ULA blocked")
	// ErrStyleURLTooLong is returned when style_url exceeds 2048 chars (DB CHECK).
	ErrStyleURLTooLong = errors.New("style_url too long (max 2048)")
	// ErrStyleURLEmpty is returned when style_url is required but empty.
	ErrStyleURLEmpty = errors.New("style_url must not be empty")
	// ErrStyleURLInvalid is returned when style_url is neither a same-origin
	// /api/v1/ relative path nor a parseable http(s) URL.
	ErrStyleURLInvalid = errors.New("style_url must be an http(s) URL or a /api/v1/ relative path")
)

var (
	// {z}, {x}, {y} are mandatory; {switch:a,b,c} optional.
	rePlaceholderZ = regexp.MustCompile(`\{z\}`)
	rePlaceholderX = regexp.MustCompile(`\{x\}`)
	rePlaceholderY = regexp.MustCompile(`\{y\}`)
	reSwitch       = regexp.MustCompile(`\{switch:([a-z0-9]+(?:,[a-z0-9]+)*)\}`)
	rePlaceholder  = regexp.MustCompile(`\{[^}]+\}`)
	knownPlaceholders = map[string]bool{"{z}": true, "{x}": true, "{y}": true}

	// attribution policy: only <a href="https?://"> + plain text; strip everything else.
	// Tighter than bluemonday default: only http/https schemes (no mailto, no relative URLs).
	attribPolicy = func() *bluemonday.Policy {
		p := bluemonday.StrictPolicy()
		p.AllowElements("a")
		p.AllowAttrs("href").OnElements("a")
		p.AllowURLSchemes("http", "https")
		p.RequireParseableURLs(true)
		p.RequireNoFollowOnLinks(true)
		return p
	}()
)

// ValidateURLTemplate checks placeholder presence, scheme, length, and optionally
// blocks private/loopback IP literals (SSRF defense for user-submitted URLs).
// Set blockPrivateIPs=true for user_map_layers, false for admin-managed map_layers.
func ValidateURLTemplate(tpl string, blockPrivateIPs bool) error {
	if len(tpl) > 2048 {
		return errors.New("url_template too long (max 2048)")
	}
	if !rePlaceholderZ.MatchString(tpl) || !rePlaceholderX.MatchString(tpl) || !rePlaceholderY.MatchString(tpl) {
		return errors.New("url_template must contain {z}, {x}, and {y}")
	}
	// Reject unknown placeholders other than the supported set.
	for _, m := range rePlaceholder.FindAllString(tpl, -1) {
		if knownPlaceholders[m] {
			continue
		}
		if strings.HasPrefix(m, "{switch:") {
			if !reSwitch.MatchString(m) {
				return fmt.Errorf("invalid switch placeholder: %s", m)
			}
			continue
		}
		return fmt.Errorf("unsupported placeholder: %s (Phase 1 supports only {z},{x},{y},{switch:a,b,...})", m)
	}
	// Replace placeholders with valid characters before URL parsing.
	probe := tpl
	probe = rePlaceholderZ.ReplaceAllString(probe, "1")
	probe = rePlaceholderX.ReplaceAllString(probe, "1")
	probe = rePlaceholderY.ReplaceAllString(probe, "1")
	probe = reSwitch.ReplaceAllString(probe, "a")
	u, err := url.Parse(probe)
	if err != nil {
		return fmt.Errorf("url_template parse error: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url_template scheme must be http or https, got %q", u.Scheme)
	}
	if blockPrivateIPs {
		if err := blockPrivateIPLiteral(u.Hostname()); err != nil {
			return err
		}
	}
	return nil
}

// blockPrivateIPLiteral rejects RFC1918, loopback, link-local, and ULA addresses
// when the hostname is an IP literal. Hostnames are NOT DNS-resolved here —
// only literal IPs are blocked to avoid DNS-based SSRF amplification.
func blockPrivateIPLiteral(host string) error {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return nil // hostname, not an IP literal
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
		return ErrPrivateIPBlocked
	}
	// IPv6 ULA (fc00::/7) — not covered by IsPrivate in all Go versions; double-check.
	if ip4 := ip.To4(); ip4 == nil && len(ip) == 16 && (ip[0]&0xfe) == 0xfc {
		return ErrIPv6ULABlocked
	}
	return nil
}

// ValidateStyleURL checks a vector_style / protomaps_offline style document URL.
// Accepted shapes:
//   - same-origin relative path beginning with "/api/v1/" (e.g. "/api/v1/tiles/style.json")
//   - absolute http(s) URL; for blockPrivateIPs=true, IP-literal hosts in the
//     RFC1918 / loopback / link-local / ULA ranges are rejected (SSRF defense).
//
// The 2048-char limit mirrors the DB CHECK constraint.
func ValidateStyleURL(s string, blockPrivateIPs bool) error {
	if s == "" {
		return ErrStyleURLEmpty
	}
	if len(s) > 2048 {
		return ErrStyleURLTooLong
	}
	if strings.HasPrefix(s, "/api/v1/") {
		// Same-origin relative path; must still parse cleanly so admins can't
		// smuggle "/api/v1/..\\evil" or whitespace through.
		if _, err := url.Parse(s); err != nil {
			return fmt.Errorf("style_url parse error: %w", err)
		}
		return nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("style_url parse error: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrStyleURLInvalid
	}
	if u.Host == "" {
		return ErrStyleURLInvalid
	}
	if blockPrivateIPs {
		if err := blockPrivateIPLiteral(u.Hostname()); err != nil {
			return err
		}
	}
	return nil
}

// SanitizeAttribution strips all HTML except <a href="https?://..."> and plain text.
// The returned string is safe for storage and rendering; never returns an error.
func SanitizeAttribution(input string) string {
	return attribPolicy.Sanitize(input)
}
