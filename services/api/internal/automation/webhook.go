package automation

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// privateRanges contains IP ranges that are blocked by default for SSRF protection.
var privateRanges = func() []net.IPNet {
	ranges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16", // link-local
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
	}
	var result []net.IPNet
	for _, r := range ranges {
		_, network, err := net.ParseCIDR(r)
		if err == nil {
			result = append(result, *network)
		}
	}
	return result
}()

// isPrivateIP checks if an IP address is in a private/local range.
func isPrivateIP(ip net.IP) bool {
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
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

	// Allowlist check
	inAllowlist := false
	for _, allowed := range allowlist {
		if strings.EqualFold(allowed, hostname) {
			inAllowlist = true
			break
		}
	}

	var transport http.RoundTripper
	if inAllowlist {
		// Allowlisted: use default transport (trusted local endpoint)
		transport = http.DefaultTransport
	} else {
		// SSRF protection: custom dialer that resolves once, checks all IPs,
		// and pins the connection to the resolved IP to prevent DNS rebinding.
		baseDialer := &net.Dialer{Timeout: 5 * time.Second}
		transport = &http.Transport{
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
					ip := net.ParseIP(ipStr)
					if ip != nil && isPrivateIP(ip) {
						return nil, fmt.Errorf("webhook URL resolves to private IP %s (SSRF blocked)", ipStr)
					}
				}
				// Pin to first resolved IP — prevents DNS rebinding on second lookup
				return baseDialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
			},
		}
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
