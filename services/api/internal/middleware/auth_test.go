package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

func insertMiddlewareAuthUser(t *testing.T, db *database.DB, userID string, isAdmin, isActive bool) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source, is_admin, is_active)
		 VALUES ($1, $2, $3, true, 'test', $4, $5)`,
		userID, "auth-"+userID, "auth-"+userID+"@example.test", isAdmin, isActive,
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func requestWithBearer(t *testing.T, token string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestJWTAuthRevalidatesAdminClaimAgainstDatabase(t *testing.T) {
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	secret := []byte(strings.Repeat("a", 32))
	userID := uuid.NewString()
	insertMiddlewareAuthUser(t, db, userID, false, true)

	token, err := auth.IssueJWT(userID, true, true, false, secret)
	if err != nil {
		t.Fatalf("issue jwt: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := middleware.NewJWTAuthWithDB(secret, db).Middleware(middleware.RequireAdmin()(next))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, requestWithBearer(t, token))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 after DB demotion", rr.Code)
	}
}

func TestJWTAuthRejectsInactiveUserEvenWithValidToken(t *testing.T) {
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	secret := []byte(strings.Repeat("a", 32))
	userID := uuid.NewString()
	insertMiddlewareAuthUser(t, db, userID, false, false)

	token, err := auth.IssueJWT(userID, false, true, false, secret)
	if err != nil {
		t.Fatalf("issue jwt: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := middleware.NewJWTAuthWithDB(secret, db).Middleware(next)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, requestWithBearer(t, token))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401 for inactive user", rr.Code)
	}
}

func TestJWTAuthReturnsServiceUnavailableWhenStateStoreUnavailable(t *testing.T) {
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	secret := []byte(strings.Repeat("a", 32))
	userID := uuid.NewString()
	insertMiddlewareAuthUser(t, db, userID, false, true)

	token, err := auth.IssueJWT(userID, false, true, false, secret)
	if err != nil {
		t.Fatalf("issue jwt: %v", err)
	}
	pool.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := middleware.NewJWTAuthWithDB(secret, db).Middleware(next)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, requestWithBearer(t, token))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503 for auth state store outage", rr.Code)
	}
}
