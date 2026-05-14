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
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/crypto"
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
		"smtp.port":         "587",
		"smtp.tls":          "starttls",
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
	return NewHandler(db, cfg, st, NewMailerWithDB(db, st, testOutboxEncryptionKey), middleware.NewRateLimiter(context.Background()), context.Background())
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

func assertEmailOutboxJob(t *testing.T, db *database.DB, templateKey EmailTemplateKey, recipient string) {
	t.Helper()
	rows, err := db.Pool.Query(context.Background(),
		`SELECT recipient_ciphertext FROM email_outbox WHERE template_key = $1`,
		string(templateKey),
	)
	if err != nil {
		t.Fatalf("query email_outbox: %v", err)
	}
	defer rows.Close()
	var count int
	for rows.Next() {
		var ciphertext string
		if err := rows.Scan(&ciphertext); err != nil {
			t.Fatalf("scan email_outbox: %v", err)
		}
		plain, err := crypto.Decrypt(ciphertext, testOutboxEncryptionKey)
		if err != nil {
			t.Fatalf("decrypt email_outbox recipient: %v", err)
		}
		if plain == recipient {
			count++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate email_outbox: %v", err)
	}
	if count != 1 {
		t.Fatalf("email_outbox count for template=%s recipient=%s = %d, want 1", templateKey, recipient, count)
	}
}

func assertNoUserCreationState(t *testing.T, db *database.DB, email string) {
	t.Helper()
	ctx := context.Background()

	var userCount, tokenCount, tagCount, ruleCount int
	var versionSum *int
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*), sum(system_artifacts_version) FROM users WHERE email = $1`,
		email,
	).Scan(&userCount, &versionSum); err != nil {
		t.Fatalf("count rolled-back users: %v", err)
	}
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*)
		   FROM email_tokens et
		   JOIN users u ON u.id = et.user_id
		  WHERE u.email = $1`,
		email,
	).Scan(&tokenCount); err != nil {
		t.Fatalf("count rolled-back email tokens: %v", err)
	}
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*)
		   FROM tags t
		   JOIN users u ON u.id = t.user_id
		  WHERE u.email = $1 AND t.is_system = true`,
		email,
	).Scan(&tagCount); err != nil {
		t.Fatalf("count rolled-back system tags: %v", err)
	}
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*)
		   FROM automation_rules ar
		   JOIN users u ON u.id = ar.user_id
		  WHERE u.email = $1 AND ar.is_system = true`,
		email,
	).Scan(&ruleCount); err != nil {
		t.Fatalf("count rolled-back system rules: %v", err)
	}
	if userCount != 0 || tokenCount != 0 || tagCount != 0 || ruleCount != 0 || versionSum != nil {
		t.Fatalf("creation state survived rollback for %s: users=%d tokens=%d tags=%d rules=%d version_sum=%v",
			email, userCount, tokenCount, tagCount, ruleCount, versionSum)
	}
}

func assertNoOAuthConnection(t *testing.T, db *database.DB, provider, providerID string) {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_connections WHERE provider = $1 AND provider_id = $2`,
		provider, providerID,
	).Scan(&count); err != nil {
		t.Fatalf("count rolled-back oauth connection: %v", err)
	}
	if count != 0 {
		t.Fatalf("oauth connection survived rollback: count=%d", count)
	}
}

func insertTokenCollisionUser(t *testing.T, db *database.DB, tokenHash, tokenType string) {
	t.Helper()
	ctx := context.Background()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	var userID string
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, email_verified, registration_source)
		 VALUES ($1, $2, true, 'test')
		 RETURNING id`,
		"token_collision_"+suffix[:8],
		"token-collision-"+suffix+"@example.com",
	).Scan(&userID); err != nil {
		t.Fatalf("insert token collision user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO email_tokens (user_id, token_hash, type, expires_at)
		 VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour')`,
		userID, tokenHash, tokenType,
	); err != nil {
		t.Fatalf("insert colliding email token: %v", err)
	}
}

func seedArtifactsThenFail(t *testing.T, seenUserID *string) database.UserCreateSeeder {
	t.Helper()
	return func(ctx context.Context, tx pgx.Tx, userID string) error {
		*seenUserID = userID
		if err := automation.EnsureSystemArtifactsTx(ctx, tx, userID); err != nil {
			return err
		}
		return errors.New("forced seeder failure")
	}
}

func TestRegisterCreatesSystemArtifacts(t *testing.T) {
	stubHIBP(t)
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	h := newDBBackedAuthHandler(t, db, map[string]string{
		"smtp.host": "smtp.invalid",
		"smtp.from": "noreply@example.com",
	})
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
	assertEmailOutboxJob(t, db, EmailTemplateVerification, email)
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

func TestTokenFailureRollsBackPasswordUserWithEmailToken(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	suffix := uuid.NewString()
	email := "token-rollback-" + suffix + "@example.com"
	tokenHash := "duplicate-verification-token-" + suffix
	insertTokenCollisionUser(t, db, tokenHash, "verification")

	seederCalled := false
	_, err := db.CreateUserWithPasswordAndEmailTokenSeeded(ctx,
		email, "token_rollback_"+strings.ReplaceAll(suffix[:8], "-", "_"), "hash", "email", net.ParseIP("127.0.0.1"),
		true, false, tokenHash, "verification", time.Now().Add(time.Hour),
		func(context.Context, pgx.Tx, string) error {
			seederCalled = true
			return nil
		})
	if err == nil {
		t.Fatal("CreateUserWithPasswordAndEmailTokenSeeded succeeded with colliding token hash")
	}
	if !strings.Contains(err.Error(), "create email token: insert") {
		t.Fatalf("error = %v, want token insert failure", err)
	}
	if seederCalled {
		t.Fatal("seeder ran after token creation failure")
	}
	assertNoUserCreationState(t, db, email)
}

func TestTokenFailureRollsBackInvitedUser(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	suffix := uuid.NewString()
	email := "invite-token-rollback-" + suffix + "@example.com"
	tokenHash := "duplicate-reset-token-" + suffix
	insertTokenCollisionUser(t, db, tokenHash, "reset")

	seederCalled := false
	_, err := db.CreateInvitedUserSeeded(ctx,
		email, "invite_token_rollback_"+strings.ReplaceAll(suffix[:8], "-", "_"), "admin", net.ParseIP("127.0.0.1"),
		true, tokenHash, time.Now().Add(time.Hour),
		func(context.Context, pgx.Tx, string) error {
			seederCalled = true
			return nil
		})
	if err == nil {
		t.Fatal("CreateInvitedUserSeeded succeeded with colliding token hash")
	}
	if !strings.Contains(err.Error(), "create email token: insert") {
		t.Fatalf("error = %v, want token insert failure", err)
	}
	if seederCalled {
		t.Fatal("seeder ran after token creation failure")
	}
	assertNoUserCreationState(t, db, email)
}

func TestSeedFailureRollsBackAdminPasswordUser(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	suffix := uuid.NewString()
	email := "admin-seed-rollback-" + suffix + "@example.com"
	var seenUserID string

	_, err := db.CreateAdminPasswordUserSeeded(ctx,
		email, "admin_seed_rollback_"+strings.ReplaceAll(suffix[:8], "-", "_"), "hash", net.ParseIP("127.0.0.1"),
		seedArtifactsThenFail(t, &seenUserID))
	if err == nil {
		t.Fatal("CreateAdminPasswordUserSeeded succeeded with failing seeder")
	}
	if seenUserID == "" {
		t.Fatal("failing seeder was not called")
	}
	assertNoUserCreationState(t, db, email)
}

func TestSeedFailureRollsBackInvitedUser(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	suffix := uuid.NewString()
	email := "invite-seed-rollback-" + suffix + "@example.com"
	var seenUserID string

	_, err := db.CreateInvitedUserSeeded(ctx,
		email, "invite_seed_rollback_"+strings.ReplaceAll(suffix[:8], "-", "_"), "admin", net.ParseIP("127.0.0.1"),
		true, "invite-seed-token-"+suffix, time.Now().Add(time.Hour),
		seedArtifactsThenFail(t, &seenUserID))
	if err == nil {
		t.Fatal("CreateInvitedUserSeeded succeeded with failing seeder")
	}
	if seenUserID == "" {
		t.Fatal("failing seeder was not called")
	}
	assertNoUserCreationState(t, db, email)
}

func TestSeedFailureRollsBackBootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	db := isolatedBootstrapDB(t)
	suffix := uuid.NewString()
	email := "bootstrap-seed-rollback-" + suffix + "@example.com"
	var seenUserID string

	_, err := db.CreateBootstrapAdminSeeded(ctx,
		email, "bootstrap_seed_rollback_"+strings.ReplaceAll(suffix[:8], "-", "_"), "hash", net.ParseIP("127.0.0.1"),
		seedArtifactsThenFail(t, &seenUserID))
	if err == nil {
		t.Fatal("CreateBootstrapAdminSeeded succeeded with failing seeder")
	}
	if seenUserID == "" {
		t.Fatal("failing seeder was not called")
	}
	assertNoUserCreationState(t, db, email)

	var userCount int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE deleted_at IS NULL`).Scan(&userCount); err != nil {
		t.Fatalf("count bootstrap users after rollback: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("bootstrap users survived rollback: count=%d", userCount)
	}
}

func TestSeedFailureRollsBackOAuthUser(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	suffix := uuid.NewString()
	email := "oauth-seed-rollback-" + suffix + "@example.com"
	provider := "test_provider_rollback"
	providerID := "provider-" + suffix
	var seenUserID string

	_, isNew, err := db.GetOrCreateUserByOAuthSeeded(ctx,
		provider, providerID, email, true, "oauth_seed_rollback_"+strings.ReplaceAll(suffix[:8], "-", "_"),
		[]byte("access"), []byte("refresh"), nil, strings.Repeat("0123456789abcdef", 4),
		true, false, seedArtifactsThenFail(t, &seenUserID))
	if err == nil {
		t.Fatal("GetOrCreateUserByOAuthSeeded succeeded with failing seeder")
	}
	if isNew {
		t.Fatal("GetOrCreateUserByOAuthSeeded reported a new user after seeder failure")
	}
	if seenUserID == "" {
		t.Fatal("failing seeder was not called")
	}
	assertNoUserCreationState(t, db, email)
	assertNoOAuthConnection(t, db, provider, providerID)
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
		"test_provider", "provider-"+suffix, "oauth-"+suffix+"@example.com", true, "oauth_"+suffix[:8],
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
		"test_provider", "provider-"+suffix, "oauth-"+suffix+"@example.com", true, "oauth_"+suffix[:8],
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

func TestOAuthUserWithoutVerifiedProviderEmailDoesNotClaimLocalEmail(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	suffix := uuid.NewString()

	user, isNew, err := db.GetOrCreateUserByOAuthSeeded(ctx,
		"google", "provider-unverified-"+suffix, "oauth-unverified-"+suffix+"@example.com", false, "oauth_"+suffix[:8],
		[]byte("access"), []byte("refresh"), nil, strings.Repeat("0123456789abcdef", 4),
		true, false, automation.EnsureSystemArtifactsTx)
	if err != nil {
		t.Fatalf("create oauth user: %v", err)
	}
	if !isNew {
		t.Fatal("OAuth lookup returned existing user")
	}
	if user.Email != nil {
		t.Fatalf("unverified provider email claimed local email %q", *user.Email)
	}
	if user.EmailVerified {
		t.Fatal("unverified provider email marked local email as verified")
	}
}
