package middleware

import (
	"net"
	"net/http"
	"strings"
)

// TrustedProxyRealIP returns middleware that sets r.RemoteAddr to the client's real IP
// when the direct connection comes from a trusted proxy.
// trustedCIDRs is a comma-separated list of CIDR ranges (e.g. "10.89.0.0/16,172.16.0.0/12").
func TrustedProxyRealIP(trustedCIDRs string) func(http.Handler) http.Handler {
	var networks []*net.IPNet
	for _, cidr := range strings.Split(trustedCIDRs, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue // skip invalid CIDRs
		}
		networks = append(networks, network)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(networks) > 0 {
				remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
				ip := net.ParseIP(remoteIP)
				if ip != nil {
					for _, network := range networks {
						if network.Contains(ip) {
							// Trusted proxy — use X-Forwarded-For
							if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
								// Take the first entry (original client)
								parts := strings.Split(xff, ",")
								clientIP := strings.TrimSpace(parts[0])
								if parsedIP := net.ParseIP(clientIP); parsedIP != nil {
									r.RemoteAddr = clientIP + ":0"
								}
							}
							break
						}
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP extracts the IP address from r.RemoteAddr (after TrustedProxyRealIP has run).
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
