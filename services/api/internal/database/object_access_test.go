package database

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestObjectAccessScopeCreateValidateAndResolve(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)

	input := model.ObjectAccessScopeInput{
		OwnerUserID: ownerID,
		ScopeKind:   "manual",
		Label:       "selected viewers",
		CreatedBy:   ownerID,
		Grants: []model.ObjectAccessGrantInput{
			{PrincipalKind: "user", PrincipalUserID: &viewerID},
			{PrincipalKind: "relationship_rule", PrincipalRef: new("friend-depth"), PrincipalSelector: map[string]any{"relationship": "friend", "max_depth": float64(2)}},
			{PrincipalKind: "server", PrincipalRef: new("server-1"), PrincipalSelector: map[string]any{"server_id": "server-1"}},
			{PrincipalKind: "link_token", LinkTokenHash: new("sha256:token-hash")},
		},
	}
	scope, err := db.CreateObjectAccessScope(ctx, input)
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}
	if scope.ID == "" {
		t.Fatal("CreateObjectAccessScope returned empty id")
	}
	if len(scope.Grants) != 4 {
		t.Fatalf("grant count = %d, want 4", len(scope.Grants))
	}

	if err := db.ValidateObjectAccessScopeForShare(ctx, scope.ID, ownerID); err != nil {
		t.Fatalf("ValidateObjectAccessScopeForShare(owner): %v", err)
	}
	if err := db.ValidateObjectAccessScopeForShare(ctx, scope.ID, viewerID); err == nil {
		t.Fatal("ValidateObjectAccessScopeForShare accepted non-owner")
	}

	resolved, err := db.ResolveVisibleAccessScopes(ctx, viewerID, nil, nil)
	if err != nil {
		t.Fatalf("ResolveVisibleAccessScopes viewer: %v", err)
	}
	if !objectAccessContainsString(resolved, scope.ID) {
		t.Fatalf("resolved scopes = %#v, want %s", resolved, scope.ID)
	}

	linkHash := "sha256:token-hash"
	resolvedByLink, err := db.ResolveVisibleAccessScopes(ctx, "", &linkHash, nil)
	if err != nil {
		t.Fatalf("ResolveVisibleAccessScopes link: %v", err)
	}
	if !objectAccessContainsString(resolvedByLink, scope.ID) {
		t.Fatalf("link resolved scopes = %#v, want %s", resolvedByLink, scope.ID)
	}
}

func TestObjectAccessScopeRejectsPlaintextLinkAndNonViewPermission(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)

	_, err := db.CreateObjectAccessScope(ctx, model.ObjectAccessScopeInput{
		OwnerUserID: ownerID,
		ScopeKind:   "manual",
		CreatedBy:   ownerID,
		Grants: []model.ObjectAccessGrantInput{
			{PrincipalKind: "link_token", LinkTokenHash: new("plaintext-token")},
		},
	})
	if err == nil {
		t.Fatal("CreateObjectAccessScope accepted plaintext link token")
	}

	_, err = db.CreateObjectAccessScope(ctx, model.ObjectAccessScopeInput{
		OwnerUserID: ownerID,
		ScopeKind:   "manual",
		CreatedBy:   ownerID,
		Grants: []model.ObjectAccessGrantInput{
			{PrincipalKind: "user", PrincipalUserID: &viewerID, Permission: "edit"},
		},
	})
	if err == nil {
		t.Fatal("CreateObjectAccessScope accepted non-view permission")
	}
}

func TestSegmentSharedVisibilityListDetailAndExport(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	unrelatedID := insertObjectAccessTestUser(t, db)

	scope, err := db.CreateObjectAccessScope(ctx, model.ObjectAccessScopeInput{
		OwnerUserID: ownerID,
		ScopeKind:   "manual",
		CreatedBy:   ownerID,
		Grants: []model.ObjectAccessGrantInput{
			{PrincipalKind: "user", PrincipalUserID: &viewerID},
		},
	})
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}
	segment, err := db.CreateSegment(ctx, ownerID, &model.SegmentCreate{
		Name:          "shared segment",
		SportGroup:    "bike",
		SegmentType:   "standard",
		GeoJSONTrack:  `{"type":"LineString","coordinates":[[7,47],[7.001,47.001]]}`,
		Visibility:    "shared",
		AccessScopeID: &scope.ID,
	})
	if err != nil {
		t.Fatalf("CreateSegment shared: %v", err)
	}

	for _, tc := range []struct {
		name   string
		userID string
		want   bool
	}{
		{name: "owner", userID: ownerID, want: true},
		{name: "viewer", userID: viewerID, want: true},
		{name: "unrelated", userID: unrelatedID, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listed, err := db.ListSegments(ctx, model.ListSegmentsParams{UserID: tc.userID, Limit: -1})
			if err != nil {
				t.Fatalf("ListSegments: %v", err)
			}
			listContains := objectAccessSegmentListContains(listed.Segments, segment.ID)
			if listContains != tc.want {
				t.Fatalf("ListSegments contains shared segment = %v, want %v", listContains, tc.want)
			}

			got, err := db.GetSegment(ctx, segment.ID, tc.userID, false, true)
			if err != nil {
				t.Fatalf("GetSegment: %v", err)
			}
			if (got != nil) != tc.want {
				t.Fatalf("GetSegment visible = %v, want %v", got != nil, tc.want)
			}

			exported, err := db.GetSegmentsForExport(ctx, []string{segment.ID}, tc.userID)
			if err != nil {
				t.Fatalf("GetSegmentsForExport: %v", err)
			}
			exportContains := objectAccessSegmentListContains(exported, segment.ID)
			if exportContains != tc.want {
				t.Fatalf("GetSegmentsForExport contains shared segment = %v, want %v", exportContains, tc.want)
			}
		})
	}
}

func TestSegmentSharedAccessScopePairing(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	otherOwnerID := insertObjectAccessTestUser(t, db)
	viewerID := insertObjectAccessTestUser(t, db)

	scope, err := db.CreateObjectAccessScope(ctx, model.ObjectAccessScopeInput{
		OwnerUserID: ownerID,
		ScopeKind:   "manual",
		CreatedBy:   ownerID,
		Grants:      []model.ObjectAccessGrantInput{{PrincipalKind: "user", PrincipalUserID: &viewerID}},
	})
	if err != nil {
		t.Fatalf("CreateObjectAccessScope owner: %v", err)
	}
	otherScope, err := db.CreateObjectAccessScope(ctx, model.ObjectAccessScopeInput{
		OwnerUserID: otherOwnerID,
		ScopeKind:   "manual",
		CreatedBy:   otherOwnerID,
		Grants:      []model.ObjectAccessGrantInput{{PrincipalKind: "user", PrincipalUserID: &viewerID}},
	})
	if err != nil {
		t.Fatalf("CreateObjectAccessScope other: %v", err)
	}

	_, err = db.CreateSegment(ctx, ownerID, objectAccessSegmentCreate("shared without scope", "shared", nil))
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("shared without access scope error = %v, want ErrSegmentValidation", err)
	}

	_, err = db.CreateSegment(ctx, ownerID, objectAccessSegmentCreate("public with scope", "public", &scope.ID))
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("public with access scope error = %v, want ErrSegmentValidation", err)
	}

	_, err = db.CreateSegment(ctx, ownerID, objectAccessSegmentCreate("shared with foreign scope", "shared", &otherScope.ID))
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("shared with foreign access scope error = %v, want ErrSegmentValidation", err)
	}
}

func insertObjectAccessTestUser(t *testing.T, db *DB) string {
	t.Helper()
	userID := uuid.NewString()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
	`, userID, "object-access-"+userID[:8], "object-access-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert object-access user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID
}

//go:fix inline
func objectAccessPtrString(v string) *string {
	return new(v)
}

func objectAccessContainsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func objectAccessSegmentCreate(name, visibility string, accessScopeID *string) *model.SegmentCreate {
	return &model.SegmentCreate{
		Name:          name,
		SportGroup:    "bike",
		SegmentType:   "standard",
		GeoJSONTrack:  `{"type":"LineString","coordinates":[[7,47],[7.001,47.001]]}`,
		Visibility:    visibility,
		AccessScopeID: accessScopeID,
	}
}

func objectAccessSegmentListContains(segments []model.Segment, segmentID string) bool {
	for _, segment := range segments {
		if segment.ID == segmentID {
			return true
		}
	}
	return false
}
