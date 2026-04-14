package geocoding

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateBaseURL_Accepts covers the happy-path shapes we expect to see
// in production: empty (use default), public HTTPS endpoints, and a
// public-hostname HTTP endpoint.
func TestValidateBaseURL_Accepts(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty falls back to default", ""},
		{"plain https", "https://nominatim.openstreetmap.org/reverse"},
		{"plain http", "http://example.com/reverse"},
		{"https with port", "https://photon.komoot.io:443/reverse"},
		{"path only", "https://example.org/api/v1/reverse"},
		{"query string", "https://example.org/reverse?key=abc"},
		{"trailing whitespace", "  https://example.org/reverse  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateBaseURL(tc.in); err != nil {
				t.Errorf("ValidateBaseURL(%q) = %v, want nil", tc.in, err)
			}
		})
	}
}

// TestValidateBaseURL_RejectsScheme ensures every non-http(s) scheme is
// rejected — these are the classic SSRF vectors (file, gopher, ftp, dict,
// ldap) and the Go http client would follow them blindly otherwise.
func TestValidateBaseURL_RejectsScheme(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"gopher://example.com/",
		"ftp://example.com/",
		"dict://example.com:11211/",
		"ldap://example.com/",
		"javascript:alert(1)",
		"data:text/plain,hi",
		"//example.com/", // scheme-relative — url.Parse treats Scheme as empty
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			err := ValidateBaseURL(in)
			if err == nil {
				t.Fatalf("ValidateBaseURL(%q) = nil, want scheme rejection", in)
			}
			if !errors.Is(err, ErrInvalidBaseURL) {
				t.Errorf("err %v is not ErrInvalidBaseURL", err)
			}
		})
	}
}

// TestValidateBaseURL_RejectsPrivateIPs exercises every range listed in the
// validator doc comment. There is no opt-in flag anymore — private IPs are
// always rejected because Skadia does not support self-hosted Photon/
// Nominatim inside the private network.
func TestValidateBaseURL_RejectsPrivateIPs(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		// IPv4 loopback.
		{"ipv4 loopback 127.0.0.1", "http://127.0.0.1:5432/"},
		{"ipv4 loopback 127.1.2.3", "http://127.1.2.3/"},
		// IPv4 RFC1918.
		{"ipv4 10.x", "http://10.0.0.1/"},
		{"ipv4 172.16.x", "http://172.16.5.5/"},
		{"ipv4 192.168.x", "http://192.168.1.100/"},
		// Link-local / AWS metadata.
		{"ipv4 aws metadata 169.254.169.254", "http://169.254.169.254/latest/meta-data/"},
		// 0.0.0.0/8 "this network".
		{"ipv4 0.0.0.0", "http://0.0.0.0/"},
		{"ipv4 0.1.2.3", "http://0.1.2.3/"},
		// CGNAT 100.64.0.0/10.
		{"ipv4 cgnat 100.64.0.1", "http://100.64.0.1/"},
		{"ipv4 cgnat 100.127.255.254", "http://100.127.255.254/"},
		// IPv6 loopback / ULA / link-local.
		{"ipv6 loopback", "http://[::1]/"},
		{"ipv6 ula fc00", "http://[fc00::1]/"},
		{"ipv6 ula fd00", "http://[fd00::1]/"},
		{"ipv6 link-local fe80", "http://[fe80::1]/"},
		// IPv4-mapped IPv6 (::ffff:a.b.c.d) — must be tested as the mapped v4.
		{"ipv6-mapped ipv4 loopback", "http://[::ffff:127.0.0.1]/"},
		{"ipv6-mapped ipv4 rfc1918", "http://[::ffff:10.0.0.1]/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBaseURL(tc.url)
			if err == nil {
				t.Fatalf("ValidateBaseURL(%q) = nil, want rejection", tc.url)
			}
			if !errors.Is(err, ErrInvalidBaseURL) {
				t.Errorf("err %v is not ErrInvalidBaseURL", err)
			}
		})
	}
}

// TestValidateBaseURL_RejectsLocalHostnames covers the symbolic names that
// resolve to private infrastructure without being IPs.
func TestValidateBaseURL_RejectsLocalHostnames(t *testing.T) {
	cases := []string{
		"http://localhost/",
		"http://localhost:5432/",
		"http://LOCALHOST/", // case-insensitive
		"http://foo.local/",
		"http://server.internal.local/",
		"http://nginx.localhost/",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if err := ValidateBaseURL(in); err == nil {
				t.Fatalf("ValidateBaseURL(%q) = nil, want rejection", in)
			}
		})
	}
}

// TestValidateBaseURL_RejectsMissingHost catches badly-formed URLs where
// url.Parse succeeds (e.g. "http:///") but there's nothing to route to.
func TestValidateBaseURL_RejectsMissingHost(t *testing.T) {
	cases := []string{
		"http://",
		"https:///reverse",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			err := ValidateBaseURL(in)
			if err == nil {
				t.Fatalf("ValidateBaseURL(%q) = nil, want empty-host rejection", in)
			}
		})
	}
}

// TestValidateBaseURL_RejectsUnparseable ensures garbage input (control bytes
// etc.) doesn't accidentally pass — url.Parse is surprisingly lenient.
func TestValidateBaseURL_RejectsUnparseable(t *testing.T) {
	// A URL with a control byte in the host portion should be rejected by
	// url.Parse. We test that ValidateBaseURL surfaces this as ErrInvalidBaseURL.
	in := "http://exa\x00mple.com/"
	err := ValidateBaseURL(in)
	if err == nil {
		t.Fatalf("ValidateBaseURL(%q) = nil, want parse error", in)
	}
	if !strings.Contains(err.Error(), "invalid geocoding base URL") {
		t.Errorf("error message %q should reference invalid base URL", err.Error())
	}
}
