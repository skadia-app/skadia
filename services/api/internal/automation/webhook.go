package automation

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// isBlockedIP checks whether an IP string is private, loopback, link-local, or unspecified.
// Mirrors the same function in handler/import_url.go.
func isBlockedIP(ipStr string) bool {
	if after, ok := strings.CutPrefix(ipStr, "::ffff:"); ok {
		ipStr = after
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.Equal(net.IPv4zero) || ipStr == "0.0.0.0" {
		return true
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified()
}

// ssrfTransport is a package-level transport with SSRF protection: resolves the hostname once,
// checks all resolved IPs, and pins the connection to the first IP to prevent DNS rebinding.
var ssrfTransport http.RoundTripper = &http.Transport{
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("webhook DNS resolution failed: %w", err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("webhook DNS resolution returned no addresses")
		}
		for _, ipStr := range ips {
			if isBlockedIP(ipStr) {
				return nil, fmt.Errorf("webhook URL resolves to private IP %s (SSRF blocked)", ipStr)
			}
		}
		// Pin to first resolved IP — prevents DNS rebinding on second lookup
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
	},
}

// SendWebhook sends a POST request to the given URL with the provided payload.
// allowlist contains hostnames/IPs that bypass the private-IP check
// (for admin-configured local integrations).
// Returns an error if the URL resolves to a private IP (SSRF protection),
// the request fails, or the response status is >= 400.
func SendWebhook(webhookURL string, payload []byte, allowlist []string) error {
	parsed, err := url.Parse(webhookURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	hostname := parsed.Hostname()

	transport := http.RoundTripper(ssrfTransport)
	if slices.ContainsFunc(allowlist, func(s string) bool { return strings.EqualFold(s, hostname) }) {
		// Allowlisted: use default transport (trusted local endpoint)
		transport = http.DefaultTransport
	}

	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Skadia-Webhook/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Drain response body to allow connection reuse, limit to prevent memory exhaustion.
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20)) //nolint:errcheck

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
