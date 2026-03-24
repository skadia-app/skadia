package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders returns middleware that sets CSP, HSTS, and other security headers.
// isHTTPS should be true when the app is served over HTTPS (enables HSTS).
func SecurityHeaders(isHTTPS bool) func(http.Handler) http.Handler {
	// Build CSP once at init
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'", // needed for MapLibre + inline styles
		"img-src 'self' data: blob: https://*.tile.openstreetmap.org https://tile.opentopomap.org https://*.tile-cyclosm.openstreetmap.fr https://server.arcgisonline.com https://*.basemaps.cartocdn.com https://protomaps.github.io",
		"connect-src 'self' https://*.tile.openstreetmap.org https://tile.opentopomap.org https://*.tile-cyclosm.openstreetmap.fr https://server.arcgisonline.com https://*.basemaps.cartocdn.com https://protomaps.github.io https://tiles.openfreemap.org https://nominatim.openstreetmap.org",
		"font-src 'self' https://protomaps.github.io",
		"worker-src 'self' blob:",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}, "; ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", csp)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
			w.Header().Set("X-XSS-Protection", "0")

			if isHTTPS {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}
