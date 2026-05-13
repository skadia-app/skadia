package llmwire

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

var defaultLocalLLMAllowlist = []string{
	"ollama:11434",
	"litellm:4000",
	"host.containers.internal:11434",
	"host.containers.internal:8000",
	"host.containers.internal:8080",
}

func NewSSRFProtectedHTTPClient(timeout time.Duration, allowlist []string) *http.Client {
	if allowlist == nil {
		allowlist = defaultLocalLLMAllowlist
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: ssrfProtectedDialContext(allowlist),
		},
	}
}

func ssrfProtectedDialContext(allowlist []string) func(context.Context, string, string) (net.Conn, error) {
	allowed := make(map[string]bool, len(allowlist))
	for _, addr := range allowlist {
		allowed[strings.ToLower(strings.TrimSpace(addr))] = true
	}
	var dialer net.Dialer
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if isAllowedLLMAddress(allowed, host, port) {
			return dialer.DialContext(ctx, network, addr)
		}
		if isKnownLocalLLMHost(host) {
			return nil, fmt.Errorf("LLM api_base address %s is not allowed", net.JoinHostPort(host, port))
		}
		ips, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("LLM DNS resolution failed: %w", err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("LLM DNS resolution returned no addresses")
		}
		for _, ip := range ips {
			if isBlockedLLMIP(ip) {
				return nil, fmt.Errorf("LLM api_base resolves to blocked private IP %s", ip)
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
	}
}

func isKnownLocalLLMHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "ollama", "litellm", "host.containers.internal":
		return true
	default:
		return false
	}
}

func isAllowedLLMAddress(allowed map[string]bool, host, port string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	port = strings.TrimSpace(port)
	return allowed[net.JoinHostPort(host, port)] || allowed[host+":"+port]
}

func isBlockedLLMIP(ipStr string) bool {
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
