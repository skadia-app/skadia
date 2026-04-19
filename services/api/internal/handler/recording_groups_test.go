package handler

// recording_groups_test.go — Integration tests for AddGroupMember.
//
// Requires TEST_DATABASE_URL to be set; the whole suite is skipped otherwise.
//
// Tests:
//   - TestAddGroupMember_HappyPath      — POST with valid group + activity → 200 OK, member added
//   - TestAddGroupMember_OwnershipDenied — group belongs to different user → 403
//   - TestAddGroupMember_Idempotent     — adding same activity twice → 200 OK both times

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// doAddGroupMember sends POST /recording-groups/{groupID}/members with the given
// activityID in the body. Returns the response code.
func doAddGroupMember(t *testing.T, h *Handler, userID, groupID, activityID string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"activity_id": activityID})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recording-groups/"+groupID+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", groupID)
	req = req.WithContext(context.WithValue(
		context.WithValue(req.Context(), middleware.UserIDKey, userID),
		chi.RouteCtxKey, rctx,
	))

	rr := httptest.NewRecorder()
	h.AddGroupMember(rr, req)
	return rr.Code
}

// TestAddGroupMember_HappyPath: create a group with two activities, add a third
// unrelated activity → should return 200 and list three members.
func TestAddGroupMember_HappyPath(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	act1 := insertMinimalActivity(t, db, userID)
	act2 := insertMinimalActivity(t, db, userID)
	act3 := insertMinimalActivity(t, db, userID)

	group, err := db.CreateRecordingGroup(ctx, userID, []string{act1, act2}, act1, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}

	h := &Handler{DB: db}
	code := doAddGroupMember(t, h, userID, group.ID, act3)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}

	// Verify three members.
	members, err := db.GetRecordingGroupMembers(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetRecordingGroupMembers: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
	found := false
	for _, m := range members {
		if m.ActivityID == act3 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected act3 to be a member after AddGroupMember")
	}
}

// TestAddGroupMember_OwnershipDenied: group belongs to userA, but request is made
// by userB → should return 403.
func TestAddGroupMember_OwnershipDenied(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userA := insertTestUser(t, pool)
	userB := insertTestUser(t, pool)

	act1 := insertMinimalActivity(t, db, userA)
	act2 := insertMinimalActivity(t, db, userA)
	actB := insertMinimalActivity(t, db, userB)

	group, err := db.CreateRecordingGroup(ctx, userA, []string{act1, act2}, act1, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}

	h := &Handler{DB: db}
	// userB tries to add to userA's group
	code := doAddGroupMember(t, h, userB, group.ID, actB)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", code)
	}
}

// TestAddGroupMember_Idempotent: adding the same activity to the same group twice
// should return 200 both times (idempotent).
func TestAddGroupMember_Idempotent(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	act1 := insertMinimalActivity(t, db, userID)
	act2 := insertMinimalActivity(t, db, userID)
	act3 := insertMinimalActivity(t, db, userID)

	group, err := db.CreateRecordingGroup(ctx, userID, []string{act1, act2}, act1, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}

	h := &Handler{DB: db}

	// First call — adds act3.
	code := doAddGroupMember(t, h, userID, group.ID, act3)
	if code != http.StatusOK {
		t.Fatalf("first add: expected 200, got %d", code)
	}

	// Second call — act3 is already in the group, should be idempotent → 200.
	code = doAddGroupMember(t, h, userID, group.ID, act3)
	if code != http.StatusOK {
		t.Fatalf("idempotent add: expected 200, got %d", code)
	}

	// Still three members (not four).
	members, err := db.GetRecordingGroupMembers(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetRecordingGroupMembers: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members after idempotent add, got %d", len(members))
	}
}
