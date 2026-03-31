package automation

import (
	"bytes"
	"fmt"
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

	// Check allowlist first
	for _, allowed := range allowlist {
		if strings.EqualFold(allowed, hostname) {
			// Allowed host — skip SSRF check
			goto performRequest
		}
	}

	// Resolve DNS and check for private IPs
	{
		addrs, err := net.LookupHost(hostname)
		if err != nil {
			return fmt.Errorf("webhook DNS resolution failed: %w", err)
		}
		for _, addr := range addrs {
			ip := net.ParseIP(addr)
			if ip == nil {
				continue
			}
			if isPrivateIP(ip) {
				return fmt.Errorf("webhook URL resolves to private IP %s (SSRF blocked)", addr)
			}
		}
	}

performRequest:
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
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

	// Drain response body (limit to prevent memory exhaustion)
	buf := make([]byte, 1024*1024) // 1MB limit
	resp.Body.Read(buf)            //nolint:errcheck

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
