package middleware

import "net/http"

// RequirePasswordChanged blockiert alle Endpoints wenn must_change_password=true.
// Nur GET /me und POST /auth/change-password sind erlaubt.
// /auth/logout und /auth/refresh sind PUBLIC und erreichen diese Middleware nie.
func RequirePasswordChanged() func(http.Handler) http.Handler {
	type allowedRoute struct {
		method string
		path   string
	}
	allowed := []allowedRoute{
		{"GET", "/api/v1/me"},                    // User-Info lesen
		{"POST", "/api/v1/auth/change-password"}, // Passwort ändern
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mustChange, _ := r.Context().Value(MustChangePWKey).(bool)
			if !mustChange {
				next.ServeHTTP(w, r)
				return
			}
			// Prüfe ob Request in Allowlist (Methode + Pfad)
			for _, a := range allowed {
				if r.Method == a.method && r.URL.Path == a.path {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, `{"error":"password_change_required"}`, http.StatusForbidden)
		})
	}
}
