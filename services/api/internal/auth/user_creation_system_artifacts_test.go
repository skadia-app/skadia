package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func stubHIBP(t *testing.T) {
	t.Helper()
	old := hibpClient
	hibpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { hibpClient = old })
}

func isolatedBootstrapDB(t *testing.T) *database.DB {
	t.Helper()
	ctx := context.Background()
	mainPool := testdb.Open(t)
	schema := "bootstrap_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	if _, err := mainPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create bootstrap schema: %v", err)
	}
	for _, table := range []string{"users", "tags", "automation_rules"} {
		if _, err := mainPool.Exec(ctx, `CREATE TABLE `+schema+`.`+table+` (LIKE public.`+table+` INCLUDING ALL)`); err != nil {
			t.Fatalf("copy table %s: %v", table, err)
		}
	}
	t.Cleanup(func() {
		_, _ = mainPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})

	cfg, err := pgxpool.ParseConfig(testdb.RequireSafeDSN(t))
	if err != nil {
		t.Fatalf("parse test dsn: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open isolated bootstrap pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return &database.DB{Pool: pool}
}

func newDBBackedAuthHandler(t *testing.T, db *database.DB, overrides map[string]string) *Handler {
	t.Helper()
	values := map[string]string{
		"registration.mode": "open",
		"app.url":           "http://localhost:5173",
	}
	for k, v := range overrides {
		values[k] = v
	}
	st := settings.NewStoreForTest(values)
	cfg := &config.Config{
		JWTSecret:          strings.Repeat("ab", 32),
		JWTSecretBytes:     []byte(strings.Repeat("a", 32)),
		AppURL:             values["app.url"],
		OAuthEncryptionKey: strings.Repeat("0123456789abcdef", 2),
	}
	return NewHandler(db, cfg, st, NewMailer(st), middleware.NewRateLimiter(context.Background()), context.Background())
}

func assertUserSystemArtifacts(t *testing.T, db *database.DB, userID string) {
	t.Helper()
	ctx := context.Background()
	var tagCount, ruleCount, version int
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM tags WHERE user_id = $1 AND is_system = true`,
		userID,
	).Scan(&tagCount); err != nil {
		t.Fatalf("count system tags: %v", err)
	}
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM automation_rules WHERE user_id = $1 AND is_system = true`,
		userID,
	).Scan(&ruleCount); err != nil {
		t.Fatalf("count system rules: %v", err)
	}
	if err := db.Pool.QueryRow(ctx,
		`SELECT system_artifacts_version FROM users WHERE id = $1`,
		userID,
	).Scan(&version); err != nil {
		t.Fatalf("query system_artifacts_version: %v", err)
	}
	if tagCount != len(automation.SystemTags) || ruleCount != len(automation.SystemRules) || version != automation.SystemArtifactsVersion {
		t.Fatalf("system artifacts mismatch: tags=%d/%d rules=%d/%d version=%d/%d",
			tagCount, len(automation.SystemTags), ruleCount, len(automation.SystemRules), version, automation.SystemArtifactsVersion)
	}
}

func assertEmailToken(t *testing.T, db *database.DB, userID, tokenType string) {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM email_tokens WHERE user_id = $1 AND type = $2`,
		userID, tokenType,
	).Scan(&count); err != nil {
		t.Fatalf("count email token %s: %v", tokenType, err)
	}
	if count != 1 {
		t.Fatalf("email token count for %s = %d, want 1", tokenType, count)
	}
}

func TestRegisterCreatesSystemArtifacts(t *testing.T) {
	stubHIBP(t)
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	h := newDBBackedAuthHandler(t, db, nil)
	suffix := uuid.NewString()
	email := "new-user-" + suffix + "@example.com"
	username := "new_user_" + strings.ReplaceAll(suffix[:8], "-", "_")

	// Force normal registration mode instead of bootstrap mode.
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO users (username, email, email_verified, registration_source)
		 VALUES ($1, $2, true, 'test')`,
		"existing_"+suffix[:8],
		"existing_"+suffix+"@example.com",
	); err != nil {
		t.Fatalf("insert existing user: %v", err)
	}

	body := bytes.NewBufferString(`{"email":"` + email + `","username":"` + username + `","password":"correct horse battery staple"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", body)
	req.RemoteAddr = net.JoinHostPort("127.0.0.1", "12345")
	rr := httptest.NewRecorder()

	h.Register(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("Register status = %d, body=%s", rr.Code, rr.Body.String())
	}

	user, err := db.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("get created user: %v", err)
	}
	assertUserSystemArtifacts(t, db, user.ID)
	assertEmailToken(t, db, user.ID, "verification")
}

func TestSeedFailureRollsBackPasswordUser(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	suffix := uuid.NewString()
	email := "rollback-" + suffix + "@example.com"

	_, err := db.CreateUserWithPasswordSeeded(ctx,
		email, "rollback_"+suffix[:8], "hash", "email", net.ParseIP("127.0.0.1"), true, false,
		func(context.Context, pgx.Tx, string) error {
			return errors.New("seed failed")
		})
	if err == nil {
		t.Fatal("CreateUserWithPasswordSeeded succeeded with failing seeder")
	}

	var exists bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`,
		email,
	).Scan(&exists); err != nil {
		t.Fatalf("check rollback user: %v", err)
	}
	if exists {
		t.Fatal("user row persisted after seeder failure")
	}
}

func TestBootstrapAdminConcurrentCreatesOneSeededAdmin(t *testing.T) {
	ctx := context.Background()
	db := isolatedBootstrapDB(t)

	const attempts = 2
	var wg sync.WaitGroup
	results := make([]struct {
		userID string
		err    error
	}, attempts)
	for i := range attempts {
		i := i
		wg.Go(func() {
			user, err := db.CreateBootstrapAdminSeeded(ctx,
				"bootstrap@example.com", "bootstrap_admin", "hash", net.ParseIP("127.0.0.1"),
				automation.EnsureSystemArtifactsTx)
			results[i].err = err
			if user != nil {
				results[i].userID = user.ID
			}
		})
	}
	wg.Wait()

	var winners []string
	for _, result := range results {
		if result.err != nil {
			t.Fatalf("bootstrap attempt failed: %v", result.err)
		}
		if result.userID != "" {
			winners = append(winners, result.userID)
		}
	}
	if len(winners) != 1 {
		t.Fatalf("bootstrap winners = %d, want 1 (%v)", len(winners), winners)
	}
	assertUserSystemArtifacts(t, db, winners[0])

	var userCount int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE deleted_at IS NULL`).Scan(&userCount); err != nil {
		t.Fatalf("count bootstrap users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("bootstrap user count = %d, want 1", userCount)
	}
}

func TestAdminCreateUserPasswordCreatesSystemArtifactsAndAccountState(t *testing.T) {
	stubHIBP(t)
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	h := newDBBackedAuthHandler(t, db, nil)
	suffix := uuid.NewString()
	email := "admin-created-" + suffix + "@example.com"
	username := "admin_created_" + strings.ReplaceAll(suffix[:8], "-", "_")

	password := "temporary password 123"
	body := bytes.NewBufferString(`{"email":"` + email + `","username":"` + username + `","password":` + `"` + password + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", body)
	req.RemoteAddr = net.JoinHostPort("127.0.0.1", "12345")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "00000000-0000-0000-0000-000000000001"))
	rr := httptest.NewRecorder()

	h.AdminCreateUser(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("AdminCreateUser status = %d, body=%s", rr.Code, rr.Body.String())
	}

	user, err := db.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("get admin-created user: %v", err)
	}
	if !user.MustChangePassword || !user.EmailVerified {
		t.Fatalf("admin-created flags: must_change_password=%v email_verified=%v", user.MustChangePassword, user.EmailVerified)
	}
	assertUserSystemArtifacts(t, db, user.ID)
}

func TestAdminCreateUserInviteCreatesSystemArtifactsAndResetToken(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	h := newDBBackedAuthHandler(t, db, map[string]string{
		"smtp.host": "smtp.invalid",
		"smtp.from": "noreply@example.com",
	})
	suffix := uuid.NewString()
	email := "admin-invited-" + suffix + "@example.com"
	username := "admin_invited_" + strings.ReplaceAll(suffix[:8], "-", "_")

	body := bytes.NewBufferString(`{"email":"` + email + `","username":"` + username + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", body)
	req.RemoteAddr = net.JoinHostPort("127.0.0.1", "12345")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "00000000-0000-0000-0000-000000000001"))
	rr := httptest.NewRecorder()

	h.AdminCreateUser(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("AdminCreateUser invite status = %d, body=%s", rr.Code, rr.Body.String())
	}

	user, err := db.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("get invited user: %v", err)
	}
	if !user.EmailVerified {
		t.Fatal("invited user email_verified=false")
	}
	assertUserSystemArtifacts(t, db, user.ID)
	assertEmailToken(t, db, user.ID, "reset")
}

func TestOAuthNewUserCreatesSystemArtifactsExistingLoginDoesNotReseed(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	suffix := uuid.NewString()

	user, isNew, err := db.GetOrCreateUserByOAuthSeeded(ctx,
		"test_provider", "provider-"+suffix, "oauth-"+suffix+"@example.com", "oauth_"+suffix[:8],
		[]byte("access"), []byte("refresh"), nil, strings.Repeat("0123456789abcdef", 4),
		true, false, automation.EnsureSystemArtifactsTx)
	if err != nil {
		t.Fatalf("create oauth user: %v", err)
	}
	if !isNew {
		t.Fatal("first OAuth lookup returned existing user")
	}
	assertUserSystemArtifacts(t, db, user.ID)

	_, isNew, err = db.GetOrCreateUserByOAuthSeeded(ctx,
		"test_provider", "provider-"+suffix, "oauth-"+suffix+"@example.com", "oauth_"+suffix[:8],
		[]byte("access-2"), []byte("refresh-2"), nil, strings.Repeat("0123456789abcdef", 4),
		true, false, func(context.Context, pgx.Tx, string) error {
			return errors.New("existing login must not reseed")
		})
	if err != nil {
		t.Fatalf("existing oauth login: %v", err)
	}
	if isNew {
		t.Fatal("second OAuth lookup unexpectedly created a new user")
	}
}
