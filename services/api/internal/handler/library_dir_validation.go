// services/api/internal/handler/library_dir_validation.go
package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// validateLibraryDirName validates a proposed library_dir_name and writes the
// appropriate HTTP response when validation fails.
//
// Returns rejected=true when the caller must return immediately (response already written).
// Shared between user and admin endpoints (spec §4/§7).
//
// Order of checks (intentional — matches Opus critic HIGH #7):
//  1. Path-safety: null bytes, length, path separators, reserved prefixes.
//  2. Clash-check: does the value exactly match another user's UUID? → 200 + clash-status.
//     MUST run before UUID-format-reject so the clash branch is reachable even when the
//     name is UUID-shaped (which it will be when the admin types userA's UUID for userB).
//  3. UUID-format-reject: generic UUID-shaped strings that aren't another user's UUID
//     but would collide with the implicit userID fallback path.
func validateLibraryDirName(ctx context.Context, w http.ResponseWriter, r *http.Request,
	pool *pgxpool.Pool, name, selfUserID string) (rejected bool) {

	// 1. Path-safety.
	if strings.ContainsRune(name, 0) || len(name) > 100 ||
		strings.Contains(name, "/") || strings.Contains(name, "\\") ||
		strings.Contains(name, "..") || name == "." || strings.HasPrefix(name, "_") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid library_dir_name", r.URL.Path)
		return true
	}

	// 2. Clash-check: another user already owns this UUID. Must run BEFORE
	// UUID-format-reject so the clash-status response is reachable.
	var clashes bool
	_ = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id::text = $1 AND id::text != $2)`,
		name, selfUserID).Scan(&clashes)
	if clashes {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "library_dir_name_clashes_with_user_uuid",
		})
		return true
	}

	// 3. UUID-format-reject: any UUID-shaped name (including selfUserID's own UUID)
	// would collide with the implicit fallback path — reject it outright.
	if len(name) == 36 && name[8] == '-' && name[13] == '-' && name[18] == '-' && name[23] == '-' {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			"library_dir_name must not be in UUID format", r.URL.Path)
		return true
	}

	return false
}
